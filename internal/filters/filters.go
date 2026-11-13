// Package filters models the FFmpeg filter system as structured, validated
// recipes. Every definition can render its own filtergraph fragment, its
// documentation and examples, which keeps the UI, the command preview and the
// man pages consistent with each other.
package filters

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/AMC-official/ffmpeg-tui/internal/ffmpeg"
)

// ArgKind describes how an argument is edited and validated.
type ArgKind int

const (
	ArgNumber ArgKind = iota
	ArgText
	ArgChoice
	ArgBool
	ArgFile
)

// Arg is one option of a filter.
type Arg struct {
	Key     string
	Label   string
	Help    string
	Default string
	Kind    ArgKind
	Choices []string
	Min     float64
	Max     float64
	// Optional args with an empty value are omitted from the graph.
	Optional bool
}

// Definition describes a filter the builder exposes.
type Definition struct {
	Name          string
	Category      string // Geometry, Color, Time, Overlay, Audio
	Summary       string
	Args          []Arg
	Template      string // video chain fragment, %key% placeholders
	AudioTemplate string // optional audio chain fragment (speed, fade, trim)
	ComplexTpl    string // full filter_complex fragment; %IN% %OUT% %IDX% placeholders
	NeedsInput    bool   // requires an extra input file
	InputLabel    string // label used in the UI for that file
	Note          string
	Examples      []string
	ManPage       string
	Tags          []string
}

// Selected is a Definition with concrete values.
type Selected struct {
	Def        Definition
	Values     map[string]string
	ExtraInput string
}

// NewSelected builds a selection with the definition's defaults.
func NewSelected(def Definition) Selected {
	s := Selected{Def: def, Values: map[string]string{}}
	for _, a := range def.Args {
		s.Values[a.Key] = a.Default
	}
	return s
}

// Value returns the current value of an argument.
func (s Selected) Value(key string) string { return s.Values[key] }

// Set updates an argument value, validating numeric bounds.
func (s Selected) Set(key, value string) error {
	for _, a := range s.Def.Args {
		if a.Key != key {
			continue
		}
		switch a.Kind {
		case ArgNumber:
			if value == "" && a.Optional {
				s.Values[key] = ""
				return nil
			}
			f, err := strconv.ParseFloat(value, 64)
			if err != nil {
				return fmt.Errorf("%s must be a number", a.Label)
			}
			if a.Min != 0 && f < a.Min {
				return fmt.Errorf("%s must be at least %g", a.Label, a.Min)
			}
			if a.Max != 0 && f > a.Max {
				return fmt.Errorf("%s must be at most %g", a.Label, a.Max)
			}
		case ArgChoice:
			ok := false
			for _, c := range a.Choices {
				if c == value {
					ok = true
				}
			}
			if !ok {
				return fmt.Errorf("%s must be one of %s", a.Label, strings.Join(a.Choices, ", "))
			}
		}
		s.Values[key] = value
		return nil
	}
	return fmt.Errorf("unknown argument %q", key)
}

// render substitutes %key% placeholders.
func (s Selected) render(tpl string) string {
	out := tpl
	for _, a := range s.Def.Args {
		val := s.Values[a.Key]
		if a.Kind == ArgNumber && val == "" {
			val = a.Default
		}
		if a.Kind == ArgFile {
			val = ffmpeg.EscapeFilterValue(val)
		}
		out = strings.ReplaceAll(out, "%"+a.Key+"%", val)
	}
	return out
}

// Render returns the fragment for the -vf/-af chain.
func (s Selected) Render() string { return s.render(s.Def.Template) }

// RenderAudio returns the audio fragment, if any.
func (s Selected) RenderAudio() string {
	if s.Def.AudioTemplate == "" {
		return ""
	}
	return s.render(s.Def.AudioTemplate)
}

// Describe renders a one-line human description of the active settings.
func (s Selected) Describe() string {
	parts := []string{}
	for _, a := range s.Def.Args {
		v := s.Values[a.Key]
		if v == "" || v == a.Default {
			continue
		}
		parts = append(parts, a.Label+": "+v)
	}
	if len(parts) == 0 {
		return s.Def.Summary
	}
	return strings.Join(parts, ", ")
}

// Graph is the result of combining a filter chain with geometry filters.
type Graph struct {
	VF          string
	AF          string
	Complex     string
	MapOut      string
	ExtraInputs []string
	Summary     string
}

func bracket(label string) string {
	if label == "" {
		return ""
	}
	return "[" + label + "]"
}

// BuildGraph combines selected filters into a -vf/-af chain or a
// -filter_complex graph. Simple video filters are flushed into labelled
// segments whenever a complex filter needs a stable input pad.
func BuildGraph(chain []Selected) Graph {
	var g Graph
	var vfParts, afParts, segments []string
	cur := "0:v"
	labelN := 0
	inputN := 0
	complex := false

	flush := func() {
		if len(vfParts) == 0 {
			return
		}
		labelN++
		next := fmt.Sprintf("v%d", labelN)
		segments = append(segments, fmt.Sprintf("%s%s%s", bracket(cur), strings.Join(vfParts, ","), bracket(next)))
		cur = next
		vfParts = nil
	}

	for _, sel := range chain {
		def := sel.Def
		if def.ComplexTpl != "" {
			complex = true
			flush()
			labelN++
			out := fmt.Sprintf("v%d", labelN)
			idx := 1
			if def.NeedsInput {
				inputN++
				idx = inputN
				if sel.ExtraInput != "" {
					g.ExtraInputs = append(g.ExtraInputs, sel.ExtraInput)
				}
			}
			seg := def.ComplexTpl
			seg = strings.ReplaceAll(seg, "%IN%", bracket(cur))
			seg = strings.ReplaceAll(seg, "%OUT%", out)
			seg = strings.ReplaceAll(seg, "%IDX%", strconv.Itoa(idx))
			seg = sel.render(seg)
			segments = append(segments, seg)
			cur = out
			continue
		}
		if frag := sel.Render(); frag != "" {
			vfParts = append(vfParts, frag)
		}
		if frag := sel.RenderAudio(); frag != "" {
			afParts = append(afParts, frag)
		}
	}

	if !complex {
		g.VF = strings.Join(vfParts, ",")
		g.AF = strings.Join(afParts, ",")
		g.Summary = summarize(chain)
		return g
	}

	flush()
	g.Complex = strings.Join(segments, ";")
	g.MapOut = bracket(cur)
	g.AF = strings.Join(afParts, ",")
	g.Summary = summarize(chain)
	return g
}

func summarize(chain []Selected) string {
	names := make([]string, 0, len(chain))
	for _, s := range chain {
		names = append(names, s.Def.Name)
	}
	return strings.Join(names, " → ")
}

// ByName finds a filter definition by name.
func ByName(name string) (Definition, bool) {
	for _, d := range All() {
		if strings.EqualFold(d.Name, name) {
			return d, true
		}
	}
	return Definition{}, false
}

// Categories returns the filter categories in display order.
func Categories() []string { return []string{"Geometry", "Color", "Time", "Overlay", "Audio"} }

// All returns every filter definition.
func All() []Definition { return definitions }

var definitions = []Definition{
	{
		Name: "crop", Category: "Geometry", ManPage: "crop",
		Summary: "Cut a rectangular region out of the frame.",
		Args: []Arg{
			{Key: "w", Label: "Width", Default: "1280", Kind: ArgNumber, Min: 1},
			{Key: "h", Label: "Height", Default: "720", Kind: ArgNumber, Min: 1},
			{Key: "x", Label: "X offset", Default: "(iw-ow)/2", Kind: ArgText},
			{Key: "y", Label: "Y offset", Default: "(ih-oh)/2", Kind: ArgText},
		},
		Template: "crop=%w%:%h%:%x%:%y%",
		Examples: []string{"-vf crop=1280:720:0:0", "-vf crop=iw/2:ih/2"},
		Tags:     []string{"cut", "region", "frame"},
	},
	{
		Name: "scale", Category: "Geometry", ManPage: "scale",
		Summary: "Resize the picture, preserving aspect ratio when using -2.",
		Args: []Arg{
			{Key: "w", Label: "Width", Default: "1920", Kind: ArgText},
			{Key: "h", Label: "Height", Default: "-2", Kind: ArgText},
			{Key: "flags", Label: "Algorithm", Default: "lanczos", Kind: ArgChoice,
				Choices: []string{"lanczos", "bicubic", "bilinear", "neighbor", "area"}},
		},
		Template: "scale=%w%:%h%:flags=%flags%",
		Examples: []string{"-vf scale=1280:-2", "-vf scale=1920:1080:flags=lanczos"},
		Tags:     []string{"resize", "resolution", "size", "downscale"},
	},
	{
		Name: "rotate", Category: "Geometry", ManPage: "rotate",
		Summary: "Rotate by an arbitrary angle in radians, filling empty space with black.",
		Args: []Arg{
			{Key: "angle", Label: "Angle (rad)", Default: "0.05", Kind: ArgNumber, Min: -6.28, Max: 6.28},
		},
		Template: "rotate=%angle%",
		Examples: []string{"-vf rotate=PI/4"},
		Tags:     []string{"turn", "angle"},
	},
	{
		Name: "hflip", Category: "Geometry", ManPage: "hflip",
		Summary:  "Mirror horizontally (left to right).",
		Template: "hflip",
		Examples: []string{"-vf hflip"},
		Tags:     []string{"flip", "mirror"},
	},
	{
		Name: "vflip", Category: "Geometry", ManPage: "vflip",
		Summary:  "Mirror vertically (top to bottom).",
		Template: "vflip",
		Examples: []string{"-vf vflip"},
		Tags:     []string{"flip", "mirror"},
	},
	{
		Name: "transpose", Category: "Geometry", ManPage: "transpose",
		Summary: "Rotate by 90 degree increments and optionally flip.",
		Args: []Arg{
			{Key: "dir", Label: "Direction", Default: "clock", Kind: ArgChoice,
				Choices: []string{"clock", "cclock", "clock_flip", "cclock_flip"}},
		},
		Template: "transpose=%dir%",
		Examples: []string{"-vf transpose=clock"},
		Tags:     []string{"rotate", "portrait"},
	},
	{
		Name: "fps", Category: "Time", ManPage: "fps",
		Summary: "Force a constant frame rate, duplicating or dropping frames.",
		Args: []Arg{
			{Key: "fps", Label: "Frames/sec", Default: "30", Kind: ArgNumber, Min: 1, Max: 240},
		},
		Template: "fps=%fps%",
		Examples: []string{"-vf fps=24", "-vf fps=60"},
		Tags:     []string{"framerate", "smooth", "gif"},
	},
	{
		Name: "pad", Category: "Geometry", ManPage: "pad",
		Summary: "Add a border around the frame, centring the picture by default.",
		Args: []Arg{
			{Key: "w", Label: "Width", Default: "1920", Kind: ArgText},
			{Key: "h", Label: "Height", Default: "1080", Kind: ArgText},
			{Key: "x", Label: "X", Default: "(ow-iw)/2", Kind: ArgText},
			{Key: "y", Label: "Y", Default: "(oh-ih)/2", Kind: ArgText},
			{Key: "color", Label: "Colour", Default: "black", Kind: ArgText},
		},
		Template: "pad=%w%:%h%:%x%:%y%:%color%",
		Examples: []string{"-vf pad=1920:1080:(ow-iw)/2:(oh-ih)/2:black"},
		Tags:     []string{"letterbox", "border", "pillarbox"},
	},
	{
		Name: "brightness", Category: "Color", ManPage: "eq",
		Summary: "Adjust brightness with the eq filter (-1 to 1).",
		Args: []Arg{
			{Key: "v", Label: "Brightness", Default: "0.05", Kind: ArgNumber, Min: -1, Max: 1},
		},
		Template: "eq=brightness=%v%",
		Examples: []string{"-vf eq=brightness=0.06"},
		Tags:     []string{"bright", "eq", "exposure"},
	},
	{
		Name: "contrast", Category: "Color", ManPage: "eq",
		Summary: "Adjust contrast with the eq filter (-1000 to 1000, 1 is neutral).",
		Args: []Arg{
			{Key: "v", Label: "Contrast", Default: "1.1", Kind: ArgNumber, Min: 0, Max: 4},
		},
		Template: "eq=contrast=%v%",
		Examples: []string{"-vf eq=contrast=1.2"},
		Tags:     []string{"eq", "levels"},
	},
	{
		Name: "gamma", Category: "Color", ManPage: "eq",
		Summary: "Adjust gamma with the eq filter (0.1 to 10, 1 is neutral).",
		Args: []Arg{
			{Key: "v", Label: "Gamma", Default: "1.1", Kind: ArgNumber, Min: 0.1, Max: 10},
		},
		Template: "eq=gamma=%v%",
		Examples: []string{"-vf eq=gamma=1.1"},
		Tags:     []string{"eq", "midtones"},
	},
	{
		Name: "saturation", Category: "Color", ManPage: "eq",
		Summary: "Adjust colour saturation (0 is greyscale, 1 is neutral).",
		Args: []Arg{
			{Key: "v", Label: "Saturation", Default: "1.2", Kind: ArgNumber, Min: 0, Max: 3},
		},
		Template: "eq=saturation=%v%",
		Examples: []string{"-vf eq=saturation=0"},
		Tags:     []string{"eq", "color", "vivid", "greyscale"},
	},
	{
		Name: "sharpen", Category: "Color", ManPage: "unsharp",
		Summary: "Sharpen with an unsharp mask.",
		Args: []Arg{
			{Key: "luma", Label: "Luma amount", Default: "1.0", Kind: ArgNumber, Min: 0, Max: 5},
			{Key: "size", Label: "Matrix size", Default: "5", Kind: ArgChoice, Choices: []string{"3", "5", "7", "9", "11"}},
		},
		Template: "unsharp=luma_msize_x=%size%:luma_msize_y=%size%:luma_amount=%luma%",
		Examples: []string{"-vf unsharp=5:5:1.0"},
		Tags:     []string{"unsharp", "detail", "crisp"},
	},
	{
		Name: "blur", Category: "Color", ManPage: "gblur",
		Summary: "Gaussian blur with a configurable radius.",
		Args: []Arg{
			{Key: "sigma", Label: "Sigma", Default: "2", Kind: ArgNumber, Min: 0, Max: 50},
		},
		Template: "gblur=sigma=%sigma%",
		Examples: []string{"-vf gblur=sigma=5"},
		Tags:     []string{"soft", "gaussian", "defocus"},
	},
	{
		Name: "noise", Category: "Color", ManPage: "noise",
		Summary: "Add film-like noise to the picture.",
		Args: []Arg{
			{Key: "strength", Label: "Strength", Default: "12", Kind: ArgNumber, Min: 0, Max: 100},
		},
		Template: "noise=alls=%strength%:allf=t+u",
		Examples: []string{"-vf noise=alls=20:allf=t+u"},
		Tags:     []string{"grain", "film"},
	},
	{
		Name: "subtitles", Category: "Overlay", ManPage: "subtitles",
		Summary: "Burn a subtitle file into the frames. Paths are escaped, never shelled.",
		Args: []Arg{
			{Key: "file", Label: "Subtitle file", Default: "", Kind: ArgFile},
			{Key: "force_style", Label: "ASS style overrides", Default: "", Optional: true, Kind: ArgText},
		},
		Template: "subtitles=%file%",
		Examples: []string{"-vf subtitles=movie.srt", "-vf \"subtitles=sub.srt:force_style='Fontsize=24'\""},
		Tags:     []string{"burn", "hardcode", "srt", "ass"},
	},
	{
		Name: "watermark", Category: "Overlay", ManPage: "overlay",
		Summary:    "Overlay a scaled image (typically PNG) in a corner.",
		NeedsInput: true, InputLabel: "Watermark image",
		Args: []Arg{
			{Key: "file", Label: "Image", Default: "", Kind: ArgFile},
			{Key: "width", Label: "Width", Default: "180", Kind: ArgNumber, Min: 8},
			{Key: "x", Label: "X", Default: "W-w-20", Kind: ArgText},
			{Key: "y", Label: "Y", Default: "H-h-20", Kind: ArgText},
		},
		ComplexTpl: "%IN%[%IDX%:v]scale=%width%:-1[wm%OUT%];[wm%OUT%]overlay=%x%:%y%[%OUT%]",
		Examples:   []string{"-i logo.png -filter_complex \"[1:v]scale=180:-1[wm];[0:v][wm]overlay=W-w-20:H-h-20\""},
		Tags:       []string{"logo", "overlay", "brand", "image"},
	},
	{
		Name: "overlay", Category: "Overlay", ManPage: "overlay",
		Summary:    "Place a second video or image over the main picture.",
		NeedsInput: true, InputLabel: "Overlay file",
		Args: []Arg{
			{Key: "file", Label: "File", Default: "", Kind: ArgFile},
			{Key: "x", Label: "X", Default: "0", Kind: ArgText},
			{Key: "y", Label: "Y", Default: "0", Kind: ArgText},
		},
		ComplexTpl: "%IN%[%IDX%:v]overlay=%x%:%y%[%OUT%]",
		Examples:   []string{"-i bg.mp4 -i overlay.mp4 -filter_complex \"[0:v][1:v]overlay=10:10\""},
		Tags:       []string{"picture in picture", "composite"},
	},
	{
		Name: "hstack", Category: "Overlay", ManPage: "hstack",
		Summary:    "Place two videos side by side horizontally.",
		NeedsInput: true, InputLabel: "Second video",
		ComplexTpl: "%IN%[%IDX%:v]hstack=inputs=2[%OUT%]",
		Examples:   []string{"-filter_complex \"[0:v][1:v]hstack=inputs=2\""},
		Tags:       []string{"stack", "side by side", "compare"},
	},
	{
		Name: "vstack", Category: "Overlay", ManPage: "vstack",
		Summary:    "Place two videos one above the other.",
		NeedsInput: true, InputLabel: "Second video",
		ComplexTpl: "%IN%[%IDX%:v]vstack=inputs=2[%OUT%]",
		Examples:   []string{"-filter_complex \"[0:v][1:v]vstack=inputs=2\""},
		Tags:       []string{"stack", "above", "compare"},
	},
	{
		Name: "split", Category: "Overlay", ManPage: "split",
		Summary:    "Duplicate the stream into two branches; the primary branch is encoded.",
		ComplexTpl: "%IN%split=2[%OUT%a][%OUT%b];[%OUT%a]null[%OUT%]",
		Note:       "A full split needs two outputs; FFmpeg-TUI keeps the primary branch and discards the duplicate.",
		Examples:   []string{"-filter_complex \"[0:v]split=2[a][b];[a]null[vout]\""},
		Tags:       []string{"duplicate", "branches", "graph"},
	},
	{
		Name: "trim", Category: "Time", ManPage: "trim",
		Summary: "Keep a time range and reset timestamps to zero.",
		Args: []Arg{
			{Key: "start", Label: "Start (s)", Default: "0", Kind: ArgNumber, Min: 0},
			{Key: "end", Label: "End (s)", Default: "30", Kind: ArgNumber, Min: 0},
		},
		Template:      "trim=start=%start%:end=%end%,setpts=PTS-STARTPTS",
		AudioTemplate: "atrim=start=%start%:end=%end%,asetpts=PTS-STARTPTS",
		Examples:      []string{"-vf \"trim=start=5:end=15,setpts=PTS-STARTPTS\""},
		Tags:          []string{"cut", "clip", "range"},
	},
	{
		Name: "fade in", Category: "Time", ManPage: "fade",
		Summary: "Fade from black (and silence) at the start.",
		Args: []Arg{
			{Key: "d", Label: "Duration (s)", Default: "1", Kind: ArgNumber, Min: 0.1, Max: 60},
		},
		Template:      "fade=t=in:st=0:d=%d%",
		AudioTemplate: "afade=t=in:st=0:d=%d%",
		Examples:      []string{"-vf fade=t=in:st=0:d=2", "-af afade=t=in:st=0:d=2"},
		Tags:          []string{"fade", "intro", "black"},
	},
	{
		Name: "fade out", Category: "Time", ManPage: "fade",
		Summary: "Fade to black (and silence) at the end of the clip.",
		Args: []Arg{
			{Key: "st", Label: "Start (s)", Default: "55", Kind: ArgNumber, Min: 0},
			{Key: "d", Label: "Duration (s)", Default: "5", Kind: ArgNumber, Min: 0.1, Max: 60},
		},
		Template:      "fade=t=out:st=%st%:d=%d%",
		AudioTemplate: "afade=t=out:st=%st%:d=%d%",
		Examples:      []string{"-vf \"fade=t=out:st=55:d=5\""},
		Tags:          []string{"fade", "outro", "ending"},
	},
	{
		Name: "speed", Category: "Time", ManPage: "setpts",
		Summary: "Change playback speed for video and audio together.",
		Args: []Arg{
			{Key: "speed", Label: "Speed", Default: "1.5", Kind: ArgNumber, Min: 0.25, Max: 4},
		},
		Template:      "setpts=PTS/%speed%",
		AudioTemplate: "atempo=%speed%",
		Note:          "atempo accepts 0.5-2.0 per instance; outside that range chain multiple atempo filters.",
		Examples:      []string{"-vf setpts=0.5*PTS -af atempo=2.0"},
		Tags:          []string{"fast", "slow", "slowmo", "timelapse"},
	},
	{
		Name: "normalize", Category: "Audio", ManPage: "loudnorm",
		Summary: "EBU R128 loudness normalisation (two-pass capable).",
		Args: []Arg{
			{Key: "I", Label: "Integrated (LUFS)", Default: "-16", Kind: ArgNumber, Min: -70, Max: -5},
			{Key: "TP", Label: "True peak (dBTP)", Default: "-1.5", Kind: ArgNumber, Min: -9, Max: 0},
			{Key: "LRA", Label: "Loudness range", Default: "11", Kind: ArgNumber, Min: 1, Max: 50},
		},
		AudioTemplate: "loudnorm=I=%I%:TP=%TP%:LRA=%LRA%",
		Note:          "Video passes through untouched; only the audio chain is normalised.",
		Examples:      []string{"-af loudnorm=I=-16:TP=-1.5:LRA=11"},
		Tags:          []string{"normalize", "normalization", "loudness", "lufs", "volume", "r128"},
	},
	{
		Name: "dynaudnorm", Category: "Audio", ManPage: "dynaudnorm",
		Summary: "Dynamic audio normalisation that evens out quiet and loud passages.",
		Args: []Arg{
			{Key: "framelen", Label: "Window (ms)", Default: "500", Kind: ArgNumber, Min: 10, Max: 8000},
			{Key: "g", Label: "Gaussian window", Default: "15", Kind: ArgNumber, Min: 1, Max: 301},
		},
		AudioTemplate: "dynaudnorm=f=%framelen%:g=%g%",
		Examples:      []string{"-af dynaudnorm=f=500:g=15"},
		Tags:          []string{"normalize", "normalization", "leveler", "podcast"},
	},
	{
		Name: "volume", Category: "Audio", ManPage: "volume",
		Summary: "Apply a gain in decibels.",
		Args: []Arg{
			{Key: "g", Label: "Gain (dB)", Default: "3", Kind: ArgNumber, Min: -60, Max: 30},
		},
		AudioTemplate: "volume=%g%dB",
		Examples:      []string{"-af volume=6dB"},
		Tags:          []string{"normalize", "gain", "loudness", "quieter"},
	},
	{
		Name: "atempo", Category: "Audio", ManPage: "atempo",
		Summary: "Change audio tempo without changing pitch.",
		Args: []Arg{
			{Key: "t", Label: "Tempo", Default: "1.5", Kind: ArgNumber, Min: 0.5, Max: 2},
		},
		AudioTemplate: "atempo=%t%",
		Examples:      []string{"-af atempo=1.25"},
		Tags:          []string{"speed", "tempo", "pitch"},
	},
	{
		Name: "silenceremove", Category: "Audio", ManPage: "silenceremove",
		Summary: "Trim silence from the beginning and end of the audio.",
		Args: []Arg{
			{Key: "threshold", Label: "Threshold (dB)", Default: "-50", Kind: ArgNumber, Min: -100, Max: 0},
		},
		AudioTemplate: "silenceremove=start_periods=1:start_threshold=%threshold%dB:stop_periods=-1:stop_threshold=%threshold%dB",
		Examples:      []string{"-af silenceremove=start_periods=1:start_threshold=-50dB"},
		Tags:          []string{"silence", "trim", "dead air"},
	},
}

// Summary renders a short list of every filter name, used by help screens.
func Names() []string {
	out := make([]string, 0, len(definitions))
	for _, d := range definitions {
		out = append(out, d.Name)
	}
	return out
}
