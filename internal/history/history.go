// Package history records every command the application executes, so that work
// can be replayed, duplicated and exported without leaving the terminal.
package history

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ffmpeg-tui/ffmpeg-tui/internal/ffmpeg"
)

// Entry is one executed command.
type Entry struct {
	ID         string    `json:"id"`
	Time       time.Time `json:"time"`
	Input      string    `json:"input"`
	Output     string    `json:"output"`
	Label      string    `json:"label,omitempty"`
	Args       []string  `json:"args"`
	Binary     string    `json:"binary,omitempty"`
	Status     string    `json:"status"`
	ExitCode   int       `json:"exit_code"`
	DurationMS int64     `json:"duration_ms"`
	Error      string    `json:"error,omitempty"`
}

// Command converts the entry back into an executable command.
func (e Entry) Command() ffmpeg.Command {
	bin := e.Binary
	if bin == "" {
		bin = "ffmpeg"
	}
	return ffmpeg.Command{Bin: bin, Args: append([]string(nil), e.Args...), Input: e.Input, Output: e.Output, Label: e.Label}
}

// FromCommand builds a history entry for a command.
func FromCommand(c ffmpeg.Command, start time.Time, status string, exitCode int, errMsg string) Entry {
	return Entry{
		ID:         fmt.Sprintf("%d", time.Now().UnixNano()),
		Time:       start,
		Input:      c.Input,
		Output:     c.Output,
		Label:      c.Label,
		Args:       c.ArgsNoBin(),
		Binary:     c.Bin,
		Status:     status,
		ExitCode:   exitCode,
		DurationMS: time.Since(start).Milliseconds(),
		Error:      errMsg,
	}
}

// Store is an append-only JSONL history file.
type Store struct {
	path    string
	limit   int
	entries []Entry
	loadErr error
}

// Open loads the history file at path. A missing file is not an error.
func Open(path string, limit int) *Store {
	s := &Store{path: path, limit: limit}
	if limit <= 0 {
		s.limit = 500
	}
	s.loadErr = s.load()
	s.trim()
	return s
}

// Err reports a problem encountered while loading (corrupt lines are skipped).
func (s *Store) Err() error { return s.loadErr }

func (s *Store) load() error {
	f, err := os.Open(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	var skipped int
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var e Entry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			skipped++
			continue
		}
		s.entries = append(s.entries, e)
	}
	if err := sc.Err(); err != nil {
		return err
	}
	if skipped > 0 {
		return fmt.Errorf("%d unreadable history entries skipped", skipped)
	}
	return nil
}

// Add appends an entry and persists it. Older entries beyond the limit are
// dropped from the in-memory view and the file is rewritten.
func (s *Store) Add(e Entry) error {
	s.entries = append([]Entry{e}, s.entries...)
	trimmed := s.trim()
	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	data, err := json.Marshal(e)
	if err != nil {
		f.Close()
		return err
	}
	_, werr := f.Write(append(data, '\n'))
	cerr := f.Close()
	if werr != nil {
		return werr
	}
	if cerr != nil {
		return cerr
	}
	if trimmed {
		return s.rewrite()
	}
	return nil
}

// Entries returns a copy of the history, newest first.
func (s *Store) Entries() []Entry {
	out := make([]Entry, len(s.entries))
	copy(out, s.entries)
	return out
}

// Len returns the number of recorded commands.
func (s *Store) Len() int { return len(s.entries) }

// Delete removes an entry by ID and rewrites the file.
func (s *Store) Delete(id string) error {
	out := s.entries[:0]
	for _, e := range s.entries {
		if e.ID != id {
			out = append(out, e)
		}
	}
	s.entries = out
	return s.rewrite()
}

// Clear empties the history.
func (s *Store) Clear() error {
	s.entries = nil
	return s.rewrite()
}

// Export writes every entry as a shell script.
func (s *Store) Export(path string) error {
	var b strings.Builder
	b.WriteString("#!/bin/sh\n")
	b.WriteString("# FFmpeg-TUI command history\n")
	b.WriteString("# generated " + time.Now().Format(time.RFC3339) + "\n\n")
	for i := len(s.entries) - 1; i >= 0; i-- {
		e := s.entries[i]
		b.WriteString("# " + e.Time.Format("2006-01-02 15:04:05") + " " + e.Status + "\n")
		b.WriteString(e.Command().OneLine() + "\n\n")
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func (s *Store) trim() bool {
	if len(s.entries) <= s.limit {
		return false
	}
	s.entries = s.entries[:s.limit]
	return true
}

func (s *Store) rewrite() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(f)
	for i := len(s.entries) - 1; i >= 0; i-- {
		data, err := json.Marshal(s.entries[i])
		if err != nil {
			continue
		}
		w.Write(data)
		w.WriteByte('\n')
	}
	if err := w.Flush(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// Search filters entries by input, output or command text.
func Search(entries []Entry, query string) []Entry {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return entries
	}
	var out []Entry
	for _, e := range entries {
		if strings.Contains(strings.ToLower(e.Command().OneLine()), q) {
			out = append(out, e)
		}
	}
	return out
}

// Inputs returns the distinct inputs in most-recent-first order, the basis of
// the "recent projects" list.
func Inputs(entries []Entry, max int) []Entry {
	seen := map[string]bool{}
	var out []Entry
	for _, e := range entries {
		if e.Input == "" || seen[e.Input] {
			continue
		}
		seen[e.Input] = true
		out = append(out, e)
		if max > 0 && len(out) >= max {
			break
		}
	}
	return out
}

// SortByTime orders entries newest first.
func SortByTime(entries []Entry) {
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Time.After(entries[j].Time) })
}
