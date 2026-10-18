# Berad Audio tools

Experimental standalone tools and reusable tool services, versioned separately
from the audio runtime/authoring module. Go 1.27.0 is the declared minimum; the
selected/tested toolchain is Go 1.27.1. Runtime dependencies are pinned in `go.mod`.
Project-authored source is MIT; see [LICENSE](LICENSE). No game media is included.

## Current commands

`cmd/audio-probe` analyzes an explicitly selected WAV/MP3/OGG source with `-check`
or runs a bounded standalone Oto audition. `-file` is required; there is no game
checkout/default manifest lookup. `-check` opens no audio device and writes no
source data. Device probing is native acceptance only when actually run on the
named host; headless checks and compilation are separate evidence.

The existing terminal editor and catalog command are being extracted next; they
are not yet available in this tools module. Their Redux commands remain usable.

## Reusable audition

`audition.NewVoice` opens no device until Play. Controls are event-loop serialized;
native reading uses the shared SDK window/gain machinery. Play prepares before
replacement, Pause retains transport, and Seek/Position use source milliseconds.
Gain updates never restart or change authored metadata. Close stops native reads
before closing its stream, is idempotent, and leaves the process-owned device alive.
Failure inspection is observational. Use this in a standalone tool process, not
alongside an independently created Ebitengine/Oto context in a game process.

## Build and checks

From this checkout, `make build` creates the probe under the SDK's `.cache/bin`.
`make clean` removes only known tool executables, preserving validation and authored
data. `make provision` explicitly provisions dependencies; it is network-enabled
and separate from offline checks. `make plan` and `make quality-gate` delegate to
the SDK checker with this module selected. The downloaded Go module does not need
repository Make/checker scripts to build its commands.

The checker tests published dependency pins and, separately, declared candidate
runtime integration. Both use isolated consumers and reject game dependencies.
See the [SDK validation contract](https://github.com/bradhannah/berad-sdk/blob/feat/berad-audio/docs/VALIDATION.md).
