// Package config persists user preferences in the platform's config directory.
// Nothing here is required for the application to run: a missing or corrupt
// config always falls back to defaults.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ffmpeg-tui/ffmpeg-tui/internal/presets"
)

// Favorite is a pinned file or directory on the dashboard.
type Favorite struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"is_dir"`
}

// Config is the on-disk user configuration.
type Config struct {
	Version      int              `json:"version"`
	FFmpegPath   string           `json:"ffmpeg_path"`
	FFprobePath  string           `json:"ffprobe_path"`
	LastDir      string           `json:"last_dir"`
	ShowHidden   bool             `json:"show_hidden"`
	SortMode     string           `json:"sort_mode"`
	Favorites    []Favorite       `json:"favorites"`
	Presets      []presets.Preset `json:"presets"`
	HistoryLimit int              `json:"history_limit"`
	ConfirmQuit  bool             `json:"confirm_quit"`
	UIRows       int              `json:"ui_rows"`
}

// Default returns a configuration with sensible cross-platform defaults.
func Default() *Config {
	cwd, _ := os.Getwd()
	return &Config{
		Version:      1,
		FFmpegPath:   "ffmpeg",
		FFprobePath:  "ffprobe",
		LastDir:      cwd,
		SortMode:     "name",
		HistoryLimit: 500,
		ConfirmQuit:  true,
	}
}

// Dir returns the configuration directory, creating it on request.
func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		home, herr := os.UserHomeDir()
		if herr != nil {
			return "", err
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "ffmpeg-tui"), nil
}

// Path returns the config file path.
func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// StateDir returns the directory used for queues and caches.
func StateDir() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// Load reads the default config file, returning defaults when it is absent.
func Load() (*Config, error) {
	path, err := Path()
	if err != nil {
		return Default(), err
	}
	return LoadFrom(path)
}

// LoadFrom reads a specific config file.
func LoadFrom(path string) (*Config, error) {
	cfg := Default()
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return cfg, err
	}
	if err := json.Unmarshal(data, cfg); err != nil {
		return Default(), fmt.Errorf("config %s is corrupt: %w", path, err)
	}
	cfg.normalize()
	return cfg, nil
}

func (c *Config) normalize() {
	if c.FFmpegPath == "" {
		c.FFmpegPath = "ffmpeg"
	}
	if c.FFprobePath == "" {
		c.FFprobePath = "ffprobe"
	}
	if c.HistoryLimit <= 0 {
		c.HistoryLimit = 500
	}
	if c.SortMode == "" {
		c.SortMode = "name"
	}
	if c.LastDir == "" {
		c.LastDir, _ = os.Getwd()
	}
	if strings.HasPrefix(c.LastDir, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			c.LastDir = filepath.Join(home, strings.TrimPrefix(c.LastDir, "~"))
		}
	}
}

// Save writes the config atomically to the default location.
func (c *Config) Save() error {
	path, err := Path()
	if err != nil {
		return err
	}
	return c.SaveTo(path)
}

// SaveTo writes the config atomically to path.
func (c *Config) SaveTo(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// AddFavorite pins a path, de-duplicating by path.
func (c *Config) AddFavorite(name, path string, isDir bool) {
	for i, f := range c.Favorites {
		if f.Path == path {
			c.Favorites[i].Name = name
			return
		}
	}
	c.Favorites = append(c.Favorites, Favorite{Name: name, Path: path, IsDir: isDir})
}

// RemoveFavorite unpins a path.
func (c *Config) RemoveFavorite(path string) {
	out := c.Favorites[:0]
	for _, f := range c.Favorites {
		if f.Path != path {
			out = append(out, f)
		}
	}
	c.Favorites = out
}

// AddPreset stores a custom preset, replacing one with the same name.
func (c *Config) AddPreset(p presets.Preset) {
	p.Builtin = false
	for i, existing := range c.Presets {
		if strings.EqualFold(existing.Name, p.Name) {
			c.Presets[i] = p
			return
		}
	}
	c.Presets = append(c.Presets, p)
}

// RemovePreset deletes a custom preset by name.
func (c *Config) RemovePreset(name string) {
	out := c.Presets[:0]
	for _, p := range c.Presets {
		if !strings.EqualFold(p.Name, name) {
			out = append(out, p)
		}
	}
	c.Presets = out
}

// PlatformName returns a human name for the current OS.
func PlatformName() string {
	switch runtime.GOOS {
	case "darwin":
		return "macOS"
	case "windows":
		return "Windows"
	case "freebsd":
		return "FreeBSD"
	case "openbsd":
		return "OpenBSD"
	case "netbsd":
		return "NetBSD"
	case "dragonfly":
		return "DragonFlyBSD"
	case "linux":
		return "Linux"
	}
	return runtime.GOOS
}
