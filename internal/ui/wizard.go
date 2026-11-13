package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/AMC-official/ffmpeg-tui/internal/config"
	"github.com/AMC-official/ffmpeg-tui/internal/ffmpeg"
	"github.com/AMC-official/ffmpeg-tui/internal/ffprobe"
	"github.com/AMC-official/ffmpeg-tui/internal/filesystem"
	"github.com/AMC-official/ffmpeg-tui/internal/filters"
	"github.com/AMC-official/ffmpeg-tui/internal/metadata"
	"github.com/AMC-official/ffmpeg-tui/internal/presets"
)

// wizStep enumerates the wizard's pages.
type wizStep int

const (
	stepInput wizStep = iota
	stepOutput
	stepContainer
	stepVideo
	stepQuality
	stepSpeed
	stepAudio
	stepAudioQuality
	stepSubtitle
	stepMetadata
	stepFilters
	stepExtraInput
	stepConfirm
	wizStepCount
)

var wizStepLabels = []string{
	"Input", "Output", "Container", "Video", "Quality", "Speed", "Audio",
	"Audio quality", "Subtitles", "Metadata", "Filters", "Extra input", "Confirm",
}

// wizardState holds the conversion wizard's configuration.
type wizardState struct {
	step int

	input      string
	output     string
	outputAuto bool
	extraInput string

	containerIdx    int
	videoIdx        int
	qualityIdx      int
	speedIdx        int
	audioIdx        int
	audioQualityIdx int
	channelsIdx     int
	sampleRateIdx   int
	subModeIdx      int
	subtitleFile    string

	meta       metadata.Values
	metaCursor int
	filters    []filters.Selected

	rawVideoFilters []string
	rawAudioFilters []string
	extraArgs       []string

	probe    *ffprobe.Result
	duration time.Duration

	entries     []filesystem.Entry
	entryCursor int
	entryOffset int
	browseDir   string

	tool       *presets.Tool
	basePreset *presets.Preset
	err        string
}

// newWizardState returns an empty wizard.
func newWizardState() wizardState {
	return wizardState{meta: metadata.New(nil), outputAuto: true}
}

// reset prepares the wizard for a new conversion.
func (w *wizardState) reset(cfg *config.Config, input string) {
	*w = newWizardState()
	w.input = input
	w.containerIdx = 0
	if input != "" {
		w.buildOutput(input, nil)
		w.loadEntries(filepath.Dir(input), cfg.ShowHidden)
		w.selectEntryByPath(input)
	} else {
		dir, _ := os.Getwd()
		w.loadEntries(dir, cfg.ShowHidden)
	}
	w.syncChoiceDefaults()
}

func (w *wizardState) loadEntries(dir string, showHidden bool) {
	entries, err := filesystem.List(dir, showHidden)
	if err != nil {
		return
	}
	w.browseDir = dir
	w.entries = entries
	w.entryCursor = 0
	w.entryOffset = 0
}

// selectEntryByPath moves the picker cursor onto path when it is listed.
func (w *wizardState) selectEntryByPath(path string) {
	if path == "" {
		return
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	for i, e := range w.entries {
		ep, err := filepath.Abs(e.Path)
		if err != nil {
			ep = e.Path
		}
		if ep == abs {
			w.entryCursor = i
			return
		}
	}
}

func (w *wizardState) ensureInput(dir string) {
	if w.browseDir == "" || len(w.entries) == 0 {
		w.loadEntries(dir, false)
	}
}

// applyPreset configures the wizard from a preset.
func (w *wizardState) applyPreset(p presets.Preset) {
	w.basePreset = &p
	if idx := containerIndex(p.Container); idx >= 0 {
		w.containerIdx = idx
	}
	w.syncChoiceDefaults()
	if p.VideoCodec != "" {
		if idx := indexOfOption(videoChoices(presets.Containers[w.containerIdx]), p.VideoCodec); idx >= 0 {
			w.videoIdx = idx
		}
	}
	if p.AudioCodec != "" {
		opts := audioChoices(presets.Containers[w.containerIdx])
		for i, o := range opts {
			if o.value == p.AudioCodec {
				w.audioIdx = i
			}
		}
	}
	if p.CRF != "" {
		for i, q := range qualityChoices(p.VideoCodec) {
			if q == p.CRF {
				w.qualityIdx = i
			}
		}
	}
	if p.EncoderPreset != "" {
		for i, s := range speedChoices(p.VideoCodec) {
			if s == p.EncoderPreset {
				w.speedIdx = i
			}
		}
	}
	if p.AudioBitrate != "" {
		for i, q := range audioQualityChoices(p.AudioCodec) {
			if q == p.AudioBitrate {
				w.audioQualityIdx = i
			}
		}
	}
	if p.Scale != "" {
		w.rawVideoFilters = append(w.rawVideoFilters, "scale="+p.Scale)
	}
	w.rawVideoFilters = append(w.rawVideoFilters, p.VideoFilters...)
	w.rawAudioFilters = append(w.rawAudioFilters, p.AudioFilters...)
	if p.Container != "" && w.outputAuto {
		w.buildOutput(w.input, w.probe)
	}
}

// applyTool configures the wizard from a tool recipe.
func (w *wizardState) applyTool(t presets.Tool) {
	w.tool = &t
	if idx := containerIndex(t.Container); idx >= 0 {
		w.containerIdx = idx
	}
	if p, ok := presets.ByName(t.Preset, nil); ok {
		w.applyPreset(p)
	}
	if idx := containerIndex(t.Container); idx >= 0 {
		w.containerIdx = idx
	}
	w.syncChoiceDefaults()
	w.rawVideoFilters = append(w.rawVideoFilters, t.VideoFilters...)
	w.rawAudioFilters = append(w.rawAudioFilters, t.AudioFilters...)
	w.extraArgs = append(w.extraArgs, t.ExtraArgs...)
	if t.NoVideo {
		w.videoIdx = 0
	}
	w.buildOutput(w.input, w.probe)
}

// containerIndex maps an extension to the container menu.
func containerIndex(ext string) int {
	ext = strings.TrimPrefix(strings.ToLower(ext), ".")
	for i, c := range presets.Containers {
		if c.Ext == ext {
			return i
		}
	}
	return -1
}

func (w *wizardState) container() presets.Container {
	idx := clampInt(w.containerIdx, 0, len(presets.Containers)-1)
	return presets.Containers[idx]
}

// videoChoices returns the selectable video options for a container.
func videoChoices(c presets.Container) []wizOption {
	if c.AudioOnly {
		return nil
	}
	var opts []wizOption
	opts = append(opts, wizOption{label: "Copy (no re-encode)", value: "copy"})
	add := func(codec, label string) {
		opts = append(opts, wizOption{label: label, value: codec})
	}
	switch c.Ext {
	case "webm":
		add("libvpx-vp9", "VP9")
		add("libsvtav1", "AV1 (SVT)")
		add("libvpx", "VP8")
	case "gif":
		add("gif", "GIF")
	case "png":
		add("png", "PNG")
	case "jpg":
		add("mjpeg", "JPEG (MJPEG)")
	case "webp":
		add("libwebp", "WebP")
	case "avif":
		add("libsvtav1", "AV1 still")
	default:
		add("libx264", "H.264 (x264)")
		add("libx265", "H.265 (x265)")
		add("libsvtav1", "AV1 (SVT-AV1)")
		add("prores", "ProRes")
		add("ffv1", "FFV1 lossless")
		add("mjpeg", "MJPEG")
	}
	opts = append(opts, wizOption{label: "None (video only file)", value: "none"})
	return opts
}

func indexOfOption(opts []wizOption, value string) int {
	for i, o := range opts {
		if o.value == value {
			return i
		}
	}
	return -1
}

// audioChoices returns selectable audio codecs for a container.
func audioChoices(c presets.Container) []wizOption {
	var opts []wizOption
	opts = append(opts, wizOption{label: "Copy (no re-encode)", value: "copy"})
	switch c.Ext {
	case "webm":
		opts = append(opts, wizOption{label: "Opus", value: "libopus"}, wizOption{label: "Vorbis", value: "libvorbis"})
	case "wav":
		opts = append(opts, wizOption{label: "PCM 16-bit", value: "pcm_s16le"}, wizOption{label: "PCM 24-bit", value: "pcm_s24le"})
	case "flac":
		opts = append(opts, wizOption{label: "FLAC", value: "flac"})
	case "ogg":
		opts = append(opts, wizOption{label: "Vorbis", value: "libvorbis"}, wizOption{label: "Opus", value: "libopus"})
	case "mp3":
		opts = append(opts, wizOption{label: "MP3 (LAME)", value: "libmp3lame"})
	case "m4a":
		opts = append(opts, wizOption{label: "AAC", value: "aac"}, wizOption{label: "ALAC (lossless)", value: "alac"})
	default:
		opts = append(opts,
			wizOption{label: "AAC", value: "aac"},
			wizOption{label: "MP3", value: "libmp3lame"},
			wizOption{label: "Opus", value: "libopus"},
			wizOption{label: "FLAC (lossless)", value: "flac"},
			wizOption{label: "AC-3", value: "ac3"},
		)
	}
	opts = append(opts, wizOption{label: "None (remove audio)", value: "none"})
	return opts
}

// qualityChoices returns CRF ladders for the codec.
func qualityChoices(codec string) []string {
	switch codec {
	case "libx264", "libx265":
		return []string{"18", "20", "23", "26", "28", "32"}
	case "libsvtav1":
		return []string{"24", "28", "32", "36", "40"}
	case "libvpx-vp9":
		return []string{"24", "28", "32", "36", "40"}
	}
	return nil
}

// speedChoices returns encoder presets for the codec.
func speedChoices(codec string) []string {
	switch codec {
	case "libx264", "libx265":
		return []string{"ultrafast", "veryfast", "fast", "medium", "slow", "veryslow"}
	case "libsvtav1":
		return []string{"6", "7", "8", "9", "10", "12"}
	case "libvpx-vp9":
		return []string{"realtime", "good", "best"}
	}
	return nil
}

// audioQualityChoices returns bitrates for an audio codec.
func audioQualityChoices(codec string) []string {
	switch codec {
	case "aac", "libmp3lame", "libopus", "libvorbis", "ac3":
		return []string{"96k", "128k", "160k", "192k", "256k", "320k"}
	}
	return nil
}

func (w *wizardState) syncChoiceDefaults() {
	c := w.container()
	if !c.AudioOnly && w.audioIdx == 0 && c.AudioArg != "" {
		// Default to the container's native audio codec rather than stream
		// copy: users almost always expect a self-contained playable file.
		for i, o := range audioChoices(c) {
			if o.value == c.AudioArg {
				w.audioIdx = i
				break
			}
		}
	}
	if c.AudioOnly {
		w.videoIdx = 0
		// Make sure an audio encoder is selected, not copy.
		opts := audioChoices(c)
		if len(opts) > 1 && w.audioIdx == 0 {
			w.audioIdx = 1
		}
	} else if len(videoChoices(c)) > 1 && w.videoIdx == 0 {
		w.videoIdx = 1
	}
	w.qualityIdx = clampInt(w.qualityIdx, 0, max(0, len(qualityChoices(w.videoCodecNow()))-1))
}

// videoCodecNow resolves the selected video codec value.
func (w *wizardState) videoCodecNow() string {
	c := w.container()
	if c.AudioOnly {
		return "none"
	}
	opts := videoChoices(c)
	if len(opts) == 0 {
		return "none"
	}
	idx := clampInt(w.videoIdx, 0, len(opts)-1)
	return opts[idx].value
}

func (w *wizardState) audioCodecNow() string {
	opts := audioChoices(w.container())
	if len(opts) == 0 {
		return "none"
	}
	idx := clampInt(w.audioIdx, 0, len(opts)-1)
	return opts[idx].value
}

func (w *wizardState) videoOptionLabel() string {
	c := w.container()
	if c.AudioOnly {
		return "none (audio only)"
	}
	opts := videoChoices(c)
	if len(opts) == 0 {
		return "none"
	}
	idx := clampInt(w.videoIdx, 0, len(opts)-1)
	return opts[idx].label
}

func (w *wizardState) audioOptionLabel() string {
	opts := audioChoices(w.container())
	idx := clampInt(w.audioIdx, 0, len(opts)-1)
	return opts[idx].label
}

func (w *wizardState) subtitleMode() string {
	switch w.subModeIdx {
	case 1:
		return "copy"
	case 2:
		return "burn"
	}
	return "none"
}

// buildOutput derives an output path from the input and container.
func (w *wizardState) buildOutput(input string, p *ffprobe.Result) {
	ext := w.container().Ext
	if ext == "" {
		ext = "mkv"
	}
	if input == "" {
		w.output = ""
		return
	}
	w.output = filesystem.DefaultOutput(input, ext)
	w.outputAuto = true
}

// needsExtraInput reports whether the current selection references a second file.
func (w *wizardState) needsExtraInput() bool {
	if w.tool != nil && w.tool.NeedsInput != "" {
		return true
	}
	for _, f := range w.filters {
		if f.Def.NeedsInput {
			return true
		}
	}
	return false
}

// stepNeeded reports whether a step should be shown in the current flow.
func (w *wizardState) stepNeeded(step int) bool {
	switch wizStep(step) {
	case stepExtraInput:
		return w.needsExtraInput()
	case stepQuality:
		return len(qualityChoices(w.videoCodecNow())) > 0
	case stepSpeed:
		return len(speedChoices(w.videoCodecNow())) > 0
	case stepAudioQuality:
		return len(audioQualityChoices(w.audioCodecNow())) > 0
	case stepVideo:
		return !w.container().AudioOnly
	}
	return true
}

// next advances to the next needed step.
func (w *wizardState) next() {
	for i := w.step + 1; i < int(wizStepCount); i++ {
		if w.stepNeeded(i) {
			w.step = i
			return
		}
	}
	w.step = int(stepConfirm)
}

// prev goes back to the previous needed step.
func (w *wizardState) prev() {
	for i := w.step - 1; i >= 0; i-- {
		if w.stepNeeded(i) {
			w.step = i
			return
		}
	}
	w.step = int(stepInput)
}

// wizOption is one selectable value.
type wizOption struct {
	label string
	value string
	hint  string
}

// stepOptions returns the options for option-based steps.
func (w *wizardState) stepOptions() ([]wizOption, int, bool) {
	switch wizStep(w.step) {
	case stepContainer:
		opts := make([]wizOption, 0, len(presets.Containers))
		for _, c := range presets.Containers {
			opts = append(opts, wizOption{label: c.Name + " (." + c.Ext + ")", value: c.Ext, hint: c.Note})
		}
		return opts, w.containerIdx, true
	case stepVideo:
		opts := videoChoices(w.container())
		return opts, w.videoIdx, len(opts) > 0
	case stepQuality:
		var opts []wizOption
		for i, q := range qualityChoices(w.videoCodecNow()) {
			opts = append(opts, wizOption{label: "CRF " + q, value: q})
			_ = i
		}
		return opts, w.qualityIdx, len(opts) > 0
	case stepSpeed:
		var opts []wizOption
		for _, s := range speedChoices(w.videoCodecNow()) {
			opts = append(opts, wizOption{label: s, value: s})
		}
		return opts, w.speedIdx, len(opts) > 0
	case stepAudio:
		opts := audioChoices(w.container())
		return opts, w.audioIdx, true
	case stepAudioQuality:
		var opts []wizOption
		for _, q := range audioQualityChoices(w.audioCodecNow()) {
			opts = append(opts, wizOption{label: q, value: q})
		}
		return opts, w.audioQualityIdx, len(opts) > 0
	case stepSubtitle:
		opts := []wizOption{
			{label: "None — do not copy or burn subtitles", value: "none"},
			{label: "Copy all subtitle streams", value: "copy"},
			{label: "Burn an external subtitle file into the video", value: "burn"},
		}
		return opts, w.subModeIdx, true
	}
	return nil, 0, false
}

// setOption applies a selection index for option-based steps.
func (w *wizardState) setOption(idx int) {
	switch wizStep(w.step) {
	case stepContainer:
		w.containerIdx = idx
		w.videoIdx = 0
		w.audioIdx = 0
		w.qualityIdx = 0
		w.speedIdx = 0
		w.syncChoiceDefaults()
		if w.outputAuto {
			w.buildOutput(w.input, w.probe)
		}
	case stepVideo:
		w.videoIdx = idx
		w.qualityIdx = 0
		w.speedIdx = 0
	case stepQuality:
		w.qualityIdx = idx
	case stepSpeed:
		w.speedIdx = idx
	case stepAudio:
		w.audioIdx = idx
		w.audioQualityIdx = 0
	case stepAudioQuality:
		w.audioQualityIdx = idx
	case stepSubtitle:
		w.subModeIdx = idx
	}
}

// selectEntry handles Enter on the input picker.
func (w *wizardState) selectEntry() (done bool, dir string) {
	if w.entryCursor < 0 || w.entryCursor >= len(w.entries) {
		return false, ""
	}
	e := w.entries[w.entryCursor]
	if e.IsDir {
		w.loadEntries(e.Path, false)
		return false, e.Path
	}
	w.input = e.Path
	w.probe = nil
	w.duration = 0
	w.err = ""
	if w.outputAuto {
		w.buildOutput(e.Path, nil)
	}
	return true, ""
}

// selectedVideoArgs returns the codec, CRF and preset choice.
func (w *wizardState) selectedVideoArgs() (codec, crf, speed string) {
	codec = w.videoCodecNow()
	if codec == "copy" || codec == "none" {
		return codec, "", ""
	}
	if q := qualityChoices(codec); len(q) > 0 {
		crf = q[clampInt(w.qualityIdx, 0, len(q)-1)]
	}
	if s := speedChoices(codec); len(s) > 0 {
		speed = s[clampInt(w.speedIdx, 0, len(s)-1)]
	}
	return codec, crf, speed
}

func (w *wizardState) selectedAudioArgs() (codec, bitrate string) {
	codec = w.audioCodecNow()
	if q := audioQualityChoices(codec); len(q) > 0 {
		bitrate = q[clampInt(w.audioQualityIdx, 0, len(q)-1)]
	}
	return codec, bitrate
}

// buildWizardCommand assembles the final FFmpeg command from the wizard.
func (m *Model) buildWizardCommand() (ffmpeg.Command, error) {
	w := &m.wiz
	if strings.TrimSpace(w.input) == "" {
		return ffmpeg.Command{}, fmt.Errorf("choose an input file first")
	}
	in := filesystem.ExpandHome(w.input)
	if !strings.Contains(in, "://") {
		if abs, err := filepath.Abs(in); err == nil {
			in = abs
		}
	}
	out := filesystem.ExpandHome(w.output)
	if out == "" {
		out = filesystem.DefaultOutput(in, w.container().Ext)
	}
	if filepath.Ext(out) == "" {
		out += "." + w.container().Ext
	}

	c := w.container()
	if w.tool != nil && w.tool.Build != nil {
		extra := w.extraInput
		cmd, err := w.tool.Build(in, filesystem.ExpandHome(extra), out, w.duration)
		if err != nil {
			return ffmpeg.Command{}, err
		}
		cmd.Bin = m.ffmpegPath
		cmd.Duration = w.duration
		cmd.Label = w.tool.Name
		for _, kv := range w.meta.Pairs() {
			cmd.Args = append(cmd.Args, "-metadata", kv.Key+"="+kv.Value)
		}
		cmd.Args = append(cmd.Args, w.extraArgs...)
		return cmd, nil
	}

	b := ffmpeg.NewBuilder(m.ffmpegPath, in).WithDuration(w.duration)
	if w.basePreset != nil {
		w.basePreset.Apply(b)
	}

	videoCodec, crf, speed := w.selectedVideoArgs()
	if c.AudioOnly || videoCodec == "none" {
		b.NoVideo()
	} else {
		b.VideoCodec(videoCodec)
		if crf != "" {
			b.CRF(crf)
		}
		if speed != "" {
			if videoCodec == "libvpx-vp9" {
				b.Extra("-deadline", speed)
			} else {
				b.EncoderPreset(speed)
			}
		}
		if videoCodec == "libsvtav1" || videoCodec == "libvpx-vp9" {
			b.VideoBitrate("0")
		}
		if c.Ext == "mp4" || c.Ext == "mov" {
			b.PixelFormat("yuv420p")
		}
	}

	audioCodec, audioBitrate := w.selectedAudioArgs()
	if audioCodec == "none" {
		b.NoAudio()
	} else {
		b.AudioCodec(audioCodec)
		if audioBitrate != "" {
			b.AudioBitrate(audioBitrate)
		}
	}
	if w.channelsIdx > 0 {
		b.Channels([]int{0, 1, 2, 6}[w.channelsIdx])
	}
	if w.sampleRateIdx > 0 {
		b.SampleRate([]int{0, 44100, 48000, 96000}[w.sampleRateIdx])
	}

	switch w.subtitleMode() {
	case "copy":
		if c.Subtitle != "" {
			b.SubtitleCodec("copy")
			b.Map("0:s?")
		}
	case "burn":
		if w.subtitleFile == "" {
			return ffmpeg.Command{}, fmt.Errorf("choose a subtitle file to burn, or select another subtitle mode")
		}
		b.BurnSubtitles(filesystem.ExpandHome(w.subtitleFile))
		b.VideoCodec(firstNonEmpty(videoCodec, "libx264"))
	}

	for _, f := range w.rawVideoFilters {
		b.AddVideoFilter(f)
	}
	for _, f := range w.rawAudioFilters {
		b.AddAudioFilter(f)
	}

	graph := filters.BuildGraph(w.filters)
	if graph.Complex != "" {
		b.SetFilterComplex(graph.Complex, graph.MapOut)
		for _, extra := range graph.ExtraInputs {
			b.AddExtraInput(extra)
		}
		if graph.VF != "" {
			b.AddVideoFilter(graph.VF)
		}
	} else if graph.VF != "" {
		b.AddVideoFilter(graph.VF)
	}
	if graph.AF != "" {
		b.AddAudioFilter(graph.AF)
	}
	if w.extraInput != "" && w.tool != nil && w.tool.NeedsInput != "" {
		b.AddExtraInput(filesystem.ExpandHome(w.extraInput))
	}
	b.SetFilterSummary(graph.Summary)

	b.MetadataPairs(w.meta.Pairs())
	if c.Faststart {
		b.Faststart(true)
	}
	if len(w.extraArgs) > 0 {
		b.Extra(w.extraArgs...)
	}
	cmd := b.Build(out)
	cmd.Label = w.label()
	return cmd, nil
}

func (w *wizardState) label() string {
	parts := []string{w.container().Name}
	if !w.container().AudioOnly {
		if v := w.videoCodecNow(); v != "" && v != "none" {
			parts = append(parts, v)
		}
	}
	if a := w.audioCodecNow(); a != "" && a != "none" {
		parts = append(parts, a)
	}
	return strings.Join(parts, " • ")
}

// enqueueWizard validates and queues the wizard's command.
func (m *Model) enqueueWizard() tea.Cmd {
	cmd, err := m.buildWizardCommand()
	if err != nil {
		m.wiz.err = err.Error()
		return nil
	}
	if err := cmd.Validate(); err != nil {
		m.wiz.err = err.Error()
		return nil
	}
	if !m.ffmpegOK {
		m.wiz.err = "ffmpeg was not found on PATH; install it or set ffmpeg_path in the config"
		return nil
	}
	m.wiz.err = ""
	name := filepath.Base(cmd.Output)
	m.jobs.Enqueue(name, cmd)
	m.queueSnapshot()
	m.stack = append(m.stack, m.screen)
	m.screen = screenQueue
	m.queueCursor = max(0, len(m.queueSnap.Jobs)-1)
	return m.flash("queued " + name)
}

// selectContainerByExt sets the container if it exists in the menu.
func (w *wizardState) selectContainerByExt(ext string) bool {
	idx := containerIndex(ext)
	if idx < 0 {
		return false
	}
	w.containerIdx = idx
	w.syncChoiceDefaults()
	return true
}

// startToolWizard opens the wizard preconfigured for a tool.
func (m *Model) startToolWizard(t presets.Tool) tea.Cmd {
	path := ""
	if e, ok := m.selectedEntry(); ok && !e.IsDir {
		path = e.Path
	}
	if path == "" {
		path = m.compare.paths[0]
	}
	m.wiz.reset(m.cfg, path)
	m.wiz.entries = m.entries
	if path != "" && m.probePath == path {
		m.wiz.probe = m.probe
		m.wiz.duration = m.probe.Duration()
	}
	m.wiz.applyTool(t)
	m.wiz.buildOutput(path, m.wiz.probe)
	m.stack = append(m.stack, m.screen)
	m.screen = screenWizard
	if path != "" && m.ffprobeOK {
		return probeCmd(m.ctx, m.ffprobePath, path)
	}
	return nil
}

// wizOptionCount returns the number of choices for the current option step.
func (w *wizardState) optionCount() int {
	opts, _, ok := w.stepOptions()
	if !ok {
		return 0
	}
	return len(opts)
}

// stepTitle returns the heading for the current step.
func (w *wizardState) stepTitle() string {
	if int(w.step) < len(wizStepLabels) {
		return wizStepLabels[w.step]
	}
	return "Wizard"
}

// parseSeconds parses a duration string for trim controls.
func parseSeconds(v string) (float64, error) { return strconv.ParseFloat(v, 64) }
