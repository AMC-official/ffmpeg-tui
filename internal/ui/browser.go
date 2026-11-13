package ui

import (
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/AMC-official/ffmpeg-tui/internal/filesystem"
	"github.com/AMC-official/ffmpeg-tui/internal/widgets"
)

func (m *Model) updateBrowser(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "esc":
		if m.browserFilter != "" {
			m.browserFilter = ""
			m.filtering = false
			m.entries = m.allEntries
			m.browserCursor = 0
			return m, nil
		}
		m.back()
		return m, nil
	case "up", "k":
		m.browserCursor = clampInt(m.browserCursor-1, 0, max(0, len(m.entries)-1))
		return m, nil
	case "down", "j":
		m.browserCursor = clampInt(m.browserCursor+1, 0, max(0, len(m.entries)-1))
		return m, nil
	case "pgup":
		m.browserCursor = clampInt(m.browserCursor-10, 0, max(0, len(m.entries)-1))
		return m, nil
	case "pgdown":
		m.browserCursor = clampInt(m.browserCursor+10, 0, max(0, len(m.entries)-1))
		return m, nil
	case "home":
		m.browserCursor = 0
		return m, nil
	case "end", "G":
		m.browserCursor = max(0, len(m.entries)-1)
		return m, nil
	case "enter", "right", "l":
		e, ok := m.selectedEntry()
		if !ok {
			return m, nil
		}
		if e.IsDir {
			m.loadDir(e.Path)
			return m, nil
		}
		return m, m.probeSelected()
	case "backspace", "left", "h":
		parent := filesystem.Parent(m.cwd)
		if parent != m.cwd {
			m.loadDir(parent)
			return m, m.flash("↑ " + parent)
		}
	case "H":
		m.showHidden = !m.showHidden
		m.loadDir(m.cwd)
		return m, m.flash("hidden files: " + boolText(m.showHidden))
	case "s":
		modes := []string{"name", "size", "date", "kind"}
		idx := 0
		for i, s := range modes {
			if s == m.sortMode {
				idx = i
			}
		}
		m.sortMode = modes[(idx+1)%len(modes)]
		filesystem.Sort(m.entries, m.sortMode)
		return m, m.flash("sorted by " + m.sortMode)
	case "r":
		m.loadDir(m.cwd)
		if e, ok := m.selectedEntry(); ok && !e.IsDir {
			return m, m.probeSelected()
		}
		return m, m.flash("reloaded " + m.cwd)
	case "/":
		return m, m.startEdit(editTarget{kind: "browser-filter"}, m.browserFilter, "filter names")
	case "g":
		return m, m.startEdit(editTarget{kind: "browser-go"}, m.cwd, "directory or file path")
	case "c", "ctrl+n":
		m.startWizardFromSelection()
		return m, nil
	case "m":
		if e, ok := m.selectedEntry(); ok && !e.IsDir {
			cmd := m.probeSelected()
			m.setScreen(screenMetadata)
			return m, cmd
		}
	case "v":
		if e, ok := m.selectedEntry(); ok && !e.IsDir {
			cmd := m.probeSelected()
			m.setScreen(screenStreams)
			return m, cmd
		}
	case "p":
		m.previewOn = !m.previewOn
		if m.previewOn {
			return m, m.probeSelected()
		}
		m.previewArt = ""
		return m, m.flash("preview hidden")
	case "a":
		e, ok := m.selectedEntry()
		if !ok {
			return m, nil
		}
		m.cfg.AddFavorite(e.Name, e.Path, e.IsDir)
		m.saveConfig()
		m.dashActions = m.buildDashActions()
		return m, m.flash("★ added to favorites: " + e.Name)
	case "b":
		m.batch = newBatchState()
		m.batch.source = m.cwd
		m.batch.init(m.cwd)
		m.setScreen(screenBatch)
		return m, nil
	case "f":
		m.flt.sync(m.wiz)
		m.setScreen(screenFilters)
		return m, nil
	case "1", "2":
		e, ok := m.selectedEntry()
		if !ok || e.IsDir {
			return m, nil
		}
		slot := int(key[0] - '1')
		m.compare.paths[slot] = e.Path
		m.compare.probes[slot] = nil
		m.compare.loading[slot] = true
		m.setScreen(screenCompare)
		return m, compareProbeCmd(m.ctx, m.ffprobePath, slot, e.Path)
	case " ":
		e, ok := m.selectedEntry()
		if !ok || e.IsDir {
			return m, nil
		}
		if m.marked[e.Path] {
			delete(m.marked, e.Path)
		} else {
			m.marked[e.Path] = true
		}
		return m, nil
	case "o":
		e, ok := m.selectedEntry()
		if !ok || e.IsDir {
			return m, nil
		}
		if !m.ffplayOK {
			return m, m.flash("ffplay is not installed; it is optional but enables previews")
		}
		cmd := exec.Command("ffplay", "-autoexit", "-window_title", e.Name, e.Path)
		return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
			if err != nil {
				return savedMsg{err: err}
			}
			return savedMsg{text: "preview closed"}
		})
	}
	return m, nil
}

func boolText(v bool) string {
	if v {
		return "on"
	}
	return "off"
}

func (m *Model) viewBrowser() string {
	st := m.styles
	width := m.contentWidth()
	height := m.bodyHeight()
	leftW := width * 56 / 100
	if leftW < 36 {
		leftW = 36
	}
	if leftW > 70 {
		leftW = 70
	}
	rightW := width - leftW - 2
	if rightW < 28 {
		rightW = 28
	}

	crumb := widgets.TruncateMiddle(m.cwd, leftW-4)
	var head []string
	head = append(head, st.Accent.Render(crumb))
	statusLine := st.Faint.Render(filesystem.HumanSize(dirSize(m.entries)) + " • " + itoa(len(m.entries)) + " entries")
	if m.browserFilter != "" {
		statusLine += st.Warn.Render(" • filter: " + m.browserFilter)
	}
	if len(m.marked) > 0 {
		statusLine += st.Accent2.Render(" • " + itoa(len(m.marked)) + " marked")
	}
	head = append(head, statusLine)
	head = append(head, "")

	listRows := height - len(head) - 2
	if listRows < 3 {
		listRows = 3
	}
	if m.browserCursor < m.browserOffset {
		m.browserOffset = m.browserCursor
	}
	if m.browserCursor >= m.browserOffset+listRows {
		m.browserOffset = m.browserCursor - listRows + 1
	}
	if m.browserOffset < 0 {
		m.browserOffset = 0
	}
	if len(m.entries) > listRows {
		end := min(len(m.entries), m.browserOffset+listRows)
		statusLine += st.Faint.Render(" • " + itoa(m.browserOffset+1) + "-" + itoa(end) + " of " + itoa(len(m.entries)))
		head[1] = statusLine
	}
	var rows []string
	if len(m.entries) == 0 {
		rows = append(rows, st.Faint.Render("This directory has no matching entries."))
	}
	for i := m.browserOffset; i < len(m.entries) && i < m.browserOffset+listRows; i++ {
		e := m.entries[i]
		marker := " "
		if m.marked[e.Path] {
			marker = st.Accent2.Render("●")
		}
		name := e.Name
		if e.IsDir {
			name += "/"
		}
		meta := ""
		if !e.IsDir {
			meta = filesystem.HumanSize(e.Size)
		}
		line := marker + " " + m.entryTag(e) + " " + widgets.Truncate(name, leftW-22)
		pad := leftW - 4 - lipgloss.Width(line) - len(meta)
		if pad < 1 {
			pad = 1
		}
		if i == m.browserCursor {
			plain := marker + " " + name
			row := st.Selected.Render(widgets.PadRight(widgets.Truncate("▸ "+plain, leftW-20)+"  "+meta, leftW-4))
			rows = append(rows, row)
		} else {
			rows = append(rows, line+strings.Repeat(" ", pad)+st.Faint.Render(meta))
		}
	}
	list := st.Panel("Media browser", strings.Join(append(head, rows...), "\n"), leftW, height, true)

	// Details.
	var detail []string
	if e, ok := m.selectedEntry(); ok {
		if e.IsDir {
			detail = append(detail, st.Text.Bold(true).Render(e.Name+"/"))
			detail = append(detail, st.Label("Type", "directory"))
			detail = append(detail, st.Label("Path", widgets.TruncateMiddle(e.Path, rightW-10)))
			detail = append(detail, "")
			detail = append(detail, st.Faint.Render("Enter opens the directory. Press a to pin it as a favorite."))
		} else {
			detail = append(detail, st.Text.Bold(true).Render(widgets.Truncate(e.Name, rightW-4)))
			detail = append(detail, st.Label("Type", filesystem.KindLabel(e.Kind)+" • "+e.Ext))
			detail = append(detail, st.Label("Size", filesystem.HumanSize(e.Size)))
			detail = append(detail, st.Label("Modified", filesystem.HumanTime(e.ModTime)))
			detail = append(detail, "")
			if m.probeLoading && m.probePath == e.Path {
				detail = append(detail, st.Muted.Render(m.spinner.View()+" probing with ffprobe…"))
			} else if m.probeErr != "" && m.probePath == e.Path {
				detail = append(detail, st.Err.Render(widgets.Truncate(m.probeErr, rightW-4)))
			} else if p := m.currentProbe(); p != nil {
				detail = append(detail, m.probeSummaryLines(p)...)
			} else {
				detail = append(detail, st.Faint.Render("Press Enter to inspect this file with ffprobe."))
			}
			if m.previewOn {
				detail = append(detail, "")
				if m.previewLoading {
					detail = append(detail, st.Muted.Render("rendering terminal preview…"))
				} else if m.previewErr != "" {
					detail = append(detail, st.Err.Render(widgets.Truncate(m.previewErr, rightW-4)))
				} else if m.previewArt != "" {
					detail = append(detail, st.Rule("Preview", rightW-4))
					for _, l := range strings.Split(m.previewArt, "\n") {
						detail = append(detail, st.Accent.Render(l))
					}
				}
			}
		}
	} else {
		detail = append(detail, st.Faint.Render("Select a file to inspect it."))
	}
	if len(m.marked) > 0 {
		detail = append(detail, "", st.Rule("Marked for batch", rightW-4))
		n := 0
		for path := range m.marked {
			if n >= 4 {
				detail = append(detail, st.Faint.Render("…"))
				break
			}
			detail = append(detail, st.Text.Render("• "+widgets.Truncate(filepath.Base(path), rightW-6)))
			n++
		}
	}
	right := st.Panel("Details", strings.Join(fitLines(detail, height-2), "\n"), rightW, height, false)
	return lipgloss.JoinHorizontal(lipgloss.Top, list, "  ", right)
}

func dirSize(entries []filesystem.Entry) int64 {
	var total int64
	for _, e := range entries {
		if !e.IsDir {
			total += e.Size
		}
	}
	return total
}
