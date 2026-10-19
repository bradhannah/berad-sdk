# Module catalog

`sdk.json` registers modules and their actual validation lanes. Unregistered Go
modules/code are rejected; the catalog is not a publication/version promise.

The intended identities are:

| Proposed directory | Intended responsibility | Current availability here |
| --- | --- | --- |
| [`berad-audio/`](berad-audio/README.md) | Cue/voice runtime, finite music, ambience and shared authoring | Experimental source on `feat/berad-audio`; terminal/catalog tools are a separate tools module |
| `berad-text/` | Text resources, layout, rendering, and geometry | Not available |
| `berad-graphics/` | A reviewed set of reusable graphics services | Not available; boundary requires design |

Module paths follow `github.com/bradhannah/berad-sdk/modules/berad-<domain>`.
For audio, pin a reviewed published revision from its development branch. Planned
text/graphics identities are not installable packages. A future tile library
is a separate design decision.

Each module will have its own `go.mod`, license notices, documented public API,
examples, and explicit check inventory. See [conventions](../docs/SDK_CONVENTIONS.md)
and [extraction](../docs/EXTRACTION_GUIDE.md) before adding one.
