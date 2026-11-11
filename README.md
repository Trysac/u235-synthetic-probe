# synthetic-probe

[![Go Reference](https://pkg.go.dev/badge/github.com/Trysac/u235-synthetic-probe.svg)](https://pkg.go.dev/github.com/Trysac/u235-synthetic-probe)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![CI](https://github.com/Trysac/u235-synthetic-probe/actions/workflows/ci.yml/badge.svg)](https://github.com/Trysac/u235-synthetic-probe/actions/workflows/ci.yml)

Synthetic HTTP probes in Go with typed failure reasons, capped body reads, an injected clock and doer, and a rolling SLO tracker with error budget remaining.

## Problem

Teams want to know that the login page or the public API answers correctly from the outside, not only that the process is up. Ad hoc probe scripts check only the status code, read unbounded bodies, measure latency with the wall clock so tests are flaky, and never say how much error budget is left. This package runs one probe with ordered, typed checks and keeps a rolling window of results against an availability target.

## Install

```sh
go get github.com/Trysac/u235-synthetic-probe
```

Requires Go 1.23 or newer. Standard library only.

## Usage

```go
package main
import ("context"; "fmt"; "net/http"; "github.com/Trysac/u235-synthetic-probe")
func main() {
    r := &syntheticprobe.Runner{Doer: http.DefaultClient}
    res := r.Run(context.Background(), syntheticprobe.Probe{Name: "home", Method: "GET", URL: "https://example.com/"})
    fmt.Printf("%+v\n", res)
}
```

The example runs one probe and prints a `Result`. `Result` never contains headers, URL, query strings or body bytes.

## API

```go
type Probe struct {
    Name         string
    Method       string
    URL          string
    Header       http.Header
    Body         []byte
    ExpectStatus []int
    BodyContains string
    MaxLatency   time.Duration
    Timeout      time.Duration
}
func (p Probe) Validate() error
```

`Probe.Validate` returns an `*Error` with `Code: "invalid_probe"` when `Name` is blank, `Method` is outside GET, HEAD, POST, PUT, PATCH, DELETE, OPTIONS, the URL does not parse as http/https with a host and without userinfo, an `ExpectStatus` value is outside 100..599, or `Timeout` is negative. `ExpectStatus` defaults to `[]int{200}`; `Timeout` defaults to 10s; `MaxLatency` of 0 disables the latency check.

```go
type Doer interface { Do(*http.Request) (*http.Response, error) }

type Runner struct {
    Doer         Doer
    Now          func() time.Time
    MaxBodyBytes int64
    OnResult     func(Result)
}
func (r *Runner) Run(ctx context.Context, p Probe) Result

type Result struct {
    Probe   string
    OK      bool
    Failure string
    Status  int
    Latency time.Duration
    At      time.Time
}
```

`*http.Client` satisfies `Doer`. `MaxBodyBytes` defaults to 1,048,576 bytes. `Run` validates the probe first; an invalid probe returns `Failure: "invalid_probe"` without calling the `Doer`. Checks run in the order transport, status, body_too_large, body, latency, and the first failure wins. A context deadline error, or a transport error wrapping `context.DeadlineExceeded`, is `"timeout"`; any other `Do` or read error is `"transport"`. The response body is closed on every path. `OnResult` receives each `Result`; no logging is performed.

```go
type Report struct {
    Total           int
    Failures        int
    SuccessRatio    float64
    BudgetRemaining float64
    Breached        bool
}
func NewTracker(target float64, window int) (*Tracker, error)
func (t *Tracker) Add(r Result)
func (t *Tracker) Report() Report
```

`NewTracker` accepts `target` only in (0, 1) exclusive and `window` only in 1..100000; otherwise it returns an `*Error` with `Code: "invalid_target"`. The tracker keeps the last `window` results in a ring buffer, is safe for concurrent use, and computes `SuccessRatio = OK / Total` (1.0 when `Total == 0`), `allowed = (1 - target) * Total`, `BudgetRemaining = 1 - Failures / allowed` (1.0 when `Total == 0`), and `Breached = SuccessRatio < target`.

Errors are `*Error{Code, Msg}` with codes `invalid_probe` and `invalid_target`. The library does not panic on input, print or exit.

## Why not blackbox_exporter

blackbox_exporter is a standalone Prometheus exporter. This package is an embeddable Go library with injectable transport and clock, typed failure reasons and an error-budget view; it needs no Prometheus.

## Documentation

- [Architecture](docs/ARCHITECTURE.md) and [decisions](docs/decisions/)
- [Testing](docs/TESTING.md) and [security](docs/SECURITY.md)
- [Project record](docs/PROJECT.md) and [stack](docs/STACK.md)
- [Changelog](CHANGELOG.md) and [contributing guide](CONTRIBUTING.md)

## License

[MIT](LICENSE) (c) 2026 Miguel Gomez
