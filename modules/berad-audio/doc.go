// Package audio implements Berad Audio's cue playback and owned voice lifetimes.
//
// Libraries freeze a caller-supplied playback Definition and asset filesystem.
// Runtime borrows a backend, owns its sources/voices, and requires regular Update
// calls for envelopes and retirement. Neither gameplay clocks nor game types enter
// this package. The backend/ebitengine package supplies the supported game adapter.
//
// The initial API is experimental. Definition is an in-memory playback contract,
// not a persisted authoring format. Package authoring owns schema-3 documents,
// source analysis and publication; the separately versioned tools module owns
// terminal UI and native probes. Consumers supply policy and media.
package audio
