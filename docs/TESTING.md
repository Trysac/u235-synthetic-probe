# Testing

## Running

```sh
go test ./...
go test -race ./...
go test -cover ./...
```

The suite uses standard Go `*_test.go` files in the root package. It does not require a separate test module or external services.

## Policy

- Tests exercise the public API and assert returned values, returned `*Error` codes and messages, and `Report` values.
- Probe tests use a fake `Doer` and a fake clock where latency or timestamps matter. There is no network access in those tests.
- The timeout test uses a real timer with a 20 ms timeout and a 1 s bound.
- Each bug fix starts with a failing test that reproduces it.
- Tests are deterministic and do not depend on test order.

## What the tests cover

- `Probe.Validate`: `invalid_probe` for a blank name, a disallowed method, `ftp://`, a URL with userinfo, an `ExpectStatus` value outside 100..599, and a negative `Timeout`; `Run` does not call the `Doer` for an invalid probe.
- `Runner.Run`: ordered failures `transport`, `status`, `body_too_large`, `body`, `latency`, with the first failure winning; default `ExpectStatus` of 200 and explicit multiple status codes; `MaxLatency` of 0 disables the latency check.
- Body handling: a body of 1,048,577 bytes with `MaxBodyBytes` 1,048,576 returns `body_too_large`; a body of exactly 1,048,576 bytes is accepted; the response body is closed on every path.
- Timeout handling: a blocking `Doer` with a 20 ms `Timeout` returns `timeout` in less than 1 s; a canceled parent context reports `timeout` only when the deadline passed, otherwise `transport`.
- Edge cases: HEAD with an empty body and a nil `Header`.
- `Result`: fields are populated; the value never contains headers, URL, query strings or body bytes.
- `Tracker`: `NewTracker` rejects target 1.0 or 0 and window 0 with `invalid_target`; after 19 OK results and 1 failure the report has `SuccessRatio` 0.95, `BudgetRemaining` 0.5 and `Breached` false; two more failures give `SuccessRatio` 0.85, `BudgetRemaining` -0.5 and `Breached` true; after 25 results only the last 20 count; `OnResult` receives each `Result`; concurrent `Add` and `Report` run under `-race`.

## Tiers

All current tests are standard `go test` tests and run in the local tier. There is no tagged conformance tier for this library.

## Layout

`*_test.go` files sit in the root package next to the code they test. The tests use small in-code values and do not require `testdata/` fixtures.
