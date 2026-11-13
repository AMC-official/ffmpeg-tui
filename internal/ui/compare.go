package ui

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/AMC-official/ffmpeg-tui/internal/ffprobe"
	"github.com/AMC-official/ffmpeg-tui/internal/filesystem"
	"github.com/AMC-official/ffmpeg-tui/internal/widgets"
)

// compareState compares two media files.
type compareState struct {
	paths   [2]string
	probes  [2]*ffprobe.Result
	errs    [2]string
	loading [2]bool
	slot    int
}

func newCompareState() compareState { return compareState{} }

// sync seeds the first slot with the current file.
func (s *compareState) sync(p *ffprobe.Result, path string) {
	if p == nil || path == "" {
		return
	}
	if s.paths[0] == "" {
		s.paths[0] = path
		s.probes[0] = p
		return
	}
	if s.paths[0] == path {
		s.probes[0] = p
	}
	if s.paths[1] == path {
		s.probes[1] = p
	}
}

func (m *Model) updateCompare(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := &m.compare
	switch msg.String() {
	case "esc":
		m.back()
	case "1":
		return m, m.startEdit(editTarget{kind: "compare-path", index: 0}, s.paths[0], "input file")
	case "2":
		return m, m.startEdit(editTarget{kind: "compare-path", index: 1}, s.paths[1], "output file")
	case "tab":
		s.slot = 1 - s.slot
	case "r":
		var cmds []tea.Cmd
		for i := 0; i < 2; i++ {
			if s.paths[i] != "" {
				s.loading[i] = true
				s.probes[i] = nil
				s.errs[i] = ""
				cmds = append(cmds, compareProbeCmd(m.ctx, m.ffprobePath, i, s.paths[i]))
			}
		}
		return m, tea.Batch(cmds...)
	case "a":
		if p := m.currentProbe(); p != nil {
			s.paths[0] = m.probePath
			s.probes[0] = p
		}
	case "b":
		if e, ok := m.selectedEntry(); ok && !e.IsDir {
			s.paths[1] = e.Path
			s.probes[1] = nil
			s.loading[1] = true
			return m, compareProbeCmd(m.ctx, m.ffprobePath, 1, e.Path)
		}
	}
	return m, nil
}

func (m *Model) viewCompare() string {
	st := m.styles
	s := &m.compare
	width := m.contentWidth()
	height := m.bodyHeight()
	slotH := 8
	if height < 24 {
		slotH = 6
	}
	topW := (width - 2) / 2
	slotA := st.Panel("Input", m.compareSlot(0, topW-4, slotH-2), topW, slotH, s.slot == 0)
	slotB := st.Panel("Output", m.compareSlot(1, topW-4, slotH-2), topW, slotH, s.slot == 1)
	top := lipgloss.JoinHorizontal(lipgloss.Top, slotA, "  ", slotB)

	bottomH := height - slotH - 1
	if bottomH < 6 {
		bottomH = 6
	}
	return top + "\n" + st.Panel("Comparison", m.compareTable(width-4, bottomH-2), width, bottomH, false)
}

func (m *Model) compareSlot(i, width, rows int) string {
	st := m.styles
	s := &m.compare
	var b []string
	path := s.paths[i]
	if path == "" {
		b = append(b, st.Faint.Render("empty — press "+fmt.Sprintf("%d", i+1)+" to enter a path"))
	} else {
		b = append(b, st.Text.Render(widgets.TruncateMiddle(path, width)))
		if s.loading[i] {
			b = append(b, st.Muted.Render("probing…"))
		} else if s.errs[i] != "" {
			b = append(b, st.Err.Render(widgets.Truncate(s.errs[i], width)))
		} else if p := s.probes[i]; p != nil {
			b = append(b, m.probeSummaryLines(p)...)
		}
	}
	return strings.Join(fitLines(b, rows), "\n")
}

func (m *Model) compareTable(width, rows int) string {
	st := m.styles
	s := &m.compare
	a, b := s.probes[0], s.probes[1]
	if a == nil || b == nil {
		return st.Faint.Render("Enter both files to compare resolution, codec, bitrate, duration, size and compression ratio.")
	}
	av, bv := a.Video(), b.Video()
	resA, resB := "—", "—"
	if av != nil {
		resA = fmt.Sprintf("%dx%d", av.Width, av.Height)
	}
	if bv != nil {
		resB = fmt.Sprintf("%dx%d", bv.Width, bv.Height)
	}
	codecA, codecB := "—", "—"
	if av != nil {
		codecA = av.CodecName
	}
	if bv != nil {
		codecB = bv.CodecName
	}
	ratio := "—"
	if a.Size() > 0 && b.Size() > 0 {
		r := float64(b.Size()) / float64(a.Size())
		ratio = fmt.Sprintf("%.2f× (%.0f%% of the input)", r, r*100)
	} else if a.BitRate() > 0 && b.BitRate() > 0 {
		r := float64(b.BitRate()) / float64(a.BitRate())
		ratio = fmt.Sprintf("%.2f× by bitrate", r)
	}
	pairs := [][3]string{
		{"Resolution", resA, resB},
		{"Codec", codecA, codecB},
		{"Bitrate", filesystem.HumanBitrate(a.BitRate()), filesystem.HumanBitrate(b.BitRate())},
		{"Duration", filesystem.HumanDuration(a.Duration()), filesystem.HumanDuration(b.Duration())},
		{"File size", filesystem.HumanSize(a.Size()), filesystem.HumanSize(b.Size())},
		{"Streams", fmt.Sprintf("%d", len(a.Streams)), fmt.Sprintf("%d", len(b.Streams))},
		{"Compression", ratio, ""},
	}
	var out []string
	out = append(out, st.Muted.Render(widgets.PadRight("METRIC", 14)+widgets.PadRight("INPUT", width/2-8)+"OUTPUT"))
	out = append(out, st.Faint.Render(strings.Repeat("─", min(width, 70))))
	for _, p := range pairs {
		out = append(out, st.Text.Render(widgets.PadRight(p[0], 14))+
			st.Blue.Render(widgets.PadRight(widgets.Truncate(p[1], width/2-8), width/2-8))+
			st.Accent.Render(widgets.Truncate(p[2], width/2-8)))
	}
	out = append(out, "")
	out = append(out, st.Faint.Render("1 / 2 edit paths · a use the browser file as input · b use the highlighted browser file as output · r re-probe"))
	return strings.Join(fitLines(out, rows), "\n")
}

// comparePathLabel is used by help text.
func comparePathLabel(p string) string { return filepath.Base(p) }
