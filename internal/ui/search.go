package ui

import (
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/ffmpeg-tui/ffmpeg-tui/internal/filesystem"
	"github.com/ffmpeg-tui/ffmpeg-tui/internal/history"
	"github.com/ffmpeg-tui/ffmpeg-tui/internal/manpages"
	"github.com/ffmpeg-tui/ffmpeg-tui/internal/presets"
	"github.com/ffmpeg-tui/ffmpeg-tui/internal/widgets"
)

// searchHit is a global search result.
type searchHit struct {
	kind     string
	title    string
	subtitle string
	arg      string
	index    int
}

// searchState is the global "/" search overlay.
type searchState struct {
	open   bool
	input  textinput.Model
	cursor int
	hits   []searchHit
}

func newSearchState() searchState {
	in := textinput.New()
	in.Prompt = "/ "
	in.Placeholder = "search files, man pages, presets, history…"
	in.CharLimit = 200
	return searchState{input: in}
}

// run rebuilds results for the current query.
func (s *searchState) run(m *Model) {
	q := strings.ToLower(strings.TrimSpace(s.input.Value()))
	s.hits = s.hits[:0]
	add := func(h searchHit) { s.hits = append(s.hits, h) }

	// Files in the current directory.
	fileCount := 0
	for _, e := range m.allEntries {
		if fileCount >= 12 {
			break
		}
		if q != "" && !strings.Contains(strings.ToLower(e.Name), q) {
			continue
		}
		kind := "file"
		if e.IsDir {
			kind = "dir"
		}
		add(searchHit{kind: kind, title: e.Name, subtitle: filesystem.KindLabel(e.Kind) + " · " + filesystem.HumanSize(e.Size), arg: e.Path})
		fileCount++
	}
	if q != "" {
		for _, h := range manpages.Search(q, 10) {
			add(searchHit{kind: "man", title: h.Entry.Name, subtitle: "manual · " + manpages.CategoryLabel(h.Entry.Category) + " · " + h.Entry.Summary, arg: h.Entry.Name})
		}
		for _, p := range presets.All(m.cfg.Presets) {
			if strings.Contains(strings.ToLower(p.Name+" "+p.Description), q) {
				add(searchHit{kind: "preset", title: p.Name, subtitle: "preset · " + p.Summary(), arg: p.Name})
			}
		}
		for i, e := range history.Search(m.hist.Entries(), q) {
			if i >= 4 {
				break
			}
			add(searchHit{kind: "history", title: filepath.Base(e.Input) + " → " + filepath.Base(e.Output), subtitle: "history · " + e.Time.Format("2006-01-02 15:04"), arg: e.ID})
		}
	}
	for _, it := range m.paletteItems() {
		if q == "" || strings.Contains(strings.ToLower(it.label), q) {
			add(searchHit{kind: "action", title: it.label, subtitle: "action · " + it.hint, arg: it.action})
		}
	}
	s.cursor = clampInt(s.cursor, 0, max(0, len(s.hits)-1))
}

func (m *Model) updateSearch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := &m.search
	switch msg.String() {
	case "esc":
		s.open = false
		return m, nil
	case "up", "ctrl+p":
		s.cursor = clampInt(s.cursor-1, 0, max(0, len(s.hits)-1))
		return m, nil
	case "down", "ctrl+n":
		s.cursor = clampInt(s.cursor+1, 0, max(0, len(s.hits)-1))
		return m, nil
	case "enter":
		return m, m.executeSearch()
	}
	var cmd tea.Cmd
	s.input, cmd = s.input.Update(msg)
	s.cursor = 0
	s.run(m)
	return m, cmd
}

// executeSearch acts on the highlighted result.
func (m *Model) executeSearch() tea.Cmd {
	s := &m.search
	if s.cursor < 0 || s.cursor >= len(s.hits) {
		return nil
	}
	hit := s.hits[s.cursor]
	s.open = false
	switch hit.kind {
	case "dir":
		m.loadDir(hit.arg)
		m.setScreen(screenBrowser)
		return m.probeSelected()
	case "file":
		m.openInBrowser(hit.arg)
		return m.probeSelected()
	case "man":
		return m.openManPage(hit.arg)
	case "preset":
		if p, ok := presetByName(hit.arg, m.cfg.Presets); ok {
			input := ""
			if e, ok := m.selectedEntry(); ok && !e.IsDir {
				input = e.Path
			}
			m.wiz.reset(m.cfg, input)
			m.wiz.entries = m.entries
			m.wiz.applyPreset(p)
			m.wiz.buildOutput(input, m.wiz.probe)
			m.stack = append(m.stack, m.screen)
			m.screen = screenWizard
			return m.flash("preset " + p.Name + " applied")
		}
	case "history":
		m.setScreen(screenHistory)
		entries := m.historyEntries()
		for i, e := range entries {
			if e.ID == hit.arg {
				m.histCursor = i
			}
		}
	case "action":
		return m.execPaletteAction(paletteItem{action: hit.arg, arg: hit.arg})
	}
	return nil
}

// viewSearch renders the search overlay.
func (m *Model) viewSearch(width, height int) string {
	st := m.styles
	s := &m.search
	s.run(m)
	panelW := min(width, 96)
	panelH := min(max(12, height-4), 24)
	rows := panelH - 6
	if rows < 3 {
		rows = 3
	}
	lines := []string{s.input.View(), ""}
	lastKind := ""
	start := 0
	if s.cursor >= rows {
		start = s.cursor - rows + 1
	}
	shown := 0
	for i := start; i < len(s.hits) && shown < rows; i++ {
		h := s.hits[i]
		if h.kind != lastKind {
			lines = append(lines, st.Faint.Render("· "+searchSectionLabel(h.kind)))
			lastKind = h.kind
			shown++
			if shown >= rows {
				break
			}
		}
		line := st.Text.Render(widgets.PadRight(widgets.Truncate(h.title, panelW/3), panelW/3)) +
			st.Faint.Render("  "+widgets.Truncate(h.subtitle, panelW-panelW/3-6))
		lines = append(lines, st.SelectedRow(line, panelW-4, i == s.cursor))
		shown++
	}
	if len(s.hits) == 0 {
		lines = append(lines, st.Warn.Render("Nothing matches. Man pages are searchable by tag, e.g. normalize."))
	}
	lines = append(lines, "", st.Faint.Render("enter open · ↑/↓ select · esc close"))
	return st.Panel("Search", strings.Join(lines, "\n"), panelW, panelH, true)
}

func searchSectionLabel(kind string) string {
	switch kind {
	case "file", "dir":
		return "Files"
	case "man":
		return "Manual pages"
	case "preset":
		return "Presets"
	case "history":
		return "History"
	case "action":
		return "Actions"
	}
	return kind
}
