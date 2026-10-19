# Berad Text

Experimental Ebitengine-first text resources, styled layout, local geometry,
retained panels/logs and bounded rendering. Canonical module:
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
checks. From the SDK checkout, use `make text-demo` for the interactive demonstration:
click the icon or input field, Enter to submit, Escape to leave editing, Space to
toggle a blocking panel, T to change paint and +/- to change size. Resize to reflow.
All input/stacking/editing is example-host code; the library only returns geometry.
This small example uses rune-wise deletion and committed input characters, not a
complete IME, grapheme editor or widget framework.

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

The caller owns content meaning, input, focus, Z/paint-order routing, viewport and
scroll state, persistence, and destination images. The SDK provides geometric
candidates, never click callbacks or editing commands.

See [API contracts](docs/API.md) for defaults, units, ownership, exposure and limits.
There is no dependency on other Berad runtimes or an application widget toolkit.

## Validation and compatibility

Text's registered unit/race/consumer lanes execute an Ebitengine test harness and
require a desktop render backend. These checks establish automated rendering,
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
