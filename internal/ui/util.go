package ui

import (
	"os"
	"path/filepath"
	"time"
)

// osStat is a tiny indirection so view code can check for existing files
// without importing os everywhere.
func osStat(path string) (os.FileInfo, error) { return os.Stat(path) }

// timeFromMillis converts a history duration into a time.Duration.
func timeFromMillis(ms int64) time.Duration { return time.Duration(ms) * time.Millisecond }

// sameFile compares two paths after cleaning, tolerating relative input.
func sameFile(a, b string) bool {
	aa, err1 := filepath.Abs(a)
	bb, err2 := filepath.Abs(b)
	if err1 != nil || err2 != nil {
		return a == b
	}
	return aa == bb
}

// listWindowStart returns the first visible row for a cursor inside a list of
// total rows, keeping the selection on screen. The caller renders rows
// [start, start+rows).
func listWindowStart(cursor, rows, total int) int {
	if rows < 1 {
		rows = 1
	}
	if total < 0 {
		total = 0
	}
	if cursor < 0 {
		cursor = 0
	}
	start := 0
	if cursor >= rows {
		start = cursor - rows + 1
	}
	if start > total-rows {
		start = total - rows
	}
	if start < 0 {
		start = 0
	}
	return start
}

// listRangeLabel describes a visible window for a panel title, e.g.
// "12-31 of 40". It returns "" when every row fits.
func listRangeLabel(start, end, total int) string {
	if total <= 0 || end-start >= total {
		return ""
	}
	return itoa(start+1) + "-" + itoa(end) + " of " + itoa(total)
}
