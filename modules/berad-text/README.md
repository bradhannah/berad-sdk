# Berad Text

Experimental Ebitengine-first text resources, styled layout, source-range/caret
geometry and bounded rendering. Canonical module:
`github.com/bradhannah/berad-sdk/modules/berad-text` (root package `text`).
Pin a reviewed available development revision; there is no stable version promise.
Minimum Go is 1.27.0; the tested toolchain is 1.27.1 with Ebitengine 2.10.4.

Supply TTF/OTF bytes or bitmap pixels and explicit glyph metrics. Sources and
sized fonts snapshot their inputs. There are no game assets, input handlers,
file watchers, hot-reload registry or application-global caches.

## Use

The [standalone example](examples/panel/main.go) is a complete Ebitengine host with
supplied Go Regular font data. The [independent consumer](testdata/consumer/consumer_test.go)
also supplies a synthetic bitmap icon, changes paint, wraps content and exercises
geometry and ownership using only the public API. Both compile in the registered
checks. From the SDK checkout, use `make text-demo` for the demonstration:
T changes paint and +/- changes font size. Resize to reflow the styled text.
The example retains its own layout and handles its own keyboard controls; the
library provides no input handlers, editor, panel or log abstraction.

Repeatable SDK Make targets:

| Target | Purpose |
| --- | --- |
| `make text-check` | Selected module/race/independent-consumer validation |
| `make build-text-demo` | Offline build into ignored `.cache/bin/berad-text-demo` |
| `make text-demo` | Build and run the interactive host |
| `make text-demo-smoke` | Build, run eight seconds with isolated HOME/cwd, then stop and reap |

Provision missing dependencies with `make provision MODULE=modules/berad-text`.
The smoke target requires a desktop backend and establishes bounded startup only.
`make clean` removes the demo executable while preserving validation evidence.
From a module-only checkout, `go run ./examples/panel` also works. Version-qualified
installation of the example can require a default-branch/tagged module identity
for Go's `latest`/deprecation lookup; a branch-only candidate does not establish
that tool-install acceptance. Applications can consume a concrete published pin.

Construction order is `ParseFont` / `NewBitmapSource`, `Source.NewFont`,
`NewContent`, `NewLayout`, `NewRenderer`, then `Renderer.Draw`. Check returned errors
and close the renderer when its owner retires. Fonts and layouts are immutable
Go-owned values; they may be shared independently of renderer lifetime.

The caller owns content meaning, input, focus, Z/paint-order routing, retained
panels/history, viewport and scroll state, persistence, and destination images.
The SDK provides source-range rectangles and offset-to-caret positions, never
click callbacks or editing commands.

See [API contracts](docs/API.md) for defaults, units, ownership, exposure and limits.
There is no dependency on other Berad runtimes or an application widget toolkit.

## Experimental API cleanup

This revision intentionally breaks the earlier experimental surface to match
current production use. It removes `Panel`/`Log`, OpenType feature/variation
configuration, bitmap RGB-preservation mode, annotation tags/hits, nearest-point
caret lookup, layout alignment and explicit line spacing, the public ink-bounds
getter, and draw-time destination clip/cache-bypass switches. Hosts retain their
own content/history, place layouts
with `GeoM`, and pass destination subimages when clipping is needed.
`Width`, `TopAlign`, `RangeRects`, `CaretAt`, local clipping and renderer limits
remain. `CopyEngineFace` remains a compatibility bridge, not the SDK rendering
identity. See the [migration details](docs/API.md#experimental-surface-migration).
Font.Measure now takes only the source text, preserving the production caller's
former zero-spacing measurement behavior. Bitmap glyphs always receive palette
tint and alpha.

## Validation and compatibility

Text's registered unit/race/consumer lanes execute an Ebitengine test harness and
require a desktop render backend. Contract fixtures include independently authored
bitmap baseline/top-alignment pixels, exact line/range/caret coordinates, overflow
rejection, LRU promotion under entry/byte pressure, and transparent/repaint pixels.
The unrelated consumer is public-API compatibility smoke coverage; it is not a
production-demand example or a complete geometry/pixel oracle. These checks establish automated rendering,
not physical input/IME or all-platform acceptance. The independent consumer uses
synthetic images and the BSD-licensed Go font fixture from `golang.org/x/image`;
production code embeds no font or game media.

From the SDK checkout, run `python3 -I -B scripts/check.py plan --module modules/berad-text`
or `quality-gate --module modules/berad-text`. The checker isolates the module,
disables workspaces/network and rejects game dependencies. The only local
replacement is in the separately copied consumer fixture, not published metadata.
The native backend environment participates in reuse identity. Rendering failure
cannot be converted to a skip/pass. Consumer compilation and behavior fixtures
remain unchanged when testing compatible API evolution.

## License

Project-authored code is MIT, including mechanisms adapted from Ultima V Redux's
text implementation. No game artwork or original data is included. Go font data
is supplied by the test/example dependency rather than copied into this module;
see [third-party notices](THIRD_PARTY_NOTICES.md).
