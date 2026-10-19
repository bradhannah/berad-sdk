# Graphics API and ownership

## Public boundary and exposure

| Surface | Consumer use | Ownership and coupling |
| --- | --- | --- |
| Root `Image`, `Mask`, `Selection`, `Operation` | Offline pixel preparation and explicit selections | Immutable CPU snapshots; returned pixel/RGBA copies belong to callers. Standard `image`/`color` types only. |
| `Frame`, `Animation`, `CycleIndex`, `Pulse` | Anchored arbitrary frames and explicit sampling | Frozen definitions, supplied time/ticks; no device, world state or implicit clock. |
| `Dissolve`, `Shuffler`, `DitherKeeps` | Coverage/permutation and ordered thresholds | Caller serializes mutable dissolve; injected RNG is borrowed only during construction. |
| Backend `Processor` | Native import, derivation, silhouette and frame draw | Construct with explicit positive limits or `DefaultLimits`; zero/closed processor rejects work with `ErrClosed`. |
| Borrowed `*ebiten.Image` sources/results | Integrate existing host draw paths | Inputs immutable until explicit `Forget`; returned cached images must not be mutated/deallocated and survive cache closure. |
| `CopyRegion`, `Surface` | Owned region copies and changing upload buffers | Copies transfer native ownership. Surface zero value is usable; close is terminal and invalidates its borrowed image/buffer. |
| `ImageCache` | Retain variants under host-defined collision-free keys | Host-serialized, bounded references; removal/clear never disposes borrowed images. |
| `*ebiten.Image` targets, `DrawImageOptions`, `GeoM`, blend/filter | Native composition with supplied transforms and clipping | Caller owns target; options passed by value are not mutated. Ebitengine 2.10.4 types/semantics are a deliberate compatibility boundary. |
| `ColourAdjustment`, `DrawAdjusted`, `DrawAdditiveChannels` | Explicit HSV/channel draw treatment | Named identity defaults, validated finite factors, caller-owned target; no illumination discovery. |

The backend imports are deliberate under SDK-API-002; there is no alternate
renderer interface. Source GPU readback requires a running Ebitengine loop.
Known disposed inputs return errors; unrelated engine/programming panics are not
recovered into successful fallback artwork. Invalid external definitions wrap
`graphics.ErrInvalid`; `errors.Is` can distinguish them. CPU resources require
constructors before use; zero frames/operations are invalid definitions.

## Images and operations

CPU images snapshot RGBA data and normalize regions to a zero origin. Returned
pixel/image copies are owned by callers; operations never mutate inputs. Images
are bounded to 8192 pixels per axis and 32 MiB of channel data. Invalid dimensions,
nonfinite parameters, unsupported operations and mismatched masks are errors.
Frame offsets/anchors are bounded to +/-2^30 pixels; uniform animation definitions
have at most 65536 frames and a nonoverflowing positive millisecond period.
`CycleIndex` separately samples integer ticks, allowing host clocks to retain
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

The Ebitengine backend additionally offers `ColourAdjustment`/`DrawAdjusted`,
delegating to the pinned engine's `colorm.ColorM.ChangeHSV`. Hue is radians and
wraps by a full turn; saturation/value factors are finite in [0,16]. Use
`DefaultColourAdjustment` for identity; zero saturation is grayscale and zero
value is black with source alpha preserved. These encoded-colour matrix semantics
are not linear-light exposure, physical illumination or an HSL-lightness promise.
The caller's premultiplied ColorScale is applied after the matrix. No blur/halo
generation is implied by RGB gain or an additive overlay.

RGB gain is independent channel multiplication with saturation. Alpha-channel
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

CPU images/masks are immutable and can be read independently. Mutable animation
surfaces and Ebitengine processors require host serialization. No implicit clock,
input polling, background worker or gameplay RNG is created. Host-supplied random
ordering is consumed during dissolve construction only; reset/reverse reuse it.

GPU import is an explicit compatibility boundary for existing Ebitengine hosts:
sources are borrowed immutable images. After host mutation, invalidate their
snapshot before reuse. CPU imports instead own copies from the start. Both source
snapshots and derived images have bounded cache retention. Keys retain complete
source/operation identities, not hashed raw pointer addresses or quantized values.
`PadAndDerive` includes padding in that same exact identity before effects execute.
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
once. Oversized valid results bypass retention. Active CPU images, animations and
externally retained GPU results remain host-owned allocations beyond these caches.
Each dissolve additionally bounds its coordinate ordering to 32 MiB. These are
per-resource/cache limits, not a global memory manager or peak-allocation promise.

`Clear` drops cached references while leaving a processor usable; `Forget` retires
the currently retained GPU-source snapshot and its derivations before re-import.
Already returned snapshots/results keep their old pixels. `Close` is terminal and
idempotent. `Stats` returns detached observations and does not generate images.

Animation milliseconds wrap by the full positive cycle, including negative time.
`CycleIndex` instead receives already converted ticks; the host determines clock
rounding. Frames use pixel offsets/anchors and padding compensates their origin.
Operations preserve order, and unsupported zero operations return errors rather
than silently producing an unchanged result. Q12 coverage above `CoverageFull`
and incorrectly sized `WritePixels` buffers are programming-contract panics.
