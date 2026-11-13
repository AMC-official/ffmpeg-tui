// Package metadata edits container tags without losing the originals.
package metadata

import (
	"sort"
	"strings"

	"github.com/AMC-official/ffmpeg-tui/internal/ffmpeg"
)

// Field is one editable tag.
type Field struct {
	Key   string
	Label string
}

// Fields are the tags exposed by the metadata editor, in display order.
var Fields = []Field{
	{"title", "Title"},
	{"artist", "Artist"},
	{"album", "Album"},
	{"album_artist", "Album artist"},
	{"genre", "Genre"},
	{"track", "Track"},
	{"disc", "Disc"},
	{"date", "Year / date"},
	{"comment", "Comment"},
	{"copyright", "Copyright"},
	{"language", "Language"},
	{"encoder", "Encoder"},
}

// Values holds the current editor state keyed by metadata key.
type Values map[string]string

// New returns a populated editor state for a file's existing tags.
func New(tags map[string]string) Values {
	v := Values{}
	for _, f := range Fields {
		if val, ok := tags[f.Key]; ok {
			v[f.Key] = sanitize(val)
		} else {
			v[f.Key] = ""
		}
	}
	return v
}

// FromExisting preserves tags that are not part of the editor form.
func FromExisting(tags map[string]string) Values {
	v := Values{}
	keys := make([]string, 0, len(tags))
	for k := range tags {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v[strings.ToLower(k)] = sanitize(tags[k])
	}
	return v
}

// Set updates a key.
func (v Values) Set(key, val string) { v[strings.ToLower(key)] = sanitize(val) }

// Get returns a key's value.
func (v Values) Get(key string) string { return v[strings.ToLower(key)] }

// Label returns the human label for a key.
func Label(key string) string {
	for _, f := range Fields {
		if f.Key == key {
			return f.Label
		}
	}
	return key
}

// Pairs returns non-empty metadata as ordered key/value pairs.
func (v Values) Pairs() []ffmpeg.KV {
	out := make([]ffmpeg.KV, 0, len(v))
	for _, f := range Fields {
		if val := strings.TrimSpace(v[f.Key]); val != "" {
			out = append(out, ffmpeg.KV{Key: f.Key, Value: val})
		}
	}
	// Keep any non-standard key the source file had.
	known := map[string]bool{}
	for _, f := range Fields {
		known[f.Key] = true
	}
	var extra []string
	for k := range v {
		if !known[k] && strings.TrimSpace(v[k]) != "" {
			extra = append(extra, k)
		}
	}
	sort.Strings(extra)
	for _, k := range extra {
		out = append(out, ffmpeg.KV{Key: k, Value: v[k]})
	}
	return out
}

// Differences reports which keys differ between the source tags and values.
func (v Values) Differences(source map[string]string) []ffmpeg.KV {
	var out []ffmpeg.KV
	for _, kv := range v.Pairs() {
		if source[kv.Key] != kv.Value {
			out = append(out, kv)
		}
	}
	return out
}

// sanitize removes control characters that would corrupt a metadata field.
func sanitize(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r == '\n' || r == '\t' {
			b.WriteRune(' ')
			continue
		}
		if r < 32 || r == 127 {
			continue
		}
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}
