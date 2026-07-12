.PHONY: all clean clean-native setup configure-llama build-llama build-bridge build-go build-full prepare-model test static static-full release

LLAMA_DIR := llama.cpp
BUILD_DIR := build
CBRIDGE_DIR := cbridge
VERSION ?= dev
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
		-DGGML_F16C=ON

build-llama: configure-llama
	cmake --build $(BUILD_DIR) --config Release -j $(shell nproc 2>/dev/null || echo 4) -- llama common ggml

build-bridge: setup
	$(CXX) -c -O2 -std=c++17 \
		-I$(CURDIR)/$(LLAMA_DIR)/include \
		-I$(CURDIR)/$(LLAMA_DIR)/common \
		-I$(CURDIR)/$(LLAMA_DIR)/ggml/include \
		$(VENDOR_INC) \
		$(CBRIDGE_DIR)/schema_bridge.cpp -o $(BRIDGE_OBJ)

build-go: build-llama build-bridge
	CGO_ENABLED=1 go build -ldflags '$(GO_LDFLAGS)' -o glean .

prepare-model:
	./scripts/prepare-embedded-model.sh

build-full: build-llama build-bridge prepare-model
	CGO_ENABLED=1 go build -tags embedded -ldflags '$(GO_LDFLAGS)' -o glean-full .

static: build-llama build-bridge
	CGO_ENABLED=1 go build -ldflags '$(GO_LDFLAGS) -extldflags=-static' -tags 'osusergo netgo' -o glean-static .

static-full: build-llama build-bridge prepare-model
	CGO_ENABLED=1 go build -ldflags '$(GO_LDFLAGS) -extldflags=-static' -tags 'osusergo netgo embedded' -o glean-full-static .

test: build-llama build-bridge
	go test -v ./...

clean-native:
	rm -f glean glean-full glean-static glean-full-static $(BRIDGE_OBJ)

clean:
	rm -rf $(BUILD_DIR) $(BRIDGE_OBJ) glean glean-full glean-static glean-full-static dist
