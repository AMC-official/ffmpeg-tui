package ui

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/ffmpeg-tui/ffmpeg-tui/internal/widgets"
)

// Palette is intentionally small: mint for structure, lavender for selection,
// amber for warnings and red for failures. Everything degrades gracefully when
// the terminal has no colour support because lipgloss strips the escapes.
const (
	colAccent  = "#7FD1B9"
	colAccent2 = "#C792EA"
	colAmber   = "#F5C06B"
	colRed     = "#FF6B81"
	colGreen   = "#7CE38B"
	colMuted   = "#7A8899"
	colFaint   = "#4B5563"
	colText    = "#D8DEE9"
	colBlue    = "#82AAFF"
)

// Styles is the complete style set for the TUI.
type Styles struct {
	AppTitle   lipgloss.Style
	Version    lipgloss.Style
	Muted      lipgloss.Style
	Faint      lipgloss.Style
	Text       lipgloss.Style
	Accent     lipgloss.Style
	Accent2    lipgloss.Style
	OK         lipgloss.Style
	Warn       lipgloss.Style
	Err        lipgloss.Style
	Blue       lipgloss.Style
	Header     lipgloss.Style
	Status     lipgloss.Style
	Selected   lipgloss.Style
	Row        lipgloss.Style
	Key        lipgloss.Style
	KeyDesc    lipgloss.Style
	PanelTitle lipgloss.Style
	Border     lipgloss.Style
	BorderHot  lipgloss.Style
	BarFill    lipgloss.Style
	BarEmpty   lipgloss.Style
	CodeBlock  lipgloss.Style
	Badge      lipgloss.Style
	Cursor     lipgloss.Style
	WizardStep lipgloss.Style
	WizardDone lipgloss.Style
}

// NewStyles returns the theme.
func NewStyles() Styles {
	if os.Getenv("NO_COLOR") != "" {
		lipgloss.SetColorProfile(termenv.Ascii)
	}
	base := lipgloss.NewStyle()
	return Styles{
		AppTitle:   base.Bold(true).Foreground(lipgloss.Color(colAccent)),
		Version:    base.Foreground(lipgloss.Color(colFaint)),
		Muted:      base.Foreground(lipgloss.Color(colMuted)),
		Faint:      base.Foreground(lipgloss.Color(colFaint)),
		Text:       base.Foreground(lipgloss.Color(colText)),
		Accent:     base.Foreground(lipgloss.Color(colAccent)),
		Accent2:    base.Foreground(lipgloss.Color(colAccent2)),
		OK:         base.Foreground(lipgloss.Color(colGreen)),
		Warn:       base.Foreground(lipgloss.Color(colAmber)),
		Err:        base.Foreground(lipgloss.Color(colRed)),
		Blue:       base.Foreground(lipgloss.Color(colBlue)),
		Header:     base.Background(lipgloss.Color("#1B2430")).Padding(0, 1),
		Status:     base.Foreground(lipgloss.Color(colMuted)).Padding(0, 1),
		Selected:   base.Background(lipgloss.Color("#2D3B4E")).Foreground(lipgloss.Color(colAccent)).Bold(true),
		Row:        base.Foreground(lipgloss.Color(colText)),
		Key:        base.Foreground(lipgloss.Color(colAccent2)),
		KeyDesc:    base.Foreground(lipgloss.Color(colFaint)),
		PanelTitle: base.Foreground(lipgloss.Color(colAccent)).Bold(true),
		Border:     base.Foreground(lipgloss.Color(colFaint)),
		BorderHot:  base.Foreground(lipgloss.Color(colAccent)),
		BarFill:    base.Foreground(lipgloss.Color(colAccent)),
		BarEmpty:   base.Foreground(lipgloss.Color(colFaint)),
		CodeBlock:  base.Foreground(lipgloss.Color(colText)),
		Badge:      base.Foreground(lipgloss.Color("#0F1720")).Background(lipgloss.Color(colAccent)).Padding(0, 1),
		Cursor:     base.Foreground(lipgloss.Color(colAccent2)).Bold(true),
		WizardStep: base.Foreground(lipgloss.Color(colMuted)),
		WizardDone: base.Foreground(lipgloss.Color(colGreen)),
	}
}

// Panel draws a bordered box with a title embedded in the top edge.
func (s Styles) Panel(title, content string, width, height int, focused bool) string {
	if width < 8 {
		width = 8
	}
	if height < 3 {
		height = 3
	}
	border := s.Border
	if focused {
		border = s.BorderHot
	}
	inner := width - 4
	titleText := widgets.Truncate(title, inner)
	titleWidth := lipgloss.Width(titleText)
	topFill := inner - titleWidth - 1
	if topFill < 0 {
		topFill = 0
	}
	var top string
	if titleText == "" {
		top = border.Render("╭" + strings.Repeat("─", inner+2) + "╮")
	} else {
		top = border.Render("╭─ ") + s.PanelTitle.Render(titleText) + border.Render(" "+strings.Repeat("─", topFill)+"╮")
	}

	lines := strings.Split(content, "\n")
	maxRows := height - 2
	if len(lines) > maxRows {
		lines = lines[:maxRows]
	}
	var body strings.Builder
	for i := 0; i < maxRows; i++ {
		line := ""
		if i < len(lines) {
			line = lines[i]
		}
		w := lipgloss.Width(line)
		if w > inner {
			line = ansi.Truncate(line, inner, "…")
			w = lipgloss.Width(line)
		}
		pad := inner - w
		if pad < 0 {
			pad = 0
		}
		body.WriteString(border.Render("│"))
		body.WriteString(" ")
		body.WriteString(line)
		body.WriteString(strings.Repeat(" ", pad))
		body.WriteString(" ")
		body.WriteString(border.Render("│"))
		body.WriteString("\n")
	}
	bottom := border.Render("╰" + strings.Repeat("─", inner+2) + "╯")
	return top + "\n" + body.String() + bottom
}

// Rule renders a horizontal divider with an optional label.
func (s Styles) Rule(label string, width int) string {
	if width <= 0 {
		return ""
	}
	if label == "" {
		return s.Border.Render(strings.Repeat("─", width))
	}
	label = " " + label + " "
	lw := lipgloss.Width(label)
	fill := width - lw
	if fill < 2 {
		return s.Faint.Render(widgets.Truncate(label, width))
	}
	left := fill / 2
	right := fill - left
	return s.Border.Render(strings.Repeat("─", left)) + s.Muted.Render(label) + s.Border.Render(strings.Repeat("─", right))
}

// ProgressBar renders a labelled bar with percentage.
func (s Styles) ProgressBar(width int, ratio float64, label string) string {
	if width < 20 {
		width = 20
	}
	barWidth := width - 8
	if label != "" {
		barWidth = width - lipgloss.Width(label) - 10
	}
	if barWidth < 8 {
		barWidth = 8
	}
	bar := widgets.FineBar(barWidth, ratio)
	// Colour only the filled portion: FineBar uses █ and lighter blocks.
	fill := int(ratio * float64(barWidth))
	if ratio > 1 {
		ratio = 1
	}
	if ratio < 0 {
		fill = 0
	}
	filled := bar
	empty := ""
	if fill < len([]rune(bar)) {
		r := []rune(bar)
		filled = string(r[:fill])
		empty = string(r[fill:])
	}
	out := s.BarFill.Render(filled) + s.BarEmpty.Render(empty)
	pct := widgets.Percent(ratio * 100)
	if label != "" {
		return s.Muted.Render(label) + " " + out + " " + s.Text.Render(pct)
	}
	return out + " " + s.Text.Render(pct)
}

// Code renders a command preview block.
func (s Styles) Code(lines []string, width int) string {
	var b strings.Builder
	for i, l := range lines {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(s.Faint.Render("│") + " " + s.CodeBlock.Render(widgets.Truncate(l, width-2)))
	}
	return b.String()
}

// BadgeText returns a small filled label.
func (s Styles) BadgeText(text string) string { return s.Badge.Render(text) }

// KeyHints renders the footer shortcut bar.
func (s Styles) KeyHints(hints []widgets.KeyHint) string {
	parts := make([]string, 0, len(hints))
	for _, h := range hints {
		if h.Key == "" {
			continue
		}
		parts = append(parts, s.Key.Render(h.Key)+" "+s.KeyDesc.Render(h.Desc))
	}
	return strings.Join(parts, s.Faint.Render(" · "))
}

// StatusIcon renders a job status glyph with its semantic colour.
func (s Styles) StatusIcon(status string) string {
	switch status {
	case "done":
		return s.OK.Render("✓")
	case "failed":
		return s.Err.Render("✖")
	case "canceled":
		return s.Muted.Render("⊘")
	case "running":
		return s.Accent.Render("⟳")
	case "paused":
		return s.Warn.Render("⏸")
	}
	return s.Muted.Render("▶")
}

// MiniBar renders a compact progress indicator for lists.
func (s Styles) MiniBar(width int, ratio float64) string {
	if width < 4 {
		width = 4
	}
	if ratio < 0 {
		return s.Faint.Render(strings.Repeat("░", width))
	}
	return s.ProgressBar(width+8, ratio, "")
}

// Label renders "key: value" with muted key.
func (s Styles) Label(key, value string) string {
	return s.Muted.Render(key+": ") + s.Text.Render(value)
}

// ErrorBox renders an error message across a width.
func (s Styles) ErrorBox(err string, width int) string {
	if width < 10 {
		width = 10
	}
	lines := widgets.Wrap(err, width-4)
	var b strings.Builder
	b.WriteString(s.Err.Render("✖ " + lines[0]))
	for _, l := range lines[1:] {
		b.WriteString("\n  " + s.Err.Render(l))
	}
	return b.String()
}

// SelectedRow renders a list row with a selection marker and highlight.
func (s Styles) SelectedRow(text string, width int, selected bool) string {
	marker := "  "
	style := s.Row
	if selected {
		marker = "▸ "
		style = s.Selected
	}
	body := marker + text
	w := lipgloss.Width(body)
	if w > width {
		body = ansi.Truncate(body, width, "…")
		w = lipgloss.Width(body)
	}
	return style.Render(body + strings.Repeat(" ", max(0, width-w)))
}

// WizardBar renders the progress of the conversion wizard.
func (s Styles) WizardBar(labels []string, active int, width int) string {
	var b strings.Builder
	for i, l := range labels {
		if i > 0 {
			b.WriteString(s.Faint.Render(" › "))
		}
		switch {
		case i < active:
			b.WriteString(s.WizardDone.Render("✓ " + l))
		case i == active:
			b.WriteString(s.Accent.Bold(true).Render(l))
		default:
			b.WriteString(s.WizardStep.Render(l))
		}
	}
	out := b.String()
	if lipgloss.Width(out) > width {
		out = ansi.Truncate(out, width, "…")
	}
	return out
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func pct(ratio float64) string {
	if ratio < 0 {
		return "--"
	}
	return fmt.Sprintf("%.0f%%", ratio*100)
}
