# Security notes

## Threat model

`github.com/Trysac/u235-synthetic-probe` is a Go library. `Runner` sends an HTTP request through the caller-supplied `Doer`; the package performs no logging and keeps no global mutable state.

### Defended

- Invalid probes: `Probe.Validate` rejects a blank `Name`, methods outside GET, HEAD, POST, PUT, PATCH, DELETE, OPTIONS, URLs that do not parse as http/https with a host and without userinfo, `ExpectStatus` values outside 100..599, and negative `Timeout`. `Run` returns `Failure: "invalid_probe"` and does not call the `Doer`.
- Invalid tracker parameters: `NewTracker` accepts `target` only in (0, 1) exclusive and `window` only in 1..100000; otherwise it returns `*Error{Code: "invalid_target"}`.
- Response body size: `Runner.MaxBodyBytes` defaults to 1,048,576 bytes. The runner reads at most `MaxBodyBytes+1` bytes with `io.LimitReader`; a larger body yields `Failure: "body_too_large"`. The response body is closed on every return path.
- Timeouts: `Probe.Timeout` defaults to 10s. `Run` uses `http.NewRequestWithContext` with `context.WithTimeout`; a deadline error, or a transport error wrapping `context.DeadlineExceeded`, yields `Failure: "timeout"`.
- Result exposure: `Result` has only `Probe`, `OK`, `Failure`, `Status`, `Latency` and `At`; it never carries headers, URL, query strings or body bytes. `OnResult` is the only callback.
- Concurrent tracker use: `Tracker` protects its ring buffer with a mutex; `Add` and `Report` are safe for concurrent use.
- Dependencies: standard library only.

### Not defended

- The library does not perform TLS certificate checks itself; it relies on the `Doer` (for example `*http.Client`) for TLS validation.
- It does not restrict target hosts or IP addresses and does not claim to prevent SSRF or DNS rebinding; callers must control probe URLs and the `Doer`.
- A caller-supplied `Doer` can perform arbitrary I/O; the library cannot constrain what that implementation does.
- It does not schedule probes, route alerts, run multi-step journeys, break down DNS timing or check certificate expiry.
- It does not redact data logged by caller code; it only keeps headers, URL and body out of `Result`.

## Publishing

Releases are git tags served through the Go module proxy and checksum database. No registry token is stored in the repository.

To report a vulnerability, see [SECURITY.md](../SECURITY.md).
