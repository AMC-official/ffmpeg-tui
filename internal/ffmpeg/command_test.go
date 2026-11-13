package ffmpeg

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func touch(t *testing.T, dir, name string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestBuilderProducesDeterministicArgs(t *testing.T) {
	dir := t.TempDir()
	in := touch(t, dir, "movie.mkv")
	out := filepath.Join(dir, "out.mp4")

	b := NewBuilder("ffmpeg", in)
	b.VideoCodec("libx264").CRF("18").EncoderPreset("slow").PixelFormat("yuv420p")
	b.AudioCodec("aac").AudioBitrate("192k")
	b.Metadata("title", "My Film").Faststart(true)
	cmd := b.Build(out)

	got := strings.Join(cmd.Argv(), "\x00")
	for _, want := range []string{
		"-hide_banner", "-nostdin", "-y", "-i", in, "-c:v", "libx264",
		"-crf", "18", "-preset", "slow", "-pix_fmt", "yuv420p",
		"-c:a", "aac", "-b:a", "192k",
		"-metadata", "title=My Film", "-movflags", "+faststart", out,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("argv %q missing %q", got, want)
		}
	}
	if cmd.Argv()[0] != "ffmpeg" {
		t.Fatalf("binary must be first: %v", cmd.Argv())
	}
}

func TestStreamCopyWithFilterIsUpgradedToEncode(t *testing.T) {
	dir := t.TempDir()
	in := touch(t, dir, "in.mp4")
	b := NewBuilder("ffmpeg", in)
	b.VideoCodec("copy").AddVideoFilter("scale=1280:-2")
	cmd := b.Build(filepath.Join(dir, "out.mp4"))
	joined := strings.Join(cmd.Args, " ")
	if !strings.Contains(joined, "-c:v libx264") {
		t.Fatalf("copy plus filter must force an encoder, got: %s", joined)
	}
}

func TestValidateRejectsUnsafeOrInvalidPaths(t *testing.T) {
	dir := t.TempDir()
	in := touch(t, dir, "in.mp4")

	empty := Command{Bin: "ffmpeg"}
	if err := empty.Validate(); err == nil {
		t.Fatal("empty input must fail validation")
	}

	same := Command{Bin: "ffmpeg", Input: in, Output: in}
	if err := same.Validate(); err == nil || !strings.Contains(err.Error(), "same file") {
		t.Fatalf("expected same-file error, got %v", err)
	}

	missing := Command{Bin: "ffmpeg", Input: in, Output: filepath.Join(dir, "missing", "out.mp4")}
	if err := missing.Validate(); err == nil {
		t.Fatal("missing output directory must fail validation")
	}

	dirInput := Command{Bin: "ffmpeg", Input: dir, Output: filepath.Join(dir, "out.mp4")}
	if err := dirInput.Validate(); err == nil {
		t.Fatal("directory input must fail validation")
	}

	nul := Command{Bin: "ffmpeg", Input: in, Output: filepath.Join(dir, "o.mp4"), Args: []string{"-metadata", "x\x00y"}}
	if err := nul.Validate(); err == nil {
		t.Fatal("NUL bytes must be rejected")
	}

	good := Command{Bin: "ffmpeg", Input: in, Output: filepath.Join(dir, "ok.mp4")}
	if err := good.Validate(); err != nil {
		t.Fatalf("valid command rejected: %v", err)
	}
}

func TestHostileFilenamesRemainSingleArguments(t *testing.T) {
	dir := t.TempDir()
	name := "weird $(rm -rf .) 'quoted' ; name.mkv"
	in := touch(t, dir, name)
	b := NewBuilder("ffmpeg", in).VideoCodec("copy")
	cmd := b.Build(filepath.Join(dir, "out.mp4"))
	argv := cmd.Argv()
	found := false
	for _, a := range argv {
		if a == in {
			found = true
		}
	}
	if !found {
		t.Fatalf("hostile path was altered: %v", argv)
	}
	preview := cmd.Preview()
	if !strings.Contains(preview, "$(rm -rf .)") ||
		!strings.Contains(preview, `'\''quoted'\''`) ||
		!strings.Contains(preview, "-i '") {
		t.Fatalf("preview must quote the path unambiguously:\n%s", preview)
	}
	if !strings.HasSuffix(preview, filepath.Join(dir, "out.mp4")) &&
		!strings.Contains(preview, "out.mp4") {
		t.Fatalf("preview missing output: %s", preview)
	}
}

func TestFilterPathEscaping(t *testing.T) {
	got := escapeFilterPath(`C:\Movies\my film.srt`)
	want := `C\:\\Movies\\my film.srt`
	if got != want {
		t.Fatalf("escaping: got %q want %q", got, want)
	}
	b := NewBuilder("ffmpeg", "in.mkv")
	b.BurnSubtitles("a:b,c.srt")
	cmd := b.Build("out.mkv")
	joined := strings.Join(cmd.Args, " ")
	if !strings.Contains(joined, `subtitles=a\:b\,c.srt`) {
		t.Fatalf("subtitle filter not escaped: %s", joined)
	}
}

func TestExtraInputsAndMapOrder(t *testing.T) {
	b := NewBuilder("ffmpeg", "in.mkv")
	b.AddExtraInput("logo.png")
	b.SetFilterComplex("[1:v]scale=100:-1[wm];[0:v][wm]overlay=10:10[v]", "[v]")
	cmd := b.Build("out.mkv")
	joined := strings.Join(cmd.Args, " ")
	i := strings.Index(joined, "-i in.mkv")
	j := strings.Index(joined, "-i logo.png")
	k := strings.Index(joined, "-filter_complex")
	if !(i >= 0 && j > i && k > j) {
		t.Fatalf("inputs must precede filter_complex: %s", joined)
	}
	if !strings.Contains(joined, "-map [v]") {
		t.Fatalf("filtergraph output must be mapped: %s", joined)
	}
}

func TestPreviewWrappedKeepsFullArgv(t *testing.T) {
	c := Command{
		Bin:    "/usr/bin/ffmpeg",
		Args:   []string{"-hide_banner", "-i", "/tmp/a b's.mp4", "-c:v", "libx264", "-crf", "23", "-movflags", "+faststart"},
		Output: "/tmp/out file.mp4",
	}
	tokens := []string{
		"ffmpeg", "-hide_banner", "-i", Quote("/tmp/a b's.mp4"), "-c:v libx264",
		"-crf 23", "-movflags +faststart", Quote("/tmp/out file.mp4"),
	}
	wide := c.PreviewWrapped(1000)
	if got, want := strings.Join(wide, " "), strings.Join(tokens, " "); got != want {
		t.Fatalf("wide preview mismatch:\n got %q\nwant %q", got, want)
	}
	narrow := c.PreviewWrapped(40)
	joined := strings.Join(narrow, " ")
	for _, tok := range tokens {
		if !strings.Contains(joined, tok) {
			t.Errorf("narrow preview lost token %q: %q", tok, joined)
		}
	}
	for _, line := range narrow {
		if len([]rune(line)) > 40 {
			t.Errorf("line exceeds width: %q (%d runes)", line, len([]rune(line)))
		}
	}
	// Continuation lines are indented for readability but never split a value
	// away from its flag.
	if len(narrow) < 2 {
		t.Fatalf("expected wrapping into several lines, got %q", narrow)
	}
	for _, line := range narrow[1:] {
		if !strings.HasPrefix(line, "  ") {
			t.Errorf("continuation line not indented: %q", line)
		}
	}
}
