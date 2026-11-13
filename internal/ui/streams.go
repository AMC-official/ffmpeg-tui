package ui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/AMC-official/ffmpeg-tui/internal/filesystem"
	"github.com/AMC-official/ffmpeg-tui/internal/widgets"
)

func (m *Model) updateStreams(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.back()
	case "up", "k":
		m.streamsScroll = max(0, m.streamsScroll-1)
	case "down", "j":
		m.streamsScroll++
	case "pgup":
		m.streamsScroll = max(0, m.streamsScroll-10)
	case "pgdown":
		m.streamsScroll += 10
	case "home":
		m.streamsScroll = 0
	case "c":
		m.startWizardFromSelection()
	case "m":
		if m.currentProbe() != nil {
			m.setScreen(screenMetadata)
		}
	}
	return m, nil
}

func (m *Model) viewStreams() string {
	st := m.styles
	p := m.currentProbe()
	width := m.contentWidth()
	height := m.bodyHeight()
	if p == nil {
		body := st.Faint.Render("No file selected. Open a media file in the browser first.")
		return st.Panel("Streams", body, width, height, true)
	}

	var b []string
	add := func(s string) { b = append(b, s) }
	add(st.Text.Bold(true).Render(filepath.Base(p.Path)) + "  " + st.Faint.Render(p.Format.FormatLongName))
	add(st.Label("Duration", filesystem.HumanDuration(p.Duration())) + "   " +
		st.Label("Size", filesystem.HumanSize(p.Size())) + "   " +
		st.Label("Bitrate", filesystem.HumanBitrate(p.BitRate())))

	add("")
	cols := []widgets.Column{
		{Header: "#", Width: 3, Right: true},
		{Header: "TYPE", Width: 9},
		{Header: "CODEC", Width: 12},
		{Header: "DETAILS", Width: max(16, width-62)},
		{Header: "RATE", Width: 10, Right: true},
		{Header: "LANG", Width: 5},
		{Header: "FLAGS", Width: 12},
	}
	table := widgets.Table(cols, streamRows(p), -1)
	for i, line := range table {
		if i == 0 {
			add(st.Muted.Render(line))
			add(st.Faint.Render(strings.Repeat("─", min(width, len([]rune(line))))))
		} else {
			add(st.Text.Render(line))
		}
	}

	if len(p.Chapters) > 0 {
		add("")
		add(st.Rule(fmt.Sprintf("Chapters (%d)", len(p.Chapters)), width))
		for _, c := range p.Chapters {
			add(st.Label(c.ChapterName(), filesystem.HumanDuration(parseClock(c.StartTime))+" → "+filesystem.HumanDuration(parseClock(c.EndTime))))
		}
	}
	if atts := p.AttachmentStreams(); len(atts) > 0 {
		add("")
		add(st.Rule(fmt.Sprintf("Attachments (%d)", len(atts)), width))
		for _, a := range atts {
			add(st.Text.Render("  "+firstNonEmpty(a.AttachmentName, "unnamed")) + "  " + st.Faint.Render(firstNonEmpty(a.AttachmentMime, a.CodecLongName)))
		}
	}
	if subs := p.SubtitleStreams(); len(subs) > 0 {
		add("")
		add(st.Rule("Subtitle disposition", width))
		for _, s := range subs {
			flags := []string{}
			for _, key := range []string{"default", "forced", "hearing_impaired", "comment", "visual_impaired"} {
				if s.Disposition[key] == 1 {
					flags = append(flags, key)
				}
			}
			add(st.Text.Render(fmt.Sprintf("  #%d %s %s", s.Index, s.CodecName, s.Language())) + st.Faint.Render("  "+strings.Join(flags, ", ")))
		}
	}
	if p.Format.Tags["title"] != "" {
		add("")
		add(st.Label("Title", p.Format.Tags["title"]))
	}
	if m.streamsScroll > len(b)-1 {
		m.streamsScroll = max(0, len(b)-1)
	}
	body := fitLines(b[m.streamsScroll:], height-2)
	for len(body) < height-2 {
		body = append(body, "")
	}
	return st.Panel("Stream viewer · "+filepath.Base(p.Path), strings.Join(body, "\n"), width, height, true)
}

func parseClock(s string) time.Duration {
	if s == "" {
		return 0
	}
	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		return 0
	}
	var h, m int
	var sec float64
	fmt.Sscanf(parts[0], "%d", &h)
	fmt.Sscanf(parts[1], "%d", &m)
	fmt.Sscanf(parts[2], "%f", &sec)
	return time.Duration((float64(h)*3600 + float64(m)*60 + sec) * float64(time.Second))
}
