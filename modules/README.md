# Module catalog

`sdk.json` registers modules and their actual validation lanes. Unregistered Go
modules/code are rejected; the catalog is not a publication/version promise.

The intended identities are:

| Proposed directory | Intended responsibility | Current availability here |
| --- | --- | --- |
| [`berad-audio/`](berad-audio/README.md) | Cue/voice runtime, finite music, ambience and shared authoring | Experimental source on `feat/berad-audio`; terminal/catalog tools are a separate tools module |
| [`berad-text/`](berad-text/README.md) | Supplied immutable fonts, styled layout, local geometry and bounded Ebitengine rendering | Experimental source on `feat/berad-text`; reviewed concrete dependency pin consumed by Ultima V Redux; independent native-render checks registered |
| `berad-graphics/` | A reviewed set of reusable graphics services | Not available; boundary requires design |

Module paths follow `github.com/bradhannah/berad-sdk/modules/berad-<domain>`.
For audio and text, pin a reviewed remotely available revision; neither has a
stable version promise. Text's standalone example installation has separate
default-branch/tag availability requirements described in its README. The planned
graphics identity is not an installable package. A future tile library is a
separate design decision.

Each module will have its own `go.mod`, license notices, documented public API,
examples, and explicit check inventory. See [conventions](../docs/SDK_CONVENTIONS.md)
and [extraction](../docs/EXTRACTION_GUIDE.md) before adding one.
