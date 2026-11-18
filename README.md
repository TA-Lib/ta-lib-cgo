# ta-lib-cgo

Go bindings for [TA-Lib](https://github.com/TA-Lib/ta-lib) that embed the C source directly —
no system library installation required. Users need only `go get`.  A working CGO environment is the only requirement, works out of the box with typical Go toolchain installations.

> **Note:** The maintainers of TA-Lib have graciously accepted this project under the official
> TA-Lib GitHub organization. The import path has changed to `github.com/TA-Lib/ta-lib-cgo`
> and this is the canonical location going forward.

> **Pure Go:** TA-Lib also publishes an official pure-Go module, [`ta-lib.github.io/go`](https://ta-lib.org/api/go/),
> generated from the same source as the C library: no cgo, batch and streaming. Use this package when you
> want the C library itself.

## Quick start

```go
import ta "github.com/TA-Lib/ta-lib-cgo"

// SMA over 10 prices with a 3-bar period.
prices := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
result := ta.Sma(prices, 3, nil)
// result: [NaN, NaN, 2.0, 3.0, 4.0, 5.0, 6.0, 7.0, 8.0, 9.0]

// MACD.
macd, signal, hist := ta.Macd(prices, 12, 26, 9, nil, nil, nil)
```

## outBuf pattern

Pass a slice as the last argument(s) to reuse an existing buffer and avoid allocation:

```go
buf := make([]float64, 0, 1000) // allocate once

for _, batch := range batches {
    result := ta.Sma(batch, 20, buf) // reuses buf if cap >= len(batch)
    process(result)
    buf = result // keep the buffer for next iteration
}
```

If `outBuf` is nil, its capacity is less than the input length, or it shares memory with an input, a new slice is allocated.

## Lookback and NaN padding

All output slices have the same length as the input. The first `Lookback` positions are filled
with `NaN` (the "warmup" period where insufficient data exists for a valid output). Integer outputs,
such as the candlestick patterns and `Maxindex`, hold `0` there.

```go
lb := ta.SmaLookback(3) // returns 2
// The first 2 values of Sma(..., 3, nil) will be NaN.
```

Use lookback functions to sub-slice into valid data:

```go
result := ta.Sma(prices, 20, nil)
lb := ta.SmaLookback(20)
validResult := result[lb:] // only valid (non-NaN) values
```

## Platform support

Requires CGO. Tested on macOS, Linux, and Windows 11 (MSYS2/GCC).

### Windows setup (MSYS2)

1. Download and install [MSYS2](https://www.msys2.org).
2. From the MSYS2 terminal, install GCC and Go:
   ```
   pacman -S mingw-w64-x86_64-gcc mingw-w64-x86_64-go
   ```
3. Use your project as normal — `go build` and `go test` work out of the box.

Or feel free to try using other C compilers, untested but might work just fine.

## Rationale

Existing Go TA-Lib approaches have significant trade-offs:

- **Hand-ported pure-Go reimplementations** diverge from the reference C implementation and require ongoing
  maintenance to stay in sync. Any upstream bug fix or edge-case change must be manually ported.
- **System-library wrappers** require TA-Lib to be pre-installed (`apt install`, `brew install`,
  etc.), complicating builds, CI, and distribution — especially across macOS and Linux.

This library takes a different approach: the TA-Lib C source is embedded directly as an
amalgamation (the `talib_amalgamation_NN.c` files), committed to the repository. CGO compiles it as
part of the normal `go build`. Algorithm correctness is guaranteed by the upstream C
implementation, and you get a single `go get` with no external dependencies.

The API is designed for high-frequency and batch processing workloads. Callers control output
allocation via the `outBuf` parameter pattern — buffers can be reused across calls to eliminate
allocations in tight loops.

## Tests

A sensible set of tests are included, some of them generated.  The objective with the test suite is to provide some decent coverage that provides some proof this thing actually works, while not introducing a significant maintenance burden of trying to exhaustively prove what TA-Lib already implements and handles.

Every TA-Lib function is also checked against `testdata/golden/`: the output of the official TA-Lib build of the same version, with default parameters, recorded by `cmd/golden`.  Those values were computed on another machine, and floating-point results can differ in the last bits between machines, so that comparison allows a relative error of 1e-9.  To check that the embedded C returns exactly the bits of the official build on your own machine (CI does this on its Linux and macOS runners):

```bash
go run ./cmd/golden -out /some/dir
TALIB_GOLDEN_DIR=/some/dir TALIB_GOLDEN_EXACT=1 go test -count=1 -run Golden ./...
```

## Maintenance - Updating TA-Lib

Updates are automated:

- **Update TA-Lib** (daily): when TA-Lib publishes a release, regenerates everything, runs the tests and opens a pull request.
- **Release**: merging that pull request tags the next minor version of this module.
- **Canary** (weekly): regenerates against TA-Lib's `dev` branch and opens an issue when that stops working, so a structural change shows up before the release that carries it.
- **CI**: tests on Linux (x86-64, arm64, musl), macOS and Windows, checks the bits against the official build, and checks that `go generate` changes nothing.

The workflows that publish (pull request, tag, issue) act only in `TA-Lib/ta-lib-cgo`; in a fork they do nothing.

To do the same by hand:

- Edit the version tag in the `//go:generate` directive in `talib.go` as needed.
- Run `go generate` from the repository root.
- Run `go test -v -count=1 ./...` to make sure it's all working
- Commit `include/`, `talib_amalgamation_*.c`, `*_gen.go`, `golden_gen_test.go` and `testdata/golden/`.

The `go generate` step requires git, CMake and a C compiler (`$CC`, else `cc`) on Linux or macOS.  It downloads the TA-Lib source and (re)generates the function wrappers and test code; `cmd/golden` then builds the official TA-Lib library from the same tag and records the output of every function in `testdata/golden/`.

The generator stops with an error, rather than guess, when TA-Lib changes in a way it does not understand: a function in `ta_func_api.xml` it did not parse from `ta_func.h`, a new kind of input, a missing file.  That is the case that needs a person.

## Versioning

This module uses its own semver sequence independent of TA-Lib's version numbering. The embedded TA-Lib version is available at runtime via the `TALibVersion` constant and is the single source of truth in the `//go:generate` directive in `talib.go`. (The reasoning: mirroring TA-Lib versions (e.g. `v0.6.4`) leaves no room for Go-side fixes without inventing non-standard version suffixes.)

To release:

- A TA-Lib update: merge its pull request.  The Release workflow tags the next minor version.
- A fix to the wrapper itself: `gh workflow run release.yml -f bump=patch` once it is on `main`.

## Repository structure

```
ta-lib-cgo
│
│   # Hand-written, committed once
├── talib.go                  CGO directives, init(), buffer/pointer helpers
├── errors.go                 TALibError type, retCodeMessage()
│
│   # Generated by cmd/gen — committed, do not edit by hand
├── talib_amalgamation_NN.c   All TA-Lib .c files, concatenated into 16 files
├── errors_gen.go             retCodeMessages map (from ta_retcode.csv)
├── functions_gen.go          Go wrapper for every TA-Lib indicator
├── lookback_gen.go           XxxLookback() companion for every indicator
├── version_gen.go            TALibVersion constant
│
│   # Test files
├── talib_test.go             Shape/invariant/behavior tests (no fixtures needed)
├── golden_test.go            Compares every function with testdata/golden
├── golden_gen_test.go        Generated by cmd/gen: one golden_test.go call per function
│
│   # Committed headers (populated by cmd/gen)
├── include/
│   ├── ta_libc.h             Public TA-Lib headers
│   ├── ta_func.h
│   ├── ta_common.h
│   ├── ta_defs.h
│   ├── ta_abstract.h
│   └── *.h                   Internal headers copied from src/ subdirectories
│
│   # Committed test fixtures (generated by cmd/golden)
├── testdata/golden/
│   ├── input.csv             Synthetic input: OHLCV, a second series, MAVP periods
│   └── <FUNC>.csv            Output of the official TA-Lib build per function
│
│   # Code generation tools
└── cmd/
    ├── gen/
    │   └── main.go           Clones TA-Lib, builds the amalgamation,
    │                         generates *_gen.go files; invoked via go generate
    └── golden/
        ├── main.go           Builds official TA-Lib, writes testdata/golden;
        │                     invoked via go generate
        └── c/dump.c          Runs every function through ta_abstract

    # Python benchmark (requires uv)
└── scripts/
    └── bench.py              Python benchmark counterpart for perf comparison
```

## Performance

Since both this library and Python's `ta-lib` package wrap the same C code, per-call performance should be equivalent. This was verified on Apple M1 Max with SMA, MACD, and ATR at 1k and 100k bars — Go with buffer reuse and Python were within noise of each other at every size. At 100k bars both are purely measuring the C library.

To reproduce:

```bash
# Go
go test -bench=. -benchmem -benchtime=3s -run='^$'

# Python (requires uv)
uv run ./scripts/bench.py
```
