# Audio catalog contract

The `catalog` package builds a detached inventory/report from an explicit manifest
and supplied asset filesystem/root. It creates no players or device context.
The standalone command takes `-library`, optional manifest-relative `-asset-root`,
and explicit JSON `-output` / Markdown `-guide` destinations. See the
[command/build guide](../README.md). Consumer source scanning, world registries,
review notes and proposed gameplay moments are separate enrichment.

## Identity, measurements and evidence

Assets expose recording identity, exact path, encoded SHA-256, format/duration,
optional sample peak/RMS, authored calibration/window/provenance and inverse cue
usage. Registered empty slots and unregistered files remain visible. Moments link
actual recording assignments/cycles and authored context. Encoded hashes identify
the supplied bytes, not creator/license validity or perceptual quality.

`-measure` streams whole-recording 44.1 kHz stereo signed 16-bit PCM. Duration uses
sample rate and four bytes per frame. Sample peak and RMS are not LUFS, true peak
or listening scores. Null level with `pcmMeasured` means silence; without it, not
measured. Silence depresses whole-file RMS; a peak-normalized impact can still sound
louder than a bed. Region calibration uses the separate authoring estimators.

Structural validation rejects dangling forward/inverse links. Reports escape authored
text and preserve provenance independently of review notes. Consumer evidence can
describe intended triggers but must not turn declarations/static references into
reachability or listening approval. Availability, assignment review, listening/loop
review and trigger coverage are independent dimensions. Generation never accepts a
recording or modifies the authoring manifest.

Output must not alias source inputs. Both destinations are checked against each
other and all inputs before either write. A genuinely missing path is allowed;
unexpected inspection failures are returned with context rather than treated as
absence. Explicit developer-selected paths are not restricted to a sandbox.
Two report outputs are not an atomic transaction:
if the second write fails, the first may exist. Derived reports are regenerable
artifacts, not authored source, persistent mixer sessions or a second backlog.
Consumers maintain semantic cue IDs, classification policy, actual event evidence,
production-asset notices and their contextual listening procedures.
