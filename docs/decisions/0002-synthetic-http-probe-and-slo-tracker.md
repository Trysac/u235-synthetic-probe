# Synthetic HTTP probe and SLO tracker

Status: accepted (generated, pending human review)

## Context

Teams need to verify external endpoints from the inside of their systems, with ordered checks and an error budget view, without pulling in a full monitoring stack.

## Decision

Implement a single Go package with a Probe type, a Runner that injects the HTTP client and clock, and a Tracker that maintains a rolling window and computes SLO metrics using mutex for concurrency.

## Consequences

The library is embeddable and testable; no external dependencies; callers schedule probes with their own ticker; TLS and DNS details are out of scope.
