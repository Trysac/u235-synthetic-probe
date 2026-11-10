// Package syntheticprobe is a small, dependency-free Go library: Synthetic HTTP probes in Go with typed failure reasons, capped body reads, an injected clock and doer, and a rolling SLO tracker with error budget remaining
//
// The public API is every exported identifier of this root package. Functions are pure: they do
// no I/O, keep no global mutable state and report invalid input through returned errors.
package syntheticprobe
