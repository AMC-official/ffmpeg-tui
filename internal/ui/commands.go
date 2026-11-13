package ui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ffmpeg-tui/ffmpeg-tui/internal/ffprobe"
	"github.com/ffmpeg-tui/ffmpeg-tui/internal/jobs"
	"github.com/ffmpeg-tui/ffmpeg-tui/internal/manpages"
	"github.com/ffmpeg-tui/ffmpeg-tui/internal/preview"
)

type probeMsg struct {
	path string
	res  *ffprobe.Result
	err  error
}

type previewMsg struct {
	path string
	res  preview.Result
	err  error
}

type jobEventMsg struct {
	ev jobs.Event
}

type toastExpireMsg struct {
	seq int
}

type liveHelpMsg struct {
	name string
	text string
	err  error
}

type compareProbeMsg struct {
	slot int
	path string
	res  *ffprobe.Result
	err  error
}

type savedMsg struct {
	text string
	err  error
}

// probeCmd runs ffprobe asynchronously.
func probeCmd(ctx context.Context, bin, path string) tea.Cmd {
	if path == "" {
		return nil
	}
	return func() tea.Msg {
		res, err := ffprobe.Probe(ctx, path, ffprobe.Options{
			Binary:  bin,
			Timeout: 30 * time.Second,
		})
		return probeMsg{path: path, res: res, err: err}
	}
}

// requestProbe probes path and records ownership of the request so that the
// result refreshes the details panel and the conversion wizard together.
func (m *Model) requestProbe(path string) tea.Cmd {
	if path == "" || !m.ffprobeOK {
		return nil
	}
	m.probePath = path
	m.probeLoading = true
	return probeCmd(m.ctx, m.ffprobePath, path)
}

// previewCmd renders an ASCII thumbnail or waveform asynchronously.
func previewCmd(ctx context.Context, bin, path string, cols, rows int, audio bool) tea.Cmd {
	if path == "" {
		return nil
	}
	return func() tea.Msg {
		var (
			res preview.Result
			err error
		)
		if audio {
			res, err = preview.Waveform(ctx, bin, path, cols, rows)
		} else {
			res, err = preview.Thumbnail(ctx, bin, path, cols, rows, 0)
		}
		return previewMsg{path: path, res: res, err: err}
	}
}

// waitJobEvent pumps one event from the runner. Every handled event re-arms it.
func waitJobEvent(mgr *jobs.Manager) tea.Cmd {
	if mgr == nil {
		return nil
	}
	return func() tea.Msg {
		return jobEventMsg{ev: <-mgr.Events()}
	}
}

// previewDuration probes a file's duration for progress estimation.
func previewDuration(ctx context.Context, bin, path string) (time.Duration, error) {
	res, err := ffprobe.Probe(ctx, path, ffprobe.Options{Binary: bin, Timeout: 10 * time.Second})
	if err != nil {
		return 0, err
	}
	return res.Duration(), nil
}

// liveHelpCmd fetches help text from the ffmpeg binary.
func liveHelpCmd(ctx context.Context, bin string, cat manpages.Category, name string) tea.Cmd {
	return func() tea.Msg {
		text, err := manpages.Live(ctx, bin, cat, name)
		return liveHelpMsg{name: name, text: text, err: err}
	}
}
