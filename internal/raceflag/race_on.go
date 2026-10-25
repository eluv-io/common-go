//go:build race

// Package raceflag reports whether the binary was built with the race detector, so tests can skip assertions that
// do not hold under it: sync.Pool drops about a quarter of its puts with the detector on, which makes pooled paths
// allocate.
package raceflag

// Enabled is true when the race detector is compiled in.
const Enabled = true
