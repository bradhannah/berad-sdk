# Tool module catalog

Standalone developer applications live under `tools/berad-<domain>/` with their
own module, public entry points, notices and dependency pins. They can depend on
runtime/authoring packages; runtime modules cannot depend on tools or terminal UI.

The approved audio extraction targets `tools/berad-audio`, with editor, catalog
and native-probe commands. It is not yet registered or installable. Its implementation
is added only after the shared authoring API exists. See the
[extraction guide](../docs/EXTRACTION_GUIDE.md) and
[validation contract](../docs/VALIDATION.md).

Use `make plan MODULE=tools/berad-audio` once registered. Unknown modules fail
explicitly. Binaries belong beneath ignored `.cache/bin`; tool cleanup must remove
only declared binaries, keeping validation results and authored project files.
