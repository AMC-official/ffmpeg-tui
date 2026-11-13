package jobs

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/AMC-official/ffmpeg-tui/internal/ffmpeg"
	"github.com/AMC-official/ffmpeg-tui/internal/ffprobe"
	"github.com/AMC-official/ffmpeg-tui/internal/history"
	"github.com/AMC-official/ffmpeg-tui/internal/queue"
)

func ffmpegAvailable(t *testing.T) string {
	t.Helper()
	bin, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed; skipping integration test")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not installed; skipping integration test")
	}
	return bin
}

func makeSample(t *testing.T, bin, dir string) string {
	t.Helper()
	out := filepath.Join(dir, "sample.mp4")
	cmd := exec.Command(bin,
		"-hide_banner", "-nostdin", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=320x240:rate=25",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=44100",
		"-t", "2", "-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p",
		"-c:a", "aac", "-shortest", out,
	)
	if data, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("cannot generate sample media with this ffmpeg build: %v\n%s", err, data)
	}
	return out
}

func TestManagerRunsJobAndReportsCompletion(t *testing.T) {
	bin := ffmpegAvailable(t)
	dir := t.TempDir()
	sample := makeSample(t, bin, dir)
	out := filepath.Join(dir, "converted.mp4")

	res, err := ffprobe.Probe(context.Background(), sample, ffprobe.Options{})
	if err != nil {
		t.Fatalf("probe sample: %v", err)
	}
	if res.Duration() < time.Second {
		t.Fatalf("sample duration: %v", res.Duration())
	}

	hist := history.Open(filepath.Join(dir, "history.jsonl"), 10)
	m := New(bin)
	m.SetHistory(hist)
	m.Start()
	defer m.Stop()

	b := ffmpeg.NewBuilder(bin, sample).WithDuration(res.Duration())
	b.VideoCodec("libx264").CRF("30").EncoderPreset("ultrafast").PixelFormat("yuv420p")
	b.AudioCodec("aac").AudioBitrate("96k")
	cmd := b.Build(out)

	job := m.Enqueue("converted.mp4", cmd)
	if job == nil || job.ID == "" {
		t.Fatal("enqueue returned no job")
	}

	deadline := time.After(90 * time.Second)
	var finished *queue.Job
	for finished == nil {
		select {
		case ev := <-m.Events():
			if ev.Kind == EventFinished && ev.JobID == job.ID {
				j := ev.Job
				finished = &j
			}
		case <-deadline:
			snap := m.Snapshot()
			for i := range snap.Jobs {
				t.Logf("job %s status=%s err=%s logs=%v", snap.Jobs[i].ID, snap.Jobs[i].Status, snap.Jobs[i].Err, snap.Jobs[i].Logs)
			}
			t.Fatal("timed out waiting for the job to finish")
		}
	}

	if finished.Status != queue.Done {
		t.Fatalf("job failed: status=%s err=%s", finished.Status, finished.Err)
	}
	if finished.Progress.Percent != 100 {
		t.Fatalf("finished job should report 100%%, got %.1f", finished.Progress.Percent)
	}
	if finished.Progress.Frame <= 0 {
		t.Fatalf("no progress frames parsed: %+v", finished.Progress)
	}
	if st, err := os.Stat(out); err != nil || st.Size() == 0 {
		t.Fatalf("output missing or empty: %v", err)
	}
	if hist.Len() != 1 {
		t.Fatalf("history entries: %d", hist.Len())
	}
	if e := hist.Entries()[0]; e.Status != "done" || e.Input != sample {
		t.Fatalf("history entry: %+v", e)
	}
}

func TestCancelQueuedJob(t *testing.T) {
	m := New("ffmpeg")
	job := m.Enqueue("queued", ffmpeg.Command{Bin: "ffmpeg", Input: "a", Output: "b", Args: []string{"-i", "a"}})
	if !m.Cancel(job.ID) {
		t.Fatal("cancel should succeed for a queued job")
	}
	snap := m.Snapshot()
	if snap.Jobs[0].Status != queue.Canceled {
		t.Fatalf("status: %s", snap.Jobs[0].Status)
	}
	if m.Cancel(job.ID) {
		t.Fatal("canceling a terminal job should fail")
	}
	if m.Retry(job.ID) != true {
		t.Fatal("retry should requeue a canceled job")
	}
	snap = m.Snapshot()
	if snap.Jobs[0].Status != queue.Queued {
		t.Fatalf("retry status: %s", snap.Jobs[0].Status)
	}
	m.Stop()
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	m := New("ffmpeg")
	m.Enqueue("one", ffmpeg.Command{Bin: "ffmpeg", Input: "a", Output: "b"})
	path := filepath.Join(dir, "queue.json")
	if err := m.Save(path); err != nil {
		t.Fatal(err)
	}
	m2 := New("ffmpeg")
	if err := m2.Load(path); err != nil {
		t.Fatal(err)
	}
	snap := m2.Snapshot()
	if len(snap.Jobs) != 1 || snap.Jobs[0].Name != "one" {
		t.Fatalf("loaded queue: %+v", snap.Jobs)
	}
	m.Stop()
	m2.Stop()
}
