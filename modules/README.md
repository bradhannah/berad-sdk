# Module catalog

There are no active runtime modules in this repository yet. `sdk.json` records the
empty active inventory; foundation checks reject unregistered Go modules/code.

The intended identities are:

| Proposed directory | Intended responsibility | Current availability here |
| --- | --- | --- |
| `berad-audio/` | Reusable audio runtime and authoring services | Not available |
| `berad-text/` | Text resources, layout, rendering, and geometry | Not available |
| `berad-graphics/` | A reviewed set of reusable graphics services | Not available; boundary requires design |

Future module paths follow
`github.com/bradhannah/berad-sdk/modules/berad-<domain>`. These are naming
conventions, not resolvable installation instructions today. A future tile library
is a separate design decision.

Each module will have its own `go.mod`, license notices, documented public API,
examples, and explicit check inventory. See [conventions](../docs/SDK_CONVENTIONS.md)
and [extraction](../docs/EXTRACTION_GUIDE.md) before adding one.
