package ui

import (
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/ffmpeg-tui/ffmpeg-tui/internal/jobs"
)

// confirmPrompt is a modal yes/no question.
type confirmPrompt struct {
	kind  string
	id    string
	title string
	body  string
}

// Update implements tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// A terminal may deliver several runes in one read (fast typing, a paste or
	// a scripted key sequence). Process them one at a time so global shortcuts
	// such as ":man crop" still open the palette instead of being swallowed as
	// a single unknown key.
	if key, ok := msg.(tea.KeyMsg); ok && key.Type == tea.KeyRunes && len(key.Runes) > 1 {
		var cmds []tea.Cmd
		for _, r := range key.Runes {
			res, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}, Alt: key.Alt})
			if next, ok := res.(*Model); ok {
				m = next
			}
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		return m, tea.Batch(cmds...)
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		if m.wiz.entries == nil {
			m.wiz.loadEntries(m.cwd, m.showHidden)
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case toastExpireMsg:
		if msg.seq == m.toastSeq {
			m.toast = ""
		}
		return m, nil

	case probeMsg:
		if msg.path == m.probePath {
			m.probeLoading = false
			if msg.err != nil {
				m.probeErr = msg.err.Error()
				m.probe = nil
			} else {
				m.probe = msg.res
				m.probeErr = ""
				m.meta.sync(m.probe)
				m.compare.sync(m.probe, msg.path)
				if m.wiz.input != "" && sameFile(msg.path, m.wiz.input) {
					m.wiz.probe = msg.res
					m.wiz.duration = msg.res.Duration()
					if m.wiz.outputAuto {
						m.wiz.buildOutput(m.wiz.input, msg.res)
					}
				}
				m.status = "probed " + msg.path
			}
		}
		return m, nil

	case previewMsg:
		m.previewLoading = false
		if msg.err != nil {
			m.previewErr = msg.err.Error()
			m.previewArt = ""
		} else {
			m.previewArt = msg.res.Art
			m.previewKind = msg.res.Kind
			m.previewErr = ""
		}
		return m, nil

	case compareProbeMsg:
		if msg.err != nil {
			m.compare.errs[msg.slot] = msg.err.Error()
		} else {
			m.compare.probes[msg.slot] = msg.res
			m.compare.paths[msg.slot] = msg.path
			m.compare.errs[msg.slot] = ""
		}
		m.compare.loading[msg.slot] = false
		return m, nil

	case liveHelpMsg:
		m.man.loading = false
		if msg.err != nil {
			m.man.liveErr = msg.err.Error()
		} else {
			m.man.live = msg.text
			m.man.liveErr = ""
			m.man.liveFor = msg.name
			m.man.scroll = 0
		}
		return m, nil

	case savedMsg:
		if msg.err != nil {
			return m, m.flash("✖ " + msg.err.Error())
		}
		return m, m.flash(msg.text)

	case jobEventMsg:
		m.queueSnapshot()
		switch msg.ev.Kind {
		case jobs.EventFinished:
			j := msg.ev.Job
			switch j.Status {
			case "done":
				return m, m.flash("✓ " + j.Name + " finished")
			case "failed":
				return m, m.flash("✖ " + j.Name + " failed: " + shorten(j.Err, 80))
			case "canceled":
				return m, m.flash("⊘ " + j.Name + " canceled")
			}
		}
		return m, waitJobEvent(m.jobs)
	}

	if m.editing {
		var cmd tea.Cmd
		m.editor, cmd = m.editor.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Modal editor input comes first so Esc/Enter mean commit/cancel.
	if m.editing {
		switch key {
		case "esc":
			m.cancelEdit()
			return m, nil
		case "enter", "tab":
			return m, m.commitEdit()
		case "ctrl+c":
			m.cancelEdit()
			return m, nil
		}
		var cmd tea.Cmd
		m.editor, cmd = m.editor.Update(msg)
		return m, cmd
	}

	if m.confirm != nil {
		switch key {
		case "y", "Y", "enter":
			return m.confirmYes()
		case "n", "N", "esc", "q":
			m.confirm = nil
			return m, nil
		}
		return m, nil
	}

	if m.help.open {
		return m.updateHelp(msg)
	}

	if m.palette.open {
		return m.updatePalette(msg)
	}
	if m.search.open {
		return m.updateSearch(msg)
	}

	switch key {
	case "ctrl+c":
		return m.requestQuit()
	case "ctrl+q":
		return m.requestQuit()
	case "ctrl+n":
		m.startWizardFromSelection()
		return m, nil
	case "?":
		m.help.open = true
		m.help.scroll = 0
		return m, nil
	case ":":
		m.palette.open = true
		m.palette.cursor = 0
		m.palette.input.SetValue("")
		m.palette.items = nil
		m.palette.refilter(m)
		cmd := m.palette.input.Focus()
		m.palette.input.Focus()
		return m, cmd
	case "/":
		m.search.open = true
		m.search.input.SetValue("")
		m.search.cursor = 0
		m.search.run(m)
		cmd := m.search.input.Focus()
		m.search.input.Focus()
		return m, cmd
	}

	switch m.screen {
	case screenDashboard:
		return m.updateDashboard(msg)
	case screenBrowser:
		return m.updateBrowser(msg)
	case screenWizard:
		return m.updateWizard(msg)
	case screenFilters:
		return m.updateFilters(msg)
	case screenQueue:
		return m.updateQueue(msg)
	case screenJobDetail:
		return m.updateJobDetail(msg)
	case screenHistory:
		return m.updateHistory(msg)
	case screenPresets:
		return m.updatePresets(msg)
	case screenMan:
		return m.updateMan(msg)
	case screenMetadata:
		return m.updateMetadata(msg)
	case screenStreams:
		return m.updateStreams(msg)
	case screenCompare:
		return m.updateCompare(msg)
	case screenBatch:
		return m.updateBatch(msg)
	case screenTools:
		return m.updateTools(msg)
	}
	return m, nil
}

// requestQuit asks for confirmation when jobs are in flight.
func (m *Model) requestQuit() (tea.Model, tea.Cmd) {
	if m.activeJob() != nil || m.queuedCount() > 0 {
		m.confirm = &confirmPrompt{
			kind:  "quit",
			title: "Quit FFmpeg-TUI?",
			body:  "Jobs are still queued or running. Quitting stops the active FFmpeg process; unfinished jobs stay in the queue.",
		}
		return m, nil
	}
	m.quitting = true
	return m, tea.Quit
}

func (m *Model) queuedCount() int {
	n := 0
	for i := range m.queueSnap.Jobs {
		if m.queueSnap.Jobs[i].Status == "queued" {
			n++
		}
	}
	return n
}

// confirmYes executes the pending confirmation.
func (m *Model) confirmYes() (tea.Model, tea.Cmd) {
	c := m.confirm
	m.confirm = nil
	if c == nil {
		return m, nil
	}
	switch c.kind {
	case "quit":
		m.quitting = true
		return m, tea.Quit
	case "cancel-job":
		if m.jobs.Cancel(c.id) {
			m.queueSnapshot()
			return m, m.flash("⊘ job canceled")
		}
	case "clear-finished":
		n := m.jobs.ClearFinished()
		m.queueSnapshot()
		return m, m.flash(itoa(n) + " finished jobs removed")
	case "clear-history":
		if err := m.hist.Clear(); err != nil {
			return m, m.flash("✖ " + err.Error())
		}
		m.refreshHistory()
		return m, m.flash("history cleared")
	case "delete-history":
		if err := m.hist.Delete(c.id); err != nil {
			return m, m.flash("✖ " + err.Error())
		}
		m.refreshHistory()
		return m, m.flash("history entry deleted")
	}
	return m, nil
}

// startWizardFromSelection opens the conversion wizard, seeding the input with
// the highlighted browser file when there is one.
func (m *Model) startWizardFromSelection() {
	var input string
	var duration time.Duration
	if m.screen == screenBrowser {
		if e, ok := m.selectedEntry(); ok && !e.IsDir {
			input = e.Path
			if m.currentProbe() != nil {
				duration = m.currentProbe().Duration()
			}
		}
	}
	m.wiz.reset(m.cfg, input)
	m.wiz.entries = m.entries
	m.wiz.duration = duration
	if input != "" {
		m.wiz.probe = m.currentProbe()
		m.wiz.buildOutput(input, m.probe)
	}
	m.stack = append(m.stack, m.screen)
	m.screen = screenWizard
}

// quickWizardFor starts a wizard for a specific file with a preset applied.
func (m *Model) quickWizardFor(path, presetName string) tea.Cmd {
	m.wiz.reset(m.cfg, path)
	m.wiz.entries = m.entries
	if m.probe != nil && path == m.probePath {
		m.wiz.probe = m.probe
		m.wiz.duration = m.probe.Duration()
	}
	if presetName != "" {
		if p, ok := presetByName(presetName, m.cfg.Presets); ok {
			m.wiz.applyPreset(p)
		}
	}
	m.wiz.buildOutput(path, m.wiz.probe)
	m.stack = append(m.stack, m.screen)
	m.screen = screenWizard
	if m.ffprobeOK {
		return probeCmd(m.ctx, m.ffprobePath, path)
	}
	return nil
}

func shorten(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
