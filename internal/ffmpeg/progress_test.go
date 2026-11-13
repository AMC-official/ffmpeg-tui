package ffmpeg

import (
	"testing"
	"time"
)

func TestParserKeyValueProgress(t *testing.T) {
	p := NewParser(10 * time.Second)
	lines := []string{
		"frame=100",
		"fps=25.0",
		"bitrate=1200.5kbits/s",
		"total_size=1048576",
		"out_time_us=4000000",
		"speed=1.5x",
		"progress=continue",
	}
	for _, l := range lines {
		if !p.FeedLine(l) {
			t.Fatalf("line %q should have changed the snapshot", l)
		}
	}
	got := p.Progress()
	if got.Frame != 100 {
		t.Fatalf("frame: %d", got.Frame)
	}
	if got.FPS != 25 {
		t.Fatalf("fps: %f", got.FPS)
	}
	if got.Bitrate != "1200.5kbits/s" {
		t.Fatalf("bitrate: %q", got.Bitrate)
	}
	if got.TotalSize != 1048576 {
		t.Fatalf("size: %d", got.TotalSize)
	}
	if got.OutTime != 4*time.Second {
		t.Fatalf("out_time: %v", got.OutTime)
	}
	if got.Speed != 1.5 || got.SpeedText != "1.5x" {
		t.Fatalf("speed: %v %q", got.Speed, got.SpeedText)
	}
	if got.Percent < 39.9 || got.Percent > 40.1 {
		t.Fatalf("percent: %f", got.Percent)
	}
	eta := got.ETA()
	if eta < 3*time.Second || eta > 5*time.Second {
		t.Fatalf("eta: %v", eta)
	}

	p.FeedLine("progress=end")
	if !p.Progress().Done || p.Progress().Percent != 100 {
		t.Fatalf("progress=end must complete the snapshot: %+v", p.Progress())
	}
}

func TestParserStderrStats(t *testing.T) {
	p := NewParser(0)
	p.FeedLine("  Duration: 00:00:20.00, start: 0.000000, bitrate: 1914 kb/s")
	p.FeedLine("frame=  250 fps= 62 q=28.0 size=    1024kB time=00:00:10.00 bitrate= 838.9kbits/s speed=2.49x")
	got := p.Progress()
	if got.Duration != 20*time.Second {
		t.Fatalf("duration not learned: %v", got.Duration)
	}
	if got.OutTime != 10*time.Second {
		t.Fatalf("time: %v", got.OutTime)
	}
	if got.Speed < 2.4 || got.Speed > 2.6 {
		t.Fatalf("speed: %v", got.Speed)
	}
	if got.Percent < 49.9 || got.Percent > 50.1 {
		t.Fatalf("percent: %v", got.Percent)
	}
}

func TestParserUnknownDurationHasNoPercent(t *testing.T) {
	p := NewParser(0)
	p.FeedLine("frame=10")
	p.FeedLine("out_time_us=1000000")
	got := p.Progress()
	if got.Percent != -1 {
		t.Fatalf("percent must stay unknown without a duration: %v", got.Percent)
	}
}

func TestFormatDurationAndTimestamp(t *testing.T) {
	if s := FormatDuration(90 * time.Second); s != "01:30" {
		t.Fatalf("duration: %s", s)
	}
	if s := FormatDuration(3661 * time.Second); s != "01:01:01" {
		t.Fatalf("duration: %s", s)
	}
	d, ok := ParseTimestamp("00:01:23.50")
	if !ok || d != 83*time.Second+500*time.Millisecond {
		t.Fatalf("timestamp: %v %v", d, ok)
	}
	if _, ok := ParseTimestamp("N/A"); ok {
		t.Fatal("N/A must not parse")
	}
}
