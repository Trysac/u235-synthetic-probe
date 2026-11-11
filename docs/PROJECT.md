# Project record

## Objective

Provide an embeddable Go library for synthetic HTTP probes with ordered, typed failure reasons, capped body reads, an injected clock and `Doer`, and a rolling SLO tracker that reports remaining error budget.

## Scope

- In scope: one probe run via `Runner.Run`, with validation, status checks, byte-substring body checks, latency bounds, timeout handling and a capped body read.
- In scope: a concurrency-safe `Tracker` over a ring buffer of the last `window` results, with `SuccessRatio`, `BudgetRemaining` and `Breached` in `Report`.
- In scope: public API documented in [README.md](../README.md), typed `*Error` values and deterministic tests with fake `Doer` and clock.
- Out of scope: scheduling probes (callers use a ticker), TLS certificate checks, DNS timing breakdown, multi-step scripted journeys and alert routing.

## Timeline

| Date | Event |
|---|---|
| 2026 | Repository created from the factory utility template. |

## Effort metrics

Measured values only; a value that has not been measured stays "pending measurement".

| Metric | Value |
|---|---|
| Tokens (input / output) | 1026359 / 71397 |
| Hours (human) | pending measurement |
| Cost (USD) | 0.0793 |

## Factory run u235 (measured from API usage)

| Date (UTC) | Phase | Model | Batch | Input | Output | Cache write | Cache read | Cost (USD) |
|---|---|---|---|---|---|---|---|---|
| 2026-11-10 | generation | deepseek-v4-pro | yes | 1559 | 13134 | 0 | 3328 | 0.0271 |
| 2026-11-10 | fix-1 | deepseek-flash | no | 16677 | 8349 | 0 | 237184 | 0.0082 |
| 2026-11-10 | fix-2 | deepseek-flash | no | 7518 | 1705 | 0 | 24192 | 0.0022 |
| 2026-11-10 | audit | deepseek-flash | no | 21579 | 14463 | 0 | 197248 | 0.0125 |
| 2026-11-10 | fix-3 | deepseek-flash | no | 13608 | 11514 | 0 | 286592 | 0.0098 |
| 2026-11-10 | threat-model | deepseek-flash | no | 22262 | 9841 | 0 | 151552 | 0.0097 |
| 2026-11-10 | fix-4 | deepseek-flash | no | 10492 | 583 | 0 | 27904 | 0.0020 |
| 2026-11-10 | docs | deepseek-flash | no | 4280 | 11808 | 0 | 384 | 0.0077 |

Total: $0.0793. Prices: deepseek line, as measured by its adapter. Providers serve the current version of each model id, so the date tells which version answered; versions, launch stages and price sources per line are in the factory's model register (docs/MODELS.md).
