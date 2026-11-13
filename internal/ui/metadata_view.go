package ui

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/AMC-official/ffmpeg-tui/internal/ffmpeg"
	"github.com/AMC-official/ffmpeg-tui/internal/ffprobe"
	"github.com/AMC-official/ffmpeg-tui/internal/filesystem"
	"github.com/AMC-official/ffmpeg-tui/internal/metadata"
	"github.com/AMC-official/ffmpeg-tui/internal/widgets"
)

// metaState is the metadata editor.
type metaState struct {
	values metadata.Values
	source map[string]string
	path   string
	cursor int
	err    string
}

func newMetaState() metaState { return metaState{values: metadata.Values{}} }

// sync reloads the editor from a probe report.
func (s *metaState) sync(p *ffprobe.Result) {
	if p == nil {
		if s.path != "" {
			s.values = metadata.Values{}
			s.source = nil
			s.path = ""
		}
		return
	}
	if s.path == p.Path && s.values != nil {
		return
	}
	s.path = p.Path
	s.values = metadata.New(p.Format.Tags)
	s.source = map[string]string{}
	for k, v := range p.Format.Tags {
		s.source[strings.ToLower(k)] = v
	}
	s.cursor = 0
	s.err = ""
}

func (m *Model) updateMetadata(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := &m.meta
	key := msg.String()
	switch key {
	case "esc":
		m.back()
		return m, nil
	case "up", "k":
		s.cursor = clampInt(s.cursor-1, 0, len(metadata.Fields)-1)
		return m, nil
	case "down", "j":
		s.cursor = clampInt(s.cursor+1, 0, len(metadata.Fields)-1)
		return m, nil
	case "home", "g":
		s.cursor = 0
		return m, nil
	case "end", "G":
		s.cursor = len(metadata.Fields) - 1
		return m, nil
	case "pgup", "ctrl+b":
		s.cursor = clampInt(s.cursor-5, 0, len(metadata.Fields)-1)
		return m, nil
	case "pgdown", "ctrl+f":
		s.cursor = clampInt(s.cursor+5, 0, len(metadata.Fields)-1)
		return m, nil
	case "enter", "e":
		f := metadata.Fields[clampInt(s.cursor, 0, len(metadata.Fields)-1)]
		return m, m.startEdit(editTarget{kind: "meta-value", key: f.Key}, s.values.Get(f.Key), f.Label)
	case "backspace", "delete":
		f := metadata.Fields[clampInt(s.cursor, 0, len(metadata.Fields)-1)]
		s.values.Set(f.Key, "")
		return m, nil
	case "s", "ctrl+s":
		return m, m.saveMetadata()
	case "r":
		if m.currentProbe() != nil {
			s.path = ""
			s.sync(m.currentProbe())
			return m, m.flash("metadata reloaded from the file")
		}
		return m, nil
	}
	return m, nil
}

// saveMetadata queues a stream-copy remux that writes the edited tags.
func (m *Model) saveMetadata() tea.Cmd {
	s := &m.meta
	if s.path == "" {
		return m.flash("✖ open a media file first")
	}
	if !m.ffmpegOK {
		return m.flash("✖ ffmpeg is not available")
	}
	pairs := s.values.Pairs()
	if len(pairs) == 0 {
		return m.flash("✖ no metadata to write")
	}
	ext := strings.TrimPrefix(filepath.Ext(s.path), ".")
	if ext == "" {
		ext = "mkv"
	}
	out := strings.TrimSuffix(s.path, filepath.Ext(s.path)) + "-tagged." + ext
	out = filesystem.UniquePath(out)

	b := ffmpeg.NewBuilder(m.ffmpegPath, s.path)
	b.VideoCodec("copy").AudioCodec("copy").SubtitleCodec("copy")
	b.Map("0")
	b.MetadataPairs(pairs)
	if ext == "mp4" || ext == "mov" || ext == "m4a" {
		b.Faststart(true)
	}
	cmd := b.Build(out)
	cmd.Label = "metadata edit • " + filepath.Base(out)
	if err := cmd.Validate(); err != nil {
		s.err = err.Error()
		return nil
	}
	s.err = ""
	m.jobs.Enqueue(filepath.Base(out), cmd)
	m.queueSnapshot()
	m.stack = append(m.stack, m.screen)
	m.screen = screenQueue
	return m.flash("queued metadata write → " + filepath.Base(out))
}

func (m *Model) viewMetadata() string {
	st := m.styles
	s := &m.meta
	width := m.contentWidth()
	height := m.bodyHeight()
	leftW := width * 58 / 100
	if leftW < 40 {
		leftW = 40
	}
	rightW := width - leftW - 2
	if rightW < 26 {
		rightW = 26
	}

	avail := height - 4 // footer hint plus panel borders
	if avail < 3 {
		avail = 3
	}
	start := listWindowStart(s.cursor, avail, len(metadata.Fields))
	end := min(len(metadata.Fields), start+avail)
	var rows []string
	for i := start; i < end; i++ {
		f := metadata.Fields[i]
		val := s.values.Get(f.Key)
		if val == "" {
			val = st.Faint.Render("—")
		}
		changed := s.source[f.Key] != s.values.Get(f.Key)
		marker := " "
		if changed {
			marker = st.Warn.Render("*")
		}
		line := fmt.Sprintf("%s %s %s", marker, st.Muted.Render(widgets.PadRight(f.Label, 14)), widgets.Truncate(val, leftW-24))
		rows = append(rows, st.SelectedRow(line, leftW-4, i == s.cursor))
	}
	rows = append(rows, "")
	rows = append(rows, st.Faint.Render("Enter edits · Backspace clears · s saves a tagged copy · * = changed"))
	if s.err != "" {
		rows = append(rows, "", st.Err.Render(s.err))
	}
	title := "Metadata"
	if label := listRangeLabel(start, end, len(metadata.Fields)); label != "" {
		title += " · " + label
	}
	title += " · " + filepath.Base(firstNonEmpty(s.path, "no file"))
	left := st.Panel(title, strings.Join(rows, "\n"), leftW, height, true)

	var right []string
	right = append(right, st.Rule("Source tags", rightW-4))
	if s.path == "" {
		right = append(right, st.Faint.Render("Select a media file and open the metadata editor."))
	} else if len(s.source) == 0 {
		right = append(right, st.Faint.Render("The file has no container tags."))
	} else {
		for _, f := range metadata.Fields {
			if v, ok := s.source[f.Key]; ok && v != "" {
				right = append(right, st.Label(f.Label, widgets.Truncate(v, rightW-20)))
			}
		}
	}
	right = append(right, "")
	right = append(right, st.Rule("New tags", rightW-4))
	pairs := s.values.Pairs()
	if len(pairs) == 0 {
		right = append(right, st.Faint.Render("No tags set."))
	}
	for _, kv := range pairs {
		right = append(right, st.Label(kv.Key, widgets.Truncate(kv.Value, rightW-20)))
	}
	right = append(right, "")
	right = append(right, st.Rule("Output", rightW-4))
	if s.path != "" {
		out := strings.TrimSuffix(filepath.Base(s.path), filepath.Ext(s.path)) + "-tagged" + filepath.Ext(s.path)
		right = append(right, st.Text.Render(widgets.Truncate(out, rightW-4)))
		right = append(right, st.Faint.Render("stream copy (no re-encode)"))
	}
	panel := st.Panel("Preview", strings.Join(fitLines(right, height-2), "\n"), rightW, height, false)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", panel)
}
