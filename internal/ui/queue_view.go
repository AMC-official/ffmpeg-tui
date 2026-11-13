package ui

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/AMC-official/ffmpeg-tui/internal/config"
	"github.com/AMC-official/ffmpeg-tui/internal/ffmpeg"
	"github.com/AMC-official/ffmpeg-tui/internal/filesystem"
	"github.com/AMC-official/ffmpeg-tui/internal/queue"
	"github.com/AMC-official/ffmpeg-tui/internal/widgets"
)

func (m *Model) updateQueue(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	jobs := m.queueSnap.Jobs
	sel := m.selectedJob()
	switch msg.String() {
	case "esc":
		m.back()
	case "up", "k":
		m.queueCursor = clampInt(m.queueCursor-1, 0, max(0, len(jobs)-1))
	case "down", "j":
		m.queueCursor = clampInt(m.queueCursor+1, 0, max(0, len(jobs)-1))
	case "home":
		m.queueCursor = 0
	case "end", "G":
		m.queueCursor = max(0, len(jobs)-1)
	case " ":
		if sel == nil {
			return m, nil
		}
		switch sel.Status {
		case queue.Running:
			if !m.jobs.PauseSupported() {
				return m, m.flash("pausing is not supported on this platform")
			}
			if m.jobs.Pause(sel.ID) {
				m.queueSnapshot()
				return m, m.flash("⏸ paused " + sel.Name)
			}
			return m, m.flash("✖ cannot pause this job")
		case queue.Paused:
			if m.jobs.Resume(sel.ID) {
				m.queueSnapshot()
				return m, m.flash("▶ resumed " + sel.Name)
			}
		case queue.Queued:
			return m, m.flash("queued jobs start automatically when the runner is free")
		}
	case "d":
		if sel == nil {
			return m, nil
		}
		if sel.Status == queue.Queued {
			if m.jobs.Cancel(sel.ID) {
				m.queueSnapshot()
				return m, m.flash("⊘ removed from the queue")
			}
		}
		m.confirm = &confirmPrompt{kind: "cancel-job", id: sel.ID, title: "Cancel job?", body: "Stop \"" + sel.Name + "\" and mark it canceled?"}
	case "r":
		if sel != nil && m.jobs.Retry(sel.ID) {
			m.queueSnapshot()
			return m, m.flash("↻ retried " + sel.Name)
		}
	case "J":
		if m.jobs.Move(m.queueCursor, m.queueCursor+1) {
			m.queueCursor++
			m.queueSnapshot()
		}
	case "K":
		if m.jobs.Move(m.queueCursor, m.queueCursor-1) {
			m.queueCursor--
			m.queueSnapshot()
		}
	case "x":
		m.confirm = &confirmPrompt{kind: "clear-finished", title: "Clear finished jobs?", body: "Remove every completed, failed and canceled job from the queue."}
	case "s":
		path := m.queueFilePath()
		if err := m.jobs.Save(path); err != nil {
			return m, m.flash("✖ " + err.Error())
		}
		return m, m.flash("✓ queue saved to " + path)
	case "l":
		path := m.queueFilePath()
		if err := m.jobs.Load(path); err != nil {
			return m, m.flash("✖ " + err.Error())
		}
		m.queueSnapshot()
		m.queueCursor = 0
		return m, m.flash("✓ queue loaded from " + path)
	case "enter":
		if sel != nil {
			m.jobFocus = sel
			m.setScreen(screenJobDetail)
		}
	case "delete":
		if sel != nil && m.jobs.Remove(sel.ID) {
			m.queueSnapshot()
			return m, m.flash("job removed")
		}
	case "ctrl+n":
		m.startWizardFromSelection()
	}
	return m, nil
}

func (m *Model) queueFilePath() string {
	dir, err := config.StateDir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, "queue.json")
}

func (m *Model) viewQueue() string {
	st := m.styles
	width := m.contentWidth()
	height := m.bodyHeight()
	jobs := m.queueSnap.Jobs
	queued, running, done, failed := 0, 0, 0, 0
	for i := range jobs {
		switch jobs[i].Status {
		case queue.Queued:
			queued++
		case queue.Running, queue.Paused:
			running++
		case queue.Done:
			done++
		default:
			failed++
		}
	}
	head := st.Label("Queued", fmt.Sprintf("%d", queued)) + "   " +
		st.Label("Running", fmt.Sprintf("%d", running)) + "   " +
		st.Label("Done", fmt.Sprintf("%d", done)) + "   " +
		st.Label("Failed", fmt.Sprintf("%d", failed)) + "   " +
		st.Faint.Render("one job runs at a time")

	listRows := height - 12
	if listRows < 4 {
		listRows = 4
	}
	start := 0
	if m.queueCursor >= listRows {
		start = m.queueCursor - listRows + 1
	}
	var rows []string
	if len(jobs) == 0 {
		rows = append(rows, st.Faint.Render("The queue is empty. Press Ctrl+N to create a conversion."))
	}
	for i := start; i < len(jobs) && i < start+listRows; i++ {
		j := jobs[i]
		name := widgets.Truncate(j.Name, max(12, width/3))
		summary := widgets.Truncate(j.SummaryLine(), max(10, width/3))
		status := j.StatusText()
		progress := ""
		switch {
		case j.Status.Active() || j.Status == queue.Done:
			progress = widgets.FineBar(12, j.Progress.Ratio()) + " " + pct(j.Progress.Ratio())
		case j.Status == queue.Failed:
			progress = widgets.Truncate(j.Err, 20)
		default:
			progress = "waiting"
		}
		plain := fmt.Sprintf("%s %s  %s  %s  %s", glyph(j.Status), name, summary, status, progress)
		if i == m.queueCursor {
			rows = append(rows, st.Selected.Render(widgets.PadRight(widgets.Truncate(plain, width-4), width-4)))
			continue
		}
		line := fmt.Sprintf("%s %s  %s  %s  %s", st.StatusIcon(string(j.Status)), st.Text.Render(name),
			st.Faint.Render(summary), st.Muted.Render(status), st.Accent.Render(progress))
		rows = append(rows, line)
	}
	list := st.Panel("Job queue", head+"\n\n"+strings.Join(rows, "\n"), width, height-8, true)

	// Selected job detail.
	detailH := 8
	var detail []string
	if sel := m.selectedJob(); sel != nil {
		detail = append(detail, st.Text.Bold(true).Render(sel.Name)+"  "+st.Muted.Render(sel.Command.Label))
		detail = append(detail, st.Label("Status", sel.StatusText())+"   "+
			st.Label("Elapsed", filesystem.HumanDuration(sel.Elapsed()))+"   "+
			st.Label("Output", filepath.Base(sel.Command.Output)))
		if sel.Progress.Known() {
			detail = append(detail, st.ProgressBar(width-6, sel.Progress.Ratio(), ""))
		}
		if sel.Err != "" {
			detail = append(detail, st.Err.Render(widgets.Truncate(sel.Err, width-4)))
		} else if len(sel.Logs) > 0 {
			detail = append(detail, st.Faint.Render(widgets.Truncate(sel.Logs[len(sel.Logs)-1], width-4)))
		}
	} else {
		detail = append(detail, st.Faint.Render("Select a job to see its command and log."))
	}
	detail = append(detail, st.Faint.Render("space pause/resume · d cancel · r retry · J/K reorder · s save · l load · x clear · enter details"))
	box := st.Panel("Selected job", strings.Join(fitLines(detail, detailH-2), "\n"), width, detailH, false)
	return list + "\n" + box
}

func stripANSI(s string) string {
	var b strings.Builder
	inEsc := false
	for _, r := range s {
		if r == 0x1b {
			inEsc = true
			continue
		}
		if inEsc {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEsc = false
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func (m *Model) updateJobDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	job := m.focusedJob()
	switch msg.String() {
	case "esc":
		m.back()
	case " ":
		if job == nil {
			return m, nil
		}
		if job.Status == queue.Running {
			if m.jobs.Pause(job.ID) {
				m.queueSnapshot()
				return m, m.flash("⏸ paused")
			}
			return m, m.flash("cannot pause this job")
		}
		if job.Status == queue.Paused {
			if m.jobs.Resume(job.ID) {
				m.queueSnapshot()
				return m, m.flash("▶ resumed")
			}
		}
	case "d":
		if job != nil {
			m.confirm = &confirmPrompt{kind: "cancel-job", id: job.ID, title: "Cancel job?", body: "Stop \"" + job.Name + "\"?"}
		}
	case "r":
		if job != nil && m.jobs.Retry(job.ID) {
			m.queueSnapshot()
			return m, m.flash("↻ retried")
		}
	case "up", "k":
		m.streamsScroll = max(0, m.streamsScroll-1)
	case "down", "j":
		m.streamsScroll++
	}
	return m, nil
}

func (m *Model) focusedJob() *queue.Job {
	id := ""
	if m.jobFocus != nil {
		id = m.jobFocus.ID
	}
	for i := range m.queueSnap.Jobs {
		if m.queueSnap.Jobs[i].ID == id {
			return &m.queueSnap.Jobs[i]
		}
	}
	return nil
}

func (m *Model) viewJobDetail() string {
	st := m.styles
	width := m.contentWidth()
	height := m.bodyHeight()
	j := m.focusedJob()
	if j == nil {
		return st.Panel("Job", st.Faint.Render("This job is no longer in the queue."), width, height, true)
	}
	var b []string
	add := func(s string) { b = append(b, s) }
	add(st.Text.Bold(true).Render(j.Name))
	add(st.Muted.Render(j.Command.Label))
	add("")
	ratio := j.Progress.Ratio()
	if j.Status == queue.Queued {
		ratio = 0
	}
	add(st.ProgressBar(width-6, ratio, j.StatusText()))
	add("")
	grid := [][4]string{
		{"Frame", widgets.HumanCount(j.Progress.Frame), "FPS", fmt.Sprintf("%.1f", j.Progress.FPS)},
		{"Bitrate", firstNonEmpty(j.Progress.Bitrate, "—"), "Speed", firstNonEmpty(j.Progress.SpeedText, "—")},
		{"Time", ffmpeg.FormatDuration(j.Progress.OutTime), "ETA", ffmpeg.FormatDuration(j.Progress.ETA())},
		{"Elapsed", filesystem.HumanDuration(j.Elapsed()), "Total", filesystem.HumanDuration(j.Progress.Duration)},
	}
	cellW := (width - 8) / 4
	for _, row := range grid {
		line := st.Muted.Render(widgets.PadRight(row[0], 8)) + st.Text.Render(widgets.PadRight(row[1], cellW-8)) +
			st.Muted.Render(widgets.PadRight(row[2], 7)) + st.Text.Render(row[3])
		add(line)
	}
	add("")
	add(st.Label("Input", filepath.Base(j.Command.Input)))
	add(st.Label("Output", widgets.TruncateMiddle(j.Command.Output, width-10)))
	if j.Command.FilterGraph != "" {
		add(st.Label("Filters", widgets.Truncate(j.Command.FilterGraph, width-12)))
	}
	add("")
	logRows := max(3, height-len(b)-6)
	add(st.Rule("Log", width))
	logs := j.Logs
	if m.streamsScroll > 0 && len(logs) > 0 {
		_ = m.streamsScroll
	}
	if len(logs) == 0 {
		add(st.Faint.Render("no log output yet"))
	}
	start := max(0, len(logs)-logRows)
	for _, l := range logs[start:] {
		add(st.Faint.Render(widgets.Truncate(l, width)))
	}
	if j.Err != "" {
		add("")
		add(st.ErrorBox(j.Err, width))
	}
	return st.Panel("Job detail", strings.Join(fitLines(b, height-2), "\n"), width, height, true)
}

// glyph returns the plain status marker used inside highlighted rows.
func glyph(s queue.Status) string {
	switch s {
	case queue.Done:
		return "✓"
	case queue.Failed:
		return "✖"
	case queue.Canceled:
		return "⊘"
	case queue.Running:
		return "⟳"
	case queue.Paused:
		return "⏸"
	}
	return "▶"
}
