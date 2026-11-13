package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/AMC-official/ffmpeg-tui/internal/presets"
	"github.com/AMC-official/ffmpeg-tui/internal/widgets"
)

// allPresets returns built-in and user presets.
func allPresets(user []presets.Preset) []presets.Preset { return presets.All(user) }

// presetByName looks a preset up by name.
func presetByName(name string, user []presets.Preset) (presets.Preset, bool) {
	return presets.ByName(name, user)
}

func (m *Model) updatePresets(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	list := allPresets(m.cfg.Presets)
	sel := presets.Preset{}
	if m.presetCursor >= 0 && m.presetCursor < len(list) {
		sel = list[m.presetCursor]
	}
	switch msg.String() {
	case "esc":
		m.back()
	case "up", "k":
		m.presetCursor = clampInt(m.presetCursor-1, 0, len(list)-1)
	case "down", "j":
		m.presetCursor = clampInt(m.presetCursor+1, 0, len(list)-1)
	case "home", "g":
		m.presetCursor = 0
	case "end", "G":
		m.presetCursor = max(0, len(list)-1)
	case "pgup", "ctrl+b":
		m.presetCursor = clampInt(m.presetCursor-10, 0, max(0, len(list)-1))
	case "pgdown", "ctrl+f":
		m.presetCursor = clampInt(m.presetCursor+10, 0, max(0, len(list)-1))
	case "enter":
		if sel.Name == "" {
			return m, nil
		}
		input := ""
		if e, ok := m.selectedEntry(); ok && !e.IsDir {
			input = e.Path
		}
		m.wiz.reset(m.cfg, input)
		m.wiz.entries = m.entries
		m.wiz.applyPreset(sel)
		if m.currentProbe() != nil && input == m.probePath {
			m.wiz.probe = m.probe
			m.wiz.duration = m.probe.Duration()
		}
		m.wiz.buildOutput(input, m.wiz.probe)
		m.stack = append(m.stack, m.screen)
		m.screen = screenWizard
		return m, m.flash("preset " + sel.Name + " applied")
	case "s":
		if sel.Name == "" {
			return m, nil
		}
		return m, m.startEdit(editTarget{kind: "preset-name"}, sel.Name+" copy", "name for the custom preset")
	case "d":
		if sel.Name != "" && !sel.Builtin {
			m.cfg.RemovePreset(sel.Name)
			m.saveConfig()
			m.presetCursor = clampInt(m.presetCursor, 0, max(0, len(allPresets(m.cfg.Presets))-1))
			return m, m.flash("custom preset removed")
		}
		if sel.Builtin {
			return m, m.flash("built-in presets cannot be deleted; press s to copy it")
		}
	}
	return m, nil
}

func (m *Model) viewPresets() string {
	st := m.styles
	width := m.contentWidth()
	height := m.bodyHeight()
	list := allPresets(m.cfg.Presets)

	leftW := width * 42 / 100
	if leftW < 36 {
		leftW = 36
	}
	if leftW > 60 {
		leftW = 60
	}
	rightW := width - leftW - 2

	type presetRow struct {
		text string
		item int
	}
	var listRows []presetRow
	lastCat := ""
	for i, p := range list {
		if p.Category != lastCat {
			listRows = append(listRows, presetRow{text: st.Muted.Render("── " + p.Category), item: -1})
			lastCat = p.Category
		}
		label := p.Name
		if !p.Builtin {
			label += " *"
		}
		line := label + "  " + st.Faint.Render(p.Summary())
		listRows = append(listRows, presetRow{
			text: st.SelectedRow(widgets.Truncate(line, leftW-4), leftW-4, i == m.presetCursor),
			item: i,
		})
	}
	avail := height - 2
	if avail < 2 {
		avail = 2
	}
	cursorRow := 0
	for i, r := range listRows {
		if r.item == m.presetCursor {
			cursorRow = i
			break
		}
	}
	start := listWindowStart(cursorRow, avail, len(listRows))
	end := min(len(listRows), start+avail)
	var rows []string
	for _, r := range listRows[start:end] {
		rows = append(rows, r.text)
	}
	title := "Presets"
	if label := listRangeLabel(start, end, len(listRows)); label != "" {
		title += " · " + label
	}
	left := st.Panel(title, strings.Join(rows, "\n"), leftW, height, true)

	var right []string
	if m.presetCursor < len(list) {
		p := list[m.presetCursor]
		right = append(right, st.Accent.Bold(true).Render(p.Name))
		if p.Builtin {
			right = append(right, st.Faint.Render("built-in preset"))
		} else {
			right = append(right, st.Faint.Render("custom preset"))
		}
		right = append(right, "")
		for _, l := range widgets.Wrap(p.Description, rightW-4) {
			right = append(right, st.Text.Render(l))
		}
		right = append(right, "")
		right = append(right, st.Label("Container", p.Container))
		if p.VideoCodec != "" {
			right = append(right, st.Label("Video", p.VideoCodec))
		}
		if p.CRF != "" {
			right = append(right, st.Label("CRF", p.CRF))
		}
		if p.EncoderPreset != "" {
			right = append(right, st.Label("Encoder preset", p.EncoderPreset))
		}
		if p.Tune != "" {
			right = append(right, st.Label("Tune", p.Tune))
		}
		if p.Profile != "" {
			right = append(right, st.Label("Profile", p.Profile))
		}
		if p.VideoBitrate != "" {
			right = append(right, st.Label("Video bitrate", p.VideoBitrate))
		}
		if p.Scale != "" {
			right = append(right, st.Label("Scale", p.Scale))
		}
		if p.FPS != "" {
			right = append(right, st.Label("FPS", p.FPS))
		}
		if p.AudioCodec != "" {
			right = append(right, st.Label("Audio", p.AudioCodec+" "+p.AudioBitrate))
		}
		if p.Channels > 0 {
			right = append(right, st.Label("Channels", itoa(p.Channels)))
		}
		if p.SampleRate > 0 {
			right = append(right, st.Label("Sample rate", itoa(p.SampleRate)))
		}
		if len(p.VideoFilters) > 0 {
			right = append(right, st.Label("Video filters", strings.Join(p.VideoFilters, ",")))
		}
		if len(p.AudioFilters) > 0 {
			right = append(right, st.Label("Audio filters", strings.Join(p.AudioFilters, ",")))
		}
		if len(p.ExtraArgs) > 0 {
			right = append(right, st.Label("Extra args", strings.Join(p.ExtraArgs, " ")))
		}
		right = append(right, "")
		right = append(right, st.Faint.Render("Enter starts a conversion with this preset · s saves a copy · d removes a custom preset"))
	}
	panel := st.Panel("Preset detail", strings.Join(fitLines(right, height-2), "\n"), rightW, height, false)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", panel)
}
