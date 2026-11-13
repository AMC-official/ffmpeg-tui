package presets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ffmpeg-tui/ffmpeg-tui/internal/ffmpeg"
)

func TestAllRequiredPresetsExist(t *testing.T) {
	required := []string{
		"Lossless", "H264", "H265", "AV1", "VP9", "Discord", "YouTube", "Twitch",
		"Blu-ray", "Podcast", "Audiobook", "Phone", "Archival", "Animation", "GIF", "Custom",
	}
	names := map[string]bool{}
	for _, p := range Builtins() {
		if names[p.Name] {
			t.Fatalf("duplicate preset %s", p.Name)
		}
		names[p.Name] = true
		if p.Description == "" || p.Category == "" || p.Container == "" {
			t.Fatalf("preset %s is incomplete", p.Name)
		}
	}
	for _, r := range required {
		if !names[r] {
			t.Errorf("missing preset %s", r)
		}
	}
}

func TestH264PresetAppliesExpectedArguments(t *testing.T) {
	p, ok := ByName("H264", nil)
	if !ok {
		t.Fatal("H264 missing")
	}
	dir := t.TempDir()
	in := filepath.Join(dir, "in.mkv")
	if err := os.WriteFile(in, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	b := ffmpeg.NewBuilder("ffmpeg", in)
	p.Apply(b)
	cmd := b.Build(filepath.Join(dir, "out.mp4"))
	joined := strings.Join(cmd.Args, " ")
	for _, want := range []string{"-c:v libx264", "-crf 20", "-preset medium", "-c:a aac", "-b:a 192k", "-movflags +faststart", "-pix_fmt yuv420p"} {
		if !strings.Contains(joined, want) {
			t.Errorf("H264 command missing %q: %s", want, joined)
		}
	}
	if cmd.Label == "" {
		t.Error("preset should label the command")
	}
}

func TestAudioOnlyPresetsDisableVideo(t *testing.T) {
	p, _ := ByName("Podcast", nil)
	b := ffmpeg.NewBuilder("ffmpeg", "in.mkv")
	p.Apply(b)
	cmd := b.Build("out.mp3")
	joined := strings.Join(cmd.Args, " ")
	if !strings.Contains(joined, "-vn") {
		t.Fatalf("audio-only preset must disable video: %s", joined)
	}
	if !strings.Contains(joined, "loudnorm") {
		t.Fatalf("podcast preset should normalise loudness: %s", joined)
	}
	if !strings.Contains(joined, "-c:a libmp3lame") {
		t.Fatalf("podcast preset should encode MP3: %s", joined)
	}
}

func TestContainersCoverRequestedOutputs(t *testing.T) {
	for _, ext := range []string{"mp4", "mkv", "mov", "avi", "webm", "mp3", "aac", "flac", "ogg", "wav", "gif", "png", "jpg", "webp", "avif"} {
		if _, ok := ContainerByExt(ext); !ok {
			t.Errorf("missing container %s", ext)
		}
	}
	if _, ok := ContainerByExt("MP4"); !ok {
		t.Error("container lookup must be case-insensitive")
	}
}

func TestToolsHaveCompleteRecipes(t *testing.T) {
	seen := map[string]bool{}
	for _, tool := range Tools() {
		if tool.Name == "" || tool.Category == "" || tool.Description == "" {
			t.Fatalf("incomplete tool: %+v", tool)
		}
		if seen[tool.Name] {
			t.Fatalf("duplicate tool %s", tool.Name)
		}
		seen[tool.Name] = true
		if tool.Container == "" {
			t.Errorf("%s has no container", tool.Name)
		}
		if tool.Preset != "" {
			if _, ok := ByName(tool.Preset, nil); !ok {
				t.Errorf("%s references unknown preset %s", tool.Name, tool.Preset)
			}
		}
	}
}
