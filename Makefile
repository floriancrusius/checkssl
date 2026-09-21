BINARY   := checkssl
BIN_DIR  := bin
DIST_DIR := dist
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS  := -s -w -X main.Version=$(VERSION)
INSTALL_PREFIX ?= /usr/local
BASH_COMP_DIR  ?= $(INSTALL_PREFIX)/etc/bash_completion.d
ZSH_COMP_DIR   ?= $(INSTALL_PREFIX)/share/zsh/site-functions
FISH_COMP_DIR  ?= $(INSTALL_PREFIX)/share/fish/vendor_completions.d
MAN_DIR        ?= $(INSTALL_PREFIX)/share/man/man1

TARGETS := \
	darwin/amd64 \
	darwin/arm64 \
	linux/amd64 \
	linux/arm64 \
	windows/amd64

.PHONY: all build build-all install install-completions install-manpage uninstall test test-race lint tidy fmt clean

all: build

build: $(BIN_DIR)/$(BINARY)

$(BIN_DIR)/$(BINARY): $(shell find . -name '*.go' -not -path './$(BIN_DIR)/*' -not -path './$(DIST_DIR)/*')
	@mkdir -p $(BIN_DIR)
	go build -trimpath -ldflags '$(LDFLAGS)' -o $@ .

build-all: $(TARGETS:%=$(DIST_DIR)/$(BINARY)-%)

$(DIST_DIR)/$(BINARY)-%: $(shell find . -name '*.go' -not -path './$(BIN_DIR)/*' -not -path './$(DIST_DIR)/*')
	@mkdir -p $(DIST_DIR)
	@os=$(word 1,$(subst /, ,$*)); \
	arch=$(word 2,$(subst /, ,$*)); \
	ext=""; \
	if [ "$$os" = "windows" ]; then ext=".exe"; fi; \
	out="$(DIST_DIR)/$(BINARY)-$$os-$$arch$$ext"; \
	echo "→ $$out"; \
	GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 \
		go build -trimpath -ldflags '$(LDFLAGS)' -o $$out .

install: $(BIN_DIR)/$(BINARY)
	install -m 0755 $(BIN_DIR)/$(BINARY) $(INSTALL_PREFIX)/bin/$(BINARY)
	@echo "installed to $(INSTALL_PREFIX)/bin/$(BINARY)"

install-completions:
	@mkdir -p $(BASH_COMP_DIR) $(ZSH_COMP_DIR) $(FISH_COMP_DIR)
	install -m 0644 completions/checkssl.bash $(BASH_COMP_DIR)/checkssl
	install -m 0644 completions/_checkssl      $(ZSH_COMP_DIR)/_checkssl
	install -m 0644 completions/checkssl.fish  $(FISH_COMP_DIR)/checkssl.fish
	@echo "completions installed:"
	@echo "  $(BASH_COMP_DIR)/checkssl"
	@echo "  $(ZSH_COMP_DIR)/_checkssl"
	@echo "  $(FISH_COMP_DIR)/checkssl.fish"

install-manpage:
	@mkdir -p $(MAN_DIR)
	install -m 0644 docs/checkssl.1 $(MAN_DIR)/checkssl.1
	@echo "man page installed to $(MAN_DIR)/checkssl.1"

uninstall:
	rm -f $(INSTALL_PREFIX)/bin/$(BINARY) \
	      $(BASH_COMP_DIR)/checkssl \
	      $(ZSH_COMP_DIR)/_checkssl \
	      $(FISH_COMP_DIR)/checkssl.fish \
	      $(MAN_DIR)/checkssl.1

test:
	go test ./...

test-race:
	go test -race ./...

lint:
	@if ! command -v golangci-lint >/dev/null 2>&1; then \
		echo "golangci-lint not found — install with: brew install golangci-lint"; \
		exit 1; \
	fi
	golangci-lint run ./...

tidy:
	go mod tidy

fmt:
	gofmt -w .

clean:
	rm -rf $(BIN_DIR) $(DIST_DIR)
