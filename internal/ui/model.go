// Package ui implements the Bubble Tea interface of FFmpeg-TUI.
package ui

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/AMC-official/ffmpeg-tui/internal/config"
	"github.com/AMC-official/ffmpeg-tui/internal/ffprobe"
	"github.com/AMC-official/ffmpeg-tui/internal/filesystem"
	"github.com/AMC-official/ffmpeg-tui/internal/history"
	"github.com/AMC-official/ffmpeg-tui/internal/jobs"
	"github.com/AMC-official/ffmpeg-tui/internal/queue"
)

// screen identifies the active page.
type screen int

const (
	screenDashboard screen = iota
	screenBrowser
	screenWizard
	screenFilters
	screenQueue
	screenJobDetail
	screenHistory
	screenPresets
	screenMan
	screenMetadata
	screenStreams
	screenCompare
	screenBatch
	screenTools
)

// editTarget routes text input back to the field being edited.
type editTarget struct {
	kind  string // "wizard-input", "wizard-output", "wizard-meta", "wizard-extra", "man", "history", "preset-name", "batch-out", "batch-template", "filter-arg", "compare-slot", "palette", "search"
	index int
	key   string
}

// Model is the root Bubble Tea model.
type Model struct {
	width, height int
	screen        screen
	stack         []screen
	styles        Styles
	cfg           *config.Config
	cfgFile       string

	ffmpegPath    string
	ffprobePath   string
	ffmpegVersion string
	ffmpegOK      bool
	ffprobeOK     bool
	ffplayOK      bool

	ctx    context.Context
	cancel context.CancelFunc

	spinner spinner.Model
	editor  textinput.Model
	edit    editTarget
	editing bool

	quitting bool
	confirm  *confirmPrompt
	toast    string
	toastSeq int
	status   string

	jobs       *jobs.Manager
	queueSnap  queue.Snapshot
	hist       *history.Store
	histCursor int
	histFilter string

	dashCursor  int
	dashActions []dashAction

	cwd           string
	entries       []filesystem.Entry
	allEntries    []filesystem.Entry
	browserCursor int
	browserOffset int
	showHidden    bool
	sortMode      string
	browserFilter string
	filtering     bool

	probe        *ffprobe.Result
	probePath    string
	probeLoading bool
	probeErr     string

	previewArt     string
	previewKind    string
	previewLoading bool
	previewErr     string
	previewOn      bool

	marked map[string]bool

	wiz           wizardState
	flt           filterState
	presetCursor  int
	presetCat     int
	man           manState
	meta          metaState
	batch         batchState
	tools         toolState
	compare       compareState
	streamsScroll int
	queueCursor   int
	jobFocus      *queue.Job

	palette paletteState
	search  searchState
	help    helpState
}

// Options configures a new Model.
type Options struct {
	Config      *config.Config
	ConfigPath  string
	FFmpegPath  string
	FFprobePath string
}

// New constructs the root model. It performs only local, fast work; probing and
// previews happen asynchronously.
func New(opts Options) *Model {
	cfg := opts.Config
	if cfg == nil {
		cfg = config.Default()
	}
	m := &Model{
		styles:      NewStyles(),
		cfg:         cfg,
		cfgFile:     opts.ConfigPath,
		ffmpegPath:  firstNonEmpty(opts.FFmpegPath, cfg.FFmpegPath, "ffmpeg"),
		ffprobePath: firstNonEmpty(opts.FFprobePath, cfg.FFprobePath, "ffprobe"),
		showHidden:  cfg.ShowHidden,
		sortMode:    cfg.SortMode,
		marked:      map[string]bool{},
		previewOn:   true,
		status:      "ready",
	}
	m.ctx, m.cancel = context.WithCancel(context.Background())
	m.spinner = spinner.New()
	m.spinner.Spinner = spinner.Dot
	m.spinner.Style = m.styles.Accent
	m.editor = textinput.New()
	m.editor.CharLimit = 1024
	m.editor.Prompt = "› "
	m.editor.TextStyle = m.styles.Text
	m.editor.PromptStyle = m.styles.Accent

	if _, err := exec.LookPath(m.ffmpegPath); err == nil {
		m.ffmpegOK = true
		m.ffmpegVersion = ffmpegVersion(m.ffmpegPath)
	}
	if _, err := exec.LookPath(m.ffprobePath); err == nil {
		m.ffprobeOK = true
	}
	if _, err := exec.LookPath("ffplay"); err == nil {
		m.ffplayOK = true
	}

	m.jobs = jobs.New(m.ffmpegPath)
	m.jobs.SetDurationResolver(func(path string) time.Duration {
		res, err := ffprobe.Probe(context.Background(), path, ffprobe.Options{Binary: m.ffprobePath, Timeout: 10 * time.Second})
		if err != nil {
			return 0
		}
		return res.Duration()
	})

	stateDir, err := config.StateDir()
	if err != nil {
		stateDir = "."
	}
	m.hist = history.Open(filepath.Join(stateDir, "history.jsonl"), cfg.HistoryLimit)
	m.jobs.SetHistory(m.hist)

	m.cwd = firstExistingDir(cfg.LastDir)
	m.loadDir(m.cwd)
	m.refreshHistory()
	m.dashActions = m.buildDashActions()
	m.wiz = newWizardState()
	m.flt = newFilterState()
	m.man = newManState()
	m.meta = newMetaState()
	m.compare = newCompareState()
	m.tools = newToolState()
	m.palette = newPaletteState()
	m.search = newSearchState()
	m.help = helpState{}
	m.jobs.Start()
	return m
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func firstExistingDir(dir string) string {
	dir = filesystem.ExpandHome(dir)
	if st, err := os.Stat(dir); err == nil && st.IsDir() {
		return dir
	}
	if home, err := os.UserHomeDir(); err == nil {
		return home
	}
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return "."
}

// ffmpegVersion returns the first line of ffmpeg -version, or "" if unknown.
func ffmpegVersion(bin string) string {
	out, err := exec.Command(bin, "-version").Output()
	if err != nil {
		return ""
	}
	line, _, _ := strings.Cut(string(out), "\n")
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "ffmpeg version ")
	if i := strings.Index(line, " Copyright"); i > 0 {
		line = line[:i]
	}
	if i := strings.Index(line, "-static"); i > 0 {
		line = line[:i]
	}
	return strings.TrimSpace(line)
}

// Init starts the event pumps and the spinner.
func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		waitJobEvent(m.jobs),
		probeCmd(m.ctx, m.ffprobePath, m.probePath),
	)
}

// Close releases background resources; call it after the program exits.
func (m *Model) Close() {
	m.jobs.Stop()
	if m.cancel != nil {
		m.cancel()
	}
	m.saveConfig()
}

// loadDir refreshes the browser listing for dir.
func (m *Model) loadDir(dir string) {
	dir = filesystem.ExpandHome(dir)
	entries, err := filesystem.List(dir, m.showHidden)
	if err != nil {
		m.flash("cannot open " + dir + ": " + err.Error())
		return
	}
	m.cwd = dir
	m.allEntries = entries
	m.entries = entries
	if m.browserFilter != "" {
		m.entries = filterEntries(m.allEntries, m.browserFilter)
	}
	m.browserCursor = 0
	m.browserOffset = 0
	m.cfg.LastDir = dir
}

// sortEntries re-sorts the listing.
func (m *Model) sortEntries() { filesystem.Sort(m.entries, m.sortMode) }

// selectedEntry returns the highlighted browser entry.
func (m *Model) selectedEntry() (filesystem.Entry, bool) {
	if m.browserCursor >= 0 && m.browserCursor < len(m.entries) {
		return m.entries[m.browserCursor], true
	}
	return filesystem.Entry{}, false
}

// flash shows a transient status message.
func (m *Model) flash(text string) tea.Cmd {
	m.toast = text
	m.toastSeq++
	seq := m.toastSeq
	return tea.Tick(4*time.Second, func(time.Time) tea.Msg { return toastExpireMsg{seq: seq} })
}

// saveConfig persists user configuration, best effort.
func (m *Model) saveConfig() {
	if m.cfg == nil {
		return
	}
	m.cfg.LastDir = m.cwd
	m.cfg.ShowHidden = m.showHidden
	m.cfg.SortMode = m.sortMode
	m.cfg.FFmpegPath = m.ffmpegPath
	m.cfg.FFprobePath = m.ffprobePath
	path := m.cfgFile
	if path == "" {
		_ = m.cfg.Save()
		return
	}
	_ = m.cfg.SaveTo(path)
}

// refreshHistory reloads the history entries.
func (m *Model) refreshHistory() {
	m.histCursor = 0
	m.histFilter = ""
}

// setScreen pushes the current screen and switches.
func (m *Model) setScreen(s screen) {
	if s == m.screen {
		return
	}
	m.stack = append(m.stack, m.screen)
	m.screen = s
	m.toast = ""
	switch s {
	case screenBrowser:
		m.filtering = false
		m.browserFilter = ""
		m.browserCursor = clampInt(m.browserCursor, 0, max(0, len(m.entries)-1))
	case screenWizard:
		m.wiz.ensureInput(m.cwd)
	case screenFilters:
		m.flt.sync(m.wiz)
	case screenMan:
		m.man.activate()
	case screenMetadata:
		m.meta.sync(m.probe)
	case screenCompare:
		m.compare.sync(m.probe, m.probePath)
	case screenQueue:
		m.queueCursor = 0
	}
	m.dashActions = m.buildDashActions()
}

// back returns to the previous screen (or the dashboard).
func (m *Model) back() {
	if len(m.stack) == 0 {
		m.screen = screenDashboard
		return
	}
	prev := m.stack[len(m.stack)-1]
	m.stack = m.stack[:len(m.stack)-1]
	m.screen = prev
	m.dashActions = m.buildDashActions()
}

// bodyHeight is the vertical space available to the active screen.
func (m *Model) bodyHeight() int {
	h := m.height - 1 - m.previewHeight() - 2
	if h < 5 {
		h = 5
	}
	return h
}

// previewHeight is the height reserved for the always-visible command preview.
func (m *Model) previewHeight() int {
	if m.height >= 34 {
		return 7
	}
	if m.height >= 24 {
		return 5
	}
	return 3
}

// contentWidth is the usable width inside the frame.
func (m *Model) contentWidth() int {
	w := m.width - 4
	if w < 30 {
		w = 30
	}
	return w
}

// queueSnapshot refreshes the cached job list from the runner.
func (m *Model) queueSnapshot() {
	m.queueSnap = m.jobs.Snapshot()
}

// activeJob returns the running or paused job.
func (m *Model) activeJob() *queue.Job {
	for i := range m.queueSnap.Jobs {
		j := &m.queueSnap.Jobs[i]
		if j.Status.Active() {
			return j
		}
	}
	return nil
}

// selectedJob returns the highlighted queue job.
func (m *Model) selectedJob() *queue.Job {
	if m.queueCursor >= 0 && m.queueCursor < len(m.queueSnap.Jobs) {
		return &m.queueSnap.Jobs[m.queueCursor]
	}
	return nil
}

// currentProbe returns the probe of the selected path, if it is current.
func (m *Model) currentProbe() *ffprobe.Result {
	if m.probe == nil || m.probePath == "" {
		return nil
	}
	if m.probe.Path != "" && filepath.Clean(m.probe.Path) != filepath.Clean(m.probePath) {
		return nil
	}
	return m.probe
}

// probeSelected starts an async probe/preview for the highlighted entry.
func (m *Model) probeSelected() tea.Cmd {
	e, ok := m.selectedEntry()
	if !ok || e.IsDir {
		return nil
	}
	if e.Kind == filesystem.KindSubtitle {
		m.probe = nil
		m.probePath = e.Path
		m.probeErr = "subtitle files have no streams to probe"
		m.previewArt = ""
		return nil
	}
	m.probePath = e.Path
	m.probeLoading = true
	m.probeErr = ""
	m.previewArt = ""
	m.previewLoading = true
	m.previewErr = ""
	cmds := []tea.Cmd{probeCmd(m.ctx, m.ffprobePath, e.Path)}
	if m.previewOn && m.ffmpegOK {
		audio := e.Kind == filesystem.KindAudio
		cmds = append(cmds, previewCmd(m.ctx, m.ffmpegPath, e.Path, 44, 14, audio))
	}
	return tea.Batch(cmds...)
}
