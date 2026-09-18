BINARY   := checkssl
BIN_DIR  := bin
DIST_DIR := dist
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS  := -s -w -X main.Version=$(VERSION)
INSTALL_PREFIX ?= /usr/local

TARGETS := \
	darwin/amd64 \
	darwin/arm64 \
	linux/amd64 \
	linux/arm64 \
	windows/amd64

.PHONY: all build build-all install uninstall test test-race lint tidy fmt clean

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

uninstall:
	rm -f $(INSTALL_PREFIX)/bin/$(BINARY)

test:
	go test ./...

test-race:
	go test -race ./...

lint:
	go vet ./...
	gofmt -l . | grep -v '^$$' && exit 1 || true

tidy:
	go mod tidy

fmt:
	gofmt -w .

clean:
	rm -rf $(BIN_DIR) $(DIST_DIR)
