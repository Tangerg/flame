# Code intelligence

The optional `flame.codeintel` package owns language-server processes, LSP queries,
formatter selection and diagnostic comparison. The installed `codeintel_lsp` model Tool
provides diagnostics, definitions, references, hover and symbols through the same
backend. Runtime no longer constructs language servers or accepts `lsp.servers`.

Select its `code-intelligence` contribution for `flame.code-intelligence.v1` to
enable formatting and diagnostic feedback around file mutations. The public
contract is owned by `packages/go-sdk/plugin/codeintel`. Runtime supplies bounded
UTF-8 source snapshots with content hashes, borrows one exact activation for the
whole mutation, validates returned versions and applies transformations through
its existing filesystem owner. The backend never commits the transformed file.
Runtime retains path locks, read evidence, workspace identity, version checks and
the acknowledged mutation result when optional analysis fails.

Documents and formatted output are limited to 2 MiB so their worst-case JSON
encoding fits the MCP transport. Go and JSON formatting run in process. Other
supported text formats use `prettier` only for explicitly admitted workspaces.
The optional installation input `prettier-workspaces` is a JSON array of absolute
workspace directories, for example `["/work/project"]`. Omitted or empty grants
skip Prettier instead of silently applying a global default style. Each grant
canonicalizes symlinks and captures the directory identity at backend activation;
only that exact workspace root is admitted, not arbitrary descendant workspaces
or a replacement directory. A changed grant requires a new activation.

An admitted workspace uses the installed backend's PATH-resolved `prettier`, its
real source path and working directory, and that project's Prettier configuration
and EditorConfig. Granting it authorizes execution of project formatter
configuration and plugins, including JavaScript configuration; admit only a
workspace whose formatter code you trust. Hook trust does not confer this grant.
The captured source still travels on stdin, and Runtime alone applies the
version-bound transformation. External formatters and
language servers own process groups and retire descendants on cancellation or
shutdown. Missing optional formatters are unsupported operations; other failures
remain visible alongside a successful edit.

The optional installation input `lsp-servers` is a JSON array of server
specifications (`name`, `command`, `args`, `languages` (extension-to-language-ID object), `rootMarkers`). Omit it to use the built-in
language-server table; an empty array disables language servers. Executables must
already be available to the installed backend. Diagnostic results must identify
the requested open-document version; missing, stale or unversioned results are
reported as unavailable, never as a successful empty analysis.

Each language-server connection admits at most 128 synchronized documents.
At capacity, it must deliver the oldest document's close before opening another;
failed release refuses admission and retains the original synchronization record.

Model-facing LSP positions use 1-based lines and 1-based UTF-16 code-unit
offsets, matching the language server's units after subtracting one. For
`a😀b`, the `b` starts at model character `4` and LSP character `3`; visual
columns and Unicode code-point counts are not interchangeable with this
coordinate. Initialization advertises only UTF-16 and rejects a server that
selects another encoding. Omitted server encoding uses the LSP UTF-16 default.

Build through `node plugins/scripts/build-packages.mjs`; the first-party catalog
selects this package's code-intelligence contribution on a fresh default install.
Disabling it removes its Tools and prevents new analysis. An existing borrowed
activation retains the usual Frame cancellation and release protections.

Rebuild the Runtime and its consumers together after removing the built-in `lsp`
Tool identity. Select the new installed release and move custom server settings
to its installation input. Preserve earlier data and executable versions for
unresolved work; there is no old configuration reader or built-in fallback.
Discovery no longer advertises a core `lsp` feature; installed providers and
their current MCP Tools describe availability.
