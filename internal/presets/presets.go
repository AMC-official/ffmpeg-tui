// Package presets contains the built-in encoding profiles and the tool
// catalogue that sits on top of them.
package presets

import (
	"strings"
	"time"

	"github.com/ffmpeg-tui/ffmpeg-tui/internal/ffmpeg"
)

// Preset is a named encoding recipe. Zero values mean "leave FFmpeg's default",
// so a Custom preset produces a minimal, honest command line.
type Preset struct {
	Name          string   `json:"name"`
	Category      string   `json:"category"`
	Description   string   `json:"description"`
	Container     string   `json:"container"` // extension without a dot
	VideoCodec    string   `json:"video_codec,omitempty"`
	AudioCodec    string   `json:"audio_codec,omitempty"`
	SubtitleCodec string   `json:"subtitle_codec,omitempty"`
	CRF           string   `json:"crf,omitempty"`
	EncoderPreset string   `json:"encoder_preset,omitempty"`
	Tune          string   `json:"tune,omitempty"`
	Profile       string   `json:"profile,omitempty"`
	PixelFormat   string   `json:"pixel_format,omitempty"`
	VideoBitrate  string   `json:"video_bitrate,omitempty"`
	MaxRate       string   `json:"max_rate,omitempty"`
	BufSize       string   `json:"buf_size,omitempty"`
	Scale         string   `json:"scale,omitempty"`
	FPS           string   `json:"fps,omitempty"`
	AudioBitrate  string   `json:"audio_bitrate,omitempty"`
	Channels      int      `json:"channels,omitempty"`
	SampleRate    int      `json:"sample_rate,omitempty"`
	VideoFilters  []string `json:"video_filters,omitempty"`
	AudioFilters  []string `json:"audio_filters,omitempty"`
	ExtraArgs     []string `json:"extra_args,omitempty"`
	Faststart     bool     `json:"faststart,omitempty"`
	AudioOnly     bool     `json:"audio_only,omitempty"`
	Builtin       bool     `json:"builtin,omitempty"`
}

// IsAudioOnly reports whether the preset produces an audio-only file.
func (p Preset) IsAudioOnly() bool {
	if p.AudioOnly {
		return true
	}
	switch p.VideoCodec {
	case "", "none", "copy":
		return p.VideoCodec == "none"
	}
	return false
}

// Summary returns a compact description used in lists.
func (p Preset) Summary() string {
	var bits []string
	if p.Container != "" {
		bits = append(bits, strings.ToUpper(p.Container))
	}
	if p.VideoCodec != "" && p.VideoCodec != "none" {
		bits = append(bits, codecLabel(p.VideoCodec))
	}
	if p.CRF != "" {
		bits = append(bits, "CRF "+p.CRF)
	} else if p.VideoBitrate != "" {
		bits = append(bits, p.VideoBitrate)
	}
	if p.AudioCodec != "" && p.AudioCodec != "none" {
		bits = append(bits, codecLabel(p.AudioCodec))
	}
	if len(bits) == 0 {
		return "manual configuration"
	}
	return strings.Join(bits, " • ")
}

func codecLabel(c string) string {
	switch c {
	case "libx264":
		return "H.264"
	case "libx265":
		return "H.265"
	case "libsvtav1", "libaom-av1":
		return "AV1"
	case "libvpx-vp9":
		return "VP9"
	case "libmp3lame":
		return "MP3"
	case "libopus":
		return "Opus"
	case "libvorbis":
		return "Vorbis"
	case "pcm_s16le":
		return "PCM 16"
	case "pcm_s24le":
		return "PCM 24"
	case "prores":
		return "ProRes"
	case "ffv1":
		return "FFV1"
	case "copy":
		return "copy"
	}
	return c
}

// Apply writes the preset's settings into a command builder. Filters from the
// preset come first so interactive filters can be appended afterwards.
func (p Preset) Apply(b *ffmpeg.Builder) *ffmpeg.Builder {
	if p.Scale != "" {
		b.AddVideoFilter("scale=" + p.Scale)
	}
	for _, f := range p.VideoFilters {
		b.AddVideoFilter(f)
	}
	for _, f := range p.AudioFilters {
		b.AddAudioFilter(f)
	}
	if p.AudioOnly || p.VideoCodec == "none" {
		b.NoVideo()
	} else if p.VideoCodec != "" {
		b.VideoCodec(p.VideoCodec)
	}
	if p.CRF != "" {
		b.CRF(p.CRF)
	}
	if p.EncoderPreset != "" {
		b.EncoderPreset(p.EncoderPreset)
	}
	if p.Tune != "" {
		b.Tune(p.Tune)
	}
	if p.Profile != "" {
		b.Profile(p.Profile)
	}
	if p.PixelFormat != "" {
		b.PixelFormat(p.PixelFormat)
	}
	if p.VideoBitrate != "" {
		b.VideoBitrate(p.VideoBitrate)
	}
	if p.MaxRate != "" || p.BufSize != "" {
		b.RateControl(p.MaxRate, p.BufSize)
	}
	if p.FPS != "" {
		b.FPS(p.FPS)
	}
	if p.AudioCodec == "none" {
		b.NoAudio()
	} else if p.AudioCodec != "" {
		b.AudioCodec(p.AudioCodec)
	}
	if p.AudioBitrate != "" {
		b.AudioBitrate(p.AudioBitrate)
	}
	if p.Channels > 0 {
		b.Channels(p.Channels)
	}
	if p.SampleRate > 0 {
		b.SampleRate(p.SampleRate)
	}
	if p.SubtitleCodec != "" {
		b.SubtitleCodec(p.SubtitleCodec)
	}
	if p.Faststart {
		b.Faststart(true)
	}
	if len(p.ExtraArgs) > 0 {
		b.Extra(p.ExtraArgs...)
	}
	b.WithLabel(p.Summary())
	return b
}

var builtins = []Preset{
	{
		Name: "Lossless", Category: "Archive", Container: "mkv", Builtin: true,
		Description: "Mathematically lossless video (x264 CRF 0) with FLAC audio. Large files, perfect quality.",
		VideoCodec:  "libx264", CRF: "0", EncoderPreset: "veryslow", AudioCodec: "flac",
	},
	{
		Name: "H264", Category: "Video", Container: "mp4", Builtin: true,
		Description: "The universal default: H.264 High profile, CRF 20, AAC 192k, faststart for streaming.",
		VideoCodec:  "libx264", CRF: "20", EncoderPreset: "medium", Profile: "high",
		PixelFormat: "yuv420p", AudioCodec: "aac", AudioBitrate: "192k", Faststart: true,
	},
	{
		Name: "H265", Category: "Video", Container: "mp4", Builtin: true,
		Description: "H.265/HEVC at roughly half the bitrate of H.264. Adds the hvc1 tag for Apple players.",
		VideoCodec:  "libx265", CRF: "24", EncoderPreset: "medium", PixelFormat: "yuv420p",
		AudioCodec: "aac", AudioBitrate: "192k", Faststart: true,
		ExtraArgs: []string{"-tag:v", "hvc1"},
	},
	{
		Name: "AV1", Category: "Video", Container: "mkv", Builtin: true,
		Description: "SVT-AV1, the fastest practical AV1 encoder. Excellent compression, slower than x264.",
		VideoCodec:  "libsvtav1", CRF: "32", EncoderPreset: "8", PixelFormat: "yuv420p",
		AudioCodec: "libopus", AudioBitrate: "128k",
	},
	{
		Name: "VP9", Category: "Web", Container: "webm", Builtin: true,
		Description: "VP9 in WebM with Opus audio. Royalty free and widely supported in browsers.",
		VideoCodec:  "libvpx-vp9", CRF: "32", VideoBitrate: "0", PixelFormat: "yuv420p",
		AudioCodec: "libopus", AudioBitrate: "128k", ExtraArgs: []string{"-row-mt", "1"},
	},
	{
		Name: "Discord", Category: "Web", Container: "mp4", Builtin: true,
		Description: "Fits the free upload limit: 720p, CRF 25, capped at 2 Mbps, AAC 128k.",
		VideoCodec:  "libx264", CRF: "25", EncoderPreset: "fast", Scale: "1280:-2",
		AudioCodec: "aac", AudioBitrate: "128k", MaxRate: "2500k", BufSize: "5000k",
		Faststart: true, PixelFormat: "yuv420p",
	},
	{
		Name: "YouTube", Category: "Publishing", Container: "mp4", Builtin: true,
		Description: "High quality upload: H.264 CRF 18, slow preset, AAC 320k, faststart.",
		VideoCodec:  "libx264", CRF: "18", EncoderPreset: "slow", Profile: "high",
		PixelFormat: "yuv420p", AudioCodec: "aac", AudioBitrate: "320k", Faststart: true,
	},
	{
		Name: "Twitch", Category: "Publishing", Container: "mp4", Builtin: true,
		Description: "Live-friendly 1080p60 CBR profile: 6 Mbps, x264, AAC 160k.",
		VideoCodec:  "libx264", EncoderPreset: "veryfast", Scale: "1920:-2", FPS: "60",
		VideoBitrate: "6000k", MaxRate: "6000k", BufSize: "12000k", AudioCodec: "aac",
		AudioBitrate: "160k", PixelFormat: "yuv420p", Faststart: true,
	},
	{
		Name: "Blu-ray", Category: "Publishing", Container: "mkv", Builtin: true,
		Description: "Disc-style master: H.264 CRF 16, AC-3 640k, high profile.",
		VideoCodec:  "libx264", CRF: "16", EncoderPreset: "slow", Profile: "high",
		PixelFormat: "yuv420p", AudioCodec: "ac3", AudioBitrate: "640k",
	},
	{
		Name: "Podcast", Category: "Audio", Container: "mp3", Builtin: true, AudioOnly: true,
		Description: "Speech-first MP3: 128k CBR, 44.1 kHz, loudness normalised to -16 LUFS.",
		AudioCodec:  "libmp3lame", AudioBitrate: "128k", SampleRate: 44100, Channels: 2,
		AudioFilters: []string{"loudnorm=I=-16:TP=-1.5:LRA=11"},
	},
	{
		Name: "Audiobook", Category: "Audio", Container: "m4a", Builtin: true, AudioOnly: true,
		Description: "Compact mono AAC at 64k for spoken word, with speech-friendly normalisation.",
		AudioCodec:  "aac", AudioBitrate: "64k", SampleRate: 44100, Channels: 1,
		AudioFilters: []string{"loudnorm=I=-19:TP=-2:LRA=9"},
	},
	{
		Name: "Phone", Category: "Device", Container: "mp4", Builtin: true,
		Description: "Small 720p file with veryfast x264 and AAC 96k, tuned for mobile playback.",
		VideoCodec:  "libx264", CRF: "26", EncoderPreset: "veryfast", Scale: "1280:-2",
		AudioCodec: "aac", AudioBitrate: "96k", PixelFormat: "yuv420p", Faststart: true,
	},
	{
		Name: "Archival", Category: "Archive", Container: "mkv", Builtin: true,
		Description: "FFV1 lossless video with FLAC audio in Matroska, the recommended preservation format.",
		VideoCodec:  "ffv1", PixelFormat: "yuv420p", AudioCodec: "flac",
		ExtraArgs: []string{"-level", "3", "-g", "1"},
	},
	{
		Name: "Animation", Category: "Video", Container: "mkv", Builtin: true,
		Description: "H.264 tuned for animation: higher CRF, animation tune, strong deblocking friendly settings.",
		VideoCodec:  "libx264", CRF: "22", EncoderPreset: "slow", Tune: "animation",
		PixelFormat: "yuv420p", AudioCodec: "aac", AudioBitrate: "192k",
	},
	{
		Name: "GIF", Category: "Animation", Container: "gif", Builtin: true, AudioOnly: false,
		Description: "Animated GIF at 15 fps, 480px wide, with Lanczos downscaling. No audio.",
		VideoCodec:  "gif", VideoFilters: []string{"fps=15", "scale=480:-1:flags=lanczos"},
		AudioCodec: "none",
	},
	{
		Name: "Custom", Category: "Custom", Container: "mkv", Builtin: true,
		Description: "Start from an empty recipe and choose every option yourself.",
		AudioCodec:  "copy", VideoCodec: "copy",
	},
}

// All returns every built-in preset followed by user presets.
func All(user []Preset) []Preset {
	out := make([]Preset, 0, len(builtins)+len(user))
	out = append(out, builtins...)
	out = append(out, user...)
	return out
}

// Builtins returns the built-in presets only.
func Builtins() []Preset { return append([]Preset(nil), builtins...) }

// ByName looks up a preset by case-insensitive name.
func ByName(name string, user []Preset) (Preset, bool) {
	for _, p := range All(user) {
		if strings.EqualFold(p.Name, name) {
			return p, true
		}
	}
	return Preset{}, false
}

// Containers is the list of output containers the wizard offers.
type Container struct {
	Ext       string
	Name      string
	VideoArg  string
	AudioArg  string
	Subtitle  string
	Faststart bool
	AudioOnly bool
	Note      string
}

// Containers lists supported output containers in menu order.
var Containers = []Container{
	{"mp4", "MP4", "libx264", "aac", "mov_text", true, false, "universal, streaming friendly"},
	{"mkv", "Matroska", "libx264", "aac", "copy", false, false, "holds anything, incl. FLAC/FFV1"},
	{"mov", "QuickTime", "libx264", "aac", "mov_text", true, false, "Apple ecosystem"},
	{"avi", "AVI", "mpeg4", "libmp3lame", "", false, false, "legacy compatibility"},
	{"webm", "WebM", "libvpx-vp9", "libopus", "webvtt", false, false, "web, royalty free"},
	{"mp3", "MP3", "", "libmp3lame", "", false, true, "audio only"},
	{"aac", "AAC (ADTS)", "", "aac", "", false, true, "raw AAC stream, audio only"},
	{"m4a", "M4A/AAC", "", "aac", "", false, true, "audio only, Apple friendly"},
	{"flac", "FLAC", "", "flac", "", false, true, "audio only, lossless"},
	{"ogg", "Ogg", "", "libvorbis", "", false, true, "audio only, open format"},
	{"wav", "WAV", "", "pcm_s16le", "", false, true, "audio only, uncompressed"},
	{"gif", "GIF", "gif", "none", "", false, false, "looping animation, no audio"},
	{"png", "PNG sequence", "png", "none", "", false, false, "image sequence"},
	{"jpg", "JPEG sequence", "mjpeg", "none", "", false, false, "image sequence"},
	{"webp", "WebP", "libwebp", "none", "", false, false, "modern still/animated image"},
	{"avif", "AVIF", "libsvtav1", "none", "", false, false, "next-gen still image"},
}

// ContainerByExt returns the container definition for an extension.
func ContainerByExt(ext string) (Container, bool) {
	ext = strings.TrimPrefix(strings.ToLower(ext), ".")
	for _, c := range Containers {
		if c.Ext == ext {
			return c, true
		}
	}
	return Container{}, false
}

// ContainerExts returns just the extensions, for completion lists.
func ContainerExts() []string {
	out := make([]string, 0, len(Containers))
	for _, c := range Containers {
		out = append(out, c.Ext)
	}
	return out
}

// Tool is a one-click recipe from the Audio/Video/Subtitle tool menus. Most
// tools are preset + filters; complex ones provide a Build function.
type Tool struct {
	Name         string
	Category     string // Audio, Video, Subtitle
	Description  string
	Preset       string
	Container    string
	Suffix       string
	VideoFilters []string
	AudioFilters []string
	ExtraArgs    []string
	NoVideo      bool
	NoAudio      bool
	NeedsInput   string // e.g. "subtitle file" when a second input is required
	Build        func(in, extra, out string, duration time.Duration) (ffmpeg.Command, error)
}

// Tools returns the built-in tool catalogue.
func Tools() []Tool {
	return []Tool{
		{
			Name: "Extract audio", Category: "Audio", Preset: "Custom", Container: "m4a", Suffix: "audio",
			Description: "Copy the audio stream into a standalone M4A and drop the video.",
			NoVideo:     true, NoAudio: false,
			ExtraArgs: []string{"-c:a", "copy"},
		},
		{
			Name: "Normalize loudness", Category: "Audio", Preset: "Podcast", Container: "mp3", Suffix: "normalized",
			Description: "Two-stage EBU R128 normalisation to -16 LUFS with a true peak ceiling of -1.5 dB.",
		},
		{
			Name: "Convert audio", Category: "Audio", Preset: "Custom", Container: "mp3", Suffix: "converted",
			Description: "Re-encode the audio track to MP3 192k, discarding video.",
			NoVideo:     true,
			ExtraArgs:   []string{"-c:a", "libmp3lame", "-b:a", "192k"},
		},
		{
			Name: "Merge (append)", Category: "Audio", Preset: "Custom", Container: "mka", Suffix: "merged",
			Description: "Append a second file with the concat filter (identical codec parameters recommended).",
			NeedsInput:  "second file",
			Build: func(in, extra, out string, d time.Duration) (ffmpeg.Command, error) {
				b := ffmpeg.NewBuilder("ffmpeg", in).AddExtraInput(extra).WithDuration(d)
				b.SetFilterComplex("[0:a][1:a]concat=n=2:v=0:a=1[aout]", "[aout]")
				b.AudioCodec("aac").AudioBitrate("192k").NoVideo()
				return b.Build(out), nil
			},
		},
		{
			Name: "Trim", Category: "Audio", Preset: "Custom", Container: "m4a", Suffix: "trim",
			Description: "Cut a section: re-encode audio with precise duration control (-ss/-t in the wizard).",
		},
		{
			Name: "Fade in/out", Category: "Audio", Preset: "Podcast", Container: "mp3", Suffix: "faded",
			Description:  "Apply a one second fade in and a two second fade out.",
			AudioFilters: []string{"afade=t=in:st=0:d=1", "afade=t=out:st=0:d=2"},
		},
		{
			Name: "Remove silence", Category: "Audio", Preset: "Podcast", Container: "mp3", Suffix: "silencecut",
			Description:  "Drop leading and trailing silence using silenceremove at -50 dB.",
			AudioFilters: []string{"silenceremove=start_periods=1:start_threshold=-50dB:stop_periods=-1:stop_threshold=-50dB"},
		},
		{
			Name: "Change volume", Category: "Audio", Preset: "Custom", Container: "mka", Suffix: "volume",
			Description:  "Apply gain with the volume filter; edit the dB value in the filter builder afterwards.",
			AudioFilters: []string{"volume=3dB"},
		},
		{
			Name: "Resample", Category: "Audio", Preset: "Custom", Container: "wav", Suffix: "48k",
			Description: "Convert to 48 kHz, 24-bit PCM stereo.",
			ExtraArgs:   []string{"-ar", "48000", "-ac", "2", "-c:a", "pcm_s24le"},
		},
		{
			Name: "Convert video", Category: "Video", Preset: "H264", Container: "mp4", Suffix: "converted",
			Description: "Standard H.264/AAC MP4 with faststart.",
		},
		{
			Name: "Resize to 1080p", Category: "Video", Preset: "H264", Container: "mp4", Suffix: "1080p",
			Description:  "Scale to 1920 wide (or 1080 tall for portrait) while preserving aspect ratio.",
			VideoFilters: []string{"scale=1920:-2:force_original_aspect_ratio=decrease"},
		},
		{
			Name: "Rotate 90°", Category: "Video", Preset: "H264", Container: "mp4", Suffix: "rotated",
			Description:  "Rotate clockwise and reset display metadata.",
			VideoFilters: []string{"transpose=1"},
		},
		{
			Name: "Speed up 2x", Category: "Video", Preset: "H264", Container: "mp4", Suffix: "2x",
			Description:  "Double playback speed for both video and audio.",
			VideoFilters: []string{"setpts=0.5*PTS"},
			AudioFilters: []string{"atempo=2.0"},
		},
		{
			Name: "Reverse", Category: "Video", Preset: "H264", Container: "mp4", Suffix: "reversed",
			Description:  "Play the clip backwards (buffers the whole stream in memory).",
			VideoFilters: []string{"reverse"},
			AudioFilters: []string{"areverse"},
		},
		{
			Name: "Frame extraction", Category: "Video", Preset: "Custom", Container: "png", Suffix: "frame",
			Description: "Export one PNG per second, numbered by ffmpeg.",
			ExtraArgs:   []string{"-vf", "fps=1", "-vsync", "vfr", "-start_number", "1"},
		},
		{
			Name: "Thumbnail sheet", Category: "Video", Preset: "Custom", Container: "jpg", Suffix: "sheet",
			Description:  "Contact sheet: one 4x4 tile image sampled every 10 seconds.",
			VideoFilters: []string{"fps=1/10", "scale=320:-1", "tile=4x4"},
			ExtraArgs:    []string{"-frames:v", "1"},
		},
		{
			Name: "GIF preview", Category: "Video", Preset: "GIF", Container: "gif", Suffix: "preview",
			Description: "Short 480px-wide looping GIF at 15 fps.",
		},
		{
			Name: "Stabilize (analyze)", Category: "Video", Preset: "Custom", Container: "mkv", Suffix: "stabilized",
			Description:  "Two-pass vidstab stabilization; the wizard queues the detect pass first, then encodes with the transform.",
			VideoFilters: []string{"vidstabdetect=shakiness=5:result=transforms.trf"},
			ExtraArgs:    []string{"-f", "null", "-"},
		},
		{
			Name: "Extract subtitles", Category: "Subtitle", Preset: "Custom", Container: "srt", Suffix: "subs",
			Description: "Copy the first subtitle stream into an SRT file.",
			ExtraArgs:   []string{"-map", "0:s:0", "-c:s", "srt"},
		},
		{
			Name: "Burn subtitles", Category: "Subtitle", Preset: "H264", Container: "mp4", Suffix: "burned",
			Description: "Hard-code a subtitle file into the video frames.",
		},
		{
			Name: "Convert subtitle format", Category: "Subtitle", Preset: "Custom", Container: "vtt", Suffix: "vtt",
			Description: "Convert the first subtitle stream to WebVTT.",
			ExtraArgs:   []string{"-map", "0:s:0", "-c:s", "webvtt"},
		},
		{
			Name: "Shift subtitle timing", Category: "Subtitle", Preset: "Custom", Container: "ass", Suffix: "shifted",
			Description: "Offset every subtitle timestamp; edit the seconds value in the filter builder.",
			ExtraArgs:   []string{"-itsoffset", "-1.5", "-map", "0:s:0", "-c:s", "ass"},
		},
		{
			Name: "Remove subtitles", Category: "Subtitle", Preset: "Custom", Container: "mkv", Suffix: "nosubs",
			Description: "Remux while dropping all subtitle streams.",
			ExtraArgs:   []string{"-sn"},
		},
	}
}
