// Package rng provides an explicitly seeded, reproducible PCG random stream.
// It preserves the golang.org/x/exp/rand sequence and counts underlying source
// draws so a host can restore the next value from a seed and call count.
//
// Construct a Source with New or Restore and retain its pointer. The host must
// serialize access, including observations and swap callbacks; Sources are not
// concurrency-safe. The package supplies no global stream, clock, implicit seed,
// persistence format, or cryptographic randomness.
package rng
