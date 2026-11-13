// Package widgets holds the small, dependency-light rendering primitives used
// across the TUI: progress bars, tables, truncation, wrapping and key hint
// bars. They return plain strings so callers own all colour decisions.
package widgets

import (
	"fmt"
	"strings"
)

// Bar renders a unicode progress bar. ratio is 0..1; negative values render an
// indeterminate track of dots.
func Bar(width int, ratio float64, fill, empty rune) string {
	if width <= 0 {
		return ""
	}
	if ratio < 0 {
		dots := strings.Repeat(string(empty), width)
		return dots
	}
	if ratio > 1 {
		ratio = 1
	}
	n := int(float64(width)*ratio + 0.5)
	if n > width {
		n = width
	}
	return strings.Repeat(string(fill), n) + strings.Repeat(string(empty), width-n)
}

// FineBar renders a bar with eighth-block resolution for smooth progress.
func FineBar(width int, ratio float64) string {
	if width <= 0 {
		return ""
	}
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}
	full := int(ratio * float64(width))
	frac := int((ratio*float64(width) - float64(full)) * 8)
	blocks := []rune{' ', '▏', '▎', '▍', '▌', '▋', '▊', '▉'}
	var b strings.Builder
	for i := 0; i < width; i++ {
		switch {
		case i < full:
			b.WriteRune('█')
		case i == full && frac > 0:
			b.WriteRune(blocks[frac])
		default:
			b.WriteRune('░')
		}
	}
	return b.String()
}

// Truncate shortens s to width runes, adding an ellipsis when cut.
func Truncate(s string, width int) string {
	if width <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= width {
		return s
	}
	if width == 1 {
		return "…"
	}
	return string(r[:width-1]) + "…"
}

// TruncateMiddle keeps both ends of a long path, which is far more readable for
// filenames than cutting the tail.
func TruncateMiddle(s string, width int) string {
	if width <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= width {
		return s
	}
	if width < 6 {
		return Truncate(s, width)
	}
	left := (width - 1) / 2
	right := width - 1 - left
	return string(r[:left]) + "…" + string(r[len(r)-right:])
}

// PadRight pads s with spaces to width runes.
func PadRight(s string, width int) string {
	r := []rune(s)
	if len(r) >= width {
		return string(r[:width])
	}
	return s + strings.Repeat(" ", width-len(r))
}

// PadLeft right-aligns s within width runes.
func PadLeft(s string, width int) string {
	r := []rune(s)
	if len(r) >= width {
		return string(r[:width])
	}
	return strings.Repeat(" ", width-len(r)) + s
}

// Wrap wraps text to width runes, preserving explicit newlines.
func Wrap(text string, width int) []string {
	if width <= 0 {
		return nil
	}
	var out []string
	for _, para := range strings.Split(text, "\n") {
		if strings.TrimSpace(para) == "" {
			out = append(out, "")
			continue
		}
		words := strings.Fields(para)
		line := ""
		for _, w := range words {
			wr := []rune(w)
			if len(line) == 0 {
				line = w
				continue
			}
			if len([]rune(line))+1+len(wr) <= width {
				line += " " + w
				continue
			}
			out = append(out, line)
			line = w
			if len(wr) > width {
				// Hard-break an overlong word.
				rest := wr
				for len(rest) > width {
					out = append(out, string(rest[:width]))
					rest = rest[width:]
				}
				line = string(rest)
			}
		}
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

// Column describes a table column.
type Column struct {
	Header string
	Width  int
	Right  bool
}

// Table renders rows with fixed-width columns and a header. Selected rows are
// marked with a leading marker rather than colour, because colour is applied by
// the caller.
func Table(cols []Column, rows [][]string, selected int) []string {
	var lines []string
	var head strings.Builder
	for i, c := range cols {
		if i > 0 {
			head.WriteString("  ")
		}
		head.WriteString(PadRight(c.Header, c.Width))
	}
	lines = append(lines, head.String())
	for ri, row := range rows {
		var b strings.Builder
		if ri == selected {
			b.WriteString("▸ ")
		} else {
			b.WriteString("  ")
		}
		for ci, c := range cols {
			if ci > 0 {
				b.WriteString("  ")
			}
			cell := ""
			if ci < len(row) {
				cell = row[ci]
			}
			if c.Right {
				b.WriteString(PadLeft(cell, c.Width))
			} else {
				b.WriteString(PadRight(cell, c.Width))
			}
		}
		lines = append(lines, b.String())
	}
	return lines
}

// KeyHint is one keyboard shortcut description.
type KeyHint struct {
	Key  string
	Desc string
}

// Hints renders "key desc" pairs separated by a middle dot.
func Hints(hints []KeyHint) string {
	parts := make([]string, 0, len(hints))
	for _, h := range hints {
		if h.Key == "" {
			continue
		}
		parts = append(parts, h.Key+" "+h.Desc)
	}
	return strings.Join(parts, "  ·  ")
}

// Tabs renders a horizontal tab strip, marking the active tab.
func Tabs(names []string, active int) string {
	parts := make([]string, 0, len(names))
	for i, n := range names {
		if i == active {
			parts = append(parts, "["+n+"]")
		} else {
			parts = append(parts, " "+n+" ")
		}
	}
	return strings.Join(parts, " ")
}

// Percent formats a percentage for display.
func Percent(v float64) string {
	if v < 0 {
		return " -- "
	}
	if v >= 100 {
		return "100%"
	}
	return fmt.Sprintf("%.0f%%", v)
}

// Spread lays two strings at the left and right edges of width.
func Spread(left, right string, width int) string {
	lw := len([]rune(left))
	rw := len([]rune(right))
	if lw+rw+1 > width {
		return Truncate(left, width)
	}
	return left + strings.Repeat(" ", width-lw-rw) + right
}

// Center centres s inside width.
func Center(s string, width int) string {
	r := []rune(s)
	if len(r) >= width {
		return s
	}
	left := (width - len(r)) / 2
	return strings.Repeat(" ", left) + s
}

// HumanCount formats large counters with thousands separators.
func HumanCount(n int64) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var parts []string
	for len(s) > 3 {
		parts = append([]string{s[len(s)-3:]}, parts...)
		s = s[:len(s)-3]
	}
	parts = append([]string{s}, parts...)
	return strings.Join(parts, ",")
}

// Sparkline draws a compact numeric series.
func Sparkline(values []float64, width int) string {
	if len(values) == 0 || width <= 0 {
		return ""
	}
	blocks := []rune("▁▂▃▄▅▆▇█")
	max := 0.0
	for _, v := range values {
		if v > max {
			max = v
		}
	}
	if max <= 0 {
		return strings.Repeat(string(blocks[0]), width)
	}
	var b strings.Builder
	for i := 0; i < width && i < len(values); i++ {
		idx := int(values[i] / max * float64(len(blocks)-1))
		if idx < 0 {
			idx = 0
		}
		if idx >= len(blocks) {
			idx = len(blocks) - 1
		}
		b.WriteRune(blocks[idx])
	}
	return b.String()
}
