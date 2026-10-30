# Flame release interpretation

This adapter interprets Flame metadata inside a neutral Qimu release. It does not
own a second archive format, content cache, release digest, integrity stamp or
execution tree. `qimu/distribution` owns those bytes and their original leases.
The composition root supplies one content Store and its shared Admission.

Every installed directory or ZIP contains `manifest.json`, `plugin.lock.json` and
exactly the listed artifacts. `plugin.json` remains Flame's product metadata. Its
namespace references a document with the public `plugin.DeclarationProtocol`;
that version is independent of Qimu's portable contract and transport versions.
Legacy directories without the neutral manifest and lock are refused.

`plugins.ProjectFrameworkDeclaration` is the only mapping from admitted Flame
metadata to Qimu Units, contracts and import slots. Both Runtime and the author
command use it. `BuildRelease` adds the actual compiled module graph or target
executable and invokes Qimu's deterministic package builder. Admission recomputes
that same complete declaration and compares its canonical identity to the neutral
manifest. A package cannot add a hidden Unit, change an import, relabel a contract,
substitute a managed runtime or change a Point contribution through metadata.
An inert metadata or content package can have no executable Units.

`contributes.locales` declares `{tag, title, entry}` for static workbench catalogs.
Tags use their canonical BCP-47 spelling and identify an actual language; `en` is
reserved for the Host. Entries stay in the Flame namespace, end in `.json` and
share the 4 MiB metadata resource limit. Admission validates a nonempty flat object
of string messages, rejecting duplicate keys, nulls and malformed Unicode. The
public catalog query accepts the exact installation, release and tag, and exposes
messages without arbitrary resource paths. It reuses the immutable content lease
and integrity checks, without evaluating package code or constructing a browser
component.

The `runtime/cmd/pluginpack` command is distributed as a prebuilt author tool.
Repository builds and independent author projects invoke that same command;
plugin implementation source never imports Runtime. The command refuses an
existing archive and any archive destination physically inside its input tree.
Source and ZIP installation resolve to the same release identity.

## Content and process ownership

`Materialize` retains the original content lease in its Candidate before another
operation can collect the release. `Publish` admits only the immutable Flame
interpretation; `Discard` returns the original claim after installation admission
or refusal. Reclamation queries current installation, business and execution
retention through the Store's original Admission. The metadata catalog remains
an immutable audit record when unretained content is collected.

`Connection` transfers a content lease to the actual process owner. The executable
digest comes from the original artifact inventory or the distribution's pinned
Node selection. The process owner retains this lease until original process exit
is proven. There is no copied execution directory or second lifecycle cache.

The configured data directory captures its physical parent at construction;
parent path aliases cannot retarget later preparation. The data namespace and
installation leaves still refuse symbolic-link redirection. Metadata projection
does not create private data directories.

Installed process entries are packaged native executables or declared managed
Node entries. A native release has one target OS and architecture; realization
refuses hosts without that variant. Managed entries name the exact qualified
interpreter version and digest. Ambient commands and installed HTTP entries are
refused by this profile. Independently configured ordinary MCP connections keep
their own existing transports outside this package adapter.

## Browser modules

The component compiler writes `modules.generated.json` from its actual output,
including static imports, dynamic chunks and exports. Authoring checks the closed
local graph and its exact shared specifiers. One versioned native Host module
profile is defined in `packages/component-sdk/runtime-profile.json`; generated
Go, TypeScript and build projections consume that original fact. The native
profile is not a portable ABI or a claim about physical interpreter bytes.

`ComponentModules` projects the admitted neutral module declaration to browser
bootstrap. It does not copy an installation's private configuration or retain a
second graph. The browser executor checks the same profile and exact shared
specifier list before loading a component. Loading trusted native JavaScript does
not establish a security sandbox.

Product metadata, diagnostics, inputs, skill parsing and presentation policy remain
Flame responsibilities. Qimu owns provider admission, exact bindings, execution
protection and release-byte integrity.
