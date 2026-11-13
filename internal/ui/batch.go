package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ffmpeg-tui/ffmpeg-tui/internal/ffmpeg"
	"github.com/ffmpeg-tui/ffmpeg-tui/internal/filesystem"
	"github.com/ffmpeg-tui/ffmpeg-tui/internal/presets"
	"github.com/ffmpeg-tui/ffmpeg-tui/internal/widgets"
)

// batchState configures folder-wide processing.
type batchState struct {
	source       string
	outputDir    string
	template     string
	presetName   string
	filterText   string
	recursive    bool
	skipExisting bool
	mirror       bool
	cursor       int
	files        []filesystem.Entry
	scanned      bool
	err          string
}

func newBatchState() batchState {
	return batchState{
		template:     "{name}",
		presetName:   "H264",
		skipExisting: true,
	}
}

// init seeds the batch screen from the current directory.
func (b *batchState) init(cwd string) {
	if b.source == "" {
		b.source = cwd
	}
	if b.outputDir == "" {
		b.outputDir = filepath.Join(cwd, "converted")
	}
	b.refresh(false)
}

// refresh scans the source folder for media files.
func (b *batchState) refresh(showHidden bool) {
	b.files = nil
	b.scanned = true
	b.err = ""
	if b.source == "" {
		return
	}
	if b.recursive {
		files, err := filesystem.WalkMedia(b.source, showHidden)
		if err != nil {
			b.err = err.Error()
			return
		}
		b.files = files
		return
	}
	entries, err := filesystem.List(b.source, showHidden)
	if err != nil {
		b.err = err.Error()
		return
	}
	for _, e := range entries {
		if e.IsDir {
			continue
		}
		if e.Kind == filesystem.KindVideo || e.Kind == filesystem.KindAudio {
			b.files = append(b.files, e)
		}
	}
}

// batchRows are the editable rows of the batch form.
var batchRows = []string{
	"Source folder",
	"Output folder",
	"Filename template",
	"Preset",
	"Recursive",
	"Skip existing",
	"Mirror directory tree",
	"Name filter",
	"Queue jobs",
}

func (m *Model) updateBatch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	b := &m.batch
	key := msg.String()
	last := len(batchRows) - 1
	switch key {
	case "esc":
		m.back()
		return m, nil
	case "tab", "down", "j":
		b.cursor = (b.cursor + 1) % len(batchRows)
		return m, nil
	case "shift+tab", "up", "k":
		b.cursor = (b.cursor - 1 + len(batchRows)) % len(batchRows)
		return m, nil
	case "left", "h":
		if b.cursor == 3 {
			b.cyclePreset(-1)
		}
		return m, nil
	case "right", "l":
		if b.cursor == 3 {
			b.cyclePreset(1)
		}
		return m, nil
	case "r":
		b.refresh(m.showHidden)
		return m, m.flash(fmt.Sprintf("%d media files found", len(b.files)))
	case "q", "ctrl+s":
		return m, m.queueBatch()
	case "enter", " ":
		switch b.cursor {
		case 0:
			return m, m.startEdit(editTarget{kind: "batch-source"}, b.source, "source directory")
		case 1:
			return m, m.startEdit(editTarget{kind: "batch-output"}, b.outputDir, "output directory")
		case 2:
			return m, m.startEdit(editTarget{kind: "batch-template"}, b.template, "{name}.{ext}")
		case 3:
			b.cyclePreset(1)
		case 4:
			b.recursive = !b.recursive
			b.refresh(m.showHidden)
		case 5:
			b.skipExisting = !b.skipExisting
		case 6:
			b.mirror = !b.mirror
		case 7:
			return m, m.startEdit(editTarget{kind: "batch-filter"}, b.filterText, "substring of the filename")
		case last:
			return m, m.queueBatch()
		}
		return m, nil
	case "s":
		return m, m.queueBatch()
	}
	return m, nil
}

func (b *batchState) cyclePreset(delta int) {
	list := batchPresetNames()
	idx := 0
	for i, n := range list {
		if n == b.presetName {
			idx = i
		}
	}
	idx = (idx + delta + len(list)) % len(list)
	b.presetName = list[idx]
}

func batchPresetNames() []string {
	var names []string
	for _, p := range presets.Builtins() {
		names = append(names, p.Name)
	}
	return names
}

// queueBatch turns every matching file into a queued job.
func (m *Model) queueBatch() tea.Cmd {
	b := &m.batch
	if !b.scanned {
		b.refresh(m.showHidden)
	}
	if len(b.files) == 0 {
		return m.flash("✖ no media files found in " + b.source)
	}
	p, ok := presets.ByName(b.presetName, m.cfg.Presets)
	if !ok {
		return m.flash("✖ unknown preset " + b.presetName)
	}
	if err := filesystem.EnsureDir(b.outputDir); err != nil {
		return m.flash("✖ cannot create output directory: " + err.Error())
	}
	ext := p.Container
	if ext == "" {
		ext = "mkv"
	}
	queued, skipped, failed := 0, 0, 0
	for i, f := range b.files {
		if b.filterText != "" && !strings.Contains(strings.ToLower(f.Name), strings.ToLower(b.filterText)) {
			continue
		}
		if queued >= 500 {
			break
		}
		var out string
		if b.mirror {
			out = filesystem.Mirror(b.source, b.outputDir, f.Path)
			out = strings.TrimSuffix(out, filepath.Ext(out)) + "." + ext
			if err := filesystem.EnsureDir(filepath.Dir(out)); err != nil {
				failed++
				continue
			}
		} else {
			tpl := b.template
			if filepath.Ext(tpl) == "" {
				tpl += "." + ext
			}
			out = filesystem.TemplateOutput(f.Path, tpl, b.outputDir, i+1)
		}
		if b.skipExisting {
			if _, err := os.Stat(out); err == nil {
				skipped++
				continue
			}
		}
		builder := ffmpeg.NewBuilder(m.ffmpegPath, f.Path)
		if d, err := previewDuration(m.ctx, m.ffprobePath, f.Path); err == nil {
			builder.WithDuration(d)
		}
		p.Apply(builder)
		cmd := builder.Build(out)
		if err := cmd.Validate(); err != nil {
			failed++
			continue
		}
		m.jobs.Enqueue(filepath.Base(out), cmd)
		queued++
	}
	m.queueSnapshot()
	m.stack = append(m.stack, m.screen)
	m.screen = screenQueue
	msg := fmt.Sprintf("queued %d jobs", queued)
	if skipped > 0 {
		msg += fmt.Sprintf(", %d skipped", skipped)
	}
	if failed > 0 {
		msg += fmt.Sprintf(", %d skipped (invalid)", failed)
	}
	return m.flash(msg)
}

func (m *Model) viewBatch() string {
	st := m.styles
	b := &m.batch
	width := m.contentWidth()
	height := m.bodyHeight()
	leftW := width * 52 / 100
	if leftW < 40 {
		leftW = 40
	}
	rightW := width - leftW - 2
	if rightW < 30 {
		rightW = 30
	}

	var rows []string
	rows = append(rows, st.Faint.Render("Process a whole folder with one preset. Existing outputs are skipped when enabled."))
	for i, label := range batchRows {
		var value string
		switch i {
		case 0:
			value = b.source
		case 1:
			value = b.outputDir
		case 2:
			value = b.template
		case 3:
			value = b.presetName
		case 4:
			value = boolLabel(b.recursive)
		case 5:
			value = boolLabel(b.skipExisting)
		case 6:
			value = boolLabel(b.mirror)
		case 7:
			value = b.filterText
			if value == "" {
				value = st.Faint.Render("(all media files)")
			}
		case 8:
			value = st.Accent.Render(fmt.Sprintf("Queue %d jobs", m.batchJobEstimate()))
		}
		line := fmt.Sprintf("%s %s", st.Muted.Render(widgets.PadRight(label, 22)), widgets.Truncate(value, leftW-30))
		rows = append(rows, st.SelectedRow(line, leftW-4, i == b.cursor))
	}
	rows = append(rows, "")
	if b.err != "" {
		rows = append(rows, st.Err.Render(b.err))
	} else {
		rows = append(rows, st.Faint.Render(fmt.Sprintf("%d files scanned · Enter edits or toggles · q queues", len(b.files))))
	}
	rows = append(rows, st.Faint.Render("Template placeholders: {name} {ext} {index} {date} {dir}"))
	left := st.Panel("Batch process", strings.Join(rows, "\n"), leftW, height, true)

	var right []string
	right = append(right, st.Rule(fmt.Sprintf("Files (%d)", len(b.files)), rightW-4))
	limit := height - 8
	if limit < 4 {
		limit = 4
	}
	shown := 0
	for _, f := range b.files {
		if b.filterText != "" && !strings.Contains(strings.ToLower(f.Name), strings.ToLower(b.filterText)) {
			continue
		}
		if shown >= limit {
			right = append(right, st.Faint.Render("…"))
			break
		}
		line := "  " + widgets.Truncate(f.Name, rightW-18) + " " + st.Faint.Render(filesystem.HumanSize(f.Size))
		right = append(right, st.Text.Render(line))
		shown++
	}
	if len(b.files) == 0 {
		right = append(right, st.Faint.Render("No media files here. Check the source folder or enable recursive mode."))
	}
	right = append(right, "", st.Rule("Output", rightW-4))
	right = append(right, st.Text.Render(widgets.TruncateMiddle(b.outputDir, rightW-4)))
	right = append(right, st.Faint.Render("files are named by the template; mirror keeps the tree"))
	panel := st.Panel("Preview", strings.Join(fitLines(right, height-2), "\n"), rightW, height, false)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", panel)
}

func (b *batchState) jobEstimate() int {
	n := 0
	for _, f := range b.files {
		if b.filterText != "" && !strings.Contains(strings.ToLower(f.Name), strings.ToLower(b.filterText)) {
			continue
		}
		n++
	}
	return n
}

func (m *Model) batchJobEstimate() int { return m.batch.jobEstimate() }

func boolLabel(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}
