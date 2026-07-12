.PHONY: all clean clean-native setup configure-llama build-llama build-bridge build-go build-thin-fast build-thin-high build-full-fast build-full-high prepare-fast prepare-high prepare-models test static static-thin-fast static-thin-high static-full-fast static-full-high release

LLAMA_DIR := llama.cpp
BUILD_DIR := build
BIN_DIR := bin
CBRIDGE_DIR := cbridge
VERSION ?= dev
CMAKE_EXTRA_ARGS ?=
GO_LDFLAGS := -s -w -X main.version=$(VERSION)
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
	git submodule update --init --recursive

configure-llama: setup
	cmake -S $(LLAMA_DIR) -B $(BUILD_DIR) \
		-DCMAKE_BUILD_TYPE=Release \
		-DLLAMA_CURL=OFF \
		-DLLAMA_BUILD_TESTS=OFF \
		-DLLAMA_BUILD_EXAMPLES=OFF \
		-DLLAMA_BUILD_SERVER=OFF \
		-DBUILD_SHARED_LIBS=OFF \
		-DGGML_OPENMP=OFF \
		-DGGML_NATIVE=OFF \
		-DGGML_BACKEND_DL=OFF \
		-DGGML_METAL=OFF \
		-DGGML_BLAS=OFF \
		-DGGML_ACCELERATE=OFF \
		-DGGML_AVX=ON \
		-DGGML_AVX2=ON \
		-DGGML_BMI2=ON \
		-DGGML_FMA=ON \
		-DGGML_F16C=ON \
		$(CMAKE_EXTRA_ARGS)

build-llama: configure-llama
	cmake --build $(BUILD_DIR) --config Release -j $(shell nproc 2>/dev/null || echo 4) -- llama llama-common ggml

build-bridge: setup
	$(CXX) -c -O2 -std=c++17 \
		-I$(CURDIR)/$(LLAMA_DIR)/include \
		-I$(CURDIR)/$(LLAMA_DIR)/common \
		-I$(CURDIR)/$(LLAMA_DIR)/ggml/include \
		$(VENDOR_INC) \
		$(CBRIDGE_DIR)/schema_bridge.cpp -o $(BRIDGE_OBJ)

build-go: build-llama build-bridge
	mkdir -p $(BIN_DIR)
	CGO_ENABLED=1 go build -ldflags '$(GO_LDFLAGS)' -o $(BIN_DIR)/glean .

build-thin-fast: build-llama build-bridge
	mkdir -p $(BIN_DIR)
	CGO_ENABLED=1 go build -ldflags '$(GO_LDFLAGS)' -o $(BIN_DIR)/glean-thin-fast .

build-thin-high: build-llama build-bridge
	mkdir -p $(BIN_DIR)
	CGO_ENABLED=1 go build -tags high -ldflags '$(GO_LDFLAGS)' -o $(BIN_DIR)/glean-thin-high .

prepare-fast:
	./scripts/prepare-embedded-model.sh fast

prepare-high:
	./scripts/prepare-embedded-model.sh high

prepare-models: prepare-fast prepare-high

build-full-fast: build-llama build-bridge prepare-fast
	mkdir -p $(BIN_DIR)
	CGO_ENABLED=1 go build -tags embedded -ldflags '$(GO_LDFLAGS)' -o $(BIN_DIR)/glean-full-fast .

build-full-high: build-llama build-bridge prepare-high
	mkdir -p $(BIN_DIR)
	CGO_ENABLED=1 go build -tags 'embedded high' -ldflags '$(GO_LDFLAGS)' -o $(BIN_DIR)/glean-full-high .

static: build-llama build-bridge
	mkdir -p $(BIN_DIR)
	CGO_ENABLED=1 go build -ldflags '$(GO_LDFLAGS) -extldflags=-static' -tags 'osusergo netgo' -o $(BIN_DIR)/glean-static .

static-thin-fast: build-llama build-bridge
	mkdir -p $(BIN_DIR)
	CGO_ENABLED=1 go build -ldflags '$(GO_LDFLAGS) -extldflags=-static' -tags 'osusergo netgo' -o $(BIN_DIR)/glean-thin-fast-static .

static-thin-high: build-llama build-bridge
	mkdir -p $(BIN_DIR)
	CGO_ENABLED=1 go build -ldflags '$(GO_LDFLAGS) -extldflags=-static' -tags 'osusergo netgo high' -o $(BIN_DIR)/glean-thin-high-static .

static-full-fast: build-llama build-bridge prepare-fast
	mkdir -p $(BIN_DIR)
	CGO_ENABLED=1 go build -ldflags '$(GO_LDFLAGS) -extldflags=-static' -tags 'osusergo netgo embedded' -o $(BIN_DIR)/glean-full-fast-static .

static-full-high: build-llama build-bridge prepare-high
	mkdir -p $(BIN_DIR)
	CGO_ENABLED=1 go build -ldflags '$(GO_LDFLAGS) -extldflags=-static' -tags 'osusergo netgo embedded high' -o $(BIN_DIR)/glean-full-high-static .

test: build-llama build-bridge
	go test -v ./...

clean-native:
	rm -rf $(BIN_DIR) $(BRIDGE_OBJ)

clean:
	rm -rf $(BUILD_DIR) $(BIN_DIR) $(BRIDGE_OBJ) glean glean-*-fast glean-*-high glean-static glean-*-static dist
