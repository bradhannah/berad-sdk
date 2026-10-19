# Graphics API and ownership

## Images and operations

CPU images snapshot RGBA data and normalize regions to a zero origin. Returned
pixel/image copies are owned by callers; operations never mutate inputs. Images
are bounded to 8192 pixels per axis and 32 MiB of channel data. Invalid dimensions,
nonfinite parameters, unsupported operations and mismatched masks are errors.
Frame offsets/anchors are bounded to +/-2^30 pixels; uniform animation definitions
have at most 65536 frames and a nonoverflowing positive millisecond period.
`CycleIndex` separately samples integer ticks, allowing host clocks to retain
their own conversion/rounding policy without overflowing tick-plus-offset.

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

Derived GPU images returned to existing host draw code are read-only borrows.
Eviction/processor closure drops cache references, never invalidates a retained
borrow, and makes no immediate-VRAM reclamation claim. Borrowers must neither
mutate nor deallocate these images. Explicit owned copies have separate lifetimes.
Mutable mask/dissolve surfaces own their resources and release them on closure;
their borrowed views must not outlive that owner. Caller drawing targets survive.
