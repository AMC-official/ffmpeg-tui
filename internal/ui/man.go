package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/AMC-official/ffmpeg-tui/internal/manpages"
	"github.com/AMC-official/ffmpeg-tui/internal/widgets"
)

// manState is the built-in documentation browser.
type manState struct {
	input       textinput.Model
	category    int // 0 = all, otherwise 1+index in manpages.Categories
	cursor      int
	scroll      int
	detail      int
	hits        []manpages.Hit
	live        string
	liveFor     string
	liveErr     string
	loading     bool
	catScrolled int
}

func newManState() manState {
	in := textinput.New()
	in.Placeholder = "search filters, encoders, muxers, options…"
	in.Prompt = "/ "
	in.CharLimit = 120
	return manState{input: in}
}

func (s *manState) activate() {
	s.refilter()
	s.input.Focus()
}

func (s *manState) activeCategory() (manpages.Category, bool) {
	if s.category <= 0 || s.category > len(manpages.Categories) {
		return "", false
	}
	return manpages.Categories[s.category-1], true
}

func (s *manState) refilter() {
	query := strings.TrimSpace(s.input.Value())
	if cat, ok := s.activeCategory(); ok {
		// Category filtering is applied after ranking so tag matches still work.
		hits := manpages.Search(query, 0)
		out := hits[:0]
		for _, h := range hits {
			if h.Entry.Category == cat {
				out = append(out, h)
			}
		}
		s.hits = out
	} else {
		s.hits = manpages.Search(query, 0)
	}
	if len(s.hits) == 0 {
		s.cursor = 0
	} else {
		s.cursor = clampInt(s.cursor, 0, len(s.hits)-1)
	}
	s.detail = 0
}

func (s *manState) selected() (manpages.Entry, bool) {
	if s.cursor >= 0 && s.cursor < len(s.hits) {
		return s.hits[s.cursor].Entry, true
	}
	return manpages.Entry{}, false
}

// openName jumps directly to a page, used by ":man crop".
func (s *manState) openName(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	s.category = 0
	s.input.SetValue(name)
	s.refilter()
	if len(s.hits) == 0 {
		if e, ok := manpages.Find(name); ok {
			s.input.SetValue(e.Name)
			s.refilter()
		}
	}
	if len(s.hits) > 0 {
		s.cursor = 0
		return true
	}
	return false
}

func (m *Model) updateMan(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := &m.man
	key := msg.String()
	switch key {
	case "esc":
		if s.input.Value() != "" {
			s.input.SetValue("")
			s.refilter()
			return m, nil
		}
		m.back()
		return m, nil
	case "ctrl+u":
		s.input.SetValue("")
		s.refilter()
		return m, nil
	case "tab":
		s.category = (s.category + 1) % (len(manpages.Categories) + 1)
		s.refilter()
		return m, nil
	case "shift+tab":
		s.category = (s.category - 1 + len(manpages.Categories) + 1) % (len(manpages.Categories) + 1)
		s.refilter()
		return m, nil
	case "up", "k":
		if key == "k" && s.input.Focused() {
			break
		}
		s.cursor = clampInt(s.cursor-1, 0, max(0, len(s.hits)-1))
		s.detail = 0
		return m, nil
	case "down", "j":
		if key == "j" && s.input.Focused() {
			break
		}
		s.cursor = clampInt(s.cursor+1, 0, max(0, len(s.hits)-1))
		s.detail = 0
		return m, nil
	case "pgup", "ctrl+b":
		s.detail = clampInt(s.detail-8, 0, 10000)
		return m, nil
	case "pgdown", "ctrl+f":
		s.detail += 8
		return m, nil
	case "home":
		s.detail = 0
		return m, nil
	case "f":
		e, ok := s.selected()
		if !ok {
			return m, nil
		}
		s.loading = true
		s.liveErr = ""
		return m, liveHelpCmd(m.ctx, m.ffmpegPath, e.Category, e.Name)
	case "m":
		if manpages.ManCommand("") == nil {
			return m, m.flash("system man(1) is not available on this system")
		}
		e, ok := s.selected()
		target := "ffmpeg"
		if ok {
			target = e.Name
		}
		cmd := manpages.ManCommand(target)
		if cmd == nil {
			return m, m.flash("no manual page available")
		}
		return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
			if err != nil {
				return savedMsg{err: fmt.Errorf("man viewer: %w", err)}
			}
			return savedMsg{text: "returned from man"}
		})
	}

	var cmd tea.Cmd
	s.input, cmd = s.input.Update(msg)
	s.refilter()
	return m, cmd
}

func (m *Model) viewMan() string {
	st := m.styles
	s := &m.man
	width := m.contentWidth()
	height := m.bodyHeight()

	listW := width * 34 / 100
	if listW < 30 {
		listW = 30
	}
	if listW > 52 {
		listW = 52
	}
	detailW := width - listW - 2
	if detailW < 30 {
		detailW = 30
	}

	var tabs []string
	tabs = append(tabs, "All")
	for _, c := range manpages.Categories {
		tabs = append(tabs, manpages.PluralLabel(c))
	}
	tabBar := widgets.Tabs(tabs, s.category)
	if lipgloss.Width(tabBar) > width {
		tabBar = widgets.Truncate(tabBar, width)
	}

	search := s.input.View()
	if s.input.Value() == "" {
		search += st.Faint.Render(fmt.Sprintf("  %d pages indexed", len(s.hits)))
	} else {
		search += st.Faint.Render(fmt.Sprintf("  %d matches", len(s.hits)))
	}

	listRows := height - 8
	if listRows < 4 {
		listRows = 4
	}
	start := 0
	if s.cursor >= listRows {
		start = s.cursor - listRows + 1
	}
	var list []string
	lastCat := manpages.Category("")
	for i := start; i < len(s.hits) && i < start+listRows; i++ {
		h := s.hits[i]
		if h.Entry.Category != lastCat {
			list = append(list, st.Faint.Render("· "+manpages.PluralLabel(h.Entry.Category)))
			lastCat = h.Entry.Category
		}
		label := h.Entry.Name
		if h.MatchedOn != "" && strings.HasPrefix(h.MatchedOn, "tag:") {
			label += " " + st.Faint.Render("("+strings.TrimPrefix(h.MatchedOn, "tag:")+")")
		}
		list = append(list, st.SelectedRow(widgets.Truncate(label, listW-4), listW-4, i == s.cursor))
	}
	if len(s.hits) == 0 {
		list = append(list, st.Warn.Render("No page matches that query."))
		list = append(list, st.Faint.Render("Try a filter name, or press Tab to search another category."))
	}
	for len(list) < listRows {
		list = append(list, "")
	}
	left := st.Panel("Manual", search+"\n"+st.Faint.Render(tabBar)+"\n"+strings.Join(list, "\n"), listW, height, true)

	detail := m.manDetail(detailW-4, height-2)
	right := st.Panel(m.manDetailTitle(), detail, detailW, height, s.input.Focused())
	return lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", right)
}

func (m *Model) manDetailTitle() string {
	e, ok := m.man.selected()
	if !ok {
		return "Page"
	}
	return fmt.Sprintf("%s · %s", e.Name, manpages.CategoryLabel(e.Category))
}

func (m *Model) manDetail(width, rows int) string {
	st := m.styles
	s := &m.man
	e, ok := s.selected()
	if !ok {
		return st.Faint.Render("Select a page with ↑/↓, or search above.")
	}
	var b []string
	add := func(format string, args ...any) { b = append(b, fmt.Sprintf(format, args...)) }
	add("%s", st.Accent.Bold(true).Render(e.Name)+"  "+st.BadgeText(manpages.CategoryLabel(e.Category)))
	b = append(b, "")
	for _, l := range widgets.Wrap(e.Summary, width) {
		b = append(b, st.Text.Bold(true).Render(l))
	}
	b = append(b, "")
	for _, l := range widgets.Wrap(e.Description, width) {
		b = append(b, st.Text.Render(l))
	}
	if len(e.Args) > 0 {
		b = append(b, "", st.Rule("Arguments", width))
		for _, a := range e.Args {
			line := st.Accent2.Render(widgets.PadRight(a.Name, 22)) + st.Text.Render(widgets.Truncate(a.Desc, width-24))
			b = append(b, line)
		}
	}
	if len(e.Examples) > 0 {
		b = append(b, "", st.Rule("Examples", width))
		for _, ex := range e.Examples {
			for _, l := range widgets.Wrap(ex, width-2) {
				b = append(b, st.CodeBlock.Render("  "+l))
			}
		}
	}
	if len(e.Links) > 0 {
		b = append(b, "", st.Rule("Links", width))
		for _, l := range e.Links {
			b = append(b, st.Blue.Render(widgets.Truncate(l, width)))
		}
	}
	if len(e.Related) > 0 {
		b = append(b, "", st.Muted.Render("Related: ")+st.Faint.Render(strings.Join(e.Related, ", ")))
	}
	if len(e.Tags) > 0 {
		b = append(b, st.Muted.Render("Tags: ")+st.Faint.Render(strings.Join(e.Tags, ", ")))
	}
	if s.live != "" || s.liveErr != "" || s.loading {
		b = append(b, "", st.Rule("Live help from ffmpeg", width))
		if s.loading {
			b = append(b, st.Muted.Render("fetching ffmpeg -h "+string(e.Category)+"="+e.Name+"…"))
		} else if s.liveErr != "" {
			b = append(b, st.Err.Render(s.liveErr))
		} else {
			lines := strings.Split(strings.TrimRight(s.live, "\n"), "\n")
			for _, l := range fitLines(lines, 40) {
				b = append(b, st.CodeBlock.Render(widgets.Truncate(l, width)))
			}
		}
	}

	if s.detail > len(b)-rows {
		s.detail = max(0, len(b)-rows)
	}
	if s.detail < 0 {
		s.detail = 0
	}
	end := s.detail + rows
	if end > len(b) {
		end = len(b)
	}
	visible := b[s.detail:end]
	for len(visible) < rows {
		visible = append(visible, "")
	}
	return strings.Join(visible, "\n")
}
