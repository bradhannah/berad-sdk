# Tool module catalog

Standalone developer applications live under `tools/berad-<domain>/` with their
own module, public entry points, notices and dependency pins. They can depend on
runtime/authoring packages; runtime modules cannot depend on tools or terminal UI.

[`tools/berad-audio`](berad-audio/README.md) is registered with the terminal editor,
catalog/report command, standalone audition and a source-probe command.
Its module pins the published authoring API. See the
[extraction guide](../docs/EXTRACTION_GUIDE.md) and
[validation contract](../docs/VALIDATION.md).

Use `make plan MODULE=tools/berad-audio`. Unknown modules fail
explicitly. Binaries belong beneath each tool module's ignored root `bin/`; tool cleanup must remove
only declared binaries, keeping validation results and authored project files.
