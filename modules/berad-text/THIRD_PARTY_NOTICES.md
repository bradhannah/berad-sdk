# Third-party fixtures and dependencies

Tests and the example import Go Regular from
`golang.org/x/image/font/gofont/goregular`. They do not copy an Ultima font or any
game artwork. Go fonts were designed by Bigelow & Holmes for the Go project and
are distributed under the Go project's BSD-style license, including commercial
use subject to its notice conditions. See the [Go fonts announcement](https://go.dev/blog/go-fonts)
and the dependency's LICENSE. Preserve dependency notices when distributing
compiled examples. The fixture is supplied explicitly; it is not an SDK default.

Ebitengine retains its Apache-2.0 license, uniseg its MIT license, and Go x/image
and x/text their BSD-style licenses. Their pinned versions and dependency closure
are recorded in go.mod/go.sum; each retains its own bundled notices. Project-authored
synthetic bitmap fixtures and SDK source are covered by this module's MIT LICENSE.
