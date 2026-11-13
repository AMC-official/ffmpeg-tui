package filters

import (
	"strings"
	"testing"
)

func TestCropRendersDefaults(t *testing.T) {
	def, ok := ByName("crop")
	if !ok {
		t.Fatal("crop filter missing")
	}
	sel := NewSelected(def)
	if got := sel.Render(); got != "crop=1280:720:(iw-ow)/2:(ih-oh)/2" {
		t.Fatalf("crop render: %s", got)
	}
	if err := sel.Set("w", "640"); err != nil {
		t.Fatal(err)
	}
	if got := sel.Render(); !strings.HasPrefix(got, "crop=640:") {
		t.Fatalf("crop after edit: %s", got)
	}
	if err := sel.Set("w", "abc"); err == nil {
		t.Fatal("non-numeric width must be rejected")
	}
}

func TestChainBuildsVF(t *testing.T) {
	scale, _ := ByName("scale")
	contrast, _ := ByName("contrast")
	chain := []Selected{NewSelected(scale), NewSelected(contrast)}
	g := BuildGraph(chain)
	if strings.Contains(g.VF, ";") {
		t.Fatalf("simple chain must not become complex: %s", g.VF)
	}
	if !strings.HasPrefix(g.VF, "scale=") || !strings.Contains(g.VF, "eq=contrast=") {
		t.Fatalf("vf: %s", g.VF)
	}
	if g.AF != "" {
		t.Fatalf("unexpected audio chain: %s", g.AF)
	}
}

func TestAudioOnlyFilter(t *testing.T) {
	def, ok := ByName("normalize")
	if !ok {
		t.Fatal("normalize filter missing")
	}
	sel := NewSelected(def)
	g := BuildGraph([]Selected{sel})
	if g.VF != "" {
		t.Fatalf("audio-only filter must not touch video: %q", g.VF)
	}
	if !strings.Contains(g.AF, "loudnorm=I=-16") {
		t.Fatalf("af: %q", g.AF)
	}
}

func TestComplexOverlayGraph(t *testing.T) {
	def, ok := ByName("watermark")
	if !ok {
		t.Fatal("watermark filter missing")
	}
	sel := NewSelected(def)
	sel.ExtraInput = "logo.png"
	chain := []Selected{sel}

	scale, _ := ByName("scale")
	chain = append([]Selected{NewSelected(scale)}, chain...)
	g := BuildGraph(chain)
	if g.Complex == "" {
		t.Fatal("overlay must produce a filter_complex graph")
	}
	if !strings.Contains(g.Complex, "[1:v]") {
		t.Fatalf("second input missing: %s", g.Complex)
	}
	if !strings.Contains(g.Complex, "overlay=") {
		t.Fatalf("overlay missing: %s", g.Complex)
	}
	if g.MapOut == "" || !strings.HasPrefix(g.MapOut, "[") {
		t.Fatalf("map label missing: %q", g.MapOut)
	}
	if len(g.ExtraInputs) != 1 || g.ExtraInputs[0] != "logo.png" {
		t.Fatalf("extra inputs: %v", g.ExtraInputs)
	}
	if strings.Contains(g.Complex, "%") {
		t.Fatalf("unexpanded placeholder: %s", g.Complex)
	}
	// The simple scale must be flushed into a labelled segment before overlay.
	if !strings.Contains(g.Complex, "scale=") {
		t.Fatalf("simple filter lost when complex filter present: %s", g.Complex)
	}
}

func TestAllDefinitionsAreWellFormed(t *testing.T) {
	names := map[string]bool{}
	for _, d := range All() {
		if d.Name == "" || d.Summary == "" || d.Category == "" {
			t.Fatalf("incomplete definition: %+v", d)
		}
		if names[d.Name] {
			t.Fatalf("duplicate filter %q", d.Name)
		}
		names[d.Name] = true
		if d.Template == "" && d.AudioTemplate == "" && d.ComplexTpl == "" {
			t.Fatalf("%s has no template", d.Name)
		}
		if d.ComplexTpl != "" && d.NeedsInput && !strings.Contains(d.ComplexTpl, "%IDX%") {
			t.Fatalf("%s needs an input index placeholder", d.Name)
		}
		for _, a := range d.Args {
			if a.Key == "" || a.Label == "" {
				t.Fatalf("%s has an incomplete argument", d.Name)
			}
			if strings.Contains(d.Template, "%"+a.Key+"%") == false && a.Kind != ArgFile {
				// Arguments may legitimately be audio-only; just report missing
				// placeholders in neither template.
				if !strings.Contains(d.AudioTemplate+d.ComplexTpl, "%"+a.Key+"%") {
					t.Logf("%s argument %s is not used by its template", d.Name, a.Key)
				}
			}
		}
	}
	for _, required := range []string{
		"crop", "scale", "rotate", "hflip", "vflip", "transpose", "fps", "pad",
		"brightness", "contrast", "gamma", "sharpen", "blur", "noise",
		"watermark", "subtitles", "overlay", "hstack", "vstack", "split",
		"trim", "fade in", "fade out", "speed", "normalize",
	} {
		if !names[required] {
			t.Fatalf("required filter missing: %s", required)
		}
	}
}
