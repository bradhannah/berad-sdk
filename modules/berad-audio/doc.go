// Package audio implements Berad Audio's cue playback and owned voice lifetimes.
//
// Libraries freeze a caller-supplied playback Definition and asset filesystem.
// Runtime borrows a backend, owns its sources/voices, and requires regular Update
// calls for envelopes and retirement. Neither gameplay clocks nor game types enter
// this package. The backend/ebitengine package supplies the supported game adapter.
//
// The initial API is experimental. Definition is an in-memory playback contract,
// not a new persisted authoring format. The existing Redux TUI/manifest remain
// unchanged while the independent module is staged for consumer integration.
package audio
