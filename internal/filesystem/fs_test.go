package filesystem

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestClassifyAndKindOf(t *testing.T) {
	cases := map[string]Kind{
		"movie.MKV":  KindVideo,
		"song.flac":  KindAudio,
		"cover.webp": KindImage,
		"subs.srt":   KindSubtitle,
		"notes.txt":  KindOther,
	}
	for name, want := range cases {
		if got := KindOf(name); got != want {
			t.Errorf("KindOf(%s) = %v want %v", name, got, want)
		}
	}
}

func TestListSortsDirectoriesFirst(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, "zeta"), 0o755)
	os.WriteFile(filepath.Join(dir, "alpha.mp4"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, ".hidden.mkv"), []byte("x"), 0o644)

	entries, err := List(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("hidden files must be skipped, got %d", len(entries))
	}
	if !entries[0].IsDir || entries[0].Name != "zeta" {
		t.Fatalf("directory must sort first: %+v", entries)
	}
	if entries[1].Kind != KindVideo {
		t.Fatalf("media classification: %+v", entries[1])
	}

	entries, _ = List(dir, true)
	if len(entries) != 3 {
		t.Fatalf("hidden files must be included, got %d", len(entries))
	}
}

func TestHumanSizeAndDuration(t *testing.T) {
	if s := HumanSize(512); s != "512 B" {
		t.Fatalf("size: %s", s)
	}
	if s := HumanSize(1536); s != "1.5 KiB" {
		t.Fatalf("size: %s", s)
	}
	if s := HumanDuration(3*time.Hour + 2*time.Minute); s != "3:02:00" {
		t.Fatalf("duration: %s", s)
	}
	if s := HumanBitrate(4_200_000); s != "4.20 Mbps" {
		t.Fatalf("bitrate: %s", s)
	}
}

func TestDefaultOutputAndUniquePath(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "movie.mkv")
	out := DefaultOutput(in, "mp4")
	if out != filepath.Join(dir, "movie.mp4") {
		t.Fatalf("default output: %s", out)
	}
	os.WriteFile(out, []byte("x"), 0o644)
	unique := UniquePath(out)
	if unique == out {
		t.Fatal("existing path must be made unique")
	}
	if filepath.Ext(unique) != ".mp4" || !strings.Contains(unique, "movie-2") {
		t.Fatalf("unique path: %s", unique)
	}
}

func TestTemplateOutput(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "S01E01 - Pilot.mkv")
	got := TemplateOutput(in, "{name}-{index}.{ext}", filepath.Join(dir, "out"), 7)
	want := filepath.Join(dir, "out", "S01E01 - Pilot-7.mkv")
	if got != want {
		t.Fatalf("template: got %s want %s", got, want)
	}
	got = TemplateOutput(in, "fixed", dir, 1)
	if filepath.Ext(got) != ".mkv" {
		t.Fatalf("template must add the source extension: %s", got)
	}
}

func TestSanitizeAndMirror(t *testing.T) {
	if got := SanitizeName(`a/b:c*d?e"f<g>h|i`); strings.ContainsAny(got, `/:*?"<>|`) {
		t.Fatalf("unsafe characters remain: %q", got)
	}
	if got := SanitizeName("   "); got != "output" {
		t.Fatalf("empty name fallback: %q", got)
	}
	root := filepath.Join("films", "raw")
	got := Mirror(root, "converted", filepath.Join(root, "2024", "clip.mkv"))
	if got != filepath.Join("converted", "2024", "clip.mkv") {
		t.Fatalf("mirror: %s", got)
	}
}

func TestCompletePath(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, "alpha"), 0o755)
	os.Mkdir(filepath.Join(dir, "alpine"), 0o755)
	matches := CompletePath(filepath.Join(dir, "al"))
	if len(matches) != 2 {
		t.Fatalf("completions: %v", matches)
	}
}
