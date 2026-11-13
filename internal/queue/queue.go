// Package queue holds the job model and the persistent, reorderable job queue.
// Execution lives in internal/jobs; this package is pure data so that it can be
// saved, loaded and tested without spawning processes.
package queue

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/AMC-official/ffmpeg-tui/internal/ffmpeg"
)

// Status is the lifecycle state of a job.
type Status string

const (
	Queued   Status = "queued"
	Running  Status = "running"
	Paused   Status = "paused"
	Done     Status = "done"
	Failed   Status = "failed"
	Canceled Status = "canceled"
)

// Terminal reports whether no further progress can happen.
func (s Status) Terminal() bool { return s == Done || s == Failed || s == Canceled }

// Active reports whether the job is running or paused.
func (s Status) Active() bool { return s == Running || s == Paused }

// Job is one queued FFmpeg command and its live state.
type Job struct {
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	Command    ffmpeg.Command  `json:"command"`
	Status     Status          `json:"status"`
	Progress   ffmpeg.Progress `json:"progress"`
	CreatedAt  time.Time       `json:"created_at"`
	StartedAt  time.Time       `json:"started_at,omitempty"`
	FinishedAt time.Time       `json:"finished_at,omitempty"`
	ExitCode   int             `json:"exit_code,omitempty"`
	Err        string          `json:"error,omitempty"`
	Logs       []string        `json:"logs,omitempty"`
	HistoryID  string          `json:"history_id,omitempty"`
}

// Icon returns a compact status glyph for lists.
func (j *Job) Icon() string {
	switch j.Status {
	case Running:
		return "⟳"
	case Done:
		return "✓"
	case Failed:
		return "✖"
	case Canceled:
		return "⊘"
	case Paused:
		return "⏸"
	}
	return "▶"
}

// StatusText returns a human status label.
func (j *Job) StatusText() string {
	switch j.Status {
	case Running:
		return "running"
	case Done:
		return "done"
	case Failed:
		return "failed"
	case Canceled:
		return "canceled"
	case Paused:
		return "paused"
	}
	return "queued"
}

// Elapsed returns how long the job ran.
func (j *Job) Elapsed() time.Duration {
	if j.StartedAt.IsZero() {
		return 0
	}
	if !j.FinishedAt.IsZero() {
		return j.FinishedAt.Sub(j.StartedAt)
	}
	return time.Since(j.StartedAt)
}

// Percent returns a 0..100 value, or -1 when unknown.
func (j *Job) Percent() float64 {
	if j.Status == Done {
		return 100
	}
	return j.Progress.Percent
}

// AddLog appends a line to the bounded log tail.
func (j *Job) AddLog(line string, max int) {
	if max <= 0 {
		max = 200
	}
	j.Logs = append(j.Logs, line)
	if len(j.Logs) > max {
		j.Logs = j.Logs[len(j.Logs)-max:]
	}
}

// ResetForRetry returns the job to the queue, clearing previous results.
func (j *Job) ResetForRetry() {
	j.Status = Queued
	j.Progress = ffmpeg.Progress{Percent: -1, Duration: j.Command.Duration}
	j.StartedAt = time.Time{}
	j.FinishedAt = time.Time{}
	j.ExitCode = 0
	j.Err = ""
	j.Logs = nil
}

// Queue is an ordered, persistable list of jobs.
type Queue struct {
	Jobs []*Job `json:"jobs"`
	seq  int
}

// New returns an empty queue.
func New() *Queue { return &Queue{} }

// Add appends a new queued job and returns it.
func (q *Queue) Add(name string, cmd ffmpeg.Command) *Job {
	q.seq++
	now := time.Now()
	if name == "" {
		name = filepath.Base(cmd.Output)
	}
	j := &Job{
		ID:        fmt.Sprintf("%d-%d", now.UnixNano(), q.seq),
		Name:      name,
		Command:   cmd,
		Status:    Queued,
		Progress:  ffmpeg.Progress{Percent: -1, Duration: cmd.Duration},
		CreatedAt: now,
	}
	q.Jobs = append(q.Jobs, j)
	return j
}

// Len returns the number of jobs.
func (q *Queue) Len() int { return len(q.Jobs) }

// Find returns a job by ID.
func (q *Queue) Find(id string) *Job {
	for _, j := range q.Jobs {
		if j.ID == id {
			return j
		}
	}
	return nil
}

// Index returns the position of a job, or -1.
func (q *Queue) Index(id string) int {
	for i, j := range q.Jobs {
		if j.ID == id {
			return i
		}
	}
	return -1
}

// Remove deletes a job by ID.
func (q *Queue) Remove(id string) bool {
	i := q.Index(id)
	if i < 0 {
		return false
	}
	q.Jobs = append(q.Jobs[:i], q.Jobs[i+1:]...)
	return true
}

// Move reorders a job from one index to another, clamping the destination.
func (q *Queue) Move(from, to int) bool {
	if from < 0 || from >= len(q.Jobs) {
		return false
	}
	if to < 0 {
		to = 0
	}
	if to >= len(q.Jobs) {
		to = len(q.Jobs) - 1
	}
	if from == to {
		return false
	}
	j := q.Jobs[from]
	q.Jobs = append(q.Jobs[:from], q.Jobs[from+1:]...)
	q.Jobs = append(q.Jobs[:to], append([]*Job{j}, q.Jobs[to:]...)...)
	return true
}

// NextQueued returns the first queued job, or nil.
func (q *Queue) NextQueued() *Job {
	for _, j := range q.Jobs {
		if j.Status == Queued {
			return j
		}
	}
	return nil
}

// Active returns the running or paused job, if any.
func (q *Queue) Active() *Job {
	for _, j := range q.Jobs {
		if j.Status.Active() {
			return j
		}
	}
	return nil
}

// Counts summarises job states.
func (q *Queue) Counts() (queued, running, done, failed int) {
	for _, j := range q.Jobs {
		switch j.Status {
		case Queued:
			queued++
		case Running, Paused:
			running++
		case Done:
			done++
		case Failed, Canceled:
			failed++
		}
	}
	return
}

// ClearFinished removes every terminal job.
func (q *Queue) ClearFinished() int {
	out := q.Jobs[:0]
	removed := 0
	for _, j := range q.Jobs {
		if j.Status.Terminal() {
			removed++
			continue
		}
		out = append(out, j)
	}
	q.Jobs = out
	return removed
}

// Snapshot returns a deep enough copy for rendering outside the runner lock.
type Snapshot struct {
	Jobs []Job
}

// Snapshot copies the queue for safe display.
func (q *Queue) Snapshot() Snapshot {
	out := Snapshot{Jobs: make([]Job, 0, len(q.Jobs))}
	for _, j := range q.Jobs {
		cp := *j
		cp.Logs = append([]string(nil), j.Logs...)
		cp.Command.Args = append([]string(nil), j.Command.Args...)
		out.Jobs = append(out.Jobs, cp)
	}
	return out
}

// Persist is the on-disk queue format.
type Persist struct {
	Version int       `json:"version"`
	SavedAt time.Time `json:"saved_at"`
	Jobs    []*Job    `json:"jobs"`
}

// Save writes the queue atomically to path.
func (q *Queue) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	p := Persist{Version: 1, SavedAt: time.Now(), Jobs: q.Jobs}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Load reads a persisted queue. Missing files yield an empty queue.
func Load(path string) (*Queue, error) {
	q := New()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return q, nil
		}
		return q, err
	}
	var p Persist
	if err := json.Unmarshal(data, &p); err != nil {
		return q, fmt.Errorf("queue file %s is corrupt: %w", filepath.Base(path), err)
	}
	q.Jobs = p.Jobs
	for _, j := range q.Jobs {
		if j.Status == Running || j.Status == Paused {
			// A restored process cannot be resumed; put it back in line.
			j.Status = Queued
		}
		if j.Progress.Duration == 0 {
			j.Progress.Duration = j.Command.Duration
		}
		if j.Progress.Percent == 0 && j.Status != Done {
			j.Progress.Percent = -1
		}
	}
	return q, nil
}

// Interrupted reports whether any restored job was mid-flight.
func (q *Queue) Interrupted() bool {
	for _, j := range q.Jobs {
		if j.Status == Queued && !j.StartedAt.IsZero() {
			return true
		}
	}
	return false
}

// SummaryLine renders a compact description of a job's command.
func (j *Job) SummaryLine() string {
	parts := []string{filepath.Base(j.Command.Input)}
	if j.Command.Output != "" {
		parts = append(parts, "→", filepath.Base(j.Command.Output))
	}
	return strings.Join(parts, " ")
}
