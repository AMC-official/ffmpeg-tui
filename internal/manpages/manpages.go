// Package manpages is FFmpeg-TUI's built-in, searchable documentation index.
//
// The index is compiled into the binary so it works offline. When ffmpeg or
// man(1) are available the UI can additionally fetch live help or open the real
// manual page, but the built-in pages remain the source of truth for the
// examples and option explanations shown next to the editor.
package manpages

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"time"
	"unicode"
)

// Category groups documentation entries.
type Category string

const (
	CatFilter   Category = "filter"
	CatEncoder  Category = "encoder"
	CatMuxer    Category = "muxer"
	CatDemuxer  Category = "demuxer"
	CatProtocol Category = "protocol"
	CatTool     Category = "tool"
	CatOption   Category = "option"
)

// Categories in display order.
var Categories = []Category{CatFilter, CatEncoder, CatMuxer, CatDemuxer, CatProtocol, CatTool, CatOption}

// ArgDoc documents one option or argument.
type ArgDoc struct {
	Name string
	Desc string
}

// Entry is a documentation page.
type Entry struct {
	Name        string
	Category    Category
	Summary     string
	Description string
	Args        []ArgDoc
	Examples    []string
	Links       []string
	Tags        []string
	Related     []string
}

// CategoryLabel returns a singular human label.
func CategoryLabel(c Category) string {
	switch c {
	case CatFilter:
		return "filter"
	case CatEncoder:
		return "encoder"
	case CatMuxer:
		return "muxer"
	case CatDemuxer:
		return "demuxer"
	case CatProtocol:
		return "protocol"
	case CatTool:
		return "tool"
	case CatOption:
		return "option"
	}
	return string(c)
}

// PluralLabel returns a plural label for group headers.
func PluralLabel(c Category) string {
	switch c {
	case CatFilter:
		return "Filters"
	case CatEncoder:
		return "Encoders"
	case CatMuxer:
		return "Muxers"
	case CatDemuxer:
		return "Demuxers"
	case CatProtocol:
		return "Protocols"
	case CatTool:
		return "Tools"
	case CatOption:
		return "Options"
	}
	return string(c)
}

// Hit is a ranked search result.
type Hit struct {
	Entry     Entry
	Score     int
	MatchedOn string
}

// All returns every documentation entry.
func All() []Entry {
	out := make([]Entry, 0, len(filters)+len(encoders)+len(muxers)+len(demuxers)+len(protocols)+len(tools)+len(options))
	out = append(out, filters...)
	out = append(out, encoders...)
	out = append(out, muxers...)
	out = append(out, demuxers...)
	out = append(out, protocols...)
	out = append(out, tools...)
	out = append(out, options...)
	return out
}

// Find looks an entry up by name, accepting category prefixes such as
// "filter:crop" or "encoder=libx264".
func Find(name string) (Entry, bool) {
	raw := strings.TrimSpace(name)
	lower := strings.ToLower(raw)
	if cat, rest, ok := splitQualified(lower); ok {
		for _, e := range All() {
			if string(e.Category) == cat && strings.EqualFold(e.Name, rest) {
				return e, true
			}
		}
	}
	for _, e := range All() {
		if strings.EqualFold(e.Name, raw) {
			return e, true
		}
	}
	// Allow ":man normalize" to resolve to the loudnorm page through tags.
	for _, e := range All() {
		for _, t := range e.Tags {
			if strings.EqualFold(t, raw) {
				return e, true
			}
		}
	}
	return Entry{}, false
}

func splitQualified(s string) (cat, rest string, ok bool) {
	for _, sep := range []string{":", "=", "/"} {
		if i := strings.Index(s, sep); i > 0 {
			c := strings.TrimSpace(s[:i])
			switch Category(c) {
			case CatFilter, CatEncoder, CatMuxer, CatDemuxer, CatProtocol, CatTool, CatOption:
				return c, strings.TrimSpace(s[i+1:]), true
			}
		}
	}
	return "", "", false
}

// Search ranks entries against a fuzzy query. Name matches outrank tag
// matches, which outrank description matches, so "/normalize" surfaces
// loudnorm, dynaudnorm and volume before unrelated filters.
func Search(query string, limit int) []Hit {
	q := strings.TrimSpace(strings.ToLower(query))
	if q == "" {
		entries := All()
		sort.SliceStable(entries, func(i, j int) bool {
			if entries[i].Category != entries[j].Category {
				return categoryOrder(entries[i].Category) < categoryOrder(entries[j].Category)
			}
			return entries[i].Name < entries[j].Name
		})
		hits := make([]Hit, 0, len(entries))
		for _, e := range entries {
			hits = append(hits, Hit{Entry: e, Score: 0})
		}
		if limit > 0 && len(hits) > limit {
			hits = hits[:limit]
		}
		return hits
	}

	var hits []Hit
	for _, e := range All() {
		score, matched := scoreEntry(q, e)
		if score > 0 {
			hits = append(hits, Hit{Entry: e, Score: score, MatchedOn: matched})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		if hits[i].Entry.Category != hits[j].Entry.Category {
			return categoryOrder(hits[i].Entry.Category) < categoryOrder(hits[j].Entry.Category)
		}
		return hits[i].Entry.Name < hits[j].Entry.Name
	})
	if limit > 0 && len(hits) > limit {
		hits = hits[:limit]
	}
	return hits
}

func categoryOrder(c Category) int {
	for i, cc := range Categories {
		if cc == c {
			return i
		}
	}
	return len(Categories)
}

func scoreEntry(q string, e Entry) (int, string) {
	name := strings.ToLower(e.Name)
	best, matched := 0, ""
	if name == q {
		return 1200, "name"
	}
	if strings.HasPrefix(name, q) {
		return 1000 - len(name), "name"
	}
	if strings.Contains(name, q) {
		return 850 - len(name), "name"
	}
	if s, ok := fuzzyScore(q, name); ok {
		best = 600 + s
		matched = "name"
	}
	for _, t := range e.Tags {
		tl := strings.ToLower(t)
		if tl == q {
			if 760 > best {
				best, matched = 760, "tag:"+t
			}
			continue
		}
		if strings.Contains(tl, q) {
			if 700 > best {
				best, matched = 700, "tag:"+t
			}
		}
	}
	if s, ok := fuzzyScore(q, strings.ToLower(e.Summary)); ok && s > 0 {
		if 400+s > best {
			best, matched = 400+s, "summary"
		}
	}
	if strings.Contains(strings.ToLower(e.Description), q) && best == 0 {
		best, matched = 250, "description"
	}
	return best, matched
}

// fuzzyScore reports whether all query runes appear in order in target and
// returns a score that rewards closeness and consecutive matches.
func fuzzyScore(query, target string) (int, bool) {
	if query == "" {
		return 0, true
	}
	if !isASCII(query) || !isASCII(target) {
		return 0, strings.Contains(target, query)
	}
	qr := []rune(query)
	tr := []rune(target)
	qi := 0
	score := 0
	lastIdx := -2
	for ti, r := range tr {
		if qi >= len(qr) {
			break
		}
		if r != qr[qi] {
			continue
		}
		score += 10
		if ti == lastIdx+1 {
			score += 15
		}
		if ti == 0 {
			score += 20
		}
		lastIdx = ti
		qi++
	}
	if qi < len(qr) {
		return 0, false
	}
	score -= len(tr) / 4
	if score < 0 {
		score = 1
	}
	return score, true
}

func isASCII(s string) bool {
	for _, r := range s {
		if r > unicode.MaxASCII {
			return false
		}
	}
	return true
}

// CategoryEntries returns all entries of one category.
func CategoryEntries(c Category) []Entry {
	var out []Entry
	for _, e := range All() {
		if e.Category == c {
			out = append(out, e)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Live fetches the authoritative help text from the installed ffmpeg binary,
// e.g. "ffmpeg -h filter=crop". It is used on demand only.
func Live(ctx context.Context, bin string, cat Category, name string) (string, error) {
	if bin == "" {
		bin = "ffmpeg"
	}
	var args []string
	switch cat {
	case CatFilter, CatEncoder, CatDecoder, CatMuxer, CatDemuxer:
		args = []string{"-hide_banner", "-h", string(cat) + "=" + name}
	case CatTool:
		args = []string{"-hide_banner", "-h"}
	default:
		args = []string{"-hide_banner", "-h", "full"}
	}
	if _, err := exec.LookPath(bin); err != nil {
		return "", fmt.Errorf("ffmpeg not available: %w", err)
	}
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(cctx, bin, args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("ffmpeg help failed: %w", err)
	}
	text := string(out)
	if len(text) > 20000 {
		text = text[:20000] + "\n… truncated"
	}
	return text, nil
}

// ManCommand returns the command that opens the system manual page, or nil
// when man(1) is not available.
func ManCommand(name string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return nil
	}
	if _, err := exec.LookPath("man"); err != nil {
		return nil
	}
	target := name
	switch name {
	case "ffmpeg", "ffprobe", "ffplay":
		target = name
	case "ffmpeg-filters":
		target = "ffmpeg-filters"
	default:
		// Filters, encoders and muxers are documented inside the main page.
		if e, ok := Find(name); ok {
			switch e.Category {
			case CatFilter:
				target = "ffmpeg-filters"
			case CatEncoder:
				target = "ffmpeg-codecs"
			case CatMuxer, CatDemuxer, CatProtocol:
				target = "ffmpeg-formats"
			default:
				target = "ffmpeg"
			}
		} else {
			target = "ffmpeg"
		}
	}
	return exec.Command("man", target)
}

// CatDecoder exists so Live can describe decoder help without inventing a
// separate category in the index.
const CatDecoder Category = "decoder"

// Counts returns the number of entries per category.
func Counts() map[Category]int {
	m := map[Category]int{}
	for _, e := range All() {
		m[e.Category]++
	}
	return m
}
