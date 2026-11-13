package ffmpeg

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Progress is a snapshot of an encoding run.
type Progress struct {
	Frame     int64
	FPS       float64
	Bitrate   string
	TotalSize int64
	OutTime   time.Duration
	Speed     float64
	SpeedText string
	Duration  time.Duration
	Done      bool
	Percent   float64 // 0..100, -1 when unknown
}

// ETA returns the estimated remaining time. It is zero when not computable.
func (p Progress) ETA() time.Duration {
	if p.Duration <= 0 || p.OutTime <= 0 || p.Speed <= 0 {
		return 0
	}
	remaining := p.Duration - p.OutTime
	if remaining <= 0 {
		return 0
	}
	return time.Duration(float64(remaining) / p.Speed)
}

// Remaining returns the unprocessed media duration.
func (p Progress) Remaining() time.Duration {
	if p.Duration <= 0 || p.OutTime <= 0 {
		return 0
	}
	if p.OutTime >= p.Duration {
		return 0
	}
	return p.Duration - p.OutTime
}

// Ratio returns progress in the range 0..1.
func (p Progress) Ratio() float64 {
	if p.Percent < 0 {
		return 0
	}
	return clamp01(p.Percent / 100)
}

// Known reports whether a meaningful percentage can be drawn.
func (p Progress) Known() bool { return p.Percent >= 0 }

// Clone returns a copy, so consumers never alias a live job's counters.
func (p Progress) Clone() Progress { return p }

// Parser consumes FFmpeg progress output.
//
// It understands both machine-readable "-progress" key=value lines on stdout
// and human-readable stats lines on stderr. FFmpeg is intentionally invoked
// with "-progress pipe:1 -nostats -loglevel error", but the human parser is
// kept for robustness when users replay a command themselves.
type Parser struct {
	prog Progress

	lastFrame  int64
	lastSize   int64
	startedAt  time.Time
	stderrDur  time.Duration
	outTimeSet bool
}

// NewParser creates a parser for an input of the given duration (0 if unknown).
func NewParser(duration time.Duration) *Parser {
	return &Parser{
		prog:      Progress{Duration: duration, Percent: -1},
		startedAt: time.Now(),
	}
}

// Progress returns the latest snapshot without advancing state.
func (p *Parser) Progress() Progress {
	out := p.prog
	if out.Duration > 0 {
		if out.OutTime >= out.Duration {
			out.Percent = 100
		} else if out.OutTime > 0 {
			out.Percent = clamp01(float64(out.OutTime)/float64(out.Duration)) * 100
		}
	} else if out.Done {
		out.Percent = 100
	}
	return out
}

// SetDuration supplies a duration learned later (e.g. from stderr).
func (p *Parser) SetDuration(d time.Duration) {
	if d > 0 && p.prog.Duration <= 0 {
		p.prog.Duration = d
	}
}

// FeedLine consumes one line of FFmpeg output. It reports whether the snapshot
// changed in a way worth redrawing.
func (p *Parser) FeedLine(line string) bool {
	line = strings.TrimRight(line, "\r\n")
	if line == "" {
		return false
	}
	if k, v, ok := strings.Cut(line, "="); ok && isProgressKey(k) && !strings.ContainsAny(k, " =") && !strings.Contains(v, "=") {
		return p.feedKey(strings.TrimSpace(k), strings.TrimSpace(v))
	}
	if strings.Contains(line, "frame=") && strings.Contains(line, "fps=") {
		return p.feedStderrStats(line)
	}
	if d := parseDurationLine(line); d > 0 && p.prog.Duration <= 0 {
		p.prog.Duration = d
		p.stderrDur = d
		return true
	}
	return false
}

func isProgressKey(k string) bool {
	switch k {
	case "frame", "fps", "bitrate", "total_size", "out_time_us", "out_time_ms",
		"out_time", "dup_frames", "drop_frames", "speed", "progress", "stream_0_0_q":
		return true
	}
	return false
}

func (p *Parser) feedKey(k, v string) bool {
	changed := false
	switch k {
	case "frame":
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			p.prog.Frame = n
			changed = n != p.lastFrame
			p.lastFrame = n
		}
	case "fps":
		if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0 {
			p.prog.FPS = f
			changed = true
		}
	case "bitrate":
		if v != "" && v != "N/A" && !strings.HasPrefix(v, "-") {
			p.prog.Bitrate = v
			changed = true
		}
	case "total_size":
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			p.prog.TotalSize = n
			changed = true
		}
	case "out_time_us":
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n >= 0 {
			p.setOutTime(time.Duration(n) * time.Microsecond)
			changed = true
		}
	case "out_time_ms":
		// Historical quirk: out_time_ms is actually microseconds. Prefer
		// out_time_us when present, but accept this for old builds.
		if !p.outTimeSet {
			if n, err := strconv.ParseInt(v, 10, 64); err == nil && n >= 0 {
				p.setOutTime(time.Duration(n) * time.Microsecond)
				changed = true
			}
		}
	case "out_time":
		if d, ok := parseTimestamp(v); ok {
			p.setOutTime(d)
			changed = true
		}
	case "speed":
		sp := strings.TrimSuffix(strings.TrimSpace(v), "x")
		if f, err := strconv.ParseFloat(sp, 64); err == nil && f >= 0 {
			p.prog.Speed = f
			p.prog.SpeedText = strings.TrimSpace(v)
			changed = true
		}
	case "progress":
		changed = true
		if v == "end" {
			p.prog.Done = true
			p.prog.Percent = 100
			if p.prog.Duration > 0 {
				p.prog.OutTime = p.prog.Duration
			}
		}
	}
	return changed
}

func (p *Parser) setOutTime(d time.Duration) {
	if d < 0 {
		return
	}
	p.prog.OutTime = d
	p.outTimeSet = true
}

var (
	stderrFrameRe   = regexp.MustCompile(`frame=\s*(\d+)`)
	stderrFPSRe     = regexp.MustCompile(`fps=\s*([\d.]+)`)
	stderrBitrateRe = regexp.MustCompile(`bitrate=\s*([\w./-]+)`)
	stderrTimeRe    = regexp.MustCompile(`time=\s*(\d+:\d{2}:\d{2}(?:\.\d+)?)`)
)

func (p *Parser) feedStderrStats(line string) bool {
	if m := stderrFrameRe.FindStringSubmatch(line); m != nil {
		if n, err := strconv.ParseInt(m[1], 10, 64); err == nil {
			p.prog.Frame = n
		}
	}
	if m := stderrFPSRe.FindStringSubmatch(line); m != nil {
		if f, err := strconv.ParseFloat(m[1], 64); err == nil {
			p.prog.FPS = f
		}
	}
	if m := stderrBitrateRe.FindStringSubmatch(line); m != nil && m[1] != "N/A" {
		p.prog.Bitrate = m[1]
	}
	if m := stderrTimeRe.FindStringSubmatch(line); m != nil {
		if d, ok := parseTimestamp(m[1]); ok {
			p.setOutTime(d)
		}
	}
	if m := speedRe.FindStringSubmatch(line); m != nil {
		if f, err := strconv.ParseFloat(m[1], 64); err == nil {
			p.prog.Speed = f
			p.prog.SpeedText = m[1] + "x"
		}
	}
	if p.prog.Duration <= 0 {
		p.prog.Duration = p.stderrDur
	}
	return true
}

var (
	speedRe     = regexp.MustCompile(`speed=\s*([\d.]+)x`)
	durationRe  = regexp.MustCompile(`Duration:\s*(\d+:\d{2}:\d{2}(?:\.\d+)?)`)
	timestampRe = regexp.MustCompile(`time=\s*(\d+:\d{2}:\d{2}(?:\.\d+)?)`)
)

func parseDurationLine(line string) time.Duration {
	m := durationRe.FindStringSubmatch(line)
	if m == nil {
		return 0
	}
	d, _ := parseTimestamp(m[1])
	return d
}

// ParseTimestamp converts "HH:MM:SS.xx" into a duration.
func ParseTimestamp(v string) (time.Duration, bool) { return parseTimestamp(v) }

func parseTimestamp(v string) (time.Duration, bool) {
	v = strings.TrimSpace(v)
	if v == "" || strings.HasPrefix(v, "-") || v == "N/A" {
		return 0, false
	}
	parts := strings.Split(v, ":")
	if len(parts) != 3 {
		return 0, false
	}
	h, err1 := strconv.Atoi(parts[0])
	m, err2 := strconv.Atoi(parts[1])
	s, err3 := strconv.ParseFloat(parts[2], 64)
	if err1 != nil || err2 != nil || err3 != nil {
		return 0, false
	}
	return time.Duration(h)*time.Hour + time.Duration(m)*time.Minute +
		time.Duration(s*float64(time.Second)), true
}

// LastTimestamp extracts the last reported "time=" value from a log buffer.
func LastTimestamp(log string) time.Duration {
	matches := timestampRe.FindAllStringSubmatch(log, -1)
	if len(matches) == 0 {
		return 0
	}
	d, _ := parseTimestamp(matches[len(matches)-1][1])
	return d
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// FormatDuration renders a duration as HH:MM:SS (or MM:SS under an hour).
func FormatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	total := int64(d / time.Second)
	h := total / 3600
	m := (total % 3600) / 60
	s := total % 60
	if h > 0 {
		return pad2(h) + ":" + pad2(m) + ":" + pad2(s)
	}
	return pad2(m) + ":" + pad2(s)
}

func pad2(n int64) string {
	if n < 10 {
		return "0" + strconv.FormatInt(n, 10)
	}
	return strconv.FormatInt(n, 10)
}
