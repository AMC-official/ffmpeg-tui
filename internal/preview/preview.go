// Package preview renders media into terminal-sized ASCII art.
//
// No image protocol is required: FFmpeg decodes a single frame (or a waveform
// picture) into raw 8-bit grayscale, which is mapped onto a luminance ramp. It
// works over SSH, in any terminal and on every supported platform, and it uses
// the same ffmpeg binary the rest of the application drives.
package preview

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Result is a rendered preview.
type Result struct {
	Kind  string // "thumbnail", "waveform", "frames"
	Art   string
	Cols  int
	Rows  int
	Error string
}

const ramp = " .:-=+*#%@"

// Thumbnail decodes one frame and renders it as ASCII.
func Thumbnail(ctx context.Context, bin, input string, cols, rows int, seek time.Duration) (Result, error) {
	if cols < 8 || rows < 3 {
		return Result{}, errors.New("preview area too small")
	}
	if input == "" {
		return Result{}, errors.New("no input")
	}
	frame, err := rawGray(ctx, bin, []string{
		"-ss", formatSeek(seek),
		"-i", input,
		"-frames:v", "1",
		"-an", "-sn", "-dn",
		"-vf", fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2:black,format=gray", cols, rows, cols, rows),
		"-f", "rawvideo", "-pix_fmt", "gray", "pipe:1",
	}, cols*rows)
	if err != nil {
		return Result{Kind: "thumbnail", Error: err.Error()}, err
	}
	return Result{Kind: "thumbnail", Art: toArt(frame, cols, rows), Cols: cols, Rows: rows}, nil
}

// Waveform renders the audio envelope as ASCII.
func Waveform(ctx context.Context, bin, input string, cols, rows int) (Result, error) {
	if cols < 8 || rows < 3 {
		return Result{}, errors.New("preview area too small")
	}
	frame, err := rawGray(ctx, bin, []string{
		"-i", input,
		"-filter_complex", fmt.Sprintf("showwavespic=s=%dx%d:colors=white", cols, rows),
		"-frames:v", "1",
		"-an", "-sn", "-dn",
		"-f", "rawvideo", "-pix_fmt", "gray", "pipe:1",
	}, cols*rows)
	if err != nil {
		return Result{Kind: "waveform", Error: err.Error()}, err
	}
	return Result{Kind: "waveform", Art: toArt(frame, cols, rows), Cols: cols, Rows: rows}, nil
}

// Duration returns the media duration via ffprobe-free arithmetic on ffmpeg's
// stderr banner. It is only used to pick a thumbnail seek point, so best effort
// is sufficient.
func Duration(ctx context.Context, bin, input string) time.Duration {
	if bin == "" {
		bin = "ffmpeg"
	}
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, _ := exec.CommandContext(cctx, bin, "-hide_banner", "-i", input, "-f", "null", "-").CombinedOutput()
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if idx := strings.Index(line, "Duration:"); idx >= 0 {
			rest := strings.TrimSpace(line[idx+len("Duration:"):])
			if i := strings.Index(rest, ","); i >= 0 {
				rest = rest[:i]
			}
			if d, ok := parseTimestamp(rest); ok {
				return d
			}
		}
	}
	return 0
}

func rawGray(ctx context.Context, bin string, args []string, want int) ([]byte, error) {
	if bin == "" {
		bin = "ffmpeg"
	}
	if _, err := exec.LookPath(bin); err != nil {
		return nil, fmt.Errorf("ffmpeg not available: %w", err)
	}
	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	full := append([]string{"-hide_banner", "-nostdin", "-loglevel", "error"}, args...)
	cmd := exec.CommandContext(cctx, bin, full...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		if len(msg) > 300 {
			msg = msg[len(msg)-300:]
		}
		return nil, fmt.Errorf("preview failed: %s", msg)
	}
	data := stdout.Bytes()
	if len(data) < want {
		return nil, fmt.Errorf("preview produced %d of %d pixels", len(data), want)
	}
	return data[:want], nil
}

func toArt(data []byte, cols, rows int) string {
	var b strings.Builder
	b.Grow((cols + 1) * rows)
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			v := data[y*cols+x]
			idx := int(v) * (len(ramp) - 1) / 255
			b.WriteByte(ramp[idx])
		}
		if y < rows-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func formatSeek(d time.Duration) string {
	if d <= 0 {
		return "0"
	}
	total := d / time.Second
	return fmt.Sprintf("%02d:%02d:%02d", int64(total)/3600, (int64(total)%3600)/60, int64(total)%60)
}

func parseTimestamp(v string) (time.Duration, bool) {
	parts := strings.Split(v, ":")
	if len(parts) != 3 {
		return 0, false
	}
	var h, m int
	var s float64
	if _, err := fmt.Sscanf(parts[0], "%d", &h); err != nil {
		return 0, false
	}
	if _, err := fmt.Sscanf(parts[1], "%d", &m); err != nil {
		return 0, false
	}
	if _, err := fmt.Sscanf(parts[2], "%f", &s); err != nil {
		return 0, false
	}
	return time.Duration(h)*time.Hour + time.Duration(m)*time.Minute + time.Duration(s*float64(time.Second)), true
}
