# Module catalog

`sdk.json` registers modules and their actual validation lanes. Unregistered Go
modules/code are rejected; the catalog is not a publication/version promise.

The registered modules are:

| Directory | Responsibility | Current availability here |
| --- | --- | --- |
| [`berad-audio/`](berad-audio/README.md) | Cue/voice runtime, finite music, ambience and shared authoring | Experimental source on `main`; terminal/catalog/probe tools are a separate production consumer |
| [`berad-text/`](berad-text/README.md) | Supplied immutable fonts, styled layout, source-range/caret geometry and bounded Ebitengine rendering | Experimental source on `main`; reviewed dependency pin consumed by Ultima V Redux; independent native-render checks registered |
| [`berad-graphics/`](berad-graphics/README.md) | Supplied pixels, procedural kernels, masks and bounded Ebitengine resources | Experimental source on `main`; reviewed dependency pin consumed by Ultima V Redux |

Module paths follow `github.com/bradhannah/berad-sdk/modules/berad-<domain>`.
Pin reviewed remotely available revisions; none has a stable-version promise.
The module guides document intentional source-breaking v0 surface cleanup.
Text's standalone example installation has separate default-branch/tag availability
requirements described in its README. New consumer functionality is added when
needed, rather than retained solely for a hypothetical future game.

Each module has its own `go.mod`, license notices, documented public API,
examples, and explicit check inventory. See [conventions](../docs/SDK_CONVENTIONS.md)
and [extraction](../docs/EXTRACTION_GUIDE.md) before adding one.
