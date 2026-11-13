package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/AMC-official/ffmpeg-tui/internal/ffprobe"
	"github.com/AMC-official/ffmpeg-tui/internal/filesystem"
	"github.com/AMC-official/ffmpeg-tui/internal/filters"
	"github.com/AMC-official/ffmpeg-tui/internal/metadata"
	"github.com/AMC-official/ffmpeg-tui/internal/widgets"
)

func (m *Model) viewWizard() string {
	st := m.styles
	w := &m.wiz
	width := m.contentWidth()
	height := m.bodyHeight()

	var labels []string
	active := 0
	for i := 0; i < int(wizStepCount); i++ {
		if w.stepNeeded(i) {
			if i == w.step {
				active = len(labels)
			}
			labels = append(labels, wizStepLabels[i])
		}
	}
	bar := st.WizardBar(labels, active, width)

	panelH := height - 2
	if panelH < 8 {
		panelH = 8
	}
	sideW := 0
	mainW := width
	if width >= 100 {
		sideW = width * 36 / 100
		mainW = width - sideW - 2
	}
	main := st.Panel(w.stepTitle(), m.wizardStepContent(mainW-4, panelH-2), mainW, panelH, true)
	if sideW > 0 {
		side := st.Panel("Summary", m.wizardSidebar(sideW-4, panelH-2), sideW, panelH, false)
		main = lipgloss.JoinHorizontal(lipgloss.Top, main, "  ", side)
	}
	return bar + "\n\n" + main
}

func (m *Model) wizardStepContent(width, rows int) string {
	st := m.styles
	w := &m.wiz
	if rows < 4 {
		rows = 4
	}
	var b []string
	add := func(s string) { b = append(b, s) }
	blank := func() { b = append(b, "") }

	switch wizStep(w.step) {
	case stepInput:
		add(st.Muted.Render("Choose a source file. Enter opens directories or selects a file."))
		add(st.Faint.Render(widgets.TruncateMiddle(w.browseDir, width)))
		blank()
		listRows := rows - 6
		if listRows < 3 {
			listRows = 3
		}
		start := w.entryOffset
		if w.entryCursor < start {
			start = w.entryCursor
		}
		if w.entryCursor >= start+listRows {
			start = w.entryCursor - listRows + 1
		}
		w.entryOffset = start
		for i := start; i < len(w.entries) && i < start+listRows; i++ {
			e := w.entries[i]
			name := e.Name
			if e.IsDir {
				name += "/"
			}
			meta := ""
			if !e.IsDir {
				meta = filesystem.HumanSize(e.Size)
			}
			line := fmt.Sprintf("%s %s", m.entryTag(e), name)
			line = widgets.Truncate(line, width-12)
			pad := width - lipgloss.Width(line) - len(meta) - 2
			if pad < 1 {
				pad = 1
			}
			line += strings.Repeat(" ", pad) + st.Faint.Render(meta)
			b = append(b, st.SelectedRow(line, width, i == w.entryCursor))
		}
		blank()
		value := w.input
		if value == "" {
			value = st.Faint.Render("no file selected — press / to type a path")
		}
		add(st.Label("Input", widgets.TruncateMiddle(value, width-10)))

	case stepOutput:
		add(st.Muted.Render("Where should the result be written?"))
		blank()
		out := w.output
		if out == "" {
			out = st.Faint.Render("(derived from the input when you continue)")
		}
		add(st.Accent.Render(widgets.TruncateMiddle(out, width)))
		blank()
		c := w.container()
		add(st.Label("Container", c.Name+" • "+st.Muted.Render(c.Note)))
		add(st.Label("Directory", filepath.Dir(w.output)))
		if w.output != "" {
			if _, err := os.Stat(w.output); err == nil {
				add(st.Warn.Render("⚠ file exists and will be overwritten"))
			}
		}
		blank()
		add(st.Faint.Render("e edits the path · Tab completes · Enter continues"))

	case stepContainer:
		add(st.Muted.Render("Pick the output container. Codec defaults follow it automatically."))
		blank()
		b = append(b, m.optionList(width, rows-5)...)

	case stepVideo:
		add(st.Muted.Render("Video codec. Copy keeps the original bitstream untouched."))
		blank()
		b = append(b, m.optionList(width, rows-5)...)

	case stepQuality:
		codec, _, _ := w.selectedVideoArgs()
		add(st.Muted.Render(fmt.Sprintf("Quality for %s — lower CRF means better quality and larger files.", codec)))
		blank()
		b = append(b, m.optionList(width, rows-5)...)
		blank()
		add(st.Faint.Render("CRF is constant-quality mode: the encoder spends bits where they matter."))

	case stepSpeed:
		codec, _, _ := w.selectedVideoArgs()
		add(st.Muted.Render(fmt.Sprintf("Encoder speed for %s. Slower presets compress better at the same quality.", codec)))
		blank()
		b = append(b, m.optionList(width, rows-5)...)

	case stepAudio:
		add(st.Muted.Render("Audio codec for the output."))
		blank()
		b = append(b, m.optionList(width, rows-5)...)

	case stepAudioQuality:
		codec, _ := w.selectedAudioArgs()
		add(st.Muted.Render(fmt.Sprintf("Audio bitrate for %s.", codec)))
		blank()
		b = append(b, m.optionList(width, rows-6)...)
		blank()
		add(st.Label("Channels", channelsLabel(w.channelsIdx)))
		add(st.Label("Sample rate", sampleRateLabel(w.sampleRateIdx)))
		add(st.Faint.Render("c cycles channels · r cycles sample rate"))

	case stepSubtitle:
		add(st.Muted.Render("Subtitle handling. Burning requires libass and forces a video re-encode."))
		blank()
		b = append(b, m.optionList(width, rows-6)...)
		blank()
		if w.subModeIdx == 2 {
			file := w.subtitleFile
			if file == "" {
				file = st.Faint.Render("no subtitle file chosen — press e")
			}
			add(st.Label("Subtitle file", widgets.TruncateMiddle(file, width-16)))
		} else if subs := probeSubtitles(m.wizSource()); subs != "" {
			add(st.Label("Embedded", subs))
		}

	case stepMetadata:
		add(st.Muted.Render("Container tags. Empty fields are omitted; existing tags are preserved unless overridden."))
		blank()
		for i, f := range metadata.Fields {
			val := w.meta.Get(f.Key)
			if val == "" {
				val = st.Faint.Render("—")
			}
			line := fmt.Sprintf("%s %s", st.Muted.Render(widgets.PadRight(f.Label, 14)), widgets.Truncate(val, width-20))
			b = append(b, st.SelectedRow(line, width, i == w.metaCursor))
			if i >= rows-8 {
				break
			}
		}
		blank()
		add(st.Faint.Render("Enter edits · Tab next field · Backspace clears · n continues"))

	case stepFilters:
		add(st.Muted.Render("Filter chain. Press f to open the interactive filter builder."))
		blank()
		if len(w.filters) == 0 && len(w.rawVideoFilters) == 0 && len(w.rawAudioFilters) == 0 {
			add(st.Faint.Render("No filters selected. The source is encoded as-is."))
		}
		for _, f := range w.filters {
			add(st.Accent.Render("• "+f.Def.Name) + " " + st.Muted.Render(widgets.Truncate(f.Describe(), width-16)))
		}
		if len(w.rawVideoFilters) > 0 {
			add(st.Label("Video chain", widgets.Truncate(strings.Join(w.rawVideoFilters, ","), width-16)))
		}
		if len(w.rawAudioFilters) > 0 {
			add(st.Label("Audio chain", widgets.Truncate(strings.Join(w.rawAudioFilters, ","), width-16)))
		}
		if len(w.filters) > 0 {
			blank()
			graph := filters.BuildGraph(w.filters)
			if graph.Complex != "" {
				add(st.Label("filter_complex", widgets.Truncate(graph.Complex, width-20)))
				add(st.Label("Map", graph.MapOut))
			} else if graph.VF != "" {
				add(st.Label("-vf", widgets.Truncate(graph.VF, width-20)))
			}
			if graph.AF != "" {
				add(st.Label("-af", widgets.Truncate(graph.AF, width-20)))
			}
		}
		blank()
		add(st.Faint.Render("d removes the last filter · n continues"))

	case stepExtraInput:
		add(st.Muted.Render("This recipe needs a second file."))
		blank()
		add(st.Accent.Render(extraInputPlaceholder(w)))
		value := w.extraInput
		if value == "" {
			value = st.Faint.Render("press e to choose a file")
		}
		add(st.Label("File", widgets.TruncateMiddle(value, width-12)))

	case stepConfirm:
		cmd, err := m.buildWizardCommand()
		if err != nil {
			add(st.Err.Render("Cannot build the command: " + err.Error()))
			blank()
			add(st.Faint.Render("Esc goes back and lets you fix it."))
			break
		}
		status := st.OK.Render("✓ command ready")
		if verr := cmd.Validate(); verr != nil {
			status = st.Err.Render("✖ " + verr.Error())
		}
		add(status)
		blank()
		previewRows := rows - 8
		if previewRows < 4 {
			previewRows = 4
		}
		lines := strings.Split(cmd.Preview(), "\n")
		if len(lines) > previewRows {
			skip := 0
			if len(lines)-previewRows > 2 {
				skip = len(lines) - previewRows + 1
			}
			lines = append([]string{"…"}, lines[skip:]...)
		}
		for _, l := range lines {
			b = append(b, st.CodeBlock.Render(widgets.Truncate(l, width)))
		}
		if warns := cmd.Warnings(); len(warns) > 0 {
			blank()
			for _, wn := range warns {
				add(st.Warn.Render("⚠ " + widgets.Truncate(wn, width-3)))
			}
		}
		blank()
		add(st.Faint.Render("Enter queues and starts this job · e edits the output path · Esc goes back"))

	default:
		add(st.Muted.Render("Continue with Enter."))
	}

	if w.err != "" {
		blank()
		b = append(b, st.ErrorBox(w.err, width))
	}
	if len(b) > rows {
		b = b[:rows]
	}
	return strings.Join(b, "\n")
}

func (m *Model) optionList(width, rows int) []string {
	st := m.styles
	w := &m.wiz
	opts, current, ok := w.stepOptions()
	if !ok || len(opts) == 0 {
		return []string{st.Faint.Render("No options for this step.")}
	}
	if rows < 3 {
		rows = 3
	}
	start := 0
	if current >= rows {
		start = current - rows + 1
	}
	var out []string
	for i := start; i < len(opts) && i < start+rows; i++ {
		o := opts[i]
		marker := "  "
		if i == current {
			marker = "● "
		}
		line := marker + o.label
		if o.hint != "" {
			line += " " + st.Faint.Render("— "+o.hint)
		}
		line = widgets.Truncate(line, width)
		out = append(out, st.SelectedRow(line, width, i == current))
	}
	return out
}

func (m *Model) wizardSidebar(width, rows int) string {
	st := m.styles
	w := &m.wiz
	var b []string
	src := m.wizSource()
	b = append(b, st.Rule("Source", width))
	if src != nil {
		b = append(b, st.Text.Render(widgets.Truncate(filepath.Base(w.input), width)))
		b = append(b, st.Muted.Render(widgets.Truncate(src.Format.FormatName, width)))
		b = append(b, m.probeSummaryLines(src)...)
	} else if w.input != "" {
		b = append(b, st.Text.Render(widgets.Truncate(filepath.Base(w.input), width)))
		if m.probeLoading {
			b = append(b, st.Muted.Render("probing…"))
		} else {
			b = append(b, st.Faint.Render("metadata unavailable"))
		}
	} else {
		b = append(b, st.Faint.Render("no input selected"))
	}
	b = append(b, "")
	b = append(b, st.Rule("Recipe", width))
	b = append(b, st.Label("Container", w.container().Name))
	if !w.container().AudioOnly {
		b = append(b, st.Label("Video", w.videoOptionLabel()))
		if codec, crf, speed := w.selectedVideoArgs(); codec != "copy" {
			if crf != "" {
				b = append(b, st.Label("Quality", "CRF "+crf))
			}
			if speed != "" {
				b = append(b, st.Label("Speed", speed))
			}
		}
	}
	b = append(b, st.Label("Audio", w.audioOptionLabel()))
	if codec, br := w.selectedAudioArgs(); br != "" && codec != "copy" {
		b = append(b, st.Label("Audio rate", br))
	}
	if w.subtitleMode() != "none" {
		b = append(b, st.Label("Subtitles", w.subtitleMode()))
	}
	if n := len(w.filters) + len(w.rawVideoFilters); n > 0 {
		b = append(b, st.Label("Filters", fmt.Sprintf("%d", n)))
	}
	if n := len(w.meta.Pairs()); n > 0 {
		b = append(b, st.Label("Tags", fmt.Sprintf("%d", n)))
	}
	if w.basePreset != nil {
		b = append(b, st.Label("Preset", w.basePreset.Name))
	}
	if w.tool != nil {
		b = append(b, st.Label("Tool", w.tool.Name))
	}
	if len(b) > rows {
		b = b[:rows]
	}
	return strings.Join(b, "\n")
}

func (m *Model) wizSource() *ffprobe.Result {
	if m.wiz.probe != nil {
		return m.wiz.probe
	}
	if m.currentProbe() != nil && filepath.Clean(m.probePath) == filepath.Clean(m.wiz.input) {
		return m.probe
	}
	return nil
}

func channelsLabel(idx int) string {
	switch idx {
	case 1:
		return "mono (1)"
	case 2:
		return "stereo (2)"
	case 3:
		return "5.1 (6)"
	}
	return "same as source"
}

func sampleRateLabel(idx int) string {
	switch idx {
	case 1:
		return "44100 Hz"
	case 2:
		return "48000 Hz"
	case 3:
		return "96000 Hz"
	}
	return "same as source"
}

func probeSubtitles(src *ffprobe.Result) string {
	if src == nil {
		return ""
	}
	var parts []string
	for _, s := range src.SubtitleStreams() {
		parts = append(parts, s.Language()+" "+s.CodecName)
	}
	return strings.Join(parts, ", ")
}

// presetNameForContainer returns the recommended preset for an extension.
func presetNameForContainer(ext string) string {
	switch ext {
	case "mp4":
		return "H264"
	case "mkv":
		return "H.265"
	case "webm":
		return "VP9"
	case "gif":
		return "GIF"
	case "mp3", "m4a", "flac", "wav", "ogg":
		return "Podcast"
	}
	return "Custom"
}
