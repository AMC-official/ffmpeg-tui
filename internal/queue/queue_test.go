package queue

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/ffmpeg-tui/ffmpeg-tui/internal/ffmpeg"
)

func cmd(in, out string) ffmpeg.Command {
	return ffmpeg.Command{Bin: "ffmpeg", Args: []string{"-i", in, "-c:v", "libx264"}, Input: in, Output: out, Label: "test"}
}

func TestQueueOrderingAndRemoval(t *testing.T) {
	q := New()
	a := q.Add("a", cmd("a.mkv", "a.mp4"))
	b := q.Add("b", cmd("b.mkv", "b.mp4"))
	c := q.Add("c", cmd("c.mkv", "c.mp4"))

	if q.Len() != 3 {
		t.Fatalf("len: %d", q.Len())
	}
	if next := q.NextQueued(); next == nil || next.ID != a.ID {
		t.Fatalf("next queued should be a")
	}
	if !q.Move(2, 0) {
		t.Fatal("move failed")
	}
	if q.Jobs[0].ID != c.ID || q.Jobs[1].ID != a.ID || q.Jobs[2].ID != b.ID {
		t.Fatalf("unexpected order: %s %s %s", q.Jobs[0].Name, q.Jobs[1].Name, q.Jobs[2].Name)
	}
	if q.Move(0, 100) != true || q.Jobs[2].ID != c.ID {
		t.Fatalf("move should clamp to the end")
	}
	if !q.Remove(a.ID) || q.Len() != 2 {
		t.Fatalf("remove failed")
	}
	if q.Index(b.ID) != 0 {
		t.Fatalf("indices shifted wrong")
	}
}

func TestQueuePersistenceRestoresInterruptedJobs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "queue.json")
	q := New()
	j := q.Add("movie", cmd("in.mkv", "out.mp4"))
	j.Status = Running
	j.StartedAt = time.Now()
	j.Progress.Percent = 42
	j.Logs = []string{"frame=1", "frame=2"}

	if err := q.Save(path); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Len() != 1 {
		t.Fatalf("len: %d", loaded.Len())
	}
	got := loaded.Jobs[0]
	if got.Status != Queued {
		t.Fatalf("interrupted job must be requeued, got %s", got.Status)
	}
	if got.Command.Output != "out.mp4" || got.Name != "movie" {
		t.Fatalf("command not preserved: %+v", got.Command)
	}
	if len(got.Logs) != 2 {
		t.Fatalf("logs not preserved: %v", got.Logs)
	}
	if loaded.Active() != nil {
		t.Fatal("restored queue must not claim an active job")
	}
}

func TestLoadMissingQueueIsEmptyNotError(t *testing.T) {
	q, err := Load(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil {
		t.Fatal(err)
	}
	if q.Len() != 0 {
		t.Fatal("missing queue must be empty")
	}
}

func TestClearFinishedAndCounts(t *testing.T) {
	q := New()
	a := q.Add("a", cmd("a", "a"))
	b := q.Add("b", cmd("b", "b"))
	q.Add("c", cmd("c", "c"))
	a.Status = Done
	b.Status = Failed
	queued, running, done, failed := q.Counts()
	if queued != 1 || running != 0 || done != 1 || failed != 1 {
		t.Fatalf("counts: %d %d %d %d", queued, running, done, failed)
	}
	if n := q.ClearFinished(); n != 2 || q.Len() != 1 {
		t.Fatalf("clear finished: %d, len %d", n, q.Len())
	}
}

func TestSnapshotIsIndependent(t *testing.T) {
	q := New()
	j := q.Add("a", cmd("a", "a"))
	snap := q.Snapshot()
	j.Logs = append(j.Logs, "live")
	j.Status = Running
	if snap.Jobs[0].Status != Queued || len(snap.Jobs[0].Logs) != 0 {
		t.Fatal("snapshot must not alias live jobs")
	}
}
