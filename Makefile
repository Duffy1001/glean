.PHONY: all clean clean-native setup build-llama build-bridge build-go test static release

LLAMA_DIR := llama.cpp
LLAMA_REPO ?= https://github.com/ggml-org/llama.cpp.git
LLAMA_REF  ?= e3546c7948e3af463d0b401e6421d5a4c2faf565
BUILD_DIR := build
CBRIDGE_DIR := cbridge
CGO_CFLAGS := -I$(CURDIR)/$(LLAMA_DIR)/include -I$(CURDIR)/$(LLAMA_DIR)/ggml/include -I$(CURDIR)/$(BUILD_DIR)/ggml/src -I$(CURDIR)/$(BUILD_DIR)/ggml/include -I$(CURDIR)/$(BUILD_DIR)/common -I$(CURDIR)/$(CBRIDGE_DIR)
VENDOR_INC := -I$(CURDIR)/$(LLAMA_DIR)/vendor

# Detect platform
UNAME_S := $(shell uname -s 2>/dev/null || echo Windows)
UNAME_M := $(shell uname -m 2>/dev/null || echo x86_64)

ifeq ($(UNAME_S),Linux)
	PLATFORM := linux
	CXX ?= g++
else ifeq ($(UNAME_S),Darwin)
	PLATFORM := darwin
	CXX ?= clang++
else
	PLATFORM := windows
	CXX ?= g++
endif

ifeq ($(UNAME_M),x86_64)
	ARCH := amd64
else ifeq ($(UNAME_M),arm64)
	ARCH := arm64
else ifeq ($(UNAME_M),aarch64)
	ARCH := arm64
else
	ARCH := $(UNAME_M)
endif

# Static archives
LLAMA_LIBS := \
	$(BUILD_DIR)/src/libllama.a \
	$(BUILD_DIR)/ggml/src/libggml.a \
	$(BUILD_DIR)/ggml/src/libggml-base.a \
	$(BUILD_DIR)/ggml/src/libggml-cpu.a \
	$(BUILD_DIR)/common/libllama-common.a \
	$(BUILD_DIR)/common/libllama-common-base.a

BRIDGE_OBJ := $(CBRIDGE_DIR)/schema_bridge.o

all: build-go

setup:
	@if [ ! -d "$(LLAMA_DIR)/.git" ]; then \
		echo "Cloning llama.cpp..."; \
		git clone --depth 1 $(LLAMA_REPO) $(LLAMA_DIR); \
		cd $(LLAMA_DIR) && git fetch --depth 1 origin $(LLAMA_REF) && git checkout $(LLAMA_REF); \
	else \
		echo "llama.cpp already present"; \
	fi

build-llama: $(LLAMA_LIBS)

$(BUILD_DIR)/CMakeCache.txt: $(LLAMA_DIR)/CMakeLists.txt
	cmake -S $(LLAMA_DIR) -B $(BUILD_DIR) \
		-DCMAKE_BUILD_TYPE=Release \
		-DLLAMA_CURL=OFF \
		-DLLAMA_BUILD_TESTS=OFF \
		-DLLAMA_BUILD_EXAMPLES=OFF \
		-DLLAMA_BUILD_SERVER=OFF \
		-DBUILD_SHARED_LIBS=OFF \
		-DGGML_OPENMP=OFF

$(LLAMA_LIBS): $(BUILD_DIR)/CMakeCache.txt
	cmake --build $(BUILD_DIR) --config Release -j $(shell nproc 2>/dev/null || echo 4) -- llama common ggml

build-bridge: $(BRIDGE_OBJ)

$(BRIDGE_OBJ): $(CBRIDGE_DIR)/schema_bridge.cpp $(CBRIDGE_DIR)/schema_bridge.h
	$(CXX) -c -O2 -std=c++17 \
		-I$(CURDIR)/$(LLAMA_DIR)/include \
		-I$(CURDIR)/$(LLAMA_DIR)/common \
		-I$(CURDIR)/$(LLAMA_DIR)/ggml/include \
		$(VENDOR_INC) \
		$< -o $@

build-go: build-llama build-bridge
	CGO_ENABLED=1 go build -o glean .

static: build-llama build-bridge
	CGO_ENABLED=1 go build -ldflags '-s -w -extldflags=-static' -tags 'osusergo netgo' -o glean-static .

test: build-bridge
	go test -v ./...

clean-native:
	rm -f glean glean-static $(BRIDGE_OBJ)

clean:
	rm -rf $(BUILD_DIR) $(BRIDGE_OBJ) glean glean-static dist
