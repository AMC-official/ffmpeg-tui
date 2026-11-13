// Package ffprobe provides a typed interface to the official ffprobe binary.
//
// FFmpeg-TUI never links against libav*; it shells out to ffprobe and decodes
// the JSON report it produces. This package is deliberately dependency free so
// that it can be reused by any part of the application.
package ffprobe

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Stream describes one elementary stream inside a media file.
type Stream struct {
	Index            int               `json:"index"`
	CodecName        string            `json:"codec_name"`
	CodecLongName    string            `json:"codec_long_name"`
	CodecType        string            `json:"codec_type"`
	CodecTagString   string            `json:"codec_tag_string"`
	Profile          string            `json:"profile"`
	Width            int               `json:"width"`
	Height           int               `json:"height"`
	CodedWidth       int               `json:"coded_width"`
	CodedHeight      int               `json:"coded_height"`
	PixFmt           string            `json:"pix_fmt"`
	Level            int               `json:"level"`
	ColorRange       string            `json:"color_range"`
	ColorSpace       string            `json:"color_space"`
	ColorTransfer    string            `json:"color_transfer"`
	ColorPrimaries   string            `json:"color_primaries"`
	ChromaLocation   string            `json:"chroma_location"`
	FieldOrder       string            `json:"field_order"`
	AvgFrameRate     string            `json:"avg_frame_rate"`
	RFrameRate       string            `json:"r_frame_rate"`
	TimeBase         string            `json:"time_base"`
	Duration         string            `json:"duration"`
	BitRate          string            `json:"bit_rate"`
	MaxBitRate       string            `json:"max_bit_rate"`
	NbFrames         string            `json:"nb_frames"`
	Channels         int               `json:"channels"`
	ChannelLayout    string            `json:"channel_layout"`
	SampleRate       string            `json:"sample_rate"`
	SampleFmt        string            `json:"sample_fmt"`
	BitsPerSample    int               `json:"bits_per_sample"`
	Disposition      map[string]int    `json:"disposition"`
	Tags             map[string]string `json:"tags"`
	NumberOfFrames   int64             `json:"nb_frames_int"`
	AttachmentName   string            `json:"-"`
	AttachmentMime   string            `json:"-"`
	IsDefault        bool              `json:"-"`
	IsForced         bool              `json:"-"`
	IsCommentary     bool              `json:"-"`
	EstimatedBitrate int64             `json:"-"`
}

// Chapter is a chapter marker as reported by ffprobe.
type Chapter struct {
	ID        int               `json:"id"`
	TimeBase  string            `json:"time_base"`
	Start     int64             `json:"start"`
	StartTime string            `json:"start_time"`
	End       int64             `json:"end"`
	EndTime   string            `json:"end_time"`
	Tags      map[string]string `json:"tags"`
}

// Format describes the container-level metadata.
type Format struct {
	Filename       string            `json:"filename"`
	NbStreams      int               `json:"nb_streams"`
	FormatName     string            `json:"format_name"`
	FormatLongName string            `json:"format_long_name"`
	StartTime      string            `json:"start_time"`
	Duration       string            `json:"duration"`
	Size           string            `json:"size"`
	BitRate        string            `json:"bit_rate"`
	ProbeScore     int               `json:"probe_score"`
	Tags           map[string]string `json:"tags"`
}

// Result is a complete probe report.
type Result struct {
	Streams  []Stream  `json:"streams"`
	Chapters []Chapter `json:"chapters"`
	Format   Format    `json:"format"`
	Path     string    `json:"-"`
	ProbedAt time.Time `json:"-"`
}

// Options controls how ffprobe is invoked.
type Options struct {
	Binary  string        // defaults to "ffprobe" (resolved through PATH)
	Timeout time.Duration // defaults to 30s
	Count   bool          // count frames/packets (slow, used on demand)
}

// ErrUnavailable is returned when the ffprobe binary cannot be found.
var ErrUnavailable = errors.New("ffprobe binary not available")

// Probe analyses path and returns a typed report.
func Probe(ctx context.Context, path string, opts Options) (*Result, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("empty media path")
	}
	if !isRemote(path) {
		abs, err := filepath.Abs(path)
		if err != nil {
			return nil, fmt.Errorf("resolve path: %w", err)
		}
		path = abs
		st, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("cannot read %s: %w", path, err)
		}
		if st.IsDir() {
			return nil, fmt.Errorf("%s is a directory", path)
		}
	}

	bin := opts.Binary
	if bin == "" {
		bin = "ffprobe"
	}
	if _, err := exec.LookPath(bin); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrUnavailable, bin)
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	args := []string{
		"-v", "error",
		"-hide_banner",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		"-show_chapters",
	}
	if opts.Count {
		args = append(args, "-count_frames", "-count_packets")
	}
	args = append(args, path)

	cmd := exec.CommandContext(cctx, bin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Stdin = nil
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("ffprobe failed: %s", lastLines(msg, 6))
	}

	var res Result
	if err := json.Unmarshal(stdout.Bytes(), &res); err != nil {
		return nil, fmt.Errorf("decode ffprobe report: %w", err)
	}
	res.Path = path
	res.ProbedAt = time.Now()
	res.normalize()
	return &res, nil
}

func (r *Result) normalize() {
	for i := range r.Streams {
		s := &r.Streams[i]
		if s.Disposition == nil {
			s.Disposition = map[string]int{}
		}
		if s.Tags == nil {
			s.Tags = map[string]string{}
		}
		s.IsDefault = s.Disposition["default"] == 1
		s.IsForced = s.Disposition["forced"] == 1
		s.IsCommentary = s.Disposition["comment"] == 1
		if s.CodecType == "attachment" {
			s.AttachmentName = s.Tags["filename"]
			s.AttachmentMime = s.Tags["mimetype"]
		}
		if n, err := strconv.ParseInt(s.NbFrames, 10, 64); err == nil {
			s.NumberOfFrames = n
		}
		if n, err := strconv.ParseInt(s.BitRate, 10, 64); err == nil {
			s.EstimatedBitrate = n
		} else if d := r.Duration(); d > 0 && s.CodecType == "video" {
			// Some containers do not store per-stream bitrates; estimate from
			// the reported stream size when available.
			if sz, err := strconv.ParseInt(s.Tags["NUMBER_OF_BYTES"], 10, 64); err == nil && sz > 0 {
				s.EstimatedBitrate = int64(float64(sz*8) / d.Seconds())
			}
		}
	}
}

// Duration returns the container duration, or the longest stream duration if
// the container itself does not declare one.
func (r *Result) Duration() time.Duration {
	if d, err := strconv.ParseFloat(r.Format.Duration, 64); err == nil && d > 0 {
		return time.Duration(d * float64(time.Second))
	}
	var best time.Duration
	for _, s := range r.Streams {
		if d, err := strconv.ParseFloat(s.Duration, 64); err == nil {
			if dd := time.Duration(d * float64(time.Second)); dd > best {
				best = dd
			}
		}
	}
	return best
}

// Size returns the reported file size in bytes (0 when unknown).
func (r *Result) Size() int64 {
	n, _ := strconv.ParseInt(r.Format.Size, 10, 64)
	return n
}

// BitRate returns the reported container bitrate in bits per second.
func (r *Result) BitRate() int64 {
	n, _ := strconv.ParseInt(r.Format.BitRate, 10, 64)
	return n
}

// Video returns the first video stream (nil when the file is audio-only).
func (r *Result) Video() *Stream {
	for i := range r.Streams {
		if r.Streams[i].CodecType == "video" && !r.Streams[i].isCoverArt() {
			return &r.Streams[i]
		}
	}
	return nil
}

// CoverArt returns the first embedded picture stream, if any.
func (r *Result) CoverArt() *Stream {
	for i := range r.Streams {
		if r.Streams[i].CodecType == "video" && r.Streams[i].isCoverArt() {
			return &r.Streams[i]
		}
	}
	return nil
}

// Audio returns the first audio stream, if any.
func (r *Result) Audio() *Stream {
	for i := range r.Streams {
		if r.Streams[i].CodecType == "audio" {
			return &r.Streams[i]
		}
	}
	return nil
}

// SubtitleStreams returns every subtitle stream.
func (r *Result) SubtitleStreams() []Stream { return r.byType("subtitle") }

// AttachmentStreams returns every attachment (fonts, images, ...).
func (r *Result) AttachmentStreams() []Stream { return r.byType("attachment") }

// StreamsOfType returns every stream of the requested codec type.
func (r *Result) StreamsOfType(t string) []Stream { return r.byType(t) }

func (r *Result) byType(t string) []Stream {
	var out []Stream
	for _, s := range r.Streams {
		if s.CodecType == t {
			out = append(out, s)
		}
	}
	return out
}

// HasVideo reports whether a real (non cover art) video stream exists.
func (r *Result) HasVideo() bool { return r.Video() != nil }

// HasAudio reports whether an audio stream exists.
func (r *Result) HasAudio() bool { return r.Audio() != nil }

// Summary returns a one line human readable description.
func (r *Result) Summary() string {
	parts := []string{}
	if v := r.Video(); v != nil {
		parts = append(parts, fmt.Sprintf("%dx%d %s", v.Width, v.Height, v.CodecName))
	}
	if a := r.Audio(); a != nil {
		parts = append(parts, fmt.Sprintf("%s %dch %s Hz", a.CodecName, a.Channels, a.SampleRate))
	}
	if len(parts) == 0 {
		parts = append(parts, r.Format.FormatName)
	}
	return strings.Join(parts, " • ")
}

func (s *Stream) isCoverArt() bool {
	if s.Disposition["attached_pic"] == 1 {
		return true
	}
	switch s.CodecName {
	case "png", "mjpeg", "bmp", "gif", "webp":
		return true
	}
	return false
}

// Title returns the stream title tag, falling back to the language code.
func (s *Stream) Title() string {
	if t := s.Tags["title"]; t != "" {
		return t
	}
	if l := s.Tags["language"]; l != "" && l != "und" {
		return l
	}
	return fmt.Sprintf("stream #%d", s.Index)
}

// Language returns the ISO language tag or "und".
func (s *Stream) Language() string {
	if l := s.Tags["language"]; l != "" {
		return l
	}
	return "und"
}

// IsHDR reports whether the stream carries PQ or HLG high dynamic range data.
func (s *Stream) IsHDR() bool {
	switch s.ColorTransfer {
	case "smpte2084", "arib-std-b67":
		return true
	}
	return strings.HasPrefix(s.ColorSpace, "bt2020")
}

// HDRLabel returns a short dynamic range description.
func (s *Stream) HDRLabel() string {
	if s.IsHDR() {
		if s.ColorTransfer == "smpte2084" {
			return "HDR10 / PQ"
		}
		return "HLG"
	}
	return "SDR"
}

// FPS estimates the frame rate from avg_frame_rate, falling back to r_frame_rate.
func (s *Stream) FPS() float64 {
	if v := ParseRate(s.AvgFrameRate); v > 0 {
		return v
	}
	return ParseRate(s.RFrameRate)
}

// ParseRate converts an ffprobe rational ("30000/1001") into a float.
func ParseRate(v string) float64 {
	if v == "" {
		return 0
	}
	num, den, ok := strings.Cut(v, "/")
	if !ok {
		f, _ := strconv.ParseFloat(v, 64)
		return f
	}
	n, err1 := strconv.ParseFloat(num, 64)
	d, err2 := strconv.ParseFloat(den, 64)
	if err1 != nil || err2 != nil || d == 0 {
		return 0
	}
	return n / d
}

// ChapterLabel returns the human readable chapter title.
func (c Chapter) ChapterName() string {
	if t := c.Tags["title"]; t != "" {
		return t
	}
	return fmt.Sprintf("Chapter %d", c.ID+1)
}

func isRemote(p string) bool {
	return strings.Contains(p, "://")
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, " | ")
}
