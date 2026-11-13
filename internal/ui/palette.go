package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/AMC-official/ffmpeg-tui/internal/config"
	"github.com/AMC-official/ffmpeg-tui/internal/filesystem"
	"github.com/AMC-official/ffmpeg-tui/internal/widgets"
)

// paletteItem is one command in the ":" palette.
type paletteItem struct {
	label  string
	hint   string
	action string
	arg    string
}

// paletteState is the command palette overlay.
type paletteState struct {
	open     bool
	input    textinput.Model
	cursor   int
	items    []paletteItem
	filtered []paletteItem
}

func newPaletteState() paletteState {
	in := textinput.New()
	in.Prompt = ": "
	in.Placeholder = "type a command…"
	in.CharLimit = 200
	return paletteState{input: in}
}

func (m *Model) paletteItems() []paletteItem {
	return []paletteItem{
		{"New conversion", "open the conversion wizard", "new", ""},
		{"Browse media", "open the file browser", "browse", ""},
		{"Batch process", "encode a folder", "batch", ""},
		{"Tools & recipes", "audio, video and subtitle tools", "tools", ""},
		{"Presets", "built-in encoding recipes", "presets", ""},
		{"Filter builder", "interactive filter graph editor", "filters", ""},
		{"Man pages", "search FFmpeg documentation", "man", ""},
		{"Man page for crop", "open the crop filter manual", "man-page", "crop"},
		{"Man page for libx264", "open the H.264 encoder manual", "man-page", "libx264"},
		{"Man page for mp4", "open the MP4 muxer manual", "man-page", "mp4"},
		{"Queue", "job queue and live progress", "queue", ""},
		{"History", "replay or export executed commands", "history", ""},
		{"Compare files", "compare two media files", "compare", ""},
		{"Stream inspector", "streams, chapters and attachments", "streams", ""},
		{"Save queue", "write the queue to the state directory", "save-queue", ""},
		{"Load queue", "restore a saved queue", "load-queue", ""},
		{"Sort by name", "browser sort order", "sort-name", ""},
		{"Sort by size", "browser sort order", "sort-size", ""},
		{"Sort by date", "browser sort order", "sort-date", ""},
		{"Toggle hidden files", "show or hide dotfiles", "toggle-hidden", ""},
		{"Reload directory", "rescan the current folder", "reload", ""},
		{"Help", "keyboard reference", "help", ""},
		{"Quit", "exit FFmpeg-TUI", "quit", ""},
	}
}

// refilter rebuilds the visible command list.
func (p *paletteState) refilter(m *Model) {
	if p.items == nil {
		p.items = m.paletteItems()
	}
	q := strings.ToLower(strings.TrimSpace(p.input.Value()))
	p.filtered = p.filtered[:0]
	for _, it := range p.items {
		if q == "" || strings.Contains(strings.ToLower(it.label), q) || fuzzyContains(q, strings.ToLower(it.label+" "+it.hint)) {
			p.filtered = append(p.filtered, it)
		}
	}
	p.cursor = clampInt(p.cursor, 0, max(0, len(p.filtered)-1))
}

func fuzzyContains(query, target string) bool {
	qi := 0
	qr := []rune(query)
	for _, r := range target {
		if qi < len(qr) && r == qr[qi] {
			qi++
		}
	}
	return qi == len(qr)
}

func (m *Model) updatePalette(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	p := &m.palette
	switch msg.String() {
	case "esc":
		p.open = false
		return m, nil
	case "up", "ctrl+p":
		p.cursor = clampInt(p.cursor-1, 0, max(0, len(p.filtered)-1))
		return m, nil
	case "down", "ctrl+n":
		p.cursor = clampInt(p.cursor+1, 0, max(0, len(p.filtered)-1))
		return m, nil
	case "enter":
		return m, m.executePalette()
	}
	var cmd tea.Cmd
	p.input, cmd = p.input.Update(msg)
	p.refilter(m)
	return m, cmd
}

// executePalette runs the selected command or a ":man name" expression.
func (m *Model) executePalette() tea.Cmd {
	p := &m.palette
	raw := strings.TrimSpace(p.input.Value())
	lower := strings.ToLower(raw)
	if strings.HasPrefix(lower, "man ") {
		page := strings.TrimSpace(raw[4:])
		p.open = false
		return m.openManPage(page)
	}
	if p.cursor >= 0 && p.cursor < len(p.filtered) {
		item := p.filtered[p.cursor]
		p.open = false
		return m.execPaletteAction(item)
	}
	p.open = false
	if raw != "" {
		return m.flash("no command matches " + raw)
	}
	return nil
}

func (m *Model) execPaletteAction(it paletteItem) tea.Cmd {
	switch it.action {
	case "new":
		m.startWizardFromSelection()
	case "browse":
		m.setScreen(screenBrowser)
		return m.probeSelected()
	case "batch":
		m.batch = newBatchState()
		m.batch.init(m.cwd)
		m.setScreen(screenBatch)
	case "tools":
		m.setScreen(screenTools)
	case "presets":
		m.setScreen(screenPresets)
	case "filters":
		m.flt.sync(m.wiz)
		m.setScreen(screenFilters)
	case "man":
		m.setScreen(screenMan)
	case "man-page":
		return m.openManPage(it.arg)
	case "queue":
		m.queueSnapshot()
		m.setScreen(screenQueue)
	case "history":
		m.setScreen(screenHistory)
	case "compare":
		m.setScreen(screenCompare)
		return compareProbeCmd(m.ctx, m.ffprobePath, 0, m.compare.paths[0])
	case "streams":
		if m.currentProbe() != nil || m.probePath != "" {
			m.setScreen(screenStreams)
		} else {
			return m.flash("open a media file first")
		}
	case "save-queue":
		path := m.queueFilePath()
		if err := m.jobs.Save(path); err != nil {
			return m.flash("✖ " + err.Error())
		}
		return m.flash("✓ queue saved to " + path)
	case "load-queue":
		path := m.queueFilePath()
		if err := m.jobs.Load(path); err != nil {
			return m.flash("✖ " + err.Error())
		}
		m.queueSnapshot()
		return m.flash("✓ queue loaded from " + path)
	case "sort-name":
		m.sortMode = "name"
		filesystem.Sort(m.entries, m.sortMode)
	case "sort-size":
		m.sortMode = "size"
		filesystem.Sort(m.entries, m.sortMode)
	case "sort-date":
		m.sortMode = "date"
		filesystem.Sort(m.entries, m.sortMode)
	case "toggle-hidden":
		m.showHidden = !m.showHidden
		m.loadDir(m.cwd)
		return m.flash("hidden files: " + boolText(m.showHidden))
	case "reload":
		m.loadDir(m.cwd)
		return m.flash("reloaded " + m.cwd)
	case "help":
		m.help.open = true
	case "quit":
		return m.requestQuitCmd()
	}
	return nil
}

func (m *Model) requestQuitCmd() tea.Cmd {
	model, cmd := m.requestQuit()
	_ = model
	return cmd
}

// openManPage jumps to a documentation page by name.
func (m *Model) openManPage(name string) tea.Cmd {
	m.setScreen(screenMan)
	if m.man.openName(name) {
		return nil
	}
	m.man.input.SetValue(name)
	m.man.refilter()
	return m.flash("no manual page found for \"" + name + "\"")
}

// viewPalette renders the command palette overlay.
func (m *Model) viewPalette(width, height int) string {
	st := m.styles
	p := &m.palette
	p.refilter(m)
	panelW := min(width, 86)
	panelH := min(max(10, height-4), 22)
	rows := panelH - 5
	if rows < 3 {
		rows = 3
	}
	var lines []string
	lines = append(lines, p.input.View())
	lines = append(lines, st.Faint.Render(strings.Repeat("─", panelW-4)))
	start := 0
	if p.cursor >= rows {
		start = p.cursor - rows + 1
	}
	for i := start; i < len(p.filtered) && i < start+rows; i++ {
		it := p.filtered[i]
		line := widgets.PadRight(it.label, panelW/2) + st.Faint.Render(widgets.Truncate(it.hint, panelW/2-6))
		lines = append(lines, st.SelectedRow(line, panelW-4, i == p.cursor))
	}
	if len(p.filtered) == 0 {
		lines = append(lines, st.Warn.Render("No command matches. Try :man crop."))
	}
	lines = append(lines, st.Faint.Render("↑/↓ select · enter run · esc close"))
	return st.Panel("Command palette", strings.Join(lines, "\n"), panelW, panelH, true)
}

// configDirLabel is used in the help screen.
func configDirLabel() string {
	dir, err := config.Dir()
	if err != nil {
		return "config directory unavailable"
	}
	return dir
}
