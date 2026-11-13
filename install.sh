#!/bin/sh
# Install the FFmpeg-TUI binary and manual page from GitHub releases.
#
#   curl -fsSL https://raw.githubusercontent.com/AMC-official/ffmpeg-tui/main/install.sh | sh
#
# Environment:
#   VERSION   release tag or version to install (default: latest)
#   PREFIX    installation prefix (default: ~/.local)
#   BIN_DIR   directory for the binary (default: $PREFIX/bin)
#   MAN_DIR   directory for the man page (default: $PREFIX/share/man/man1)
#   DRY_RUN   set to 1 to print what would happen without changing anything
#
# Examples:
#   VERSION=v1.0.0 PREFIX=/usr/local sudo -E sh install.sh
#   BIN_DIR=$HOME/bin sh install.sh
set -eu

REPO="AMC-official/ffmpeg-tui"
PROJECT="ffmpeg-tui"
BASE_URL="https://github.com/${REPO}/releases"
DRY_RUN="${DRY_RUN:-0}"

log() { printf '%s\n' "$*" >&2; }
die() { log "error: $*"; exit 1; }

die_no_release() {
	log "error: no published release could be found for ${REPO}."
	log ""
	if [ -f "$(dirname "$0")/go.mod" ] && [ -d "$(dirname "$0")/cmd/ffmpeg-tui" ]; then
		log "This looks like a source checkout, not an installed copy. install.sh"
		log "downloads prebuilt release archives; to run from source build it instead:"
		log ""
		log "    make build"
		log "    ./ffmpeg-tui -dir ~/Videos"
		log ""
	else
		log "The repository has no GitHub release matching the request yet."
		log ""
	fi
	log "Once a release exists, either re-run this script, or pick a tag explicitly:"
	log ""
	log "    VERSION=v1.0.0 sh install.sh"
	log ""
	log "Building from source only needs Go and the ffmpeg/ffprobe binaries."
	exit 1
}

need() {
	for cmd in "$@"; do
		command -v "$cmd" >/dev/null 2>&1 || die "required command not found: $cmd"
	done
}

detect_os() {
	os=$(uname -s 2>/dev/null || echo unknown)
	case "$os" in
	Linux) echo linux ;;
	Darwin) echo darwin ;;
	FreeBSD) echo freebsd ;;
	OpenBSD) echo openbsd ;;
	NetBSD) echo netbsd ;;
	DragonFly) echo dragonfly ;;
	*) die "unsupported operating system: $os (build from source instead)" ;;
	esac
}

detect_arch() {
	arch=$(uname -m 2>/dev/null || echo unknown)
	case "$arch" in
	x86_64 | amd64) echo amd64 ;;
	aarch64 | arm64) echo arm64 ;;
	i386 | i486 | i586 | i686 | x86) echo 386 ;;
	riscv64) echo riscv64 ;;
	*) die "unsupported architecture: $arch (build from source instead)" ;;
	esac
}

latest_tag() {
	if command -v curl >/dev/null 2>&1; then
		url=$(curl -fsSLI -o /dev/null -w '%{url_effective}' "$BASE_URL/latest" 2>/dev/null || true)
	elif command -v wget >/dev/null 2>&1; then
		url=$(wget -q -O /dev/null --server-response "$BASE_URL/latest" 2>&1 | awk '/^  Location:/{loc=$2} END{print loc}')
	else
		die "curl or wget is required"
	fi
	tag=${url##*/}
	case "$tag" in
	v[0-9]*) echo "$tag" ;;
	*) die_no_release ;;
	esac
}

download() {
	url=$1
	out=$2
	if command -v curl >/dev/null 2>&1; then
		curl -fsSL --retry 3 --retry-delay 1 -o "$out" "$url"
	else
		wget -q -O "$out" "$url"
	fi
}

tmpdir=$(mktemp -d 2>/dev/null || mktemp -d -t ffmpeg-tui)
trap 'rm -rf "$tmpdir"' EXIT INT TERM

os=$(detect_os)
arch=$(detect_arch)
case "$os/$arch" in
linux/386 | linux/amd64 | linux/arm64 | linux/riscv64) ;;
darwin/amd64 | darwin/arm64) ;;
freebsd/386 | freebsd/amd64 | freebsd/arm64) ;;
openbsd/386 | openbsd/amd64 | openbsd/arm64) ;;
netbsd/386 | netbsd/amd64 | netbsd/arm64) ;;
dragonfly/amd64) ;;
*) die "no prebuilt binary for $os/$arch; build from source with: go build ./cmd/ffmpeg-tui" ;;
esac

if [ -n "${VERSION:-}" ]; then
	tag="v${VERSION#v}"
else
	tag=$(latest_tag)
fi
version=${tag#v}
archive="${PROJECT}_${version}_${os}_${arch}.tar.gz"

prefix="${PREFIX:-$HOME/.local}"
bin_dir="${BIN_DIR:-$prefix/bin}"
man_dir="${MAN_DIR:-$prefix/share/man/man1}"

log "FFmpeg-TUI ${version} for ${os}/${arch}"
log "downloading ${BASE_URL}/download/${tag}/${archive}"
download "${BASE_URL}/download/${tag}/${archive}" "$tmpdir/$archive"

if download "${BASE_URL}/download/${tag}/checksums.txt" "$tmpdir/checksums.txt" 2>/dev/null; then
	expected=$(awk -v name="$archive" '$2 == name { print $1 }' "$tmpdir/checksums.txt")
	if [ -n "$expected" ]; then
		if command -v sha256sum >/dev/null 2>&1; then
			actual=$(sha256sum "$tmpdir/$archive" | awk '{ print $1 }')
		elif command -v shasum >/dev/null 2>&1; then
			actual=$(shasum -a 256 "$tmpdir/$archive" | awk '{ print $1 }')
		else
			actual=""
		fi
		if [ -n "$actual" ] && [ "$actual" != "$expected" ]; then
			die "checksum mismatch for $archive"
		fi
		if [ -n "$actual" ]; then
			log "checksum verified"
		fi
	fi
else
	log "warning: checksums.txt not available, skipping verification"
fi

tar -xzf "$tmpdir/$archive" -C "$tmpdir"
[ -f "$tmpdir/$PROJECT" ] || die "archive did not contain the $PROJECT binary"

if [ "$DRY_RUN" = "1" ]; then
	log "would install $tmpdir/$PROJECT to $bin_dir/$PROJECT"
	exit 0
fi

mkdir -p "$bin_dir"
if command -v install >/dev/null 2>&1; then
	install -m 0755 "$tmpdir/$PROJECT" "$bin_dir/$PROJECT"
else
	cp "$tmpdir/$PROJECT" "$bin_dir/$PROJECT"
	chmod 0755 "$bin_dir/$PROJECT"
fi

if [ -f "$tmpdir/man/ffmpeg-tui.1" ] && mkdir -p "$man_dir" 2>/dev/null; then
	if command -v install >/dev/null 2>&1; then
		install -m 0644 "$tmpdir/man/ffmpeg-tui.1" "$man_dir/ffmpeg-tui.1"
	else
		cp "$tmpdir/man/ffmpeg-tui.1" "$man_dir/ffmpeg-tui.1"
	fi
fi

log "installed $bin_dir/$PROJECT"
"$bin_dir/$PROJECT" -version >/dev/null 2>&1 || true
case ":$PATH:" in
*":$bin_dir:"*) ;;
*) log "add it to your PATH: export PATH=\"$bin_dir:\$PATH\"" ;;
esac
log "ffmpeg and ffprobe must be installed separately"
