.PHONY: all clean clean-native setup configure-llama build-llama build-bridge build-go build-thin-fast build-full-fast prepare-fast test static static-thin-fast static-full-fast release

LLAMA_DIR := llama.cpp
BUILD_DIR := build
BIN_DIR := bin
CBRIDGE_DIR := cbridge
VERSION ?= dev
CMAKE_EXTRA_ARGS ?=
GO_LDFLAGS := -s -w -X github.com/duffy1001/glean.Version=$(VERSION)
CGO_CFLAGS := -I$(CURDIR)/$(LLAMA_DIR)/include -I$(CURDIR)/$(LLAMA_DIR)/ggml/include -I$(CURDIR)/$(BUILD_DIR)/ggml/src -I$(CURDIR)/$(BUILD_DIR)/ggml/include -I$(CURDIR)/$(BUILD_DIR)/common -I$(CURDIR)/$(CBRIDGE_DIR)
VENDOR_INC := -I$(CURDIR)/$(LLAMA_DIR)/vendor
VULKAN_INCLUDE_DIR = $(shell awk -F= '/^Vulkan_INCLUDE_DIR:PATH=/{print $$2}' $(BUILD_DIR)/CMakeCache.txt 2>/dev/null)
VULKAN_INCLUDE_FLAG = $(if $(VULKAN_SDK),-I$(VULKAN_SDK)/include,$(if $(VULKAN_INCLUDE_DIR),-I$(VULKAN_INCLUDE_DIR)))

# Detect platform
UNAME_S := $(shell uname -s 2>/dev/null || echo Windows)
UNAME_M := $(shell uname -m 2>/dev/null || echo x86_64)

ifeq ($(UNAME_S),Linux)
	PLATFORM := linux
	CXX ?= g++
	BACKEND_CMAKE_ARGS := -DGGML_VULKAN=ON
	BACKEND_TARGET := ggml-vulkan
	BACKEND_OBJ := $(CBRIDGE_DIR)/vulkan_loader.o
else ifeq ($(UNAME_S),Darwin)
	PLATFORM := darwin
	CXX ?= clang++
	BACKEND_CMAKE_ARGS := -DGGML_METAL=ON -DGGML_METAL_EMBED_LIBRARY=ON
	BACKEND_TARGET := ggml-metal
	BACKEND_OBJ :=
else
	PLATFORM := windows
	CXX ?= g++
	BACKEND_CMAKE_ARGS := -DGGML_VULKAN=ON
	BACKEND_TARGET := ggml-vulkan
	BACKEND_OBJ := $(CBRIDGE_DIR)/vulkan_loader.o
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
		$(BACKEND_CMAKE_ARGS) \
		$(CMAKE_EXTRA_ARGS)

build-llama: configure-llama
	cmake --build $(BUILD_DIR) --config Release -j $(shell nproc 2>/dev/null || echo 4) -- llama llama-common ggml $(BACKEND_TARGET)

build-bridge: setup
	$(CXX) -c -O2 -std=c++17 \
		-I$(CURDIR)/$(LLAMA_DIR)/include \
		-I$(CURDIR)/$(LLAMA_DIR)/common \
		-I$(CURDIR)/$(LLAMA_DIR)/ggml/include \
		$(VENDOR_INC) \
		$(CBRIDGE_DIR)/schema_bridge.cpp -o $(BRIDGE_OBJ)
	@if [ -n "$(BACKEND_OBJ)" ]; then \
		$(CC) -c -O2 -I$(CURDIR)/$(LLAMA_DIR)/ggml/include $(VULKAN_INCLUDE_FLAG) $(CBRIDGE_DIR)/vulkan_loader.c -o $(BACKEND_OBJ); \
	fi

build-go: build-llama build-bridge
	mkdir -p $(BIN_DIR)
	CGO_ENABLED=1 go build -ldflags '$(GO_LDFLAGS)' -o $(BIN_DIR)/glean ./cmd/glean

build-thin-fast: build-llama build-bridge
	mkdir -p $(BIN_DIR)
	CGO_ENABLED=1 go build -ldflags '$(GO_LDFLAGS)' -o $(BIN_DIR)/glean-thin-fast ./cmd/glean

prepare-fast:
	./scripts/prepare-embedded-model.sh fast

build-full-fast: build-llama build-bridge prepare-fast
	mkdir -p $(BIN_DIR)
	CGO_ENABLED=1 go build -tags embedded -ldflags '$(GO_LDFLAGS)' -o $(BIN_DIR)/glean-full-fast ./cmd/glean

static: build-llama build-bridge
	mkdir -p $(BIN_DIR)
	CGO_ENABLED=1 go build -ldflags '$(GO_LDFLAGS)' -tags 'osusergo netgo' -o $(BIN_DIR)/glean-static ./cmd/glean

static-thin-fast: build-llama build-bridge
	mkdir -p $(BIN_DIR)
	CGO_ENABLED=1 go build -ldflags '$(GO_LDFLAGS)' -tags 'osusergo netgo' -o $(BIN_DIR)/glean-thin-fast-static ./cmd/glean

static-full-fast: build-llama build-bridge prepare-fast
	mkdir -p $(BIN_DIR)
	CGO_ENABLED=1 go build -ldflags '$(GO_LDFLAGS)' -tags 'osusergo netgo embedded' -o $(BIN_DIR)/glean-full-fast-static ./cmd/glean

test: build-llama build-bridge
	go test -v ./...

clean-native:
	rm -rf $(BIN_DIR) $(BRIDGE_OBJ) $(BACKEND_OBJ)

clean:
	rm -rf $(BUILD_DIR) $(BIN_DIR) $(BRIDGE_OBJ) $(BACKEND_OBJ) glean glean-*-fast glean-static glean-*-static dist
