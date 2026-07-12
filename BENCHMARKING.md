# Benchmarking And Evaluation

## Evaluator modes

Run the evaluator against the versioned corpus:

```sh
go run ./cmd/glean-eval --mode quality --corpus benchdata/corpus-v1
go run ./cmd/glean-eval --mode warm --corpus benchdata/corpus-v1 --warmups 2
go run ./cmd/glean-eval --mode subprocess --corpus benchdata/corpus-v1 --binary ./bin/glean
```

The JSON report includes:

- Per-sample extraction metrics (warm/quality modes).
- Per-sample quality score (`exact_match`) and a `valid` boolean.
- An aggregate `quality` summary.
- Basic machine/build `metadata`.

## Microbenchmarks

Deterministic microbenchmarks (no model required):

```sh
make bench-micro
# or
go test -run '^$$' -bench . -benchmem ./...
```

## Corpus

The evaluator loads `benchdata/<corpus>/manifest.json` and then reads each case directory:

```text
case.json
input.txt (or input.bin)
schema.json
expected.json
```
