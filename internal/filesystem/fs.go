// Package filesystem contains the path handling and media classification used
// by the browser, the conversion wizard and batch processing. It performs no
// shell expansion and treats every name as opaque data.
package filesystem

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Kind classifies a filesystem entry by the media it holds.
type Kind int

const (
	KindOther Kind = iota
	KindDir
	KindVideo
	KindAudio
	KindImage
	KindSubtitle
)

var videoExts = map[string]bool{
	".mp4": true, ".mkv": true, ".mov": true, ".avi": true, ".webm": true,
	".m4v": true, ".mpg": true, ".mpeg": true, ".ts": true, ".m2ts": true,
	".flv": true, ".wmv": true, ".3gp": true, ".ogv": true, ".vob": true,
	".mxf": true, ".dv": true, ".y4m": true, ".gifv": true, ".mts": true,
}

var audioExts = map[string]bool{
	".mp3": true, ".aac": true, ".flac": true, ".ogg": true, ".opus": true,
	".wav": true, ".m4a": true, ".wma": true, ".aiff": true, ".aif": true,
	".alac": true, ".ape": true, ".mka": true, ".ac3": true, ".dts": true,
	".amr": true,
}

var imageExts = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".webp": true, ".avif": true,
	".bmp": true, ".tif": true, ".tiff": true, ".gif": true, ".heic": true,
}

var subtitleExts = map[string]bool{
	".srt": true, ".ass": true, ".ssa": true, ".vtt": true, ".sub": true,
	".sup": true, ".ttml": true,
}

// Entry is one item in a directory listing.
type Entry struct {
	Name    string
	Path    string
	IsDir   bool
	Size    int64
	ModTime time.Time
	Kind    Kind
	Ext     string
	// Err carries a stat error for unreadable entries rather than hiding them.
	Err error
}

// Classify maps a file extension (with or without dot) to a media Kind.
func Classify(ext string) Kind {
	ext = strings.ToLower(ext)
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	switch {
	case videoExts[ext]:
		return KindVideo
	case audioExts[ext]:
		return KindAudio
	case imageExts[ext]:
		return KindImage
	case subtitleExts[ext]:
		return KindSubtitle
	}
	return KindOther
}

// KindOf returns the media kind for a path.
func KindOf(path string) Kind {
	ext := strings.ToLower(filepath.Ext(path))
	k := Classify(ext)
	if k == KindOther {
		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".gif" {
			return KindImage
		}
	}
	return k
}

// IsMedia reports whether the path looks like a supported media file.
func IsMedia(path string) bool { return KindOf(path) != KindOther }

// List reads dir and returns sorted entries. Directories come first, then files
// ordered by name. Hidden entries are skipped unless showHidden is set.
func List(dir string, showHidden bool) ([]Entry, error) {
	dir = ExpandHome(dir)
	if dir == "" {
		dir = "."
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	des, err := os.ReadDir(abs)
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, 0, len(des))
	for _, de := range des {
		name := de.Name()
		if !showHidden && strings.HasPrefix(name, ".") {
			continue
		}
		e := Entry{Name: name, Path: filepath.Join(abs, name), IsDir: de.IsDir()}
		e.Ext = strings.ToLower(filepath.Ext(name))
		if e.IsDir {
			e.Kind = KindDir
		} else {
			e.Kind = Classify(e.Ext)
		}
		if info, err := de.Info(); err == nil {
			e.Size = info.Size()
			e.ModTime = info.ModTime()
		} else {
			e.Err = err
		}
		entries = append(entries, e)
	}
	Sort(entries, "name")
	return entries, nil
}

// Sort orders entries in place. Supported modes: name, size, date, kind.
func Sort(entries []Entry, mode string) {
	sort.SliceStable(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		if a.IsDir != b.IsDir {
			return a.IsDir
		}
		switch mode {
		case "size":
			if a.Size != b.Size {
				return a.Size > b.Size
			}
		case "date":
			if !a.ModTime.Equal(b.ModTime) {
				return a.ModTime.After(b.ModTime)
			}
		case "kind":
			if a.Kind != b.Kind {
				return a.Kind < b.Kind
			}
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})
}

// Parent returns the parent directory, or the same directory at the filesystem root.
func Parent(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return dir
	}
	parent := filepath.Dir(abs)
	if parent == abs {
		return abs
	}
	return parent
}

// ExpandHome expands a leading ~ to the user's home directory.
func ExpandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		if home, err := os.UserHomeDir(); err == nil {
			if len(p) == 1 {
				return home
			}
			return filepath.Join(home, p[2:])
		}
	}
	return p
}

// HumanSize formats a byte count using binary units.
func HumanSize(n int64) string {
	if n < 0 {
		return "?"
	}
	const unit = 1024
	if n < unit {
		return strconv.FormatInt(n, 10) + " B"
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

// HumanBitrate formats bits per second.
func HumanBitrate(bps int64) string {
	if bps <= 0 {
		return "?"
	}
	switch {
	case bps >= 1_000_000_000:
		return fmt.Sprintf("%.2f Gbps", float64(bps)/1e9)
	case bps >= 1_000_000:
		return fmt.Sprintf("%.2f Mbps", float64(bps)/1e6)
	case bps >= 1_000:
		return fmt.Sprintf("%.0f kbps", float64(bps)/1e3)
	}
	return strconv.FormatInt(bps, 10) + " bps"
}

// HumanTime renders a file modification time compactly.
func HumanTime(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.Format("2006-01-02 15:04")
}

// HumanDuration renders a media duration.
func HumanDuration(d time.Duration) string {
	if d <= 0 {
		return "—"
	}
	total := int64(d.Round(time.Second) / time.Second)
	h := total / 3600
	m := (total % 3600) / 60
	s := total % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// DefaultOutput derives an output path beside the input with a new extension.
func DefaultOutput(input, ext string) string {
	ext = strings.TrimPrefix(ext, ".")
	if ext == "" {
		ext = "mp4"
	}
	dir := filepath.Dir(input)
	base := strings.TrimSuffix(filepath.Base(input), filepath.Ext(input))
	return filepath.Join(dir, base+"."+ext)
}

// OutputInDir derives an output path with a new directory and extension.
func OutputInDir(input, dir, ext string) string {
	name := strings.TrimSuffix(filepath.Base(input), filepath.Ext(input)) + "." + strings.TrimPrefix(ext, ".")
	return filepath.Join(dir, name)
}

// UniquePath returns path, or path with a numeric suffix, when path exists.
func UniquePath(path string) string {
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		return path
	}
	dir := filepath.Dir(path)
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	ext := filepath.Ext(path)
	for i := 2; i < 10000; i++ {
		candidate := filepath.Join(dir, fmt.Sprintf("%s-%d%s", base, i, ext))
		if _, err := os.Stat(candidate); errors.Is(err, fs.ErrNotExist) {
			return candidate
		}
	}
	return path
}

// SanitizeName makes a filename safe on every supported platform.
func SanitizeName(name string) string {
	replacer := strings.NewReplacer(
		"/", "-", `\`, "-", ":", "-", "*", "-", "?", "", `"`, "", "<", "-",
		">", "-", "|", "-", "\x00", "",
	)
	out := strings.TrimSpace(replacer.Replace(name))
	out = strings.Trim(out, " .")
	if out == "" {
		out = "output"
	}
	if len(out) > 180 {
		out = out[:180]
	}
	return out
}

// TemplateOutput expands a batch template. Supported placeholders:
// {name} base name, {ext} extension without dot, {index} 1-based counter,
// {date} YYYY-MM-DD, {dir} parent directory name.
func TemplateOutput(input, template, outDir string, index int) string {
	base := strings.TrimSuffix(filepath.Base(input), filepath.Ext(input))
	ext := strings.TrimPrefix(filepath.Ext(input), ".")
	dirName := filepath.Base(filepath.Dir(input))
	repl := strings.NewReplacer(
		"{name}", SanitizeName(base),
		"{ext}", ext,
		"{index}", strconv.Itoa(index),
		"{date}", time.Now().Format("2006-01-02"),
		"{dir}", SanitizeName(dirName),
	)
	name := repl.Replace(template)
	if filepath.Ext(name) == "" && ext != "" {
		name += "." + ext
	}
	return filepath.Join(outDir, name)
}

// WalkMedia recursively collects media files under root.
func WalkMedia(root string, showHidden bool) ([]Entry, error) {
	root = ExpandHome(root)
	var out []Entry
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable subtrees are skipped, not fatal
		}
		name := d.Name()
		if !showHidden && strings.HasPrefix(name, ".") && path != root {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		k := KindOf(path)
		if k == KindOther || k == KindSubtitle || k == KindImage {
			return nil
		}
		e := Entry{Path: path, Name: name, Kind: k, Ext: strings.ToLower(filepath.Ext(name))}
		if info, err := d.Info(); err == nil {
			e.Size = info.Size()
			e.ModTime = info.ModTime()
		}
		out = append(out, e)
		return nil
	})
	Sort(out, "name")
	return out, err
}

// CompletePath returns directory entries matching a path prefix, for Tab
// completion in text inputs.
func CompletePath(prefix string) []string {
	prefix = ExpandHome(prefix)
	dir := filepath.Dir(prefix)
	base := filepath.Base(prefix)
	if strings.HasSuffix(prefix, string(os.PathSeparator)) {
		dir = prefix
		base = ""
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var matches []string
	for _, e := range entries {
		if !strings.HasPrefix(strings.ToLower(e.Name()), strings.ToLower(base)) {
			continue
		}
		p := filepath.Join(dir, e.Name())
		if e.IsDir() {
			p += string(os.PathSeparator)
		}
		matches = append(matches, p)
		if len(matches) >= 64 {
			break
		}
	}
	sort.Strings(matches)
	return matches
}

// EnsureDir creates dir (and parents) when missing.
func EnsureDir(dir string) error {
	if dir == "" {
		return errors.New("empty directory")
	}
	return os.MkdirAll(dir, 0o755)
}

// Mirror relocates a source path under a new root while preserving its tree.
func Mirror(srcRoot, dstRoot, srcFile string) string {
	rel, err := filepath.Rel(srcRoot, srcFile)
	if err != nil {
		rel = filepath.Base(srcFile)
	}
	return filepath.Join(dstRoot, rel)
}

// KindLabel returns a short label for a media kind.
func KindLabel(k Kind) string {
	switch k {
	case KindVideo:
		return "video"
	case KindAudio:
		return "audio"
	case KindImage:
		return "image"
	case KindSubtitle:
		return "subs"
	case KindDir:
		return "dir"
	}
	return "file"
}
