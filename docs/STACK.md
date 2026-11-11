# Stack

## Used

| Tool | Why |
|---|---|
| Go >= 1.23 | Oldest supported release line; one toolchain covers build, test and format. |
| Go modules | Built-in dependency management with checksum verification (`go.sum`). |
| Standard library only | The library depends on `net/http`, `net/url`, `context`, `io`, `time` and `sync`; no external modules. |
| gofmt + go vet | The canonical formatter and static checks; no configuration to maintain. |
| go test (race detector, cover) | Built-in tests, runnable examples, race detection and coverage. |

## Discarded

| Tool | Why not |
|---|---|
| Third-party HTTP client or probe libraries | `net/http` plus the injectable `Doer` interface cover the required behavior without dependencies. |
| testify or other assertion packages | The standard `testing` package keeps the module dependency-free. |
| A standalone exporter | The package is an embeddable library; callers choose their own scheduling and metrics. |
