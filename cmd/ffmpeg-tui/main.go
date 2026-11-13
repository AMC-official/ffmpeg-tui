// Command ffmpeg-tui is a keyboard-driven terminal interface for the official
// FFmpeg command line tools.
//
// It is a single, dependency-light binary: no daemon, no services, no libav
// bindings. Every operation is an explicit ffmpeg/ffprobe argv executed through
// os/exec without a shell.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/AMC-official/ffmpeg-tui/internal/config"
	"github.com/AMC-official/ffmpeg-tui/internal/filesystem"
	"github.com/AMC-official/ffmpeg-tui/internal/ui"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "ffmpeg-tui:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		showVersion = flag.Bool("version", false, "print the version and exit")
		ffmpegPath  = flag.String("ffmpeg", "", "path to the ffmpeg binary (default: config or $PATH)")
		ffprobePath = flag.String("ffprobe", "", "path to the ffprobe binary (default: config or $PATH)")
		configPath  = flag.String("config", "", "config file to use instead of the default")
		startDir    = flag.String("dir", "", "directory to open in the media browser")
		noColor     = flag.Bool("no-color", false, "disable colour output")
	)
	flag.Usage = func() {
		out := flag.CommandLine.Output()
		fmt.Fprintf(out, "FFmpeg-TUI %s — a terminal interface for FFmpeg\n\n", ui.Version)
		fmt.Fprintf(out, "Usage:\n  ffmpeg-tui [flags]\n\nFlags:\n")
		flag.PrintDefaults()
		fmt.Fprintf(out, "\nRuntime dependencies: ffmpeg, ffprobe (optional: ffplay, man)\n")
		fmt.Fprintf(out, "Keyboard: Ctrl+N new · Ctrl+Q quit · / search · : palette · ? help\n")
	}
	flag.Parse()

	if *showVersion {
		fmt.Printf("ffmpeg-tui %s\n", ui.Version)
		return nil
	}
	if *noColor {
		os.Setenv("NO_COLOR", "1")
	}

	path := *configPath
	cfg, err := loadConfig(path)
	if err != nil {
		// A corrupt config must never prevent the application from starting.
		fmt.Fprintln(os.Stderr, "ffmpeg-tui: warning:", err)
		cfg = config.Default()
	}
	if *ffmpegPath != "" {
		cfg.FFmpegPath = *ffmpegPath
	}
	if *ffprobePath != "" {
		cfg.FFprobePath = *ffprobePath
	}
	if *startDir != "" {
		cfg.LastDir = *startDir
	}
	if *startDir != "" || *ffmpegPath != "" || *ffprobePath != "" {
		if st, err := os.Stat(cfg.LastDir); err == nil && st.IsDir() {
			cfg.LastDir = filesystem.ExpandHome(cfg.LastDir)
		}
	}

	warnMissingTools(cfg)

	model := ui.New(ui.Options{
		Config:      cfg,
		ConfigPath:  path,
		FFmpegPath:  cfg.FFmpegPath,
		FFprobePath: cfg.FFprobePath,
	})

	program := tea.NewProgram(model,
		tea.WithAltScreen(),
		tea.WithReportFocus(),
	)
	if _, err := program.Run(); err != nil {
		model.Close()
		return fmt.Errorf("terminal interface: %w", err)
	}
	model.Close()
	return nil
}

func loadConfig(path string) (*config.Config, error) {
	if path == "" {
		return config.Load()
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	return config.LoadFrom(abs)
}

// warnMissingTools prints an actionable note before the TUI starts. Missing
// ffmpeg is not fatal: the interface opens and explains what is missing.
func warnMissingTools(cfg *config.Config) {
	if _, err := exec.LookPath(cfg.FFmpegPath); err != nil {
		fmt.Fprintln(os.Stderr, "ffmpeg-tui: ffmpeg not found on PATH; install it or pass -ffmpeg /path/to/ffmpeg")
	}
	if _, err := exec.LookPath(cfg.FFprobePath); err != nil {
		fmt.Fprintln(os.Stderr, "ffmpeg-tui: ffprobe not found on PATH; metadata inspection will be unavailable")
	}
}
