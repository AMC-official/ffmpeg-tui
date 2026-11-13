BINARY := ffmpeg-tui
PKG := ./cmd/ffmpeg-tui
VERSION ?= 0.1.0
PREFIX ?= /usr/local
MANPREFIX ?= $(PREFIX)/share/man
LDFLAGS := -s -w -X github.com/ffmpeg-tui/ffmpeg-tui/internal/ui.Version=$(VERSION)

GO ?= go

.PHONY: all build run test vet fmt install install-man clean cross

all: build

build:
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(BINARY) $(PKG)

run:
	$(GO) run $(PKG)

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

fmt:
	gofmt -w $$(find . -name '*.go' -not -path './.git/*')

install:
	$(GO) install -trimpath -ldflags "$(LDFLAGS)" $(PKG)

install-man:
	install -d $(DESTDIR)$(MANPREFIX)/man1
	install -m 0644 man/ffmpeg-tui.1 $(DESTDIR)$(MANPREFIX)/man1/ffmpeg-tui.1

clean:
	rm -f $(BINARY)
	rm -rf dist

# Cross-compilation for the supported targets. Pure Go, so no toolchain
# gymnastics are required; pause/resume is stubbed on Windows by design.
cross:
	@mkdir -p dist
	@for target in \
		linux/amd64 linux/386 linux/arm64 linux/riscv64 \
		darwin/amd64 darwin/arm64 \
		windows/amd64 windows/386 windows/arm64 \
		freebsd/amd64 freebsd/386 freebsd/arm64 \
		openbsd/amd64 openbsd/386 \
		netbsd/amd64 netbsd/386 \
		dragonfly/amd64; do \
		os=$${target%/*}; arch=$${target#*/}; \
		out=dist/$(BINARY)-$(VERSION)-$$os-$$arch; \
		if [ "$$os" = "windows" ]; then out=$$out.exe; fi; \
		echo "  $$os/$$arch"; \
		GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $$out $(PKG) || exit 1; \
	done
	@echo "cross-compiled into dist/"
