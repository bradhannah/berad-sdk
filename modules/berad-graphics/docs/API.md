# Graphics API and ownership

## Public boundary and exposure

| Surface | Consumer use | Ownership and coupling |
| --- | --- | --- |
| Root `Image`, `Mask`, `Selection`, `Operation` | Offline pixel preparation and explicit selections | Immutable CPU snapshots; returned pixel/RGBA copies belong to callers. Standard `image`/`color` types only. |
| `CycleIndex`, `Pulse` | Explicit cycle/phase sampling | Supplied time/ticks; host-owned frame arrays, placement and clock conversion. |
| `Dissolve`, `Shuffler`, `DitherKeeps` | Coverage/permutation and ordered thresholds | Caller serializes mutable dissolve; injected RNG is borrowed only during construction. |
| Backend `Processor` | Native import, derivation and silhouette | Construct with explicit positive limits or `DefaultLimits`; zero/closed processor rejects work with `ErrClosed`. |
| Borrowed `*ebiten.Image` sources/results | Integrate existing host draw paths | Inputs immutable until explicit `Forget`; returned cached images must not be mutated/deallocated and survive cache closure. |
| `CopyRegion`, `Surface` | Owned region copies and changing upload buffers | Copies transfer native ownership. Surface zero value is usable; close is terminal and invalidates its borrowed image/buffer. |
| `ImageCache` | Retain variants under host-defined collision-free keys | Host-serialized, bounded references; removal/clear never disposes borrowed images. |
| `*ebiten.Image` targets, `DrawImageOptions`, `GeoM`, blend/filter | Native composition with supplied transforms and clipping | Caller owns target; options passed by value are not mutated. Ebitengine 2.10.4 types/semantics are a deliberate compatibility boundary. |
| `DrawAdditiveChannels` | Explicit independent-channel draw treatment | Caller-owned target, supplied RGB/alpha and blend; no illumination discovery. |

The backend imports are deliberate under SDK-API-002; there is no alternate
renderer interface. Source GPU readback requires a running Ebitengine loop.
Known disposed inputs return errors; unrelated engine/programming panics are not
recovered into successful fallback artwork. Invalid external definitions wrap
`graphics.ErrInvalid`; `errors.Is` can distinguish them. CPU resources require
constructors before use; zero operations are invalid definitions.

## Images and operations

`NewPixels` snapshots tightly packed RGBA channel data at a zero origin. Returned
pixel/image copies are owned by callers; operations never mutate inputs. Images
are bounded to 8192 pixels per axis and 32 MiB of channel data. Invalid dimensions,
nonfinite parameters, unsupported operations and mismatched masks are errors.
`CycleIndex` accepts 1 through 65536 frames and samples integer ticks, allowing host clocks to retain
their own conversion/rounding policy without overflowing tick-plus-offset.
`Dissolve.UpdatePixels` updates only changed cells in an initialized caller buffer;
its changed flag lets the host avoid redundant uploads. Reset requires a fresh
`WritePixels` into that buffer. No updates or resets consume additional randomness.

Operations execute in order. Selectors choose destination writes, not admissible
source samples. Scroll wraps with positive modulo; wave/rise clamp samples to the
selected rectangular bounds. Palette/terrain selection is supplied by the host.
Operation factors are in [0,8192], with alpha factors limited to [0,1]. Wave
phases/row phases are finite within +/-2^30; pulse endpoints are within +/-8192.
These limits bound arithmetic/work rather than silently accepting overflowing inputs.

RGB gain is independent channel multiplication with saturation, including RGB in
alpha-zero pixels. Zero gain makes RGB black regardless of alpha; alpha itself is
unchanged. Alpha-channel
scaling is explicitly distinct from conventional opacity: it preserves RGB and
can represent emissive data. It exists for compatibility with deliberate channel
effects, not as an ordinary premultiplied-opacity promise. Additive alpha pulse
also preserves RGB contribution while modulating alpha. These operations retain
existing consumer appearance; renaming does not silently change their maths.

Outlines use four-neighbor edge detection and eight-neighbor expansion into
transparent pixels, including holes. Padding is explicit; nothing infers that an
18-pixel image was previously padded. Contour-only output clears every source body
pixel, including partial alpha, after applying face and backing rings.

## Execution and integration

CPU images/masks are immutable and can be read independently. Mutable surfaces
and Ebitengine processors require host serialization. No implicit clock,
input polling, background worker or gameplay RNG is created. Host-supplied random
ordering is consumed during dissolve construction only; reset/reverse reuse it.

GPU import is an explicit compatibility boundary for existing Ebitengine hosts:
sources are borrowed immutable images. After host mutation, invalidate their
snapshot before reuse. CPU imports instead own copies from the start. Both source
snapshots and derived images have bounded cache retention. Keys retain complete
source/operation identities, not hashed raw pointer addresses or quantized values.
`CopyRegion` prepares an owned zero-origin native region with explicit transparent
insets. Prepare that source before requesting effects; padding is not a processor
operation and dimensions never imply that padding has already been applied.
`ImageCache` separately retains host-created variants under explicit collision-free
scope keys. Hosts own those keys' meaning; entry/byte limits apply without interpreting
game identities. Oversized replacement bypass removes the old entry rather than
returning stale artwork, and invalid replacement leaves the prior entry intact.

Derived GPU images returned to existing host draw code are read-only borrows.
Eviction/processor closure drops cache references, never invalidates a retained
borrow, and makes no immediate-VRAM reclamation claim. Borrowers must neither
mutate nor deallocate these images. Explicit owned copies have separate lifetimes.
Mutable mask/dissolve surfaces own their resources and release them on closure;
their borrowed views must not outlive that owner. Caller drawing targets survive.

Default processor limits are 512 source entries and 512 derived entries with
separate 32 MiB payload budgets. Derived accounting includes retained input and
operation operands; immutable snapshots may be counted conservatively more than
once. Oversized valid results bypass retention. Active CPU images, host frame arrays and
externally retained GPU results remain host-owned allocations beyond these caches.
Each dissolve additionally bounds its coordinate ordering to 32 MiB. These are
per-resource/cache limits, not a global memory manager or peak-allocation promise.

`Clear` drops cached references while leaving a processor usable; `Forget` retires
the currently retained GPU-source snapshot and its derivations before re-import.
Already returned results keep their old pixels. `Close` is terminal and idempotent.
`Stats` returns detached source/derived entry counts and payload bytes and does not
generate images. No hit/miss/eviction counters are exposed. `ImageCache.Size` and
`Bytes` likewise observe retention without changing LRU order.

`CycleIndex` receives already converted ticks and wraps negative ticks positively;
the host determines clock rounding, frame arrays, origins and drawing transforms.
Operations preserve order, and unsupported zero operations return errors rather
than silently producing an unchanged result. Q12 coverage above `CoverageFull`
and incorrectly sized `WritePixels` buffers are programming-contract panics.

## v0 production-surface cleanup

This experimental revision removes facilities without a production consumer;
examples and test fixtures do not establish a separate API requirement. It is a
source-breaking v0 change and consumers must update deliberately:

- Removed `ColourAdjustment`, `DefaultColourAdjustment`, `DrawAdjusted`, `Frame`,
  `NewFrame`, `Animation`, `NewAnimation` and `Processor.DrawFrame`. Hosts retain
  native drawing options and frame arrays; `CycleIndex` remains available.
- Removed CPU `TintRGB`, `Opacity`, `Image.Crop`, `Image.Pad` and `NewImage`.
  Import packed channels with `NewPixels`. Use native `CopyRegion`/`Insets` for
  owned cropped/padded images, and ordinary native draw opacity for presentation.
- Removed `Processor.PadAndDerive`. Its former game border path received already
  padded images; real native region padding remains supported.
- Native `Bounds`, `Processor.Snapshot` and `Processor.DeriveImage` are private
  implementation details. `Derive` and `Silhouette` accept immutable native
  sources. CPU results can be uploaded directly from an owned `CopyRGBA` result.
- Removed `Dissolve.Update` and allocating `Dissolve.Pixels`. Initialize an owned
  RGBA buffer with `WritePixels`, then use `UpdatePixels` and its changed flag.
- `Stats` retains only counts and bytes. `Close`, `Clear`, `Forget`, error
  identities and explicit cache budgets remain supported ownership controls.

RGB gain now also transforms alpha-zero emissive RGB. This fixes the independent
channel contract, including zero gain after `AlphaChannelScale(0)`; ordinary
nontransparent gain and the existing alpha-only/additive effects are unchanged.
