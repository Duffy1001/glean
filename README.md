# glean

`glean` turns unstructured text into schema-validated JSON using a small local
language model. It runs entirely on your machine through llama.cpp, constrains
generation with JSON Schema, and writes only JSON to standard output.

```sh
printf '%s\n' \
  'Alice is 30 years old' \
  'Bob is 25 years old' |
  glean --fields name,age
```

```json
[
  {
    "age": "30",
    "name": "Alice"
  },
  {
    "age": "25",
    "name": "Bob"
  }
]
```

## Features

- Local GGUF inference with no hosted API or API key
- JSON Schema constrained generation through llama.cpp
- Final output validation against the original schema
- Automatic chunking for streamed array extraction
- Overlap-aware merging and primary-key deduplication
- A small Qwen3 model tuned for fast local extraction
- Pure JSON on stdout; optional diagnostics on stderr
- Automatic backend discovery with safe CPU fallback

## Install

Install the latest thin release on Linux or macOS:

```sh
curl -fsSL https://raw.githubusercontent.com/duffy1001/glean/master/install.sh | sh
```

Install the full-fast edition, which includes the fast model:

```sh
curl -fsSL https://raw.githubusercontent.com/duffy1001/glean/master/install.sh |
  GLEAN_VARIANT=full GLEAN_MODEL=fast sh
```

The installer supports `amd64` and `arm64`, verifies the release checksum, and
installs to `/usr/local/bin` when writable, otherwise to `~/.local/bin`. Set
`GLEAN_INSTALL_DIR` to choose another directory or `GLEAN_FORCE=1` for a
noninteractive replacement.

On Windows PowerShell:

```powershell
irm https://raw.githubusercontent.com/duffy1001/glean/master/install.ps1 | iex
```

Thin editions download their default model on first use. Full editions include
the fast model and work offline; on first use they extract a verified
copy to the model cache so llama.cpp can memory-map it. Selecting a different
model is not supported. Run with `--verbose` to see model
loading, extraction, and download status.

## Usage

Read from stdin:

```sh
someSource | glean --fields name,age --pk name
```

Read one or more files:

```sh
glean --fields host,status logs/*.txt
```

Summarize input with the built-in schema:

```sh
glean report.txt
```

Use a custom schema:

```sh
glean --schema schema.json input.txt
```

Produce compact JSON:

```sh
glean --compact --fields name,email contacts.txt
```

Successful runs write one JSON document followed by a newline to stdout.
Normal progress is silent. Errors are written to stderr, and `--verbose`
enables model, chunking, timing, and deduplication diagnostics.

## Field Extraction

`--fields` is a shorthand for an array schema. Every named field is required
and represented as a JSON string.

```sh
printf '%s\n' \
  'db-01 status is up' \
  'db-02 status is down' |
  glean --fields host,status
```

Use `--pk` when overlapping chunks or repeated source records should be
deduplicated and merged:

```sh
glean --fields timestamp,host,status --pk host events.log
```

Records with the same primary key are merged in first-seen order. Fields from
later records are added, and later values win when the same field conflicts.
This also removes duplicates introduced by overlapping chunks.

## Streaming And Chunking

Schemas with root `"type": "array"` are processed incrementally from stdin or
files. `glean` splits input at line boundaries, runs extraction for each chunk,
validates every result, and merges the arrays before printing the final JSON
document. Chunks do not overlap by default. Supplying `--pk` enables a small
overlap, followed by primary-key merging. Oversized or token-truncated chunks
are split and retried.

This keeps input memory bounded for commands such as:

```sh
journalctl | glean --fields timestamp,service,message --pk timestamp
```

Output is accumulated before it is printed because final validation and
primary-key merging require the complete result. Schemas with a non-array root
are processed as one input and are not merge-chunked.

## Custom Schemas

Given `people.schema.json`:

```json
{
  "type": "array",
  "items": {
    "type": "object",
    "properties": {
      "name": { "type": "string" },
      "age": { "type": "integer" },
      "active": { "type": "boolean" }
    },
    "required": ["name", "age", "active"],
    "additionalProperties": false
  }
}
```

Run:

```sh
glean --schema people.schema.json --pk name people.txt
```

The schema is used twice:

1. llama.cpp converts it to a grammar that constrains token generation.
2. `glean` validates the generated document against the original schema.

Grammar conversion supports the JSON Schema subset implemented by the pinned
llama.cpp version, including common object, array, enum, union, reference, and
constraint forms. It is not full JSON Schema support. A valid schema may still
be rejected if llama.cpp cannot convert it. `--no-grammar` skips constrained
generation but keeps final schema validation enabled.

Grammar constraints enforce output shape, not factual accuracy or extraction
completeness. Review results when correctness is critical.

## Models

| Choice | Model | Quantization | Download | Use case |
| --- | --- | --- | ---: | --- |
| `fast` | Qwen3 0.6B | Q4_K_M | about 400 MB | Default, lower latency |
Select a model with:

```sh
glean --model fast --schema schema.json input.txt
```

Downloads are SHA-256 verified and cached under:

- `$XDG_CACHE_HOME/glean/models` when `XDG_CACHE_HOME` is set
- `~/Library/Caches/glean/models` on macOS
- `~/.cache/glean/models` on other platforms

## Release Editions

Each supported platform has two release assets:

| Edition | Model | Typical size | Model behavior |
| --- | --- | ---: | --- |
| `thin-fast` | Qwen3 0.6B | about 15 MB | Downloads the model on first use |
| `full-fast` | Qwen3 0.6B | about 400 MB | Includes the model |

Asset names follow `glean-{thin|full}-fast-{os}-{arch}` with `.exe` on
Windows. The installer always installs the selected asset as `glean`.
Published SHA-256 values are in `checksums.txt`. Current release targets are
Linux amd64/arm64, macOS arm64, and Windows amd64.

The amd64 CPU build targets an AVX2, BMI2, FMA, and F16C baseline. ARM builds
are produced natively for their target architecture.

## Options

| Option | Default | Description |
| --- | --- | --- |
| `--schema FILE` | | JSON Schema used for generation and validation |
| `--fields LIST` | | Comma-separated string fields for array extraction |
| `--pk FIELD` | unset | Primary key used to merge array records |
| `--model NAME` | `fast` | Model choice: `fast` |
| `--max-tokens N` | `2048` | Maximum generated tokens per inference chunk |
| `--ctx N` | `8192` | Model context window |
| `--chunk-lines N` | `4` | Maximum source lines per root-array inference chunk; use `1` for one source line per inference or `0` to disable the line cap |
| `--threads N` | `4` | CPU inference threads |
| `--device NAME` | `auto` | Device policy: `auto`, `cpu`, or `gpu` |
| `--gpu-layers N` | `-1` | Layers to offload when GPU inference is selected |
| `--compact` | `false` | Print compact rather than indented JSON |
| `--no-grammar` | `false` | Disable grammar constraints; validation remains enabled |
| `--verbose` | `false` | Write progress and native diagnostics to stderr |
| `--version` | `false` | Print the version and build variant |
| `--report` | `false` | Print detected backends, devices, memory, and default selection as JSON |

`--schema` takes precedence when both `--schema` and `--fields` are supplied.
Positional arguments are input file paths. With no positional arguments,
`glean` reads stdin.

## Build From Source

Requirements:

- Go 1.26 or newer
- Git and Make
- CMake
- A C compiler and a C++17 compiler
- CGO enabled
- Vulkan headers, `glslc`, and SPIR-V headers on Linux/Windows build hosts

Clone and build:

```sh
git clone https://github.com/duffy1001/glean.git
cd glean
make setup
make
```

`make setup` checks out the pinned llama.cpp revision. The resulting executable
is `./bin/glean`.

Run unit tests:

```sh
make test
```

Build a stripped Linux executable with statically linked C++ runtimes:

```sh
make static
```

Build a named edition. Full editions download, verify, and zstd-compress their
matching model into an ignored local asset before linking:

```sh
make build-thin-fast
make build-full-fast
```

Build both release editions for the current Linux or macOS platform:

```sh
./release.sh all
```

The native build intentionally disables network support, examples, tests,
server components, shared libraries, and OpenMP in llama.cpp.

## Device Selection

Release binaries contain CPU support plus the platform accelerator backend:

- Linux and Windows include Vulkan.
- macOS includes Metal with embedded shader source.

GPU drivers and the Vulkan loader are optional system capabilities, not hard
runtime dependencies. If they are missing or unusable, `glean` continues with
CPU. Use `glean --report` to inspect what the process sees.

`auto` prefers a detected GPU or iGPU on every platform. `--device cpu` always
disables offload, while `--device gpu` requires an available accelerator.
Automatic GPU initialization failures retry on CPU. For very small inputs,
GPU startup and dispatch overhead may still make CPU faster.
