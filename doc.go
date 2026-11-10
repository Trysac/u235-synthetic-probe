// Package syntheticprobe provides synthetic HTTP probes with typed failure reasons,
// capped body reads, an injectable clock and doer, and a rolling SLO tracker with
// error budget remaining.
//
// The probe result never carries headers, URL, or body bytes.
package syntheticprobe
