# Berad Text API contracts

## Resources and construction

`ParseFont` copies supplied TTF/OTF bytes before parsing. `NewBitmapSource` copies
supplied pixels and glyph descriptors, normalizing atlas coordinates internally.
Sources allow up to 32 MiB; bitmap dimensions are positive and at most 8192 per
axis, with at most 65536 glyph descriptors. Rectangles refer to the input image's
coordinate system, including a nonzero origin. Empty glyph rectangles represent
spacing. Missing bitmap glyphs are contextual layout errors; the host explicitly
chooses any replacement. TTF missing-glyph behavior follows the engine/font.

`NewFont` snapshots size and language. Size is finite, positive and at most 4096 pixels. Bitmap
size scales its ascent-plus-descent to that size. A glyph's BearingY is relative
to the baseline, and Advance is independent of pixel width. Supplied bitmap glyphs
always receive paint tint and alpha; there is no RGB-preservation option. Intrinsic
colour glyphs from the outline-font engine retain the engine's RGB/paint-alpha
behavior, independently of supplied bitmap descriptors.
Source metrics and advances must be finite and nonnegative, with positive total
ascent-plus-descent. Finite negative bearings are valid overhangs. Scaled metrics
must remain finite and valid; construction rejects scaling overflow/underflow.

Sources/fonts are immutable Go-owned resources. They have no global registration,
reload or mutable replacement identity. Independent sources with identical data
and configuration may share equivalent raster identity within one renderer;
different data cannot collide merely because application names are equal.
Engine source-owned glyph caches follow retained font/source references.

## Public exposure and ownership

| Surface | Why public | Ownership / mutation |
| --- | --- | --- |
| `image.Image`, rectangles, colours | Caller-supplied pixels, regions and paint | Inputs copied by source/content construction; palette values read during serialized Draw |
| `*ebiten.Image` destination | Direct Ebitengine integration | Borrowed for Draw; never closed or disposed by SDK |
| `ebiten.GeoM`, `ebiten.Filter` | Placement and sampling | Draw options passed by value; local clip rectangle is read during Draw |
| `language.Tag` | BCP47 shaping hint | Immutable value in font configuration |
| `CopyEngineFace` | Compatibility with host APIs using Ebitengine text/v2 | Detached mutable face configuration with shared read-only parsed Source; never call Source.UnsafeInternal or mutate its internals |
| `Source`, `Font`, `Content`, `Layout` | Reusable construction and geometry | Private authoritative state; detached collection results; no cached image pointers |
| `Renderer` | Explicitly owned cache and drawing service | Host-serialized; terminal idempotent Close |

Feature/variation mutation of a copied engine face cannot change the SDK Font.
Create a new Font for a new SDK configuration. The compatibility face is not a
render/cache handle and is not used as SDK identity. Engine coupling is explicitly
to Ebitengine 2.10.4; dependency upgrades need consumer and pixel acceptance.

## Content, layout and paint

`NewContent` preserves valid UTF-8 and snapshots spans and paint names. Text
plus span metadata is limited to 1 MiB, and there are at most 65536 spans. Font
references are immutable. Use bitmap mappings (including U+FFFC when desired) for
inline icons; text and icon positions remain explicit UTF-8 byte ranges.

Layout uses horizontal Ebitengine shaping. Adjacent equal-font configurations
shape together even across paint boundaries. Mixed fonts align by baseline;
`TopAlign` explicitly selects top alignment. `Width=0` disables soft wrapping;
positive width wraps at grapheme-safe break opportunities, allowing a single
oversized grapheme to make progress. No whitespace is trimmed and no hyphen is
inserted. LF, CR and CRLF are hard breaks without rewriting source bytes. Empty
content and trailing hard breaks retain caret-bearing empty lines; an empty
span or DefaultFont supplies their metrics.

Lines use natural metrics and start at local X=0. Hosts align a layout by applying
a placement transform, or place individual row layouts for host-specific spacing.
Bounds are logical placement bounds; rendering separately retains full glyph ink,
including negative bearings/overhangs. Raster padding never changes advances.
MaxWork defaults to an 8 MiB text-processing budget; layouts additionally limit
lines to 8192, glyphs to 131072, and logical/ink coordinates to finite values in
[-2^30, 2^30] pixels. Supplied Width cannot exceed 2^30. Accumulated line heights
and ink extents are checked before a layout is returned, including natural line
gaps from supplied bitmap metrics.
This bounds pathological repeated-prefix wrapping; it is not an optimal paragraph
line-breaking algorithm. A budget error never truncates or rewrites source.

Palette names resolve at Draw. The empty name defaults to white if absent; other
missing names/nil colours are errors. Explicit zero alpha remains transparent.
Repainting does not change geometry. Changing typography or width requires a new
layout. A foreground boundary inside one shaped cluster uses the leading source
span's paint; RangeRects reports actual cluster coverage instead of promising
independent letter painting within a ligature. Background/highlight decoration
can be drawn by the host using returned rectangles.

## Input-independent queries

All coordinates are local pixels. The host owns screen-coordinate conversion,
clipping eligibility, Z routing, focus, selection state, input and activation.

Pass a destination subimage for destination-coordinate clipping. DrawOptions.LocalClip
clips layout pixels before applying the placement transform, including nonzero ink
origins. Neither changes retained query geometry; the host applies the corresponding
clip when routing input. A clipped draw reuses the same raster as an unclipped draw.

- `CaretAt` validates UTF-8 offsets and reports Requested versus resolved Offset.
  Positions inside a shaped cluster snap to its leading site. Upstream selects
  the preceding visual line at a soft wrap; the default selects the following line.
- `RangeRects` validates a half-open byte range and reports requested versus
  represented coverage. Ranges cutting clusters expand explicitly. Empty ranges
  return no rectangles; hard-break characters themselves have no painted rectangle.

Results refer to the immutable Content snapshot. Hosts revalidate source offsets
after edit/reflow. IME/preedit and synthetic ellipsis projection are host
responsibilities. Full bidi-affinity, vertical editing and exact interior-ligature
caret guarantees are not claimed. This limitation never changes accepted source
text. Queries are CPU-only and remain usable after any Renderer closes.

## Rendering and limits

Renderer construction validates separate entry, cache-byte, render-byte and
dimension limits. Defaults: 512 retained rasters, 16 MiB accounted cache bytes,
64 MiB per raster and 8192 pixels per dimension. Dimension/render limits are
checked before raster allocation. Oversized cache entries bypass retention without
evicting useful labels. There is no caller-selected bypass mode; a separate renderer
or ClearCaches can be used when an independently fresh raster is required.

The bounded LRU holds composed images and retained Content/Layout references.
Accounted bytes include raster pixel payload plus source text; they exclude
object/map overhead, host snapshots, font data and engine glyph caches. This is
not a total GPU/heap limit. Cache identity uses immutable content/typography/layout
and numeric paint values, not mutable face pointer addresses. Equivalent scaled
fonts therefore reuse existing rasters.

All Renderer methods require host serialization. No background worker, clock or
input pump is created. Immutable font/content/layout operations support independent
reads; callers must not mutate supplied palettes or engine compatibility faces
concurrently with their use. Raster eviction deallocates private images; none can
escape. Temporary bitmap uploads and bypass rasters retire after use. Close is
idempotent/terminal for rendering, releases private rasters and never disposes
the target. Fonts and layouts follow their independent Go lifetimes. No immediate
total-VRAM reclamation is promised. Stats are detached, observational lifetime
counters; ClearCaches retains the lifetime peak/counters.

## Retained content and inspection

Hosts retain Content/Layout pointers for as long as needed and own panel/history
storage, limits, eviction, truncation notices, viewport/follow-tail, persistence
and reflow. There is no SDK panel or log container. Content.Text preserves the
source; Content.Spans and Layout.Lines return detached slices. Layout.Content
retains the immutable source snapshot. These inspection operations remain useful
without exposing mutable renderer resources.

Font.Measure remains the lightweight compatibility measurement API. It uses the
engine's zero-spacing measurement convention, preserving the existing production
measurement behavior without a configurable spacing argument. This is not a request
for NewLayout's natural line advance. Empty text measures zero; invalid input and
nonfinite resulting extents are errors. Use NewLayout for natural multiline layout
and source-range/caret geometry. CopyEngineFace does not depend on a measurement
spacing argument and remains unchanged.

## Experimental surface migration

This is an intentional breaking cleanup of the earlier experimental API, based on
the supported production consumers. Remove these usages when upgrading:

- `Panel`, `Log`, `NewLog` and their methods: retain content/history in the host.
- `Feature`, `Variation`, and FontOptions.Features/Variations: no current consumer
  requires configurable OpenType features/axes. Size and Language remain.
- Glyph.Colored: supplied bitmap pixels always use palette tint and alpha. Remove
  the field from descriptors; no production consumer selected the old true mode.
- Span.Tag, `Hit`, `HitTestLocal`, `NearestCaret`, Rect.Contains and Caret.Inside:
  source-offset queries remain; pointer hit routing is not provided by this API.
- `Align`, its constants, LayoutOptions.Align and LayoutOptions.LineSpacing:
  use natural SDK layout and host placement/row policy.
- Font.Measure's lineSpacing argument: call Measure(value). The only production
  caller supplied zero; that engine-compatible behavior remains fixed internally.
- Layout.InkBounds: the renderer still computes and preserves full ink internally.
- DrawOptions.Clip and DrawOptions.Uncached: use destination subimages and explicit
  renderer ownership/clearing. Automatic oversized-cache bypass remains.

`DefaultFont` remains for spanless empty content. Content inspection, Width,
TopAlign, MaxWork, RangeRects, CaretAt and the CopyEngineFace compatibility bridge
remain. Future capabilities require a concrete consumer rather than a synthetic
demo. The standalone example now demonstrates only fonts, styled wrapping and
repainting; it is not evidence of additional production demand.

## Evidence and evolution

The module and independent consumer use a running Ebitengine harness. Module fixtures
compare raster output against the engine and independently authored bitmap pixels,
including mixed metrics/top alignment, bearings, and paint. Exact bitmap line,
range and caret coordinates and combining-cluster offsets are checked without
deriving expectations from private shaping records. Cache fixtures exercise actual
hit pixels, LRU promotion under entry/byte pressure, automatic oversized bypass,
repainting/transparency, limits and closure. Overflow/negative-definition cases
exercise construction/layout/measurement rejection. The independent consumer is
a public-API compatibility smoke fixture, not a complete pixel/geometry oracle.
These are automated observations, not
physical native keyboard/IME or all-platform acceptance. Keep the independent
consumer unchanged across compatible changes; v0 API changes still require notes
and review. No persisted asset format is introduced.
