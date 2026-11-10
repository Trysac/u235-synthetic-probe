# Architecture

## Purpose

`github.com/Trysac/u235-synthetic-probe` is a small, standard-library-only Go library. It runs one synthetic HTTP probe with ordered, typed failure reasons, capped body reads, an injectable `Doer` and clock, and keeps a rolling window of results against an availability target.

## Shape

```text
caller code --> package syntheticprobe (repository root)
                    |
                    +-- Probe, Runner, Result
                    +-- Tracker, Report, Error
```

The module exposes one public package, `syntheticprobe`, at the repository root. Its public API is `Probe`, `Doer`, `Runner`, `Result`, `NewTracker`, `Tracker`, `Report` and `Error`.

- **Probe execution**: `Runner.Run` validates a `Probe`, builds an `*http.Request` with `http.NewRequestWithContext`, calls the injected `Doer`, reads a capped body and returns a `Result`. It uses `Runner.Now` for timestamps and latency.
- **Failure reasons**: `Result.Failure` is one of the typed values used by the code: `invalid_probe`, `timeout`, `transport`, `status`, `body_too_large`, `body` or `latency`. Checks run in that order after validation, and the first failure wins.
- **Errors**: invalid `Probe` and invalid tracker parameters return `*Error` with codes `invalid_probe` and `invalid_target`. `Run` reports probe failures through `Result.Failure`, not by panicking or exiting.
- **Side effects**: the only outbound I/O is the caller-supplied `Doer`. `OnResult` is the only callback. The package performs no logging and does not keep global mutable state.
- **Concurrency**: `Tracker` protects its ring buffer with a mutex, so `Add` and `Report` are safe for concurrent use.

## Quality contract

The test suite exercises the public API with deterministic fake `Doer` and clock values, plus a real-timer timeout test with a 20 ms timeout and a 1 s bound. See [TESTING.md](TESTING.md).

## Decisions

Architecture decisions are recorded in [decisions/](decisions/) as numbered ADRs.
