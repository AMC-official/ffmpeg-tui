package ui

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ffmpeg-tui/ffmpeg-tui/internal/filters"
	"github.com/ffmpeg-tui/ffmpeg-tui/internal/widgets"
)

// filterState is the interactive filter builder.
type filterState struct {
	cursor      int
	chainCursor int
	argCursor   int
	chain       []filters.Selected
	inChain     bool
	err         string
}

func newFilterState() filterState { return filterState{} }

// sync copies the wizard's chain into the builder.
func (f *filterState) sync(w wizardState) {
	f.chain = append([]filters.Selected(nil), w.filters...)
	f.chainCursor = clampInt(f.chainCursor, 0, max(0, len(f.chain)-1))
	f.argCursor = 0
}

func (m *Model) updateFilters(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	f := &m.flt
	key := msg.String()
	defs := filters.All()

	switch key {
	case "esc":
		if f.inChain {
			f.inChain = false
			return m, nil
		}
		m.back()
		return m, nil
	case "tab":
		f.inChain = !f.inChain
		f.argCursor = 0
		return m, nil
	case "up", "k":
		if f.inChain && len(f.chain) > 0 {
			f.argCursor = clampInt(f.argCursor-1, 0, max(0, len(f.chain[f.chainCursor].Def.Args)-1))
		} else {
			f.cursor = clampInt(f.cursor-1, 0, len(defs)-1)
		}
		return m, nil
	case "down", "j":
		if f.inChain && len(f.chain) > 0 {
			f.argCursor = clampInt(f.argCursor+1, 0, max(0, len(f.chain[f.chainCursor].Def.Args)-1))
		} else {
			f.cursor = clampInt(f.cursor+1, 0, len(defs)-1)
		}
		return m, nil
	case "enter", "e":
		if !f.inChain || len(f.chain) == 0 {
			sel := filters.NewSelected(defs[clampInt(f.cursor, 0, len(defs)-1)])
			if sel.Def.NeedsInput {
				sel.ExtraInput = m.wiz.extraInput
			}
			f.chain = append(f.chain, sel)
			f.chainCursor = len(f.chain) - 1
			f.argCursor = 0
			f.inChain = true
			f.err = ""
			return m, nil
		}
		sel := &f.chain[f.chainCursor]
		if len(sel.Def.Args) == 0 {
			return m, nil
		}
		arg := sel.Def.Args[clampInt(f.argCursor, 0, len(sel.Def.Args)-1)]
		if arg.Kind == filters.ArgFile {
			return m, m.startEdit(editTarget{kind: "filter-arg", index: f.chainCursor, key: arg.Key}, sel.Value(arg.Key), arg.Label)
		}
		return m, m.startEdit(editTarget{kind: "filter-arg", index: f.chainCursor, key: arg.Key}, sel.Value(arg.Key), arg.Label)
	case "left", "h":
		if f.inChain {
			f.adjustArg(-1)
		}
		return m, nil
	case "right", "l":
		if f.inChain {
			f.adjustArg(1)
		}
		return m, nil
	case "d", "delete":
		if f.inChain && len(f.chain) > 0 {
			f.chain = append(f.chain[:f.chainCursor], f.chain[f.chainCursor+1:]...)
			f.chainCursor = clampInt(f.chainCursor, 0, max(0, len(f.chain)-1))
			f.argCursor = 0
			f.err = ""
			if len(f.chain) == 0 {
				f.inChain = false
			}
		}
		return m, nil
	case "x":
		f.chain = nil
		f.inChain = false
		f.chainCursor = 0
		f.err = ""
		return m, nil
	case "a", "ctrl+s":
		if m.wiz.input == "" && m.wiz.basePreset == nil {
			m.wiz.reset(m.cfg, "")
			m.wiz.entries = m.entries
		}
		m.wiz.filters = append([]filters.Selected(nil), f.chain...)
		m.back()
		return m, m.flash(fmt.Sprintf("%d filters applied", len(f.chain)))
	}
	return m, nil
}

// setArg validates and stores an argument value for a chain entry.
func (f *filterState) setArg(index int, key, value string) error {
	if index < 0 || index >= len(f.chain) {
		return fmt.Errorf("filter no longer selected")
	}
	if err := f.chain[index].Set(key, value); err != nil {
		return err
	}
	f.err = ""
	return nil
}

// adjustArg steps the focused argument up or down.
func (f *filterState) adjustArg(delta int) {
	if len(f.chain) == 0 {
		return
	}
	sel := &f.chain[f.chainCursor]
	if len(sel.Def.Args) == 0 {
		return
	}
	arg := sel.Def.Args[clampInt(f.argCursor, 0, len(sel.Def.Args)-1)]
	cur := sel.Value(arg.Key)
	switch arg.Kind {
	case filters.ArgChoice:
		idx := 0
		for i, c := range arg.Choices {
			if c == cur {
				idx = i
			}
		}
		idx = (idx + delta + len(arg.Choices)) % len(arg.Choices)
		_ = sel.Set(arg.Key, arg.Choices[idx])
	case filters.ArgBool:
		if cur == "1" || cur == "true" {
			_ = sel.Set(arg.Key, "0")
		} else {
			_ = sel.Set(arg.Key, "1")
		}
	case filters.ArgNumber:
		v, err := strconv.ParseFloat(cur, 64)
		if err != nil {
			return
		}
		step := 1.0
		if v != 0 {
			step = v * 0.1
		}
		if arg.Max-arg.Min <= 2 {
			step = 0.05
		}
		if strings.Contains(cur, ".") || step < 1 {
			step = 0.1
		}
		v += float64(delta) * step
		if arg.Max != 0 || arg.Min != 0 {
			if v < arg.Min {
				v = arg.Min
			}
			if v > arg.Max {
				v = arg.Max
			}
		}
		_ = sel.Set(arg.Key, strconv.FormatFloat(v, 'f', -1, 64))
	}
}

func (m *Model) viewFilters() string {
	st := m.styles
	f := &m.flt
	width := m.contentWidth()
	height := m.bodyHeight()
	defs := filters.All()

	leftW := width * 38 / 100
	if leftW < 34 {
		leftW = 34
	}
	if leftW > 60 {
		leftW = 60
	}
	rightW := width - leftW - 2
	if rightW < 30 {
		rightW = 30
	}

	var list []string
	lastCat := ""
	visible := 0
	for i, d := range defs {
		if d.Category != lastCat {
			list = append(list, st.Muted.Render("── "+d.Category+" "))
			lastCat = d.Category
			visible++
		}
		line := d.Name
		if d.ComplexTpl != "" {
			line += " ⧉"
		}
		row := st.SelectedRow("  "+line, leftW-4, i == f.cursor && !f.inChain)
		list = append(list, row)
		visible++
	}
	list = fitLines(list, height-4)
	left := st.Panel("Filters", strings.Join(list, "\n"), leftW, height, !f.inChain)

	right := st.Panel(m.filterRightTitle(), m.filterRight(width-4), rightW, height, f.inChain)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", right)
}

func (m *Model) filterRightTitle() string {
	if m.flt.inChain && len(m.flt.chain) > 0 {
		return "Chain · " + m.flt.chain[m.flt.chainCursor].Def.Name
	}
	return "Documentation"
}

func (m *Model) filterRight(width int) string {
	st := m.styles
	f := &m.flt
	defs := filters.All()
	var b []string
	add := func(s string) { b = append(b, s) }

	if f.inChain && len(f.chain) > 0 {
		sel := f.chain[clampInt(f.chainCursor, 0, len(f.chain)-1)]
		add(st.Text.Bold(true).Render(sel.Def.Name) + " " + st.Faint.Render("("+sel.Def.Category+")"))
		b = append(b, widgets.Wrap(sel.Def.Summary, width)...)
		if len(sel.Def.Args) > 0 {
			add("")
			add(st.Muted.Render("Arguments — ←/→ adjust, Enter edits"))
			for i, a := range sel.Def.Args {
				val := sel.Value(a.Key)
				if val == "" {
					val = st.Faint.Render("—")
				}
				line := fmt.Sprintf("%s %s", st.Muted.Render(widgets.PadRight(a.Label, 16)), val)
				b = append(b, st.SelectedRow(line, width, i == f.argCursor))
			}
		}
		if sel.Def.Note != "" {
			add("")
			for _, l := range widgets.Wrap("Note: "+sel.Def.Note, width) {
				add(st.Warn.Render(l))
			}
		}
	} else {
		d := defs[clampInt(f.cursor, 0, len(defs)-1)]
		add(st.Text.Bold(true).Render(d.Name) + " " + st.Faint.Render("("+d.Category+")"))
		for _, l := range widgets.Wrap(d.Summary, width) {
			add(st.Text.Render(l))
		}
		if len(d.Args) > 0 {
			add("")
			add(st.Muted.Render("Arguments"))
			for _, a := range d.Args {
				desc := a.Help
				if desc == "" {
					desc = "default " + a.Default
				}
				add(st.Faint.Render("  "+widgets.PadRight(a.Key, 12)) + st.Muted.Render(widgets.Truncate(desc, width-16)))
			}
		}
		if d.Note != "" {
			add("")
			for _, l := range widgets.Wrap(d.Note, width) {
				add(st.Warn.Render(l))
			}
		}
		if len(d.Examples) > 0 {
			add("")
			add(st.Muted.Render("Examples"))
			for _, ex := range d.Examples {
				for _, l := range widgets.Wrap(ex, width-2) {
					add(st.CodeBlock.Render("  " + l))
				}
			}
		}
	}

	add("")
	add(st.Rule("Chain", width))
	if len(f.chain) == 0 {
		add(st.Faint.Render("No filters. Enter adds the highlighted filter."))
	} else {
		for i, sel := range f.chain {
			line := fmt.Sprintf("%d. %s — %s", i+1, sel.Def.Name, sel.Describe())
			b = append(b, st.SelectedRow(widgets.Truncate(line, width), width, i == f.chainCursor && f.inChain))
		}
		graph := filters.BuildGraph(f.chain)
		add("")
		if graph.Complex != "" {
			add(st.Label("-filter_complex", ""))
			for _, l := range widgets.Wrap(graph.Complex, width-2) {
				add(st.CodeBlock.Render("  " + l))
			}
			add(st.Label("map", graph.MapOut))
		} else {
			if graph.VF != "" {
				add(st.Label("-vf", ""))
				for _, l := range widgets.Wrap(graph.VF, width-2) {
					add(st.CodeBlock.Render("  " + l))
				}
			}
			if graph.AF != "" {
				add(st.Label("-af", ""))
				for _, l := range widgets.Wrap(graph.AF, width-2) {
					add(st.CodeBlock.Render("  " + l))
				}
			}
		}
		for _, extra := range graph.ExtraInputs {
			add(st.Label("extra input", widgets.TruncateMiddle(extra, width-16)))
		}
		add("")
		add(st.Faint.Render("a applies to the wizard · d removes · x clears"))
	}
	if f.err != "" {
		add("")
		add(st.Err.Render(f.err))
	}
	return strings.Join(fitLines(b, m.bodyHeight()-2), "\n")
}

// fitLines truncates a rendered block to n lines.
func fitLines(lines []string, n int) []string {
	if n < 1 {
		n = 1
	}
	if len(lines) > n {
		return lines[:n]
	}
	return lines
}
