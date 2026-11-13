package ui

import (
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ffmpeg-tui/ffmpeg-tui/internal/history"
	"github.com/ffmpeg-tui/ffmpeg-tui/internal/queue"
	"github.com/ffmpeg-tui/ffmpeg-tui/internal/widgets"
)

// dashAction is one selectable item on the dashboard.
type dashAction struct {
	section string
	label   string
	desc    string
	kind    string
	arg     string
}

// buildDashActions assembles the dashboard menu from config and history.
func (m *Model) buildDashActions() []dashAction {
	queued, running := 0, 0
	for i := range m.queueSnap.Jobs {
		switch m.queueSnap.Jobs[i].Status {
		case queue.Queued:
			queued++
		case queue.Running, queue.Paused:
			running++
		}
	}
	queueHint := "no jobs"
	if queued+running > 0 {
		queueHint = itoa(queued) + " queued · " + itoa(running) + " running"
	}
	actions := []dashAction{
		{"Start", "New conversion", "Ctrl+N · guided wizard with live command preview", "wizard", ""},
		{"Start", "Browse media", "files, folders and metadata via ffprobe", "browse", ""},
		{"Start", "Batch process", "encode a folder with templates and mirroring", "batch", ""},
		{"Start", "Tools & recipes", "audio, video and subtitle one-click recipes", "tools", ""},
		{"Library", "Presets", "lossless, web, device and archival recipes", "presets", ""},
		{"Library", "Filter builder", "crop, scale, overlay, noise, loudnorm and more", "filters", ""},
		{"Library", "Man pages", "offline searchable FFmpeg documentation", "man", ""},
		{"Work", "Queue", queueHint, "queue", ""},
		{"Work", "History", itoa(m.hist.Len()) + " recorded commands", "history", ""},
		{"Work", "Compare files", "resolution, bitrate, size and compression ratio", "compare", ""},
		{"Work", "Stream inspector", "streams, chapters, attachments and disposition", "streams", ""},
	}
	if len(m.cfg.Favorites) > 0 {
		for _, f := range m.cfg.Favorites {
			kind := "favorite-file"
			if f.IsDir {
				kind = "favorite-dir"
			}
			actions = append(actions, dashAction{"Favorites", "★ " + f.Name, filepath.Base(f.Path), kind, f.Path})
		}
	}
	recent := history.Inputs(m.hist.Entries(), 5)
	for _, r := range recent {
		actions = append(actions, dashAction{"Recent projects", filepath.Base(r.Input), r.Input, "recent", r.Input})
	}
	return actions
}

func (m *Model) updateDashboard(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	actions := m.dashActions
	if len(actions) == 0 {
		m.dashActions = m.buildDashActions()
		actions = m.dashActions
	}
	switch msg.String() {
	case "up", "k":
		m.dashCursor = clampInt(m.dashCursor-1, 0, len(actions)-1)
	case "down", "j":
		m.dashCursor = clampInt(m.dashCursor+1, 0, len(actions)-1)
	case "home", "g":
		m.dashCursor = 0
	case "end", "G":
		m.dashCursor = len(actions) - 1
	case "enter", "right", "l":
		if m.dashCursor >= 0 && m.dashCursor < len(actions) {
			return m.execDashAction(actions[m.dashCursor])
		}
	case "n", "ctrl+n":
		m.startWizardFromSelection()
	case "b":
		m.setScreen(screenBrowser)
		return m, m.probeSelected()
	case "x":
		m.batch = newBatchState()
		m.batch.init(m.cwd)
		m.setScreen(screenBatch)
	case "p":
		m.setScreen(screenPresets)
	case "f":
		m.flt.sync(m.wiz)
		m.setScreen(screenFilters)
	case "m":
		m.setScreen(screenMan)
	case "q":
		m.queueSnapshot()
		m.setScreen(screenQueue)
	case "h":
		m.setScreen(screenHistory)
	case "t":
		m.setScreen(screenTools)
	case "c":
		m.setScreen(screenCompare)
	case "r":
		m.loadDir(m.cwd)
		m.dashActions = m.buildDashActions()
		return m, m.flash("reloaded " + m.cwd)
	}
	return m, nil
}

// execDashAction runs the highlighted dashboard item.
func (m *Model) execDashAction(a dashAction) (tea.Model, tea.Cmd) {
	switch a.kind {
	case "wizard":
		m.startWizardFromSelection()
	case "browse":
		m.setScreen(screenBrowser)
		return m, m.probeSelected()
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
	case "queue":
		m.queueSnapshot()
		m.setScreen(screenQueue)
	case "history":
		m.setScreen(screenHistory)
	case "compare":
		m.setScreen(screenCompare)
		return m, compareProbeCmd(m.ctx, m.ffprobePath, 0, m.compare.paths[0])
	case "streams":
		if m.currentProbe() != nil {
			m.setScreen(screenStreams)
		} else if e, ok := m.selectedEntry(); ok && !e.IsDir {
			m.setScreen(screenStreams)
			return m, probeCmd(m.ctx, m.ffprobePath, e.Path)
		} else {
			return m, m.flash("open a media file first")
		}
	case "favorite-dir":
		m.loadDir(a.arg)
		m.setScreen(screenBrowser)
		return m, m.probeSelected()
	case "favorite-file", "recent":
		m.openInBrowser(a.arg)
	}
	return m, nil
}

// openInBrowser loads the parent directory and highlights path.
func (m *Model) openInBrowser(path string) {
	dir := filepath.Dir(path)
	m.loadDir(dir)
	base := filepath.Base(path)
	for i, e := range m.entries {
		if e.Name == base {
			m.browserCursor = i
		}
	}
	m.setScreen(screenBrowser)
}

func (m *Model) viewDashboard() string {
	st := m.styles
	width := m.contentWidth()
	height := m.bodyHeight()
	m.dashActions = m.buildDashActions()

	jobsH := min(9, max(5, height/3))
	if len(m.queueSnap.Jobs) == 0 {
		jobsH = 4
	}
	jobsBox := st.Panel("Jobs", m.dashboardJobs(width-4, jobsH-2), width, jobsH, false)

	menuH := height - jobsH - 1
	if menuH < 5 {
		menuH = 5
	}
	menu := st.Panel("Recent projects · Favorites · Presets · Queue · History", m.dashboardMenu(width-4, menuH-2), width, menuH, true)

	glance := st.Faint.Render("ffmpeg " + firstNonEmpty(m.ffmpegVersion, "not found") + "  ·  " + m.cwd)
	if m.ffmpegVersion == "" {
		glance = st.Err.Render("ffmpeg was not found on PATH") + st.Faint.Render("  ·  install ffmpeg or set ffmpeg_path in "+m.configPathLabel())
	}
	return lipgloss.JoinVertical(lipgloss.Left, jobsBox, menu, glance)
}

func (m *Model) dashboardJobs(width, rows int) string {
	st := m.styles
	jobs := m.queueSnap.Jobs
	if len(jobs) == 0 {
		return st.Faint.Render("No jobs yet. Start with New conversion (n) or open the queue (q).")
	}
	var b []string
	shown := 0
	for i := range jobs {
		j := jobs[i]
		if shown >= rows-1 {
			b = append(b, st.Faint.Render("… "+itoa(len(jobs)-shown)+" more in the queue"))
			break
		}
		line := st.StatusIcon(string(j.Status)) + " " + st.Text.Render(widgets.Truncate(j.Name, max(12, width/3))) + "  " +
			st.Faint.Render(widgets.Truncate(j.Command.Label, max(10, width/3))) + "  "
		if j.Status == queue.Running || j.Status == queue.Paused {
			line += st.ProgressBar(max(20, width/3), j.Progress.Ratio(), "")
			if j.Progress.ETA() > 0 {
				line += " " + st.Faint.Render("ETA "+widgets.Truncate(j.Progress.ETA().String(), 12))
			}
		} else {
			line += st.Muted.Render(j.StatusText())
		}
		b = append(b, line)
		shown++
	}
	return strings.Join(b, "\n")
}

func (m *Model) dashboardMenu(width, rows int) string {
	st := m.styles
	actions := m.dashActions
	var b []string
	actionLines := make([]int, len(actions))
	lastSection := ""
	for i, a := range actions {
		if a.section != lastSection {
			if lastSection != "" {
				b = append(b, "")
			}
			b = append(b, st.Muted.Render(a.section))
			lastSection = a.section
		}
		label := a.label
		desc := a.desc
		line := st.SelectedRow("  "+label, width, i == m.dashCursor)
		if i != m.dashCursor {
			body := "  " + st.Text.Render(label)
			pad := width - lipgloss.Width(body) - len(desc) - 3
			if pad < 1 {
				pad = 1
				desc = widgets.Truncate(desc, width/2)
			}
			line = body + strings.Repeat(" ", pad) + st.Faint.Render(desc)
		}
		actionLines[i] = len(b)
		b = append(b, line)
	}
	if rows > 0 && len(b) > rows {
		cursor := clampInt(m.dashCursor, 0, len(actions)-1)
		start := listWindowStart(actionLines[cursor], rows, len(b))
		b = b[start:min(len(b), start+rows)]
	}
	for len(b) < rows {
		b = append(b, "")
	}
	return strings.Join(b, "\n")
}

func (m *Model) configPathLabel() string {
	if m.cfgFile != "" {
		return m.cfgFile
	}
	return "config.json"
}
