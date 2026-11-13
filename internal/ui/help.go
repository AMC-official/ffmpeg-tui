package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/AMC-official/ffmpeg-tui/internal/config"
	"github.com/AMC-official/ffmpeg-tui/internal/widgets"
)

// helpState is the keyboard reference overlay.
type helpState struct {
	open   bool
	scroll int
}

type helpEntry struct{ key, desc string }

type helpSection struct {
	title   string
	entries []helpEntry
}

func helpSections() []helpSection {
	return []helpSection{
		{"Global", []helpEntry{
			{"Ctrl+N", "new conversion wizard"},
			{"Ctrl+Q / Ctrl+C", "quit (confirms while jobs are running)"},
			{"/", "global search: files, man pages, presets, history"},
			{":", "command palette (try :man crop)"},
			{"?", "this help"},
			{"Esc", "back / dismiss overlay"},
			{"Tab", "next field or panel; completes paths in text fields"},
			{"↑ ↓ / j k", "move selection"},
		}},
		{"Jobs & queue", []helpEntry{
			{"Space", "pause / resume the selected job (SIGSTOP/SIGCONT)"},
			{"d", "cancel the running job or remove a queued one"},
			{"Delete", "remove a finished job from the queue"},
			{"r", "retry a failed or finished job"},
			{"J / K", "move the job down / up in the queue"},
			{"s / l", "save / load the queue file"},
			{"x", "clear finished jobs"},
			{"Enter", "open the job detail with live statistics and log"},
		}},
		{"Browser", []helpEntry{
			{"Enter", "open a directory or probe the selected file"},
			{"Backspace", "parent directory"},
			{"H", "toggle hidden files"},
			{"s", "cycle sort: name, size, date, kind"},
			{"Space", "mark a file for batch attention"},
			{"c", "convert the selected file"},
			{"m / v", "metadata editor / stream inspector"},
			{"p", "toggle the ASCII thumbnail preview"},
			{"o", "open in ffplay (optional dependency)"},
			{"a", "pin as favorite"},
			{"b", "batch process this folder"},
			{"1 / 2", "put the file in compare slot A / B"},
		}},
		{"Wizard", []helpEntry{
			{"Enter", "accept the step and continue"},
			{"Esc", "previous step (or leave the wizard)"},
			{"← →", "change the selected option"},
			{"1…9", "jump to an option directly"},
			{"e", "edit a path or metadata field"},
			{"Tab", "complete a path"},
			{"f", "open the filter builder"},
			{"Ctrl+S", "queue the command from the confirmation step"},
		}},
		{"Filter builder", []helpEntry{
			{"Enter", "add the highlighted filter / edit the selected argument"},
			{"Tab", "switch between the filter list and the chain"},
			{"← →", "adjust the selected argument (numeric step or choice)"},
			{"d", "remove the selected filter"},
			{"x", "clear the chain"},
			{"a / Ctrl+S", "apply the chain to the wizard"},
		}},
		{"Man pages", []helpEntry{
			{"Type", "fuzzy search across names, tags and summaries"},
			{"Tab", "cycle category (filters, encoders, muxers, tools…)"},
			{"PgUp / PgDn", "scroll the page"},
			{"f", "fetch live help from the installed ffmpeg (-h filter=…)"},
			{"m", "open the system man page when available"},
			{"Esc", "clear the query, then leave the screen"},
		}},
	}
}

func (m *Model) updateHelp(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "?", "q", "enter":
		m.help.open = false
	case "up", "k":
		m.help.scroll = max(0, m.help.scroll-1)
	case "down", "j":
		m.help.scroll++
	case "pgup":
		m.help.scroll = max(0, m.help.scroll-10)
	case "pgdown":
		m.help.scroll += 10
	case "home", "g":
		m.help.scroll = 0
	case "end", "G":
		m.help.scroll = 1000
	}
	return m, nil
}

func (m *Model) viewHelp(width, height int) string {
	st := m.styles
	panelW := min(width, 96)
	panelH := min(height, 30)
	inner := panelW - 4
	var lines []string
	lines = append(lines, st.Accent.Bold(true).Render("FFmpeg-TUI")+"  "+st.Faint.Render("keyboard reference"))
	lines = append(lines, "")
	for _, sec := range helpSections() {
		lines = append(lines, st.Rule(sec.title, inner))
		for _, e := range sec.entries {
			lines = append(lines, st.Key.Render(widgets.PadRight(e.key, 18))+st.KeyDesc.Render(e.desc))
		}
		lines = append(lines, "")
	}
	lines = append(lines, st.Rule("Safety", inner))
	for _, l := range widgets.Wrap("Commands are executed with exec.Command and an explicit argv. Nothing is ever passed to a shell. Paths are validated, filtergraph values are escaped, and the preview shows exactly the arguments that will run.", inner) {
		lines = append(lines, st.Text.Render(l))
	}
	lines = append(lines, "")
	lines = append(lines, st.Rule("Files", inner))
	lines = append(lines, st.Label("Config", m.configPathLabel()))
	lines = append(lines, st.Label("History", configDirLabel()+"/history.jsonl"))
	lines = append(lines, st.Label("Queue", m.queueFilePath()))
	lines = append(lines, st.Label("Platform", config.PlatformName()))
	lines = append(lines, "")
	lines = append(lines, st.Faint.Render("↑/↓ scroll · esc close"))

	visible := panelH - 2
	if m.help.scroll > len(lines)-visible {
		m.help.scroll = max(0, len(lines)-visible)
	}
	if m.help.scroll < 0 {
		m.help.scroll = 0
	}
	end := min(len(lines), m.help.scroll+visible)
	return st.Panel("Help", strings.Join(lines[m.help.scroll:end], "\n"), panelW, panelH, true)
}
