# Berad Graphics

Experimental supplied-image processing, frames, procedural animation, masks and
Ebitengine integration. Module `github.com/bradhannah/berad-sdk/modules/berad-graphics`;
root package `graphics` is CPU-only, with `backend/ebitengine` for device resources.
Candidate source is under development; no published version is promised yet.

Minimum Go 1.27.0; selected toolchain 1.27.1, Ebitengine 2.10.4. Project source is
MIT, including algorithms adapted from Ultima V Redux. No game media is bundled.
Ebitengine retains its Apache-2.0 license and dependencies retain their own notices.
Tests and the independent consumer generate their own synthetic images.

Hosts supply pixels, frame references, colours, selection masks, time/progress and
presentation randomness. Hosts retain assets, tile identities, world/lighting and
camera rules, input, effect presets and sequencing. There is no file watcher.

See [API and ownership](docs/API.md). From the SDK checkout, use `make provision
MODULE=modules/berad-graphics` for explicit dependency setup, `make graphics-check`
for strict formatting/lint/tests/race/consumer checks, and `make quality-gate` for
the aggregate gate. CPU tests need no renderer; backend/consumer tests require a
real Ebitengine render harness. Native visual acceptance is separate.
