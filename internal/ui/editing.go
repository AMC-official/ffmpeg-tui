package ui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ffmpeg-tui/ffmpeg-tui/internal/ffprobe"
	"github.com/ffmpeg-tui/ffmpeg-tui/internal/filesystem"
)

// startEdit opens the inline editor for a target.
func (m *Model) startEdit(t editTarget, initial, placeholder string) tea.Cmd {
	m.edit = t
	m.editing = true
	m.editor.Width = max(20, min(m.width-16, 76))
	m.editor.Placeholder = placeholder
	m.editor.SetValue(initial)
	m.editor.CursorEnd()
	return m.editor.Focus()
}

// cancelEdit discards the edit.
func (m *Model) cancelEdit() {
	m.editing = false
	m.editor.Blur()
	m.edit = editTarget{}
}

// commitEdit applies the editor value to its target.
func (m *Model) commitEdit() tea.Cmd {
	val := strings.TrimSpace(m.editor.Value())
	target := m.edit
	m.editing = false
	m.editor.Blur()
	m.edit = editTarget{}

	switch target.kind {
	case "wizard-input":
		m.wiz.input = filesystem.ExpandHome(val)
		m.wiz.probe = nil
		m.wiz.duration = 0
		m.wiz.err = ""
		if m.wiz.output == "" || m.wiz.outputAuto {
			m.wiz.buildOutput(m.wiz.input, nil)
		}
		m.wiz.loadEntries(filepath.Dir(m.wiz.input), m.showHidden)
		m.wiz.selectEntryByPath(m.wiz.input)
		if p := m.wiz.input; p != "" {
			if st, err := filepath.Abs(p); err == nil {
				p = st
			}
			return m.requestProbe(p)
		}
		return nil
	case "wizard-output":
		m.wiz.output = filesystem.ExpandHome(val)
		m.wiz.outputAuto = false
		return nil
	case "wizard-extra":
		m.wiz.extraInput = filesystem.ExpandHome(val)
		return nil
	case "wizard-meta":
		m.wiz.meta.Set(target.key, val)
		return nil
	case "meta-value":
		m.meta.values.Set(target.key, val)
		return nil
	case "wizard-sub-file":
		m.wiz.subtitleFile = filesystem.ExpandHome(val)
		return nil
	case "filter-arg":
		if err := m.flt.setArg(target.index, target.key, val); err != nil {
			return m.flash("✖ " + err.Error())
		}
		return nil
	case "batch-output":
		m.batch.outputDir = filesystem.ExpandHome(val)
		return nil
	case "batch-source":
		m.batch.source = filesystem.ExpandHome(val)
		m.batch.refresh(m.showHidden)
		return nil
	case "batch-template":
		m.batch.template = val
		return nil
	case "batch-filter":
		m.batch.filterText = val
		return nil
	case "browser-filter":
		m.browserFilter = val
		m.filtering = val != ""
		m.entries = filterEntries(m.allEntries, val)
		m.browserCursor = 0
		m.browserOffset = 0
		return nil
	case "history-filter":
		m.histFilter = val
		return nil
	case "browser-go":
		path := filesystem.ExpandHome(val)
		if path == "" {
			return nil
		}
		if st, err := os.Stat(path); err == nil {
			target := path
			if !st.IsDir() {
				target = filepath.Dir(path)
			}
			m.loadDir(target)
			if !st.IsDir() {
				base := filepath.Base(path)
				for i, e := range m.entries {
					if e.Name == base {
						m.browserCursor = i
					}
				}
				return m.probeSelected()
			}
			return nil
		}
		return m.flash("✖ no such path: " + path)
	case "compare-path":
		m.compare.paths[target.index] = filesystem.ExpandHome(val)
		m.compare.probes[target.index] = nil
		m.compare.errs[target.index] = ""
		m.compare.loading[target.index] = true
		return compareProbeCmd(m.ctx, m.ffprobePath, target.index, m.compare.paths[target.index])
	case "preset-name":
		return m.savePresetAs(val)
	case "man-query":
		m.man.input.SetValue(val)
		m.man.cursor = 0
		m.man.detail = 0
		m.man.refilter()
		return nil
	case "wizard-filters":
		// Filter arguments are edited on the filter screen.
		return nil
	}
	return nil
}

// savePresetAs stores the highlighted preset under a new custom name.
func (m *Model) savePresetAs(name string) tea.Cmd {
	if name == "" {
		return m.flash("✖ preset name cannot be empty")
	}
	all := allPresets(m.cfg.Presets)
	if m.presetCursor < 0 || m.presetCursor >= len(all) {
		return m.flash("✖ no preset selected")
	}
	p := all[m.presetCursor]
	p.Name = name
	p.Builtin = false
	m.cfg.AddPreset(p)
	m.saveConfig()
	return m.flash("✓ preset \"" + name + "\" saved to config")
}

func filterEntries(entries []filesystem.Entry, query string) []filesystem.Entry {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return entries
	}
	var out []filesystem.Entry
	for _, e := range entries {
		if strings.Contains(strings.ToLower(e.Name), q) {
			out = append(out, e)
		}
	}
	return out
}

func compareProbeCmd(ctx context.Context, bin string, slot int, path string) tea.Cmd {
	if path == "" {
		return nil
	}
	return func() tea.Msg {
		res, err := ffprobe.Probe(ctx, path, ffprobe.Options{Binary: bin, Timeout: 30 * time.Second})
		return compareProbeMsg{slot: slot, path: path, res: res, err: err}
	}
}
