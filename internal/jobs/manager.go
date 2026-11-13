// Package jobs executes queued FFmpeg commands and streams their progress.
//
// Exactly one job runs at a time (encoding is usually IO and CPU bound, and
// serial execution keeps progress and pause semantics predictable). The runner
// never invokes a shell: the job's argv is handed directly to exec.Command.
package jobs

import (
	"bufio"
	"errors"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/AMC-official/ffmpeg-tui/internal/ffmpeg"
	"github.com/AMC-official/ffmpeg-tui/internal/history"
	"github.com/AMC-official/ffmpeg-tui/internal/queue"
)

// EventKind identifies a runner event.
type EventKind int

const (
	EventChanged EventKind = iota
	EventProgress
	EventLog
	EventFinished
)

// Event is a snapshot delivered to the UI. Job is a copy, never a live pointer.
type Event struct {
	Kind  EventKind
	Job   queue.Job
	JobID string
	Error string
}

// Manager owns the queue and the worker goroutine.
type Manager struct {
	mu      sync.Mutex
	q       *queue.Queue
	bin     string
	events  chan Event
	wake    chan struct{}
	stop    chan struct{}
	stopped chan struct{}

	proc     *exec.Cmd
	cancel   bool
	pauseOK  bool
	savePath string
	history  *history.Store
	started  bool
	probeDur func(string) time.Duration
	lastEmit time.Time
	logLimit int
}

// New creates a manager. The worker starts on Start.
func New(bin string) *Manager {
	return &Manager{
		q:        queue.New(),
		bin:      bin,
		events:   make(chan Event, 256),
		wake:     make(chan struct{}, 1),
		stop:     make(chan struct{}),
		stopped:  make(chan struct{}),
		logLimit: 240,
	}
}

// SetHistory attaches a history store. Completed jobs are recorded there.
func (m *Manager) SetHistory(h *history.Store) {
	m.mu.Lock()
	m.history = h
	m.mu.Unlock()
}

// SetDurationResolver supplies a fallback duration probe for jobs created
// without one (replayed history, loaded queues).
func (m *Manager) SetDurationResolver(f func(string) time.Duration) {
	m.mu.Lock()
	m.probeDur = f
	m.mu.Unlock()
}

// SetSavePath enables autosave of the queue after every state change.
func (m *Manager) SetSavePath(p string) {
	m.mu.Lock()
	m.savePath = p
	m.mu.Unlock()
}

// Queue returns the live queue. Callers must not retain it across goroutines;
// use Snapshot for rendering.
func (m *Manager) Queue() *queue.Queue { return m.q }

// Replace swaps the queue contents (used when loading a saved queue).
func (m *Manager) Replace(q *queue.Queue) {
	m.mu.Lock()
	if q != nil {
		m.q = q
	}
	m.mu.Unlock()
	m.persist()
	m.emit(Event{Kind: EventChanged})
	m.kick()
}

// Events returns the event channel for the UI pump.
func (m *Manager) Events() <-chan Event { return m.events }

// Snapshot returns a copy of the queue for rendering.
func (m *Manager) Snapshot() queue.Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.q.Snapshot()
}

// Start launches the worker goroutine (idempotent).
func (m *Manager) Start() {
	m.mu.Lock()
	if m.started {
		m.mu.Unlock()
		return
	}
	m.started = true
	m.mu.Unlock()
	go m.loop()
}

// Stop terminates the worker and its child process.
func (m *Manager) Stop() {
	select {
	case <-m.stop:
		return
	default:
	}
	close(m.stop)
	m.mu.Lock()
	if m.proc != nil && m.proc.Process != nil {
		_ = m.proc.Process.Kill()
	}
	m.mu.Unlock()
	select {
	case <-m.stopped:
	case <-time.After(3 * time.Second):
	}
}

// Enqueue adds a command and wakes the worker.
func (m *Manager) Enqueue(name string, cmd ffmpeg.Command) *queue.Job {
	if cmd.Bin == "" {
		cmd.Bin = m.bin
	}
	m.mu.Lock()
	job := m.q.Add(name, cmd)
	job.Progress.Duration = cmd.Duration
	cp := *job
	m.mu.Unlock()
	m.persist()
	m.emit(Event{Kind: EventChanged, Job: cp, JobID: cp.ID})
	m.kick()
	return &cp
}

// Cancel stops a job. Queued jobs are canceled immediately; running ones are
// killed and marked canceled.
func (m *Manager) Cancel(id string) bool {
	m.mu.Lock()
	j := m.q.Find(id)
	if j == nil {
		m.mu.Unlock()
		return false
	}
	switch j.Status {
	case queue.Queued:
		j.Status = queue.Canceled
		j.FinishedAt = time.Now()
	case queue.Running, queue.Paused:
		m.cancel = true
		if m.proc != nil && m.proc.Process != nil {
			_ = m.proc.Process.Kill()
		}
	case queue.Done, queue.Failed, queue.Canceled:
		m.mu.Unlock()
		return false
	}
	cp := *j
	m.mu.Unlock()
	m.persist()
	m.emit(Event{Kind: EventChanged, Job: cp, JobID: cp.ID})
	m.kick()
	return true
}

// Retry resets a finished job back to queued and wakes the worker.
func (m *Manager) Retry(id string) bool {
	m.mu.Lock()
	j := m.q.Find(id)
	if j == nil || !j.Status.Terminal() {
		m.mu.Unlock()
		return false
	}
	j.ResetForRetry()
	cp := *j
	m.mu.Unlock()
	m.persist()
	m.emit(Event{Kind: EventChanged, Job: cp, JobID: cp.ID})
	m.kick()
	return true
}

// Pause suspends the running process (SIGSTOP on Unix). It returns false when
// the platform or job state does not allow it.
func (m *Manager) Pause(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	j := m.q.Find(id)
	if j == nil || j.Status != queue.Running || m.proc == nil {
		return false
	}
	if err := pauseProcess(m.proc); err != nil {
		return false
	}
	j.Status = queue.Paused
	cp := *j
	go func() {
		m.persist()
		m.emit(Event{Kind: EventChanged, Job: cp, JobID: cp.ID})
	}()
	return true
}

// Resume continues a paused job.
func (m *Manager) Resume(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	j := m.q.Find(id)
	if j == nil || j.Status != queue.Paused || m.proc == nil {
		return false
	}
	if err := resumeProcess(m.proc); err != nil {
		return false
	}
	j.Status = queue.Running
	cp := *j
	go func() {
		m.persist()
		m.emit(Event{Kind: EventChanged, Job: cp, JobID: cp.ID})
	}()
	return true
}

// PauseSupported reports whether the current platform can suspend a process.
func (m *Manager) PauseSupported() bool { return pauseSupported }

// Move reorders the queue.
func (m *Manager) Move(from, to int) bool {
	m.mu.Lock()
	ok := m.q.Move(from, to)
	m.mu.Unlock()
	if ok {
		m.persist()
		m.emit(Event{Kind: EventChanged})
	}
	return ok
}

// Remove deletes a job. Running jobs must be canceled first.
func (m *Manager) Remove(id string) bool {
	m.mu.Lock()
	j := m.q.Find(id)
	if j == nil || j.Status.Active() {
		m.mu.Unlock()
		return false
	}
	ok := m.q.Remove(id)
	m.mu.Unlock()
	if ok {
		m.persist()
		m.emit(Event{Kind: EventChanged})
	}
	return ok
}

// ClearFinished removes every completed, failed or canceled job.
func (m *Manager) ClearFinished() int {
	m.mu.Lock()
	n := m.q.ClearFinished()
	m.mu.Unlock()
	if n > 0 {
		m.persist()
		m.emit(Event{Kind: EventChanged})
	}
	return n
}

// Save writes the queue to path.
func (m *Manager) Save(path string) error {
	m.mu.Lock()
	q := m.q
	m.mu.Unlock()
	return q.Save(path)
}

// Load replaces the queue with the contents of path.
func (m *Manager) Load(path string) error {
	q, err := queue.Load(path)
	if err != nil {
		return err
	}
	m.Replace(q)
	return nil
}

func (m *Manager) kick() {
	select {
	case m.wake <- struct{}{}:
	default:
	}
}

func (m *Manager) loop() {
	defer close(m.stopped)
	for {
		select {
		case <-m.stop:
			return
		default:
		}
		m.mu.Lock()
		job := m.q.NextQueued()
		m.mu.Unlock()
		if job == nil {
			select {
			case <-m.wake:
				continue
			case <-m.stop:
				return
			case <-time.After(500 * time.Millisecond):
				continue
			}
		}
		m.run(job)
	}
}

func (m *Manager) run(job *queue.Job) {
	m.mu.Lock()
	job.Status = queue.Running
	job.StartedAt = time.Now()
	job.FinishedAt = time.Time{}
	job.Err = ""
	job.Logs = nil
	job.Progress = ffmpeg.Progress{Percent: -1, Duration: job.Command.Duration}
	m.cancel = false
	cmd := job.Command
	started := job.StartedAt
	id := job.ID
	needProbe := job.Progress.Duration <= 0 && m.probeDur != nil && cmd.Input != ""
	resolver := m.probeDur
	cp := *job
	m.mu.Unlock()

	if needProbe && resolver != nil {
		if d := resolver(cmd.Input); d > 0 {
			m.mu.Lock()
			job.Progress.Duration = d
			m.mu.Unlock()
		}
	}
	m.upgradeJob(id, func(j *queue.Job) { cp = *j })
	m.emit(Event{Kind: EventChanged, Job: cp, JobID: id})
	m.persist()

	bin := cmd.Bin
	if bin == "" {
		bin = m.bin
	}
	argv := append([]string{"-loglevel", "error", "-nostats", "-progress", "pipe:1"}, cmd.Args...)
	argv = append(argv, cmd.Output)
	proc := exec.Command(bin, argv...)
	configureProcess(proc)

	stdout, err := proc.StdoutPipe()
	if err != nil {
		m.finish(job, started, -1, "cannot capture ffmpeg stdout: "+err.Error(), false)
		return
	}
	stderr, err := proc.StderrPipe()
	if err != nil {
		m.finish(job, started, -1, "cannot capture ffmpeg stderr: "+err.Error(), false)
		return
	}
	if err := proc.Start(); err != nil {
		m.finish(job, started, -1, "cannot start ffmpeg: "+err.Error(), false)
		return
	}

	m.mu.Lock()
	m.proc = proc
	m.mu.Unlock()

	m.mu.Lock()
	expected := job.Progress.Duration
	m.mu.Unlock()
	parser := ffmpeg.NewParser(expected)
	var errBuf strings.Builder
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)
		for sc.Scan() {
			if parser.FeedLine(sc.Text()) {
				m.progress(job, parser)
			}
		}
	}()

	go func() {
		defer wg.Done()
		sc := bufio.NewScanner(stderr)
		sc.Buffer(make([]byte, 64*1024), 512*1024)
		for sc.Scan() {
			line := sc.Text()
			parser.FeedLine(line)
			if errBuf.Len() < 64*1024 {
				errBuf.WriteString(line)
				errBuf.WriteByte('\n')
			}
			m.log(job, line)
		}
	}()

	wg.Wait()
	waitErr := proc.Wait()

	m.mu.Lock()
	m.proc = nil
	canceled := m.cancel
	m.cancel = false
	job.Progress = parser.Progress()
	m.mu.Unlock()

	exitCode := 0
	errMsg := ""
	if waitErr != nil {
		var ee *exec.ExitError
		if errors.As(waitErr, &ee) {
			exitCode = ee.ExitCode()
		} else {
			exitCode = -1
		}
		errMsg = lastErrorLines(errBuf.String(), 8)
		if errMsg == "" {
			errMsg = waitErr.Error()
		}
	}
	m.finish(job, started, exitCode, errMsg, canceled)
}

func (m *Manager) progress(job *queue.Job, parser *ffmpeg.Parser) {
	now := time.Now()
	m.mu.Lock()
	job.Progress = parser.Progress()
	since := now.Sub(m.lastEmit)
	shouldEmit := since > 120*time.Millisecond
	if shouldEmit {
		m.lastEmit = now
	}
	cp := *job
	id := job.ID
	m.mu.Unlock()
	if shouldEmit {
		m.emit(Event{Kind: EventProgress, Job: cp, JobID: id})
	}
}

func (m *Manager) log(job *queue.Job, line string) {
	if strings.TrimSpace(line) == "" {
		return
	}
	m.mu.Lock()
	job.AddLog(line, m.logLimit)
	m.mu.Unlock()
	m.emit(Event{Kind: EventLog, JobID: job.ID, Job: queue.Job{ID: job.ID, Logs: []string{line}}})
}

// upgradeJob copies a live job into out while holding the queue lock.
func (m *Manager) upgradeJob(id string, out func(*queue.Job)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if j := m.q.Find(id); j != nil {
		out(j)
	}
}

func (m *Manager) finish(job *queue.Job, started time.Time, exitCode int, errMsg string, canceled bool) {
	m.mu.Lock()
	job.ExitCode = exitCode
	job.FinishedAt = time.Now()
	switch {
	case canceled:
		job.Status = queue.Canceled
		job.Err = "canceled"
	case exitCode == 0 && errMsg == "":
		job.Status = queue.Done
		job.Progress.Percent = 100
		job.Progress.Done = true
		if job.Progress.Duration > 0 {
			job.Progress.OutTime = job.Progress.Duration
		}
	default:
		job.Status = queue.Failed
		job.Err = errMsg
	}
	cp := *job
	cp.Logs = append([]string(nil), job.Logs...)
	cmd := job.Command
	hist := m.history
	m.mu.Unlock()

	if hist != nil {
		status := string(cp.Status)
		entry := history.FromCommand(cmd, started, status, exitCode, cp.Err)
		entry.ID = cp.ID
		_ = hist.Add(entry)
	}
	m.persist()
	m.emit(Event{Kind: EventFinished, Job: cp, JobID: cp.ID, Error: cp.Err})
}

func (m *Manager) emit(e Event) {
	select {
	case m.events <- e:
	default:
		// Slow consumer: drop progress/log events rather than block encoding.
		if e.Kind == EventFinished || e.Kind == EventChanged {
			select {
			case m.events <- e:
			case <-time.After(50 * time.Millisecond):
			}
		}
	}
}

func (m *Manager) persist() {
	m.mu.Lock()
	path := m.savePath
	q := m.q
	m.mu.Unlock()
	if path == "" {
		return
	}
	_ = q.Save(path)
}

func lastErrorLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	var kept []string
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "frame=") || strings.HasPrefix(l, "size=") {
			continue
		}
		kept = append(kept, l)
	}
	return strings.Join(kept, " | ")
}

// ErrNoBinary is returned when ffmpeg is missing from PATH.
var ErrNoBinary = errors.New("ffmpeg binary not found")

// EnsureStdout is a small helper used by tests to drain a reader.
func EnsureStdout(r io.Reader) { _, _ = io.Copy(io.Discard, r) }
