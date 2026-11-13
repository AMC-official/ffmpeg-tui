package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/AMC-official/ffmpeg-tui/internal/presets"
	"github.com/AMC-official/ffmpeg-tui/internal/widgets"
)

// toolState is the audio/video/subtitle recipe browser.
type toolState struct {
	cursor int
	cat    int
}

var toolCategories = []string{"All", "Audio", "Video", "Subtitle"}

func newToolState() toolState { return toolState{} }

// filtered returns tools for the active category.
func (s *toolState) filtered() []presets.Tool {
	all := presets.Tools()
	if s.cat <= 0 {
		return all
	}
	cat := toolCategories[s.cat]
	var out []presets.Tool
	for _, t := range all {
		if t.Category == cat {
			out = append(out, t)
		}
	}
	return out
}

func (m *Model) updateTools(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := &m.tools
	tools := s.filtered()
	switch msg.String() {
	case "esc":
		m.back()
	case "tab":
		s.cat = (s.cat + 1) % len(toolCategories)
		s.cursor = 0
	case "shift+tab":
		s.cat = (s.cat - 1 + len(toolCategories)) % len(toolCategories)
		s.cursor = 0
	case "up", "k":
		s.cursor = clampInt(s.cursor-1, 0, max(0, len(tools)-1))
	case "down", "j":
		s.cursor = clampInt(s.cursor+1, 0, max(0, len(tools)-1))
	case "home", "g":
		s.cursor = 0
	case "end", "G":
		s.cursor = max(0, len(tools)-1)
	case "pgup", "ctrl+b":
		s.cursor = clampInt(s.cursor-10, 0, max(0, len(tools)-1))
	case "pgdown", "ctrl+f":
		s.cursor = clampInt(s.cursor+10, 0, max(0, len(tools)-1))
	case "enter":
		if s.cursor >= 0 && s.cursor < len(tools) {
			return m, m.startToolWizard(tools[s.cursor])
		}
	}
	return m, nil
}

func (m *Model) viewTools() string {
	st := m.styles
	s := &m.tools
	width := m.contentWidth()
	height := m.bodyHeight()
	tools := s.filtered()

	leftW := width * 40 / 100
	if leftW < 34 {
		leftW = 34
	}
	if leftW > 58 {
		leftW = 58
	}
	rightW := width - leftW - 2

	var tabs []string
	for _, c := range toolCategories {
		tabs = append(tabs, c)
	}
	tabsRow := st.Faint.Render(widgets.Tabs(tabs, s.cat))

	type toolRow struct {
		text string
		item int
	}
	var listRows []toolRow
	lastCat := ""
	for i, t := range tools {
		if t.Category != lastCat {
			listRows = append(listRows, toolRow{text: st.Muted.Render("── " + t.Category), item: -1})
			lastCat = t.Category
		}
		listRows = append(listRows, toolRow{
			text: st.SelectedRow(widgets.Truncate("  "+t.Name, leftW-4), leftW-4, i == s.cursor),
			item: i,
		})
	}
	if len(tools) == 0 {
		listRows = append(listRows, toolRow{text: st.Faint.Render("No tools in this category."), item: -1})
	}
	avail := height - 3 // panel borders and the category tab row
	if avail < 2 {
		avail = 2
	}
	cursorRow := 0
	for i, r := range listRows {
		if r.item == s.cursor {
			cursorRow = i
			break
		}
	}
	start := listWindowStart(cursorRow, avail, len(listRows))
	end := min(len(listRows), start+avail)
	rows := make([]string, 0, end-start)
	for _, r := range listRows[start:end] {
		rows = append(rows, r.text)
	}
	title := "Tools"
	if label := listRangeLabel(start, end, len(listRows)); label != "" {
		title += " · " + label
	}
	left := st.Panel(title, tabsRow+"\n"+strings.Join(rows, "\n"), leftW, height, true)

	var right []string
	if s.cursor >= 0 && s.cursor < len(tools) {
		t := tools[s.cursor]
		right = append(right, st.Accent.Bold(true).Render(t.Name)+"  "+st.BadgeText(t.Category))
		right = append(right, "")
		for _, l := range widgets.Wrap(t.Description, rightW-4) {
			right = append(right, st.Text.Render(l))
		}
		right = append(right, "")
		right = append(right, st.Label("Preset", t.Preset))
		right = append(right, st.Label("Container", "."+t.Container))
		if len(t.VideoFilters) > 0 {
			right = append(right, st.Label("Video filters", strings.Join(t.VideoFilters, ",")))
		}
		if len(t.AudioFilters) > 0 {
			right = append(right, st.Label("Audio filters", strings.Join(t.AudioFilters, ",")))
		}
		if len(t.ExtraArgs) > 0 {
			right = append(right, st.Label("Extra args", strings.Join(t.ExtraArgs, " ")))
		}
		if t.NeedsInput != "" {
			right = append(right, st.Warn.Render("This tool needs a second file: "+t.NeedsInput))
		}
		right = append(right, "")
		right = append(right, st.Faint.Render("Enter opens the wizard with this recipe applied."))
		if src := m.currentProbe(); src != nil {
			right = append(right, "")
			right = append(right, st.Rule("Current source", rightW-4))
			right = append(right, m.probeSummaryLines(src)...)
		}
	} else {
		right = append(right, st.Faint.Render("Select a tool."))
	}
	panel := st.Panel("Recipe", strings.Join(fitLines(right, height-2), "\n"), rightW, height, false)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", panel)
}

var _ = fmt.Sprintf
