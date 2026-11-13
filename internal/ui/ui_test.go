package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/AMC-official/ffmpeg-tui/internal/config"
	"github.com/AMC-official/ffmpeg-tui/internal/ffmpeg"
	"github.com/AMC-official/ffmpeg-tui/internal/jobs"
	"github.com/AMC-official/ffmpeg-tui/internal/presets"
	"github.com/AMC-official/ffmpeg-tui/internal/queue"
)

func newTestModel(t *testing.T) *Model {
	t.Helper()
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("HOME", root)
	media := filepath.Join(root, "media")
	if err := os.MkdirAll(filepath.Join(media, "series"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(media, "movie.mp4"), []byte("not really a video"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(media, "music.flac"), make([]byte, 1024), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(media, "subtitles.srt"), []byte("1\n00:00:00,000 --> 00:00:01,000\nHi\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	cfg.LastDir = media
	m := New(Options{
		Config:      cfg,
		ConfigPath:  filepath.Join(root, "config", "config.json"),
		FFmpegPath:  "ffmpeg",
		FFprobePath: "ffprobe",
	})
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return m
}

func TestModelRendersDashboard(t *testing.T) {
	m := newTestModel(t)
	defer m.Close()
	view := m.View()
	for _, want := range []string{"FFmpeg-TUI", "Jobs", "New conversion", "Command preview", "Recent projects"} {
		if !strings.Contains(view, want) {
			t.Errorf("dashboard missing %q:\n%s", want, view)
		}
	}
	if lines := strings.Count(view, "\n") + 1; lines > m.height+2 {
		t.Errorf("view is %d lines for a %d-line terminal", lines, m.height)
	}
}

func TestEveryScreenRendersWithoutPanic(t *testing.T) {
	m := newTestModel(t)
	defer m.Close()

	m.startWizardFromSelection()
	m.batch.init(m.cwd)
	m.flt.sync(m.wiz)
	m.man.activate()
	m.tools.cat = 1
	m.compare.paths[0] = filepath.Join(m.cwd, "movie.mp4")
	demoJob := queue.Job{
		ID:       "demo-job",
		Name:     "demo.mp4",
		Status:   queue.Running,
		Progress: ffmpeg.Progress{Percent: 42, Frame: 120, FPS: 25, Duration: 10 * time.Second, OutTime: 4 * time.Second},
	}
	demoJob.Command = ffmpeg.Command{Bin: "ffmpeg", Input: filepath.Join(m.cwd, "movie.mp4"), Output: filepath.Join(m.cwd, "out.mp4")}
	m.jobFocus = &demoJob
	m.queueSnap.Jobs = []queue.Job{demoJob}

	screens := map[screen]string{
		screenDashboard: "Jobs",
		screenBrowser:   "Media browser",
		screenWizard:    "Input",
		screenFilters:   "Filters",
		screenQueue:     "Job queue",
		screenJobDetail: "Job detail",
		screenHistory:   "History",
		screenPresets:   "Presets",
		screenMan:       "Manual",
		screenMetadata:  "Metadata",
		screenStreams:   "Streams",
		screenCompare:   "Comparison",
		screenBatch:     "Batch process",
		screenTools:     "Tools",
	}
	for s, marker := range screens {
		m.screen = s
		view := m.View()
		if view == "" {
			t.Fatalf("screen %d rendered nothing", s)
		}
		if !strings.Contains(view, marker) {
			t.Errorf("screen %d missing %q:\n%s", s, marker, view)
		}
		if lines := strings.Count(view, "\n") + 1; lines > m.height+3 {
			t.Errorf("screen %d rendered %d lines for height %d", s, lines, m.height)
		}
	}
}

func TestTinyTerminalShowsGuidance(t *testing.T) {
	m := newTestModel(t)
	defer m.Close()
	m.Update(tea.WindowSizeMsg{Width: 40, Height: 10})
	view := m.View()
	if !strings.Contains(view, "Resize") {
		t.Fatalf("tiny terminal should explain the minimum size:\n%s", view)
	}
}

func TestHelpAndPaletteOverlays(t *testing.T) {
	m := newTestModel(t)
	defer m.Close()
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	if !m.help.open || !strings.Contains(m.View(), "keyboard reference") {
		t.Fatal("? should open the help overlay")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.help.open {
		t.Fatal("esc should close help")
	}

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(":")})
	if !m.palette.open {
		t.Fatal(": should open the palette")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("man crop")})
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.screen != screenMan {
		t.Fatalf(":man crop should open the man screen, got %d", m.screen)
	}
	if e, ok := m.man.selected(); !ok || e.Name != "crop" {
		t.Fatalf("expected crop page, got %v %v", e.Name, ok)
	}
}

func TestGlobalSearchFindsLoudnessFilters(t *testing.T) {
	m := newTestModel(t)
	defer m.Close()
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("normalize")})
	if !m.search.open {
		t.Fatal("search overlay should be open")
	}
	found := map[string]bool{}
	for _, h := range m.search.hits {
		if h.kind == "man" {
			found[h.title] = true
		}
	}
	for _, want := range []string{"loudnorm", "dynaudnorm", "volume"} {
		if !found[want] {
			t.Errorf("search did not surface manual page %s", want)
		}
	}
	if !strings.Contains(m.View(), "Manual pages") {
		t.Error("search results should be grouped by section")
	}
}

func TestWizardBuildsValidCommand(t *testing.T) {
	m := newTestModel(t)
	defer m.Close()
	in := filepath.Join(m.cwd, "movie.mp4")
	m.wiz.reset(m.cfg, in)
	m.wiz.buildOutput(in, nil)
	m.wiz.containerIdx = containerIndex("mp4")
	m.wiz.syncChoiceDefaults()
	container, _ := presets.ContainerByExt("mp4")
	m.wiz.videoIdx = indexOfOption(videoChoices(container), "libx264")
	out := filepath.Join(m.cwd, "out.mp4")
	m.wiz.output = out

	cmd, err := m.buildWizardCommand()
	if err != nil {
		t.Fatalf("build command: %v", err)
	}
	joined := strings.Join(cmd.Args, " ")
	for _, want := range []string{"-c:v libx264", "-crf", "-c:a aac", "-movflags +faststart"} {
		if !strings.Contains(joined, want) {
			t.Errorf("command missing %q: %s", want, joined)
		}
	}
	if cmd.Validate() != nil {
		t.Fatalf("valid command rejected: %v", cmd.Validate())
	}
	if !strings.Contains(cmd.Preview(), "ffmpeg \\") {
		t.Error("preview should render a multi-line command")
	}
}

func TestWizardRejectsMissingInput(t *testing.T) {
	m := newTestModel(t)
	defer m.Close()
	m.wiz.reset(m.cfg, "")
	m.wiz.output = filepath.Join(m.cwd, "out.mp4")
	if _, err := m.buildWizardCommand(); err == nil {
		t.Fatal("wizard must refuse to build without an input")
	}
}

func TestFilterBuilderAppliesToWizard(t *testing.T) {
	m := newTestModel(t)
	defer m.Close()
	m.startWizardFromSelection()
	m.flt.sync(m.wiz)
	m.screen = screenFilters
	m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // add the highlighted filter
	if len(m.flt.chain) != 1 {
		t.Fatalf("filter not added: %d", len(m.flt.chain))
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	if len(m.wiz.filters) != 1 {
		t.Fatalf("filter chain not applied: %d", len(m.wiz.filters))
	}
}

func TestBrowserNavigation(t *testing.T) {
	m := newTestModel(t)
	defer m.Close()
	m.setScreen(screenBrowser)
	if len(m.entries) == 0 {
		t.Fatal("browser should list entries")
	}
	// Directories sort first.
	if !m.entries[0].IsDir || m.entries[0].Name != "series" {
		t.Fatalf("unexpected first entry: %+v", m.entries[0])
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if filepath.Base(m.cwd) != "series" {
		t.Fatalf("enter should descend into the directory, cwd=%s", m.cwd)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if m.cwd != filepath.Dir(filepath.Join(m.cwd, "series")) {
		t.Fatalf("backspace should go up, cwd=%s", m.cwd)
	}
}

func TestQueueEnqueueValidationFailure(t *testing.T) {
	m := newTestModel(t)
	defer m.Close()
	m.wiz.reset(m.cfg, "")
	m.wiz.output = filepath.Join(m.cwd, "nope.mp4")
	m.enqueueWizard()
	if m.wiz.err == "" {
		t.Fatal("queueing a command with no input must surface an error")
	}
	if len(m.queueSnap.Jobs) != 0 {
		t.Fatal("invalid command must not enter the queue")
	}
}

func TestEntryFilteringKeepsOriginalListing(t *testing.T) {
	m := newTestModel(t)
	defer m.Close()
	m.setScreen(screenBrowser)
	all := len(m.entries)
	m.browserFilter = "movie"
	m.entries = filterEntries(m.allEntries, m.browserFilter)
	if len(m.entries) != 1 || len(m.allEntries) != all {
		t.Fatalf("filtering destroyed the listing: %d/%d", len(m.entries), len(m.allEntries))
	}
	m.browserFilter = ""
	m.entries = m.allEntries
	if len(m.entries) != all {
		t.Fatal("clearing the filter should restore the listing")
	}
}

func TestAudioOnlyWizardDisablesVideo(t *testing.T) {
	m := newTestModel(t)
	defer m.Close()
	in := filepath.Join(m.cwd, "music.flac")
	m.wiz.reset(m.cfg, in)
	m.wiz.selectContainerByExt("mp3")
	m.wiz.buildOutput(in, nil)
	cmd, err := m.buildWizardCommand()
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(cmd.Args, " ")
	if !strings.Contains(joined, "-vn") || !strings.Contains(joined, "-c:a libmp3lame") {
		t.Fatalf("audio-only command wrong: %s", joined)
	}
}

// TestJobPumpReArmsAfterFinishedEvent guards the regression where the finished
// branch returned only the toast command, leaving the runner event channel
// unread and freezing progress for every later job.
func TestJobPumpReArmsAfterFinishedEvent(t *testing.T) {
	m := newTestModel(t)
	defer m.Close()

	for _, status := range []queue.Status{queue.Done, queue.Failed, queue.Canceled} {
		_, cmd := m.Update(jobEventMsg{ev: jobs.Event{
			Kind: jobs.EventFinished,
			Job:  queue.Job{Name: "job", Status: status, Err: "boom"},
		}})
		if cmd == nil {
			t.Fatalf("%s: no command returned", status)
		}
		msg := cmd()
		batch, ok := msg.(tea.BatchMsg)
		if !ok {
			t.Fatalf("%s: expected a batch containing the event pump, got %T", status, msg)
		}
		if len(batch) < 2 {
			t.Fatalf("%s: pump not re-armed, batch has %d commands", status, len(batch))
		}
	}
}
