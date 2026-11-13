// Package ffmpeg models FFmpeg invocations safely and transparently.
//
// A Command is a pure description of an FFmpeg argv. It is never run through a
// shell: every argument is passed verbatim to exec.Command, so filenames with
// spaces, quotes, globs or "$(rm -rf /)" are inert data.
package ffmpeg

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// KV is an ordered metadata pair. Order is preserved so that generated
// command lines are stable and reproducible.
type KV struct {
	Key   string
	Value string
}

// Command is a fully described FFmpeg invocation.
type Command struct {
	Bin         string        // ffmpeg binary (name or absolute path)
	Args        []string      // arguments after the binary, excluding output path
	Input       string        // primary input, for display and validation
	ExtraInputs []string      // additional inputs (watermarks, overlays, ...)
	Output      string        // output path (last argv element)
	Duration    time.Duration // expected input duration, used for progress
	Label       string        // short human description, e.g. "MP4 • H.264"
	FilterGraph string        // readable filter/filter_complex summary
}

// Argv returns the complete argv including the binary.
func (c Command) Argv() []string {
	argv := make([]string, 0, len(c.Args)+2)
	argv = append(argv, c.Bin)
	argv = append(argv, c.Args...)
	argv = append(argv, c.Output)
	return argv
}

// ArgsNoBin returns just the arguments (no binary, no output).
func (c Command) ArgsNoBin() []string {
	out := make([]string, len(c.Args))
	copy(out, c.Args)
	return out
}

// OneLine renders the command as a single shell-like line. This is for display
// only; it is never executed.
func (c Command) OneLine() string {
	parts := make([]string, 0, len(c.Args)+2)
	parts = append(parts, Quote(filepath.Base(c.Bin)))
	for i := 0; i < len(c.Args); i++ {
		a := c.Args[i]
		if !strings.HasPrefix(a, "-") && i > 0 {
			parts = append(parts, Quote(a))
			continue
		}
		if strings.HasPrefix(a, "-") && i+1 < len(c.Args) && !strings.HasPrefix(c.Args[i+1], "-") {
			// Keep flag and value visually paired, except for -i which reads
			// better on its own line in Preview.
			parts = append(parts, a, Quote(c.Args[i+1]))
			i++
			continue
		}
		parts = append(parts, a)
	}
	parts = append(parts, Quote(c.Output))
	return strings.Join(parts, " ")
}

// Preview renders a multi-line, copy-pasteable command using backslash
// continuations, exactly as the user would type it.
func (c Command) Preview() string {
	var lines []string
	bin := filepath.Base(c.Bin)
	lines = append(lines, bin+" \\")
	args := c.Args
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
			lines = append(lines, "  "+a+" "+Quote(args[i+1])+" \\")
			i++
			continue
		}
		lines = append(lines, "  "+a+" \\")
	}
	lines = append(lines, "  "+Quote(c.Output))
	return strings.Join(lines, "\n")
}

// PreviewWrapped packs the quoted argv greedily into lines of at most width
// characters. It is used for the always-visible command preview, where the full
// argument list matters more than one-argument-per-line readability.
func (c Command) PreviewWrapped(width int) []string {
	if width < 20 {
		width = 20
	}
	tokens := make([]string, 0, len(c.Args)+2)
	tokens = append(tokens, Quote(filepath.Base(c.Bin)))
	for i := 0; i < len(c.Args); i++ {
		a := c.Args[i]
		if strings.HasPrefix(a, "-") && i+1 < len(c.Args) && !strings.HasPrefix(c.Args[i+1], "-") {
			tokens = append(tokens, a+" "+Quote(c.Args[i+1]))
			i++
			continue
		}
		tokens = append(tokens, a)
	}
	tokens = append(tokens, Quote(c.Output))
	var lines []string
	line := ""
	for _, t := range tokens {
		switch {
		case line == "":
			line = t
		case len([]rune(line))+1+len([]rune(t)) <= width:
			line += " " + t
		default:
			lines = append(lines, line)
			line = "  " + t
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}

// Validate performs the safety and sanity checks that must pass before a job is
// allowed into the queue. It never touches the shell and rejects NUL bytes,
// directory inputs, missing outputs and identical input/output paths.
func (c Command) Validate() error {
	if strings.TrimSpace(c.Bin) == "" {
		return errors.New("ffmpeg binary is not configured")
	}
	if strings.TrimSpace(c.Input) == "" {
		return errors.New("no input file selected")
	}
	if strings.TrimSpace(c.Output) == "" {
		return errors.New("no output file selected")
	}
	for _, a := range append(c.ArgsNoBin(), c.Output) {
		if strings.ContainsRune(a, 0) {
			return errors.New("argument contains a NUL byte")
		}
	}
	if !isRemote(c.Input) {
		st, err := os.Stat(c.Input)
		if err != nil {
			return fmt.Errorf("input unavailable: %w", err)
		}
		if st.IsDir() {
			return fmt.Errorf("input %s is a directory", filepath.Base(c.Input))
		}
		if st.Size() == 0 {
			return fmt.Errorf("input %s is empty", filepath.Base(c.Input))
		}
	}
	outDir := filepath.Dir(c.Output)
	if st, err := os.Stat(outDir); err != nil {
		return fmt.Errorf("output directory %s does not exist", outDir)
	} else if !st.IsDir() {
		return fmt.Errorf("output parent %s is not a directory", outDir)
	}
	if st, err := os.Stat(c.Output); err == nil && st.IsDir() {
		return fmt.Errorf("output %s is a directory", filepath.Base(c.Output))
	}
	if samePath(c.Input, c.Output) {
		return errors.New("input and output are the same file")
	}
	return nil
}

// Warnings returns non-fatal observations about the command.
func (c Command) Warnings() []string {
	var out []string
	if c.Output != "" && filepath.Ext(c.Output) == "" {
		out = append(out, "output has no extension; ffmpeg may not infer a container")
	}
	if c.Duration == 0 {
		out = append(out, "input duration unknown; progress will be time-only")
	}
	return out
}

func samePath(a, b string) bool {
	aa, err1 := filepath.Abs(a)
	bb, err2 := filepath.Abs(b)
	if err1 != nil || err2 != nil {
		return a == b
	}
	if aa == bb {
		return true
	}
	// Case-insensitive filesystems.
	if os.PathSeparator == '\\' {
		return strings.EqualFold(aa, bb)
	}
	return false
}

func isRemote(p string) bool { return strings.Contains(p, "://") }

// Quote renders an argument for display. Arguments are never re-parsed, so this
// only has to be unambiguous for a human reader.
func Quote(s string) string {
	if s == "" {
		return `""`
	}
	safe := true
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' ||
			strings.ContainsRune("._-/@%+=:,", r)) {
			safe = false
			break
		}
	}
	if safe {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// escapeFilterPath escapes a filename for use inside a filtergraph value
// (subtitles=..., movie=...). The filtergraph grammar uses : , ' [ ] ; and \ as
// metacharacters, and paths are not passed through a shell.
func escapeFilterPath(p string) string {
	var b strings.Builder
	for _, r := range p {
		switch r {
		case '\\', ':', '\'', ',', '[', ']', ';', '=':
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// EscapeFilterValue escapes a raw value used in a filter option.
func EscapeFilterValue(v string) string {
	var b strings.Builder
	for _, r := range v {
		switch r {
		case '\\', ':', '\'', ',', '[', ']', ';':
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// Builder assembles a Command with a deterministic argument order.
type Builder struct {
	bin string

	input       string
	extraInputs []string
	seekStart   string
	trimLength  string

	videoCodec string
	audioCodec string
	subCodec   string
	noVideo    bool
	noAudio    bool

	crf         string
	encoderFast string
	tune        string
	profile     string
	pixFmt      string
	videoRate   string
	maxRate     string
	bufSize     string
	fps         string

	audioRate   string
	channels    int
	sampleRate  int
	audioFilter []string

	videoFilters   []string
	filterComplex  string
	mapVOut        string
	maps           []string
	metadata       []KV
	chapters       []KV
	extra          []string
	format         string
	faststart      bool
	shortest       bool
	overwrite      bool
	duration       time.Duration
	label          string
	filterSummary  string
	removeMetadata bool
}

// NewBuilder starts a command for the given input. bin may be empty, in which
// case "ffmpeg" is used.
func NewBuilder(bin, input string) *Builder {
	if strings.TrimSpace(bin) == "" {
		bin = "ffmpeg"
	}
	return &Builder{bin: bin, input: input, overwrite: true}
}

// Input returns the builder's primary input.
func (b *Builder) Input() string { return b.input }

// WithDuration records the expected duration for progress estimation.
func (b *Builder) WithDuration(d time.Duration) *Builder { b.duration = d; return b }

// WithLabel sets a short human readable description.
func (b *Builder) WithLabel(l string) *Builder { b.label = l; return b }

// Overwrite controls the -y flag. Existing outputs are overwritten when true.
func (b *Builder) Overwrite(on bool) *Builder { b.overwrite = on; return b }

// SeekStart adds -ss before the input (fast seek).
func (b *Builder) SeekStart(ts string) *Builder {
	if ts != "" {
		b.seekStart = ts
	}
	return b
}

// Trim limits the output duration with -t.
func (b *Builder) Trim(d string) *Builder {
	if d != "" {
		b.trimLength = d
	}
	return b
}

// AddExtraInput adds an additional -i input.
func (b *Builder) AddExtraInput(path string) *Builder {
	if path != "" {
		b.extraInputs = append(b.extraInputs, path)
	}
	return b
}

// VideoCodec sets -c:v. "copy" passes video through untouched.
func (b *Builder) VideoCodec(c string) *Builder { b.videoCodec = c; return b }

// AudioCodec sets -c:a.
func (b *Builder) AudioCodec(c string) *Builder { b.audioCodec = c; return b }

// SubtitleCodec sets -c:s.
func (b *Builder) SubtitleCodec(c string) *Builder { b.subCodec = c; return b }

// NoVideo disables video (-vn).
func (b *Builder) NoVideo() *Builder { b.noVideo = true; return b }

// NoAudio disables audio (-an).
func (b *Builder) NoAudio() *Builder { b.noAudio = true; return b }

// CRF sets -crf (quality based encoding).
func (b *Builder) CRF(v string) *Builder { b.crf = v; return b }

// EncoderPreset sets -preset (speed/efficiency tradeoff).
func (b *Builder) EncoderPreset(v string) *Builder { b.encoderFast = v; return b }

// Tune sets -tune.
func (b *Builder) Tune(v string) *Builder { b.tune = v; return b }

// Profile sets -profile:v.
func (b *Builder) Profile(v string) *Builder { b.profile = v; return b }

// PixelFormat sets -pix_fmt.
func (b *Builder) PixelFormat(v string) *Builder { b.pixFmt = v; return b }

// VideoBitrate sets -b:v.
func (b *Builder) VideoBitrate(v string) *Builder { b.videoRate = v; return b }

// RateControl sets -maxrate and -bufsize.
func (b *Builder) RateControl(max, buf string) *Builder {
	b.maxRate, b.bufSize = max, buf
	return b
}

// FPS sets -r.
func (b *Builder) FPS(v string) *Builder { b.fps = v; return b }

// AudioBitrate sets -b:a.
func (b *Builder) AudioBitrate(v string) *Builder { b.audioRate = v; return b }

// Channels sets -ac.
func (b *Builder) Channels(n int) *Builder { b.channels = n; return b }

// SampleRate sets -ar.
func (b *Builder) SampleRate(n int) *Builder { b.sampleRate = n; return b }

// AddVideoFilter appends a comma separated video filter to -vf.
func (b *Builder) AddVideoFilter(f string) *Builder {
	if f = strings.TrimSpace(f); f != "" {
		b.videoFilters = append(b.videoFilters, f)
	}
	return b
}

// AddAudioFilter appends an audio filter to -af.
func (b *Builder) AddAudioFilter(f string) *Builder {
	if f = strings.TrimSpace(f); f != "" {
		b.audioFilter = append(b.audioFilter, f)
	}
	return b
}

// SetFilterComplex installs a full -filter_complex graph and the video output
// label to map. It is used by overlay/watermark/stack filters.
func (b *Builder) SetFilterComplex(graph, mapOut string) *Builder {
	b.filterComplex = graph
	b.mapVOut = mapOut
	return b
}

// Map adds a -map stream specifier.
func (b *Builder) Map(spec string) *Builder {
	if spec != "" {
		b.maps = append(b.maps, spec)
	}
	return b
}

// BurnSubtitles renders a subtitle file into the video stream.
func (b *Builder) BurnSubtitles(path string) *Builder {
	if path == "" {
		return b
	}
	return b.AddVideoFilter("subtitles=" + escapeFilterPath(path))
}

// Metadata appends -metadata key=value.
func (b *Builder) Metadata(key, value string) *Builder {
	if key == "" {
		return b
	}
	b.metadata = append(b.metadata, KV{Key: strings.ToLower(key), Value: value})
	return b
}

// MetadataPairs appends several metadata pairs.
func (b *Builder) MetadataPairs(pairs []KV) *Builder {
	for _, p := range pairs {
		b.Metadata(p.Key, p.Value)
	}
	return b
}

// ChapterMetadata appends a chapter metadata entry.
func (b *Builder) ChapterMetadata(key, value string) *Builder {
	if key != "" {
		b.chapters = append(b.chapters, KV{Key: key, Value: value})
	}
	return b
}

// RemoveMetadata adds -map_metadata -1 (strip container tags).
func (b *Builder) RemoveMetadata() *Builder { b.removeMetadata = true; return b }

// Format forces the output muxer (-f).
func (b *Builder) Format(f string) *Builder { b.format = f; return b }

// Faststart moves the MP4 index to the front (-movflags +faststart).
func (b *Builder) Faststart(on bool) *Builder { b.faststart = on; return b }

// Shortest stops encoding at the shortest stream (-shortest).
func (b *Builder) Shortest(on bool) *Builder { b.shortest = on; return b }

// Extra appends raw arguments immediately before the output. They are still
// passed as discrete argv entries and never interpreted by a shell.
func (b *Builder) Extra(args ...string) *Builder {
	b.extra = append(b.extra, args...)
	return b
}

// SetFilterSummary records the user-facing filter description.
func (b *Builder) SetFilterSummary(s string) *Builder { b.filterSummary = s; return b }

// VideoFilters returns the configured filter chain.
func (b *Builder) VideoFilters() []string { return append([]string(nil), b.videoFilters...) }

// AudioFilters returns the configured audio filter chain.
func (b *Builder) AudioFilters() []string { return append([]string(nil), b.audioFilter...) }

// Build produces the final immutable Command for output path out.
func (b *Builder) Build(out string) Command {
	if b.videoCodec == "copy" && len(b.videoFilters) > 0 {
		// A copied stream cannot be filtered; encoding is required.
		b.videoCodec = "libx264"
	}
	if b.videoCodec == "copy" && b.filterComplex != "" {
		b.videoCodec = "libx264"
	}
	if b.audioCodec == "copy" && (len(b.audioFilter) > 0 || b.channels > 0 || b.sampleRate > 0) {
		b.audioCodec = "aac"
	}
	if b.noAudio {
		b.audioCodec = ""
	}
	if b.noVideo {
		b.videoCodec = ""
	}

	args := []string{"-hide_banner", "-nostdin"}
	if b.overwrite {
		args = append(args, "-y")
	}
	if b.seekStart != "" {
		args = append(args, "-ss", b.seekStart)
	}
	args = append(args, "-i", b.input)
	for _, e := range b.extraInputs {
		args = append(args, "-i", e)
	}

	vf := strings.Join(b.videoFilters, ",")
	af := strings.Join(b.audioFilter, ",")
	if b.filterComplex != "" {
		args = append(args, "-filter_complex", b.filterComplex)
	} else if vf != "" {
		args = append(args, "-vf", vf)
	}
	if af != "" {
		args = append(args, "-af", af)
	}

	for _, m := range b.maps {
		args = append(args, "-map", m)
	}
	if b.mapVOut != "" {
		args = append(args, "-map", b.mapVOut)
	}
	if b.noVideo {
		args = append(args, "-vn")
	}
	if b.noAudio {
		args = append(args, "-an")
	}
	if b.videoCodec != "" {
		args = append(args, "-c:v", b.videoCodec)
	}
	if b.crf != "" {
		args = append(args, "-crf", b.crf)
	}
	if b.encoderFast != "" {
		args = append(args, "-preset", b.encoderFast)
	}
	if b.tune != "" {
		args = append(args, "-tune", b.tune)
	}
	if b.profile != "" {
		args = append(args, "-profile:v", b.profile)
	}
	if b.pixFmt != "" {
		args = append(args, "-pix_fmt", b.pixFmt)
	}
	if b.videoRate != "" {
		args = append(args, "-b:v", b.videoRate)
	}
	if b.maxRate != "" {
		args = append(args, "-maxrate", b.maxRate)
	}
	if b.bufSize != "" {
		args = append(args, "-bufsize", b.bufSize)
	}
	if b.fps != "" {
		args = append(args, "-r", b.fps)
	}
	if b.audioCodec != "" {
		args = append(args, "-c:a", b.audioCodec)
	}
	if b.audioRate != "" {
		args = append(args, "-b:a", b.audioRate)
	}
	if b.channels > 0 {
		args = append(args, "-ac", strconv.Itoa(b.channels))
	}
	if b.sampleRate > 0 {
		args = append(args, "-ar", strconv.Itoa(b.sampleRate))
	}
	if b.subCodec != "" {
		args = append(args, "-c:s", b.subCodec)
	}
	if b.removeMetadata {
		args = append(args, "-map_metadata", "-1")
	}
	for _, m := range b.metadata {
		args = append(args, "-metadata", m.Key+"="+m.Value)
	}
	for _, c := range b.chapters {
		args = append(args, "-metadata", c.Key+"="+c.Value)
	}
	if b.faststart {
		args = append(args, "-movflags", "+faststart")
	}
	if b.shortest {
		args = append(args, "-shortest")
	}
	if b.trimLength != "" {
		args = append(args, "-t", b.trimLength)
	}
	if b.format != "" {
		args = append(args, "-f", b.format)
	}
	args = append(args, b.extra...)

	summary := b.filterSummary
	if summary == "" {
		var parts []string
		if vf != "" {
			parts = append(parts, vf)
		}
		if b.filterComplex != "" {
			parts = append(parts, b.filterComplex)
		}
		summary = strings.Join(parts, " + ")
	}

	return Command{
		Bin:         b.bin,
		Args:        args,
		Input:       b.input,
		ExtraInputs: append([]string(nil), b.extraInputs...),
		Output:      out,
		Duration:    b.duration,
		Label:       b.label,
		FilterGraph: summary,
	}
}

// AudioCodecChoices are the codecs audio jobs are allowed to choose from.
var AudioCodecChoices = []string{"copy", "aac", "libmp3lame", "libopus", "libvorbis", "flac", "pcm_s16le", "pcm_s24le", "none"}

// VideoCodecChoices are the codecs video jobs are allowed to choose from.
var VideoCodecChoices = []string{"copy", "libx264", "libx265", "libsvtav1", "libvpx-vp9", "prores", "mjpeg", "gif", "png", "none"}
