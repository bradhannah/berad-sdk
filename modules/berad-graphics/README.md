# Berad Graphics

Experimental supplied-image processing, procedural animation, masks and
Ebitengine integration. Module `github.com/bradhannah/berad-sdk/modules/berad-graphics`;
root package `graphics` is CPU-only, with `backend/ebitengine` for device resources.
Pin a reviewed, remotely resolvable pseudo-version. There is no stable release or
version tag; the v0 API removals are documented in [API and ownership](docs/API.md#v0-production-surface-cleanup).

Minimum Go 1.27.0; selected toolchain 1.27.1, Ebitengine 2.10.4. Project source is
MIT, including algorithms adapted from Ultima V Redux. No game media is bundled.
Ebitengine retains its Apache-2.0 license and dependencies retain their own notices.
Tests and the independent consumer generate their own synthetic images.

Hosts supply pixels, colours, selection masks, time/progress and
presentation randomness. Hosts retain assets, tile identities, world/lighting and
camera rules, input, effect presets and sequencing. There is no file watcher.

See [API and ownership](docs/API.md). From the SDK checkout, use `make provision
MODULE=modules/berad-graphics` for explicit dependency setup, `make graphics-check`
for strict formatting/lint/tests/race/consumer checks, and `make quality-gate` for
the aggregate gate. CPU tests need no renderer; backend/consumer tests require a
real Ebitengine render harness. Native visual acceptance is separate.

## Process a supplied native image

```go
processor, err := ebitengine.NewProcessor(ebitengine.DefaultLimits())
if err != nil { return err }
defer processor.Close()
drawable, err := processor.Derive(source, graphics.Outline(color.RGBA{R: 255, A: 255}, 0))
if err != nil { return err }
target.DrawImage(drawable, nil) // caller owns target; do not dispose drawable
```

Use `graphics` for the root module import and `ebitengine` for its backend import;
`source` is a borrowed immutable `*ebiten.Image`; `target` belongs to the caller.
Run this example inside the Ebitengine loop: importing a native source requires
readback. To prepare pixels without a renderer, use `graphics.NewPixels(width,
height, rgbaBytes)` and `Image.Apply`; `Pixels` and `CopyRGBA` return owned copies.
Upload a CPU result with `ebiten.NewImageFromImage(result.CopyRGBA())` and release
that host-owned native image when finished. The complete buildable
[atelier example](examples/atelier/main.go) and
[independent consumer](testdata/consumer/consumer_test.go) use only public APIs and
generated artwork. From the SDK checkout, `make graphics-demo` builds/runs the
example; `make graphics-demo-smoke` checks bounded native startup in isolation.

The API contract defines independent channel semantics, explicit cycle sampling,
resource budgets and closure. Geometry and effects remain useful for arbitrary
supplied images; the SDK has no tile IDs, world coordinates or asset catalog.
Hosts retain frame arrays, placement, ordinary draw opacity and animation policy.
