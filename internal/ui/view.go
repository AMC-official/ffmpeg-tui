package ui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/AMC-official/ffmpeg-tui/internal/ffmpeg"
	"github.com/AMC-official/ffmpeg-tui/internal/filesystem"
	"github.com/AMC-official/ffmpeg-tui/internal/filters"
	"github.com/AMC-official/ffmpeg-tui/internal/presets"
	"github.com/AMC-official/ffmpeg-tui/internal/widgets"
)

// Version is the application version, overridable at link time.
var Version = "0.1.0"

// View implements tea.Model.
func (m *Model) View() string {
	if m.quitting {
		return ""
	}
	if m.width == 0 || m.height == 0 {
		return "Loading FFmpeg-TUI…"
	}
	if m.width < 56 || m.height < 16 {
		msg := fmt.Sprintf("Terminal is %dx%d.\nFFmpeg-TUI needs at least 56x16.\nResize the window or press Ctrl+Q to quit.", m.width, m.height)
		return lipgloss.NewStyle().Padding(1).Render(msg)
	}

	header := m.viewHeader()
	body := m.viewBody()
	preview := m.viewPreview()
	hints := m.viewHints()
	status := m.viewStatus()
	out := header + "\n" + body + "\n" + preview + "\n" + hints + "\n" + status
	return out
}

func (m *Model) viewBody() string {
	w := m.contentWidth()
	h := m.bodyHeight()
	switch {
	case m.confirm != nil:
		return m.viewConfirm(w, h)
	case m.editing:
		return m.viewEditor(w, h)
	case m.help.open:
		return m.viewHelp(w, h)
	case m.palette.open:
		return m.viewPalette(w, h)
	case m.search.open:
		return m.viewSearch(w, h)
	}
	switch m.screen {
	case screenDashboard:
		return m.viewDashboard()
	case screenBrowser:
		return m.viewBrowser()
	case screenWizard:
		return m.viewWizard()
	case screenFilters:
		return m.viewFilters()
	case screenQueue:
		return m.viewQueue()
	case screenJobDetail:
		return m.viewJobDetail()
	case screenHistory:
		return m.viewHistory()
	case screenPresets:
		return m.viewPresets()
	case screenMan:
		return m.viewMan()
	case screenMetadata:
		return m.viewMetadata()
	case screenStreams:
		return m.viewStreams()
	case screenCompare:
		return m.viewCompare()
	case screenBatch:
		return m.viewBatch()
	case screenTools:
		return m.viewTools()
	}
	return ""
}

func (m *Model) viewHeader() string {
	st := m.styles
	innerW := m.width - 2
	left := st.AppTitle.Render("FFmpeg-TUI") + " " + st.Version.Render("v"+Version)
	if m.ffmpegVersion != "" {
		left += st.Faint.Render("  ·  ffmpeg " + m.ffmpegVersion)
	} else {
		left += st.Err.Render("  ·  ffmpeg not found")
	}
	if !m.ffprobeOK {
		left += st.Warn.Render("  ·  ffprobe missing")
	}

	var right string
	if j := m.activeJob(); j != nil {
		right = st.Accent.Render("⟳ "+widgets.Truncate(j.Name, 28)) + " " +
			st.Faint.Render(pct(j.Progress.Ratio()))
	} else if queued := m.queuedCount(); queued > 0 {
		right = st.Muted.Render(fmt.Sprintf("%d queued", queued))
	} else {
		right = st.Faint.Render(widgets.TruncateMiddle(m.cwd, 40))
	}
	leftW := lipgloss.Width(left)
	rightW := lipgloss.Width(right)
	gap := innerW - leftW - rightW
	if gap < 1 {
		right = ansi.Truncate(right, max(0, innerW-leftW-1), "…")
		rightW = lipgloss.Width(right)
		gap = max(0, innerW-leftW-rightW)
	}
	return st.Header.Render(left + strings.Repeat(" ", gap) + right)
}

// viewPreview renders the always-visible command preview.
func (m *Model) viewPreview() string {
	st := m.styles
	title, lines := m.previewLines()
	ph := m.previewHeight()
	rows := ph - 2
	if rows < 1 {
		rows = 1
	}
	if len(lines) > rows {
		kept := append([]string(nil), lines[:rows]...)
		kept[rows-1] = st.Faint.Render(fmt.Sprintf("… %d more argument lines", len(lines)-rows+1))
		lines = kept
	}
	var body []string
	for _, l := range lines {
		body = append(body, st.CodeBlock.Render(widgets.Truncate(l, m.contentWidth()-4)))
	}
	for len(body) < rows {
		body = append(body, "")
	}
	return st.Panel(title, strings.Join(body, "\n"), m.contentWidth(), ph, false)
}

// previewLines returns the title and command preview for the active screen.
func (m *Model) previewLines() (string, []string) {
	const title = "Command preview"
	switch m.screen {
	case screenWizard:
		cmd, err := m.buildWizardCommand()
		if err != nil {
			return title, []string{m.styles.Warn.Render("⚠ " + err.Error())}
		}
		return title + " · " + firstNonEmpty(cmd.Label, "wizard"), cmd.PreviewWrapped(m.contentWidth() - 4)
	case screenFilters:
		old := m.wiz.filters
		m.wiz.filters = m.flt.chain
		cmd, err := m.buildWizardCommand()
		m.wiz.filters = old
		if err != nil {
			return title + " · filters", []string{m.styles.Faint.Render("Filter chain: " + firstNonEmpty(filtersSummary(m.flt.chain), "empty"))}
		}
		return title + " · filters", cmd.PreviewWrapped(m.contentWidth() - 4)
	case screenBrowser, screenStreams:
		e, ok := m.selectedEntry()
		if ok && !e.IsDir {
			return "ffprobe command", []string{
				"ffprobe \\",
				"  -v error -hide_banner -print_format json \\",
				"  -show_format -show_streams -show_chapters \\",
				"  " + ffmpeg.Quote(e.Path),
			}
		}
		return title, []string{m.styles.Faint.Render("Select a media file to inspect it. Conversions start with n.")}
	case screenQueue:
		if sel := m.selectedJob(); sel != nil {
			return title + " · queued job", sel.Command.PreviewWrapped(m.contentWidth() - 4)
		}
		return title, []string{m.styles.Faint.Render("No job selected.")}
	case screenJobDetail:
		if j := m.focusedJob(); j != nil {
			return title + " · running job", j.Command.PreviewWrapped(m.contentWidth() - 4)
		}
	case screenHistory:
		entries := m.historyEntries()
		if m.histCursor >= 0 && m.histCursor < len(entries) {
			return title + " · history", entries[m.histCursor].Command().PreviewWrapped(m.contentWidth() - 4)
		}
		return title, []string{m.styles.Faint.Render("No history entry selected.")}
	case screenPresets:
		all := allPresets(m.cfg.Presets)
		if m.presetCursor >= 0 && m.presetCursor < len(all) {
			p := all[m.presetCursor]
			input := "<input>"
			if e, ok := m.selectedEntry(); ok && !e.IsDir {
				input = e.Path
			}
			b := ffmpeg.NewBuilder(m.ffmpegPath, input)
			p.Apply(b)
			cmd := b.Build(filesystem.DefaultOutput(input, firstNonEmpty(p.Container, "mkv")))
			return title + " · preset " + p.Name, cmd.PreviewWrapped(m.contentWidth() - 4)
		}
	case screenMetadata:
		if p := m.currentProbe(); p != nil && m.meta.path != "" {
			ext := strings.TrimPrefix(filepath.Ext(m.meta.path), ".")
			out := strings.TrimSuffix(m.meta.path, filepath.Ext(m.meta.path)) + "-tagged." + ext
			b := ffmpeg.NewBuilder(m.ffmpegPath, m.meta.path)
			b.VideoCodec("copy").AudioCodec("copy").SubtitleCodec("copy").Map("0")
			b.MetadataPairs(m.meta.values.Pairs())
			cmd := b.Build(out)
			return title + " · metadata (stream copy)", cmd.PreviewWrapped(m.contentWidth() - 4)
		}
		return title, []string{m.styles.Faint.Render("Open a media file to edit its tags.")}
	case screenCompare:
		c := m.compare
		var lines []string
		for i := 0; i < 2; i++ {
			if c.paths[i] == "" {
				continue
			}
			lines = append(lines, "ffprobe -v error -show_format -show_streams \\")
			lines = append(lines, "  "+ffmpeg.Quote(c.paths[i]))
		}
		if len(lines) == 0 {
			lines = []string{m.styles.Faint.Render("Enter two files to compare.")}
		}
		return "ffprobe · comparison", lines
	case screenMan:
		if e, ok := m.man.selected(); ok && len(e.Examples) > 0 {
			return "Example · " + e.Name, []string{"ffmpeg \\", "  " + e.Examples[0]}
		}
		return "Manual", []string{m.styles.Faint.Render("Search the bundled FFmpeg documentation. Try normalize.")}
	case screenBatch:
		b := m.batch
		if len(b.files) > 0 {
			p, _ := presets.ByName(b.presetName, m.cfg.Presets)
			builder := ffmpeg.NewBuilder(m.ffmpegPath, b.files[0].Path)
			p.Apply(builder)
			out := filesystem.TemplateOutput(b.files[0].Path, b.template, b.outputDir, 1)
			cmd := builder.Build(out)
			return "Batch · example for " + filepath.Base(b.files[0].Path), cmd.PreviewWrapped(m.contentWidth() - 4)
		}
		return "Batch", []string{m.styles.Faint.Render("Pick a source folder to preview the batch command.")}
	case screenTools:
		tools := m.tools.filtered()
		if m.tools.cursor >= 0 && m.tools.cursor < len(tools) {
			t := tools[m.tools.cursor]
			return "Recipe · " + t.Name, []string{
				"preset: " + t.Preset,
				"container: ." + t.Container,
				"output suffix: " + t.Suffix,
			}
		}
	case screenDashboard:
		if j := m.activeJob(); j != nil {
			return title + " · active job", strings.Split(j.Command.Preview(), "\n")
		}
		return title, []string{m.styles.Faint.Render("Start a conversion with n; the exact ffmpeg argv appears here before anything runs.")}
	}
	return title, []string{m.styles.Faint.Render("No command on this screen.")}
}

func filtersSummary(chain []filters.Selected) string {
	names := make([]string, 0, len(chain))
	for _, s := range chain {
		names = append(names, s.Def.Name)
	}
	return strings.Join(names, ",")
}

// viewHints renders the shortcut bar for the active screen.
func (m *Model) viewHints() string {
	hints := m.screenHints()
	if m.editing {
		hints = []widgets.KeyHint{{Key: "enter", Desc: "save"}, {Key: "esc", Desc: "cancel"}, {Key: "ctrl+c", Desc: "cancel"}}
	}
	if m.confirm != nil {
		hints = []widgets.KeyHint{{Key: "y", Desc: "confirm"}, {Key: "n/esc", Desc: "cancel"}}
	}
	line := m.styles.KeyHints(hints)
	return ansi.Truncate(line, m.width, "…")
}

func (m *Model) screenHints() []widgets.KeyHint {
	switch m.screen {
	case screenDashboard:
		return []widgets.KeyHint{{Key: "↑↓", Desc: "select"}, {Key: "enter", Desc: "open"}, {Key: "n", Desc: "new"}, {Key: "b", Desc: "browse"}, {Key: "q", Desc: "queue"}, {Key: "?", Desc: "help"}}
	case screenBrowser:
		return []widgets.KeyHint{{Key: "↑↓", Desc: "move"}, {Key: "enter", Desc: "inspect"}, {Key: "c", Desc: "convert"}, {Key: "space", Desc: "mark"}, {Key: "a", Desc: "favorite"}, {Key: "/", Desc: "filter"}, {Key: "esc", Desc: "back"}}
	case screenWizard:
		return []widgets.KeyHint{{Key: "enter", Desc: "continue"}, {Key: "←→", Desc: "change"}, {Key: "e", Desc: "edit"}, {Key: "f", Desc: "filters"}, {Key: "esc", Desc: "back"}}
	case screenFilters:
		return []widgets.KeyHint{{Key: "enter", Desc: "add/edit"}, {Key: "tab", Desc: "chain"}, {Key: "←→", Desc: "adjust"}, {Key: "d", Desc: "remove"}, {Key: "a", Desc: "apply"}, {Key: "esc", Desc: "back"}}
	case screenQueue:
		return []widgets.KeyHint{{Key: "space", Desc: "pause/resume"}, {Key: "d", Desc: "cancel"}, {Key: "r", Desc: "retry"}, {Key: "J/K", Desc: "reorder"}, {Key: "s/l", Desc: "save/load"}, {Key: "enter", Desc: "details"}}
	case screenJobDetail:
		return []widgets.KeyHint{{Key: "space", Desc: "pause/resume"}, {Key: "d", Desc: "cancel"}, {Key: "r", Desc: "retry"}, {Key: "esc", Desc: "back"}}
	case screenHistory:
		return []widgets.KeyHint{{Key: "enter", Desc: "replay"}, {Key: "d", Desc: "delete"}, {Key: "x", Desc: "export"}, {Key: "c", Desc: "clear"}, {Key: "/", Desc: "filter"}, {Key: "esc", Desc: "back"}}
	case screenPresets:
		return []widgets.KeyHint{{Key: "enter", Desc: "apply"}, {Key: "s", Desc: "save copy"}, {Key: "d", Desc: "delete custom"}, {Key: "esc", Desc: "back"}}
	case screenMan:
		return []widgets.KeyHint{{Key: "type", Desc: "search"}, {Key: "tab", Desc: "category"}, {Key: "f", Desc: "live help"}, {Key: "m", Desc: "system man"}, {Key: "pgup/dn", Desc: "scroll"}, {Key: "esc", Desc: "back"}}
	case screenMetadata:
		return []widgets.KeyHint{{Key: "enter", Desc: "edit"}, {Key: "s", Desc: "save copy"}, {Key: "r", Desc: "reload"}, {Key: "esc", Desc: "back"}}
	case screenStreams:
		return []widgets.KeyHint{{Key: "↑↓", Desc: "scroll"}, {Key: "c", Desc: "convert"}, {Key: "m", Desc: "metadata"}, {Key: "esc", Desc: "back"}}
	case screenCompare:
		return []widgets.KeyHint{{Key: "1/2", Desc: "set path"}, {Key: "r", Desc: "re-probe"}, {Key: "a", Desc: "use browser file"}, {Key: "esc", Desc: "back"}}
	case screenBatch:
		return []widgets.KeyHint{{Key: "↑↓", Desc: "field"}, {Key: "enter", Desc: "edit/toggle"}, {Key: "q", Desc: "queue jobs"}, {Key: "r", Desc: "rescan"}, {Key: "esc", Desc: "back"}}
	case screenTools:
		return []widgets.KeyHint{{Key: "↑↓", Desc: "select"}, {Key: "tab", Desc: "category"}, {Key: "enter", Desc: "open wizard"}, {Key: "esc", Desc: "back"}}
	}
	return []widgets.KeyHint{{Key: "?", Desc: "help"}, {Key: "esc", Desc: "back"}}
}

func (m *Model) viewStatus() string {
	st := m.styles
	if m.toast != "" {
		line := m.toast
		if strings.HasPrefix(line, "✖") {
			line = st.Err.Render(line)
		} else {
			line = st.OK.Render(line)
		}
		return st.Status.Render(ansi.Truncate(line, m.width-2, "…"))
	}
	status := m.status
	if j := m.activeJob(); j != nil {
		status = fmt.Sprintf("%s %s · %s", m.spinner.View(), j.Name, widgets.Percent(j.Progress.Ratio()))
	}
	if status == "" {
		status = "ready"
	}
	return st.Status.Render(ansi.Truncate(st.Faint.Render(status), m.width-2, "…"))
}

// viewEditor renders the inline text editor modal.
func (m *Model) viewEditor(width, height int) string {
	st := m.styles
	panelW := min(width, 86)
	panelH := 7
	var title, help string
	switch m.edit.kind {
	case "wizard-input":
		title, help = "Input file", "Enter accepts · Tab completes the path · Esc cancels"
	case "wizard-output":
		title, help = "Output file", "The extension selects the container; Enter accepts"
	case "wizard-extra":
		title, help = "Second input", "Used by overlay, stack and merge recipes"
	case "wizard-meta":
		title, help = "Metadata · "+m.edit.key, "Empty values are omitted from the command"
	case "wizard-sub-file":
		title, help = "Subtitle file", "SRT, ASS or WebVTT; the path is escaped for the filtergraph"
	case "filter-arg":
		title, help = "Filter argument · "+m.edit.key, "Enter applies · Esc keeps the previous value"
	case "meta-value":
		title, help = "Tag · "+m.edit.key, "Enter saves the value locally; s writes it to a file"
	case "browser-filter":
		title, help = "Filter files", "Substring match on names"
	case "browser-go":
		title, help = "Go to path", "A directory or file path"
	case "history-filter":
		title, help = "Filter history", "Substring match on the command line"
	case "batch-output", "batch-source":
		title, help = "Directory", "Tab completes the path"
	case "batch-template":
		title, help = "Filename template", "{name} {ext} {index} {date} {dir}"
	case "batch-filter":
		title, help = "Name filter", "Empty includes every file"
	case "preset-name":
		title, help = "Custom preset name", "Saved to the config file"
	case "compare-path":
		title, help = "Compare path", "A media file to probe"
	default:
		title, help = "Edit", "Enter accepts · Esc cancels"
	}
	content := m.editor.View() + "\n\n" + st.Faint.Render(help)
	return st.Panel(title, content, panelW, panelH, true)
}

// viewConfirm renders the confirmation modal.
func (m *Model) viewConfirm(width, height int) string {
	st := m.styles
	c := m.confirm
	panelW := min(width, 72)
	body := []string{}
	for _, l := range widgets.Wrap(c.body, panelW-6) {
		body = append(body, st.Text.Render(l))
	}
	body = append(body, "", st.Warn.Render("y / enter confirms · n / esc cancels"))
	panelH := min(max(8, len(body)+4), height)
	return st.Panel(c.title, strings.Join(body, "\n"), panelW, panelH, true)
}
