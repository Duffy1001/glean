# glean

Turn text from ordinary Unix commands, logs, files, and scripts into
schema-validated JSON without sending the data to a service.

```sh
some-command | glean --fields name,status | jq .
```

`glean` is a local command-line extraction tool. It combines a small GGUF
language model with JSON Schema constrained decoding, so it can handle text
that is difficult to parse with regular expressions while still producing a
machine-checkable result.

## Why Glean

Many useful data sources are neither APIs nor stable formats:

- A log line mixes timestamps, identifiers, and a free-form message.
- A command prints an aligned table whose spacing changes between versions.
- A support ticket contains a few facts buried in prose.
- A shell pipeline needs structured output but cannot depend on Python,
  credentials, a network service, or a separate application.

`glean` is intended for those gaps. A full release is a single executable with
the fast model included. It can run offline on a fresh machine, in an
air-gapped environment, or inside a small operational script. A thin release
is much smaller and downloads the model into a verified local cache on first
use.

The model is not a replacement for a deterministic parser when the input
format is already known and exact. For example, a purpose-built `lsblk` parser
is more reliable than any language model. `glean` is useful when writing and
maintaining that parser would be harder than describing the fields you need.

## Quick Start

Extract fields from prose:

```sh
printf '%s\n' \
  'Alice is 30 years old' \
  'Bob is 25 years old' |
  glean --fields name,age
```

```json
[
  {"age":"30","name":"Alice"},
  {"age":"25","name":"Bob"}
]
```

Parse a log stream with one record per line:

```sh
journalctl -o short-iso --no-pager |
  glean --fields timestamp,service,message --delimiter '\n' --compact
```

Read directly from files:

```sh
glean --fields host,status logs/*.txt
```

Extract a summary object with the built-in schema:

```sh
glean incident-notes.txt
```

The successful output is one JSON document on stdout. Normal progress is
silent. Errors go to stderr. Use `--verbose` for model, backend, timing,
chunking diagnostics.

## Installation

Install the small thin release on Linux or macOS:

```sh
curl -fsSL https://raw.githubusercontent.com/duffy1001/glean/master/install.sh | sh
```

Install the offline full release:

```sh
curl -fsSL https://raw.githubusercontent.com/duffy1001/glean/master/install.sh |
  GLEAN_VARIANT=full sh
```

Both installers place the selected executable at `glean`, verify its SHA-256
checksum, and install to `/usr/local/bin` when writable or `~/.local/bin`
otherwise. Set `GLEAN_INSTALL_DIR` to choose another directory. Set
`GLEAN_FORCE=1` for a noninteractive replacement.

Windows PowerShell:

```powershell
irm https://raw.githubusercontent.com/duffy1001/glean/master/install.ps1 | iex
```

The thin edition downloads the model on first use. The full edition contains
the model and extracts a verified copy to the model cache on first use so
llama.cpp can memory-map it. Neither edition embeds a network dependency.

## Schemas

`--fields` creates a root array of objects with exactly the named string fields:

```sh
printf '%s\n' \
  'db-01 is up' \
  'db-02 is down' |
  glean --fields host,status
```

For integers, booleans, enums, nested objects, or stricter contracts, provide a JSON Schema:

```json
{
  "type": "array",
  "items": {
    "type": "object",
    "properties": {
      "name": {"type": "string"},
      "age": {"type": "integer"},
      "active": {"type": "boolean"}
    },
    "required": ["name", "age", "active"],
    "additionalProperties": false
  }
}
```

```sh
glean --schema people.schema.json people.txt
```

The schema is used twice:

1. llama.cpp converts it to a grammar that constrains generation.
2. `glean` validates the final generated document against the original schema.

The supported grammar is the JSON Schema subset implemented by the pinned
llama.cpp revision, not full JSON Schema. A schema accepted by the validator
may still be rejected during grammar conversion. `--no-grammar` disables
constrained generation but does not disable final validation.

Grammar guarantees output shape, not factual accuracy. Review extracted data
when correctness is important.

## Large Input

Root-array schemas are processed incrementally. Records are split on
`--delimiter`, packed into context-sized chunks, extracted, validated, and
written as soon as each chunk completes.

Use `--delimiter '\n'` for line-oriented input, `--delimiter '\0'` for
NUL-delimited input, or any other string, including multi-character separators
like `||`.

Use `--atomic` when downstream tools require a complete JSON array on stdout.
It buffers array items and emits nothing unless the full extraction succeeds.

```sh
cat application.log |
  glean --fields timestamp,level,message --delimiter '\n'
```

Use NUL-delimited input when records can contain newlines:

```sh
git ls-files -z |
  glean --delimiter '\0' --fields path --compact
```

Oversized records are split and retried. Output is one JSON array, but its items
are written incrementally by default. If a later chunk fails, the command exits
nonzero and stdout may contain a partial JSON stream unless `--atomic` is used.

## Local Devices

Release binaries include CPU support and an optional platform accelerator:

- Linux and Windows: Vulkan
- macOS: Metal with embedded shader source

GPU drivers and the Vulkan loader are optional system capabilities. If they are
missing, `glean` continues on CPU. Inspect what the binary can see:

```sh
glean --report
```

By default, `--device auto` uses a detected GPU or integrated GPU. If no
accelerator is available, it uses CPU. Force a mode when needed:

```sh
glean --device gpu --fields name,status input.txt
glean --device cpu --fields name,status input.txt
```

Automatic GPU initialization failures retry on CPU. `--device gpu` is strict
and fails when no usable accelerator is detected. `--gpu-layers 0` disables
offload; `--gpu-layers -1` means all available layers.

For the small fast model, GPU startup and dispatch overhead can outweigh the
benefit on tiny inputs. On one local machine, a 40-record workload measured:

| Device | Generation speed | Generation time |
| --- | ---: | ---: |
| 13th-gen Intel CPU | 26.9 tok/s | 48.1 seconds |
| RTX 4070 Ti via Vulkan | 47.1 tok/s | 27.5 seconds |

These are representative measurements, not guarantees. Workload size, driver,
backend, CPU, and model output length all matter.

## Models And Releases

The supported model is Qwen3 0.6B Q4_K_M:

| Edition | Executable size | Model behavior |
| --- | ---: | --- |
| `thin-fast` | about 15 MB | Downloads the model on first use |
| `full-fast` | about 400 MB | Includes the model for offline use |

Release assets are named:

```text
glean-thin-fast-<os>-<arch>
glean-full-fast-<os>-<arch>
```

The release workflow uploads these binaries and `checksums.txt` to GitHub
Releases, so users can download them from the release page.

The installer renames the selected asset to `glean`. Published SHA-256 values
are in `checksums.txt`. Current release targets are Linux amd64/arm64, macOS
arm64, and Windows amd64.

Models are cached under:

- `$XDG_CACHE_HOME/glean/models` when `XDG_CACHE_HOME` is set
- `~/Library/Caches/glean/models` on macOS
- `~/.cache/glean/models` on other platforms

## Options

| Option | Default | Description |
| --- | --- | --- |
| `--schema FILE` | | JSON Schema used for generation and validation |
| `--fields LIST` | | Comma-separated string fields for array extraction; names must be unique and non-empty |
| `--model NAME` | `fast` | Supported model: `fast` |
| `--max-tokens N` | `2048` | Maximum generated tokens per inference chunk |
| `--ctx N` | `8192` | Model context window |
| `--atomic` | `false` | Buffer array output and only emit it after all chunks succeed |
| `--delimiter STRING` | `\n` | Record separator for array extraction; supports `\n`, `\t`, `\r`, `\0`, `\\`, and multi-character strings |
| `--threads N` | `4` | CPU inference threads |
| `--device NAME` | `auto` | Device policy: `auto`, `cpu`, or `gpu` |
| `--gpu-layers N` | `-1` | Layers to offload when GPU inference is selected |
| `--compact` | `false` | Print compact rather than indented JSON |
| `--no-grammar` | `false` | Disable grammar constraints; validation remains enabled |
| `--verbose` | `false` | Write diagnostics to stderr |
| `--version` | `false` | Print version and build variant |
| `--report` | `false` | Print detected backends, devices, memory, and default selection |

`--schema` takes precedence over `--fields`. Positional arguments are input
file paths. With no positional arguments, `glean` reads stdin.

## Build From Source

Requirements:

- Go 1.26 or newer
- Git and Make
- CMake
- A C compiler and a C++17 compiler
- CGO enabled
- Vulkan headers, `glslc`, and SPIR-V headers on Linux/Windows build hosts

```sh
cd glean
make setup
make
./bin/glean --report
```

Run tests:

```sh
make test
```

Build the two local editions:

```sh
make build-thin-fast
make build-full-fast
```

Build release assets for the current Linux or macOS platform:

```sh
./release.sh all
```

The native build disables network support, examples, tests, server components,
shared libraries, and OpenMP in llama.cpp. Runtime GPU drivers remain optional.
