package ui

import (
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/AMC-official/ffmpeg-tui/internal/filesystem"
	"github.com/AMC-official/ffmpeg-tui/internal/metadata"
)

func (m *Model) updateWizard(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	w := &m.wiz
	key := msg.String()

	if key == "esc" {
		if w.step == int(stepInput) {
			m.back()
			return m, nil
		}
		w.prev()
		return m, nil
	}

	switch wizStep(w.step) {
	case stepInput:
		switch key {
		case "up", "k":
			w.entryCursor--
		case "down", "j":
			w.entryCursor++
		case "pgup":
			w.entryCursor -= 10
		case "pgdown":
			w.entryCursor += 10
		case "home", "g":
			w.entryCursor = 0
		case "end", "G":
			w.entryCursor = len(w.entries) - 1
		case "left", "h", "backspace":
			w.loadEntries(filesystem.Parent(w.browseDir), m.showHidden)
			return m, nil
		case "right", "l":
			if e, ok := w.entryAtCursor(); ok && e.IsDir {
				w.loadEntries(e.Path, m.showHidden)
			}
			return m, nil
		case "enter":
			if _, ok := w.entryAtCursor(); !ok {
				if strings.TrimSpace(w.input) == "" {
					return m, m.flash("choose a file or press / to type a path")
				}
				w.step = int(stepOutput)
				if m.ffprobeOK {
					return m, m.requestProbe(w.input)
				}
				return m, nil
			}
			done, _ := w.selectEntry()
			if done {
				if m.ffprobeOK {
					w.step = int(stepOutput)
					return m, m.requestProbe(w.input)
				}
				w.next()
			}
			return m, nil
		case "/", "e":
			return m, m.startEdit(editTarget{kind: "wizard-input"}, w.input, "path to input file")
		}
		w.entryCursor = clampInt(w.entryCursor, 0, max(0, len(w.entries)-1))
		return m, nil

	case stepOutput:
		switch key {
		case "enter":
			if strings.TrimSpace(w.output) == "" {
				w.buildOutput(w.input, w.probe)
			}
			if strings.TrimSpace(w.output) == "" {
				return m, m.flash("✖ choose an output path")
			}
			w.next()
			return m, nil
		case "e", "ctrl+e":
			return m, m.startEdit(editTarget{kind: "wizard-output"}, w.output, "path to output file")
		case "tab":
			w.output = completeValue(w.output)
			w.outputAuto = false
			return m, nil
		case "n":
			w.next()
			return m, nil
		}
		return m, nil

	case stepMetadata:
		switch key {
		case "up", "k":
			w.metaCursor = clampInt(w.metaCursor-1, 0, len(metadata.Fields)-1)
		case "down", "j":
			w.metaCursor = clampInt(w.metaCursor+1, 0, len(metadata.Fields)-1)
		case "enter", "e":
			f := metadata.Fields[clampInt(w.metaCursor, 0, len(metadata.Fields)-1)]
			return m, m.startEdit(editTarget{kind: "wizard-meta", key: f.Key}, w.meta.Get(f.Key), f.Label)
		case "tab":
			w.metaCursor = (w.metaCursor + 1) % len(metadata.Fields)
			return m, nil
		case "n", "right":
			w.next()
			return m, nil
		case "backspace", "delete":
			f := metadata.Fields[clampInt(w.metaCursor, 0, len(metadata.Fields)-1)]
			w.meta.Set(f.Key, "")
			return m, nil
		}
		return m, nil

	case stepFilters:
		switch key {
		case "f", "enter":
			m.flt.sync(m.wiz)
			m.stack = append(m.stack, m.screen)
			m.screen = screenFilters
			return m, nil
		case "d", "backspace":
			if len(w.filters) > 0 {
				w.filters = w.filters[:len(w.filters)-1]
			}
			return m, nil
		case "n", "right":
			w.next()
			return m, nil
		}
		return m, nil

	case stepExtraInput:
		switch key {
		case "enter":
			if strings.TrimSpace(w.extraInput) == "" {
				return m, m.startEdit(editTarget{kind: "wizard-extra"}, "", extraInputPlaceholder(w))
			}
			w.next()
			return m, nil
		case "e":
			return m, m.startEdit(editTarget{kind: "wizard-extra"}, w.extraInput, extraInputPlaceholder(w))
		case "tab":
			w.extraInput = completeValue(w.extraInput)
			return m, nil
		}
		return m, nil

	case stepConfirm:
		switch key {
		case "enter", "ctrl+s":
			return m, m.enqueueWizard()
		case "e":
			return m, m.startEdit(editTarget{kind: "wizard-output"}, w.output, "path to output file")
		case "left", "h":
			w.prev()
			return m, nil
		}
		return m, nil
	}

	// Option-based steps.
	opts, current, ok := w.stepOptions()
	if !ok {
		if key == "enter" || key == "n" {
			w.next()
		}
		return m, nil
	}
	switch key {
	case "up", "left", "k", "h":
		w.setOption((current - 1 + len(opts)) % len(opts))
	case "down", "right", "j", "l":
		w.setOption((current + 1) % len(opts))
	case "enter", "n", "tab":
		if wizStep(w.step) == stepSubtitle && w.subModeIdx == 2 && w.subtitleFile == "" {
			return m, m.startEdit(editTarget{kind: "wizard-sub-file"}, "", "path to .srt / .ass / .vtt file")
		}
		w.next()
	case "e":
		if wizStep(w.step) == stepSubtitle {
			return m, m.startEdit(editTarget{kind: "wizard-sub-file"}, w.subtitleFile, "path to subtitle file")
		}
	default:
		if len(key) == 1 && key[0] >= '1' && key[0] <= '9' {
			idx := int(key[0] - '1')
			if idx < len(opts) {
				w.setOption(idx)
			}
		}
	}
	return m, nil
}

func (w *wizardState) entryAtCursor() (filesystem.Entry, bool) {
	if w.entryCursor >= 0 && w.entryCursor < len(w.entries) {
		return w.entries[w.entryCursor], true
	}
	return filesystem.Entry{}, false
}

func extraInputPlaceholder(w *wizardState) string {
	if w.tool != nil && w.tool.NeedsInput != "" {
		return w.tool.NeedsInput
	}
	for _, f := range w.filters {
		if f.Def.NeedsInput {
			return f.Def.InputLabel
		}
	}
	return "second input file"
}

// completeValue expands a path prefix to the longest common completion.
func completeValue(v string) string {
	v = filesystem.ExpandHome(strings.TrimSpace(v))
	if v == "" {
		if wd, err := os.Getwd(); err == nil {
			v = wd + string(os.PathSeparator)
		}
	}
	matches := filesystem.CompletePath(v)
	if len(matches) == 0 {
		return v
	}
	if len(matches) == 1 {
		return matches[0]
	}
	prefix := matches[0]
	for _, m := range matches[1:] {
		for !strings.HasPrefix(m, prefix) && prefix != "" {
			prefix = prefix[:len(prefix)-1]
		}
	}
	if prefix == "" {
		return v
	}
	return prefix
}

// completePathDir is used by the batch screen.
func completePathDir(v string) string {
	v = completeValue(v)
	if st, err := os.Stat(v); err == nil && !st.IsDir() {
		return filepath.Dir(v)
	}
	return v
}
