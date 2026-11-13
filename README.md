# 📹️ FFmpeg-TUI

A production-quality terminal interface for FFmpeg, written entirely in Go.

**⚠️ Alpha Software:** FFmpeg-TUI is currently in early alpha. While core features work, you should expect bugs, missing edge cases, and potential breaking changes. Please [open an issue](https://github.com/AMC-official/ffmpeg-tui/issues) if you find something broken!

FFmpeg-TUI is a thin management layer over the official `ffmpeg`, `ffprobe` and
(optionally) `ffplay` binaries — the same way lazygit sits on top of git. It does
not link libav*, does not bundle a media stack and does not talk to a server. It
generates FFmpeg command lines, shows them to you verbatim, validates them and
runs them through `os/exec` without a shell.

```
 FFmpeg-TUI v0.1.0  ·  ffmpeg 7.0.2                              ~/films
 ─────────────────────────────────────────────────────────────────────
 Jobs
 ▶ Convert movie.mkv            42% ███████░░░░░░░  ETA 00:04:30
 ✓ music.flac                   done
 ⟳ episode01.mp4                running
 ✖ failed.mov                   failed: Invalid data found

 Recent projects · Favorites · Presets · Queue · History
   New conversion        Ctrl+N · guided wizard with live command preview
   Browse media          files, folders and metadata via ffprobe
   Batch process         encode a folder with templates and mirroring
   Tools & recipes       audio, video and subtitle one-click recipes
   Presets               lossless, web, device and archival recipes
   Filter builder        crop, scale, overlay, noise, loudnorm and more
   Man pages             offline searchable FFmpeg documentation
   Queue                 2 queued · 1 running
   History               38 recorded commands
 ─────────────────────────────────────────────────────────────────────
 Command preview · wizard
 ffmpeg \
   -hide_banner -nostdin -y -i movie.mkv \
   -c:v libx264 -crf 20 -preset medium -pix_fmt yuv420p \
   -c:a aac -b:a 192k -movflags +faststart movie.mp4
```

> Additional note: AI was partially used In order to Help make and debug this project

## 🧠️ Philosophy

- Entirely Go, one static binary, no daemon, no background service.
- GPLv3, cross-platform, keyboard-first.
- Bubble Tea, Bubbles and Lipgloss; the standard library everywhere else.
- No Electron, no Qt, no GTK, no browser, no embedded Chromium.
- The command line is never hidden: the exact argv that will run is always on
  screen, and every job can be replayed from history.

## 📦️ Requirements

Runtime:

- `ffmpeg`
- `ffprobe`

Optional:

- `ffplay` — full-quality local preview from the browser (`o`).
- `man` — open the system manual pages from the bundled documentation screen.

Build:

- Go 1.24 or newer.

## 💻️ Supported platforms

Linux, FreeBSD, OpenBSD, NetBSD, DragonFlyBSD, macOS and Windows on amd64,
i386, arm64 and riscv64. Everything is pure Go; no cgo is required. Pausing a
running job uses POSIX job-control signals, so it is available everywhere except
Windows, where the interface reports that pausing is unsupported instead of
pretending.

**Fully working (tested)**
  - FreeBSD (amd64)
  - Linux (and64)

**Expected to work (untested)**
  - Windows (any)
  - MacOS/Darwin (any)

**Experimenal**
  - OpenBSD (any)
  - NetBSD (any)
  - DragonflyBSD (any)

> the app has been tested on only amd64, so it *is* expected to work on i386 too, arm64 and riscv64 remain untested

## 🚀️ Install a release

Prebuilt static binaries are published for Linux, macOS, Windows and the BSDs on
every `v*` tag.

Linux, macOS and BSDs:

```sh
curl -fsSL https://raw.githubusercontent.com/AMC-official/ffmpeg-tui/main/install.sh | sh
```

The script verifies the SHA-256 checksum, installs to `~/.local/bin` (override
with `PREFIX` or `BIN_DIR`) and installs the manual page. Read it first if you
prefer: [install.sh](install.sh). Pick a version with `VERSION=v1.0.0`.

Windows (PowerShell):

```powershell
irm https://raw.githubusercontent.com/AMC-official/ffmpeg-tui/main/install.ps1 | iex
```

It installs to `%LOCALAPPDATA%\ffmpeg-tui\bin` and adds that directory to your
user `PATH`.

You can also download an archive from the
[releases page](https://github.com/AMC-official/ffmpeg-tui/releases) and read the
manual with `man ffmpeg-tui` after installing
[man/ffmpeg-tui.1](man/ffmpeg-tui.1) into your `man1` directory.

## 🔧️ Build and install

```sh
make build            # ./ffmpeg-tui
make install-local    # installs the binary and man page from this checkout
make install          # installs only the binary to $GOBIN (or ~/go/bin)
make install-man      # installs only man/ffmpeg-tui.1
go install github.com/AMC-official/ffmpeg-tui/cmd/ffmpeg-tui@latest
```

`make install-local` installs to `/usr/local/bin` and
`/usr/local/share/man/man1`. Use a user prefix to avoid root:

```sh
make install-local PREFIX=$HOME/.local
# or stage it: make install-local DESTDIR=/tmp/pkgroot PREFIX=/usr/local
```

Cross-compile examples:

```sh
make cross            # linux/darwin/windows/freebsd/openbsd/netbsd/dragonfly
GOOS=freebsd GOARCH=riscv64 go build ./cmd/ffmpeg-tui
```

## ✨️ Usage

```sh
ffmpeg-tui                       # open the dashboard in the current directory
ffmpeg-tui -dir ~/Videos         # start in a folder
ffmpeg-tui -ffmpeg /opt/ffmpeg   # use a specific ffmpeg build
ffmpeg-tui -config ./my.json     # isolated configuration
ffmpeg-tui -version
```

Flags: `-ffmpeg`, `-ffprobe`, `-config`, `-dir`, `-no-color`, `-version`.

## 🖥️ Screens

- **Dashboard** — live jobs, recent projects, favorites, presets, queue, history.
- **Media browser** — videos, audio, images and subtitles, with `ffprobe`
  metadata, streams, chapters, attachments and a terminal-native ASCII preview
  (a real decoded frame or audio waveform, no image protocol required).
- **Conversion wizard** — input → output → container → video → quality → speed →
  audio → audio quality → subtitles → metadata → filters → confirmation.
  Outputs: MP4, MKV, MOV, AVI, WebM, MP3, AAC, FLAC, OGG, WAV, GIF, PNG/JPEG
  sequences, WebP and AVIF.
- **Job queue** — multiple jobs, live progress (frame, FPS, bitrate, time, ETA,
  speed), pause/resume, cancel, retry, reorder, save and load.
- **Presets** — Lossless, H264, H265, AV1, VP9, Discord, YouTube, Twitch,
  Blu-ray, Podcast, Audiobook, Phone, Archival, Animation, GIF and Custom, plus
  user presets stored in the config file.
- **Filter builder** — crop, scale, rotate, flip, transpose, fps, brightness,
  contrast, gamma, saturation, sharpen, blur, noise, watermark, subtitles,
  overlay, pad, h/v stack, split, trim, fade, speed and normalisation. Each
  selection previews its generated chain and full command.
- **Batch processing** — whole folders, recursive, output templates
  (`{name}`, `{ext}`, `{index}`, `{date}`, `{dir}`), skip existing files and
  mirror the directory tree.
- **Audio tools** — extract, normalise, convert, merge, trim, fade, remove
  silence, volume, resample.
- **Video tools** — convert, resize, rotate, stabilise (analyse pass), speed,
  reverse, frame extraction, thumbnail/contact sheet, GIF preview.
- **Subtitle tools** — extract, burn, remove, shift timing, convert to SRT, ASS
  or WebVTT.
- **Metadata editor** — title, artist, album, genre, track, disc, year, comment,
  copyright, language and encoder; writes a stream copy (`-c copy`).
- **Stream viewer** — every stream with codec, bitrate, resolution, colour space,
  HDR, language, disposition, chapters and attachments.
- **Compare** — input vs output resolution, codec, bitrate, duration, file size
  and compression ratio.
- **Man pages** — a bundled, offline, fuzzy-searchable index of filters,
  encoders, muxers, demuxers, protocols, tools and options. `:man crop`,
  `:man libx264`, `:man mp4`; `/normalize` surfaces `loudnorm`, `dynaudnorm` and
  `volume`. Press `f` to fetch live help from the installed ffmpeg and `m` to
  open the system manual page when available.
- **History** — every executed command with its outcome; replay to a fresh
  output, delete, or export as a shell script.

## ⌨️ Keyboard

| Key | Action |
| --- | --- |
| `Ctrl+N` | new conversion |
| `Ctrl+Q` / `Ctrl+C` | quit (confirmation while jobs are active) |
| `/` | global search: files, man pages, presets, history, actions |
| `:` | command palette (`:man crop`) |
| `?` | help |
| `↑ ↓` / `j k` | move |
| `Space` | pause/resume a job, or mark a browser file |
| `Delete` | cancel/remove a job, clear a tag |
| `Tab` | next field, or complete a path |
| `g` / `G` | first / last item in a list |
| `PgUp` / `PgDn` | move a page through a list |
| `Enter` | open, inspect, accept a step |
| `Esc` | back / dismiss |

Every screen shows its own shortcut bar above the status line. Long lists scroll
to keep the selection visible and show the range, for example
`Tools · 10-26 of 26`, in the panel title.

A manual page covering the flags, screens, keys, files and safety model lives at
[man/ffmpeg-tui.1](man/ffmpeg-tui.1).

## 🔒️ Safety model

- FFmpeg is **never** invoked through a shell. Every command is an explicit
  `exec.Command(bin, argv...)` built from discrete arguments.
- Paths are validated before a job enters the queue: input exists and is not a
  directory or empty, the output directory exists, input and output are not the
  same file, and NUL bytes are rejected.
- Filenames with spaces, quotes, globs or `$(...)` are inert data. The preview
  quotes them unambiguously for humans; execution never re-parses them.
- Values embedded in a filtergraph (subtitle files, watermark paths) are escaped
  for FFmpeg's filtergraph grammar.
- Nothing runs until you confirm a screen that shows the exact argv.
- Jobs record the finished command and exit status in history; a failed job
  keeps its stderr tail.

## 📄️ Configuration

The config is optional and human-editable JSON:

- Linux/BSD: `$XDG_CONFIG_HOME/ffmpeg-tui/config.json` (usually `~/.config/...`)
- macOS: `~/Library/Application Support/ffmpeg-tui/config.json`
- Windows: `%AppData%\ffmpeg-tui\config.json`

State files live beside it: `history.jsonl` (append-only command history),
`queue.json` (saved queue) and `command-history.sh` (export). Missing or corrupt
files fall back to defaults; a corrupt file never prevents startup.

```json
{
  "ffmpeg_path": "ffmpeg",
  "ffprobe_path": "ffprobe",
  "last_dir": "/home/me/Videos",
  "show_hidden": false,
  "sort_mode": "name",
  "favorites": [{"name": "Films", "path": "/home/me/Videos", "is_dir": true}],
  "presets": [],
  "history_limit": 500
}
```

## 🔬️ Architecture

```
cmd/ffmpeg-tui/        entry point, flags, terminal bootstrap
internal/ui/           Bubble Tea model, screens, styles, modals
internal/ffmpeg/       argv builder, validation, preview, progress parser
internal/ffprobe/      typed ffprobe JSON report
internal/jobs/         worker, process control, event stream
internal/queue/        persistent job model
internal/presets/      built-in encoding recipes and tool catalogue
internal/filters/      filter definitions and filtergraph assembly
internal/metadata/     tag editing
internal/preview/      ASCII frame and waveform rendering
internal/manpages/     bundled documentation index and live/system help
internal/history/      JSONL command history
internal/config/       platform config paths and persistence
internal/filesystem/   listing, classification, templates, safe paths
internal/widgets/      bars, tables, wrapping and other primitives
```

Design notes:

- One FFmpeg process at a time; the queue is serial and ordered, so progress and
  pause semantics stay predictable. Everything is event-driven and the UI never
  blocks on a process.
- Progress is read from `-progress pipe:1` (machine-readable `key=value` lines)
  with a parser that also understands human `frame=… time=…` stderr stats.
- Panic-free rendering at small sizes: panels clamp, long lines truncate with
  ANSI-aware width measurement, and terminals below 56×16 show guidance.

## ✅️ Releases

Tag a commit and push the tag:

```sh
git tag v1.0.0
git push origin v1.0.0
```

`.github/workflows/release.yml` runs the tests, then GoReleaser
(`.goreleaser.yml`) builds static binaries for Linux, macOS, Windows, FreeBSD,
OpenBSD, NetBSD and DragonFlyBSD, writes `checksums.txt`, and attaches the
archives (with README, license and man page) to the GitHub release.

## 🧑‍💻️ Development

```sh
make test      # unit tests plus an end-to-end ffmpeg/job test
make vet       # go vet ./...
make fmt       # gofmt -w .
make run       # go run ./cmd/ffmpeg-tui
```

The end-to-end test generates a two-second clip with `lavfi` and skips cleanly
when ffmpeg/ffprobe are not installed.

## 📃️ License

GPLv3 — see [LICENSE](LICENSE).
