package manpages

import (
	"strings"
	"testing"
)

func TestSearchNormalizeFindsLoudnessFilters(t *testing.T) {
	hits := Search("normalize", 0)
	if len(hits) < 3 {
		t.Fatalf("expected several results, got %d", len(hits))
	}
	want := map[string]bool{"loudnorm": false, "dynaudnorm": false, "volume": false}
	for _, h := range hits {
		if _, ok := want[h.Entry.Name]; ok {
			want[h.Entry.Name] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("search normalize did not surface %s (top results: %s)", name, topNames(hits, 6))
		}
	}
	if hits[0].Entry.Category != CatFilter {
		t.Errorf("normalize should rank filters first, got %s", hits[0].Entry.Category)
	}
}

func topNames(hits []Hit, n int) string {
	var names []string
	for i, h := range hits {
		if i >= n {
			break
		}
		names = append(names, h.Entry.Name)
	}
	return strings.Join(names, ", ")
}

func TestFindByCategoryAndName(t *testing.T) {
	cases := map[string]Category{
		"crop":    CatFilter,
		"libx264": CatEncoder,
		"mp4":     CatMuxer,
		"ffmpeg":  CatTool,
		"-crf":    CatOption,
	}
	for name, cat := range cases {
		e, ok := Find(name)
		if !ok {
			t.Fatalf("%s not found", name)
		}
		if e.Category != cat {
			t.Fatalf("%s: got category %s want %s", name, e.Category, cat)
		}
	}
	if e, ok := Find("filter:crop"); !ok || e.Name != "crop" {
		t.Fatalf("qualified lookup failed: %v %v", e, ok)
	}
	if _, ok := Find("definitely-not-a-page"); ok {
		t.Fatal("unknown page must not resolve")
	}
}

func TestFuzzySubsequenceMatch(t *testing.T) {
	hits := Search("loudnrm", 10)
	if len(hits) == 0 || hits[0].Entry.Name != "loudnorm" {
		t.Fatalf("fuzzy subsequence should find loudnorm: %s", topNames(hits, 3))
	}
}

func TestIndexCoverage(t *testing.T) {
	counts := Counts()
	for _, cat := range Categories {
		if counts[cat] == 0 {
			t.Errorf("no pages for category %s", cat)
		}
	}
	if counts[CatFilter] < 20 {
		t.Errorf("filter documentation too thin: %d", counts[CatFilter])
	}
	for _, e := range All() {
		if e.Summary == "" || e.Description == "" {
			t.Errorf("%s has no prose", e.Name)
		}
		if len(e.Examples) == 0 && (e.Category == CatFilter || e.Category == CatEncoder || e.Category == CatMuxer) {
			t.Errorf("%s has no examples", e.Name)
		}
		if len(e.Links) == 0 {
			t.Errorf("%s has no links", e.Name)
		}
	}
}

func TestCategoryFilterKeepsTagMatches(t *testing.T) {
	hits := Search("normalize", 0)
	var filters int
	for _, h := range hits {
		if h.Entry.Category == CatFilter {
			filters++
		}
	}
	if filters < 3 {
		t.Fatalf("expected at least three filter matches, got %d", filters)
	}
	hits = Search("metadata", 0)
	var categories = map[Category]bool{}
	for _, h := range hits {
		categories[h.Entry.Category] = true
	}
	if !categories[CatMuxer] && !categories[CatOption] {
		t.Fatalf("metadata should surface muxer or option pages: %s", topNames(hits, 6))
	}
}
