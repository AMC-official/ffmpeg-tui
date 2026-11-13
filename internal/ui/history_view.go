package ui

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ffmpeg-tui/ffmpeg-tui/internal/config"
	"github.com/ffmpeg-tui/ffmpeg-tui/internal/filesystem"
	"github.com/ffmpeg-tui/ffmpeg-tui/internal/history"
	"github.com/ffmpeg-tui/ffmpeg-tui/internal/widgets"
)

// historyEntries returns the filtered history.
func (m *Model) historyEntries() []history.Entry {
	entries := m.hist.Entries()
	if m.histFilter != "" {
		entries = history.Search(entries, m.histFilter)
	}
	return entries
}

func (m *Model) updateHistory(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	entries := m.historyEntries()
	key := msg.String()
	switch key {
	case "esc":
		if m.histFilter != "" {
			m.histFilter = ""
			return m, nil
		}
		m.back()
	case "up", "k":
		m.histCursor = clampInt(m.histCursor-1, 0, max(0, len(entries)-1))
	case "down", "j":
		m.histCursor = clampInt(m.histCursor+1, 0, max(0, len(entries)-1))
	case "home", "g":
		m.histCursor = 0
	case "end", "G":
		m.histCursor = max(0, len(entries)-1)
	case "pgup", "ctrl+b":
		m.histCursor = clampInt(m.histCursor-10, 0, max(0, len(entries)-1))
	case "pgdown", "ctrl+f":
		m.histCursor = clampInt(m.histCursor+10, 0, max(0, len(entries)-1))
	case "/":
		return m, m.startEdit(editTarget{kind: "history-filter"}, m.histFilter, "filter commands")
	case "enter":
		if m.histCursor < len(entries) {
			return m, m.replayHistory(entries[m.histCursor])
		}
	case "d":
		if m.histCursor < len(entries) {
			e := entries[m.histCursor]
			m.confirm = &confirmPrompt{kind: "delete-history", id: e.ID, title: "Delete history entry?", body: "Remove the record of " + filepath.Base(e.Input) + " → " + filepath.Base(e.Output) + "?"}
		}
	case "c":
		m.confirm = &confirmPrompt{kind: "clear-history", title: "Clear history?", body: "Delete every recorded command. Queue and config are unaffected."}
	case "x":
		dir, err := config.StateDir()
		if err != nil {
			dir = "."
		}
		path := filepath.Join(dir, "command-history.sh")
		if err := m.hist.Export(path); err != nil {
			return m, m.flash("✖ " + err.Error())
		}
		return m, m.flash("✓ history exported to " + path)
	}
	return m, nil
}

// replayHistory queues a recorded command again.
func (m *Model) replayHistory(e history.Entry) tea.Cmd {
	cmd := e.Command()
	cmd.Bin = m.ffmpegPath
	if cmd.Output != "" {
		if _, err := osStat(cmd.Output); err == nil {
			cmd.Output = filesystem.UniquePath(cmd.Output)
		}
	}
	if err := cmd.Validate(); err != nil {
		return m.flash("✖ cannot replay: " + err.Error())
	}
	m.jobs.Enqueue(filepath.Base(cmd.Output), cmd)
	m.queueSnapshot()
	m.stack = append(m.stack, m.screen)
	m.screen = screenQueue
	return m.flash("replayed " + filepath.Base(cmd.Output))
}

func (m *Model) viewHistory() string {
	st := m.styles
	width := m.contentWidth()
	height := m.bodyHeight()
	entries := m.historyEntries()

	var head []string
	head = append(head, st.Label("Recorded commands", fmt.Sprintf("%d", m.hist.Len())))
	if m.histFilter != "" {
		head = append(head, st.Warn.Render("filter: "+m.histFilter))
	}
	head = append(head, st.Faint.Render("Every executed command is recorded with its outcome. Enter replays it to a fresh output path."))

	listRows := height - 13
	if listRows < 4 {
		listRows = 4
	}
	var rows []string
	if len(entries) == 0 {
		rows = append(rows, st.Faint.Render("No history yet. Commands appear here after a job runs."))
	}
	start := listWindowStart(m.histCursor, listRows, len(entries))
	end := min(len(entries), start+listRows)
	for i := start; i < end; i++ {
		e := entries[i]
		icon := m.styles.StatusIcon(e.Status)
		when := e.Time.Format("01-02 15:04")
		line := fmt.Sprintf("%s %s  %s  %s", icon, st.Faint.Render(when),
			st.Text.Render(widgets.Truncate(filepath.Base(e.Input), width/3)),
			st.Muted.Render(widgets.Truncate(e.Command().OneLine(), max(10, width/2))))
		if i == m.histCursor {
			plain := fmt.Sprintf("%s %s  %s  %s", glyphStatus(e.Status), when,
				widgets.Truncate(filepath.Base(e.Input), width/3),
				widgets.Truncate(e.Command().OneLine(), width/2))
			line = st.Selected.Render(widgets.PadRight(widgets.Truncate(plain, width-4), width-4))
		}
		rows = append(rows, line)
	}
	title := "History"
	if label := listRangeLabel(start, end, len(entries)); label != "" {
		title += " · " + label
	}
	list := st.Panel(title, strings.Join(append(head, "", strings.Join(rows, "\n")), "\n"), width, height-8, true)

	detailH := 8
	var detail []string
	if m.histCursor < len(entries) {
		e := entries[m.histCursor]
		detail = append(detail, st.Text.Bold(true).Render(filepath.Base(e.Input))+" → "+st.Accent.Render(filepath.Base(e.Output)))
		detail = append(detail, st.Label("Status", e.Status)+"   "+
			st.Label("Exit", fmt.Sprintf("%d", e.ExitCode))+"   "+
			st.Label("Duration", filesystem.HumanDuration(timeFromMillis(e.DurationMS)))+"   "+
			st.Label("When", e.Time.Format("2006-01-02 15:04:05")))
		cmd := e.Command()
		lines := strings.Split(cmd.Preview(), "\n")
		detail = append(detail, st.CodeBlock.Render(widgets.Truncate(lines[0], width-4)))
		if len(lines) > 1 {
			detail = append(detail, st.CodeBlock.Render(widgets.Truncate(lines[1], width-4)))
		}
		if e.Error != "" {
			detail = append(detail, st.Err.Render(widgets.Truncate(e.Error, width-4)))
		}
	} else {
		detail = append(detail, st.Faint.Render("Nothing selected."))
	}
	detail = append(detail, st.Faint.Render("enter replay · d delete · x export · c clear · / filter"))
	box := st.Panel("Detail", strings.Join(fitLines(detail, detailH-2), "\n"), width, detailH, false)
	return lipgloss.JoinVertical(lipgloss.Left, list, box)
}

func glyphStatus(s string) string {
	switch s {
	case "done":
		return "✓"
	case "failed":
		return "✖"
	case "canceled":
		return "⊘"
	}
	return "·"
}
