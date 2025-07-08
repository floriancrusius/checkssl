# Detect the operating system
OS := $(shell uname)

# Define the output directory
OUTPUT_DIR = dist

# Define the binary name
BINARY_NAME = checkssl

# Set the target based on the operating system
ifeq ($(OS), Linux)
	TARGET = node18-linux-x64
	BINARY_SUFFIX = -linux
else ifeq ($(OS), Darwin)
	TARGET = node18-macos-x64
	BINARY_SUFFIX = -macos
else
	$(error Unsupported OS: $(OS))
endif

# Default target
all: build

# Build target
build:
	@echo "Building binary for $(OS)..."
	@mkdir -p $(OUTPUT_DIR)
	@pkg . --out-path $(OUTPUT_DIR) --targets $(TARGET)

install:
	@if [ ! -f "$(OUTPUT_DIR)/$(BINARY_NAME)" ]; then \
		echo "Binary not found. Building..."; \
		$(MAKE) build; \
	fi
	@echo "Installing binary..."
	@SRC="$(OUTPUT_DIR)/$(BINARY_NAME)"; \
	DEST_BIN="/usr/local/bin/$(BINARY_NAME)"; \
	echo "Copying $$SRC to $$DEST_BIN"; \
	cp $$SRC $$DEST_BIN; \
	chmod +x $$DEST_BIN

# Clean target
clean:
	@echo "Cleaning up..."
	@rm -rf $(OUTPUT_DIR)

# Phony targets
.PHONY: all build clean install