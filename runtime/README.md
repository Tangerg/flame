# Flame Runtime

Flame Runtime is the local product backend for Flame. It owns durable agent semantics and exposes the same behavior through an in-process Go binding and the Runtime Protocol.

Runtime is not another agent framework. Scope owns process execution, strategies, tools, and provider libraries; Runtime adapts those capabilities to Flame's Session, Run, Goal, Plan, persistence, recovery, and protocol model.

## Public surfaces

- The module-root `runtime.Runtime` is the concrete in-process binding.
- The module-root `runtime.Client` attaches to an existing Runtime over HTTP/SSE with the same typed operation methods.
- `protocol` contains binding-neutral requests, responses, events, errors, and validation.
- `contract` contains generated machine-readable protocol artifacts and the generated API reference.
- `localruntime` owns the local deployment layout and the strict credential-file handoff.
  It resolves the data directory beneath a product root and names the database and local
  token inside it, so the Runtime process and a trusted desktop client read one layout
  rather than each composing a path.

All Runtime operations enter one delivery endpoint. The Go binding avoids JSON and HTTP encoding but uses the same admission, capability, idempotency, Application, error, and event semantics as the HTTP binding.

RPC parameters and `_meta` use exact, case-sensitive schema field names. Unknown members, duplicate members, invalid Unicode, trailing JSON values, and explicit `null` in typed fields are rejected. Omit optional fields; use the declared change variants to clear configuration. Opaque tool arguments may contain `null`. Clients must not rely on case folding or replacement of malformed text.

Go consumers accepting authored JSON use `protocol.DecodeRequest(encoded, &request)`
before invoking either binding. It shares the transport's strict decoding and generated
value validation, preserves opaque numeric evidence, and leaves the target unchanged on
failure. Required JSON fields must be present even when their value is false or an empty
collection. Ordinary JSON decoding can erase omissions and explicit nulls before the
binding observes them.

The shared TypeScript client validates request parameters through the generated method
contract before reserving a mutation identity or sending a request. Authored-value
consumers use `checkRequest` from `@flame/runtime-contract/client/request` for the same
validation and typed result. Typed request objects reject unknown members at every
depth; declared maps and opaque tool arguments keep their own keys. Shared result
shapes continue to accept additional fields. Request metadata is validated before
publishing the request identity or calling the transport. OpenRPC derives closed
request components, including `_meta`, from the same wire graph used by the
generated client checks; its result references retain the reusable schema shapes.

## Protocol 2026-10-08.1

Portable releases publish `views` alongside themes, Skills and MCP declarations. The only
admitted view type is `sessionTrajectory`, a bounded, self-contained HTML entry.
`plugins.readView` addresses an enabled, approved installation by exact selected digest
and declared view ID. `plugins.readTrajectory` applies the same authorization and delegates
to the existing Session trajectory query. No second trajectory store or execution path exists.

The client binds the Session before opening the isolated page. The page receives the
initial result once and can request only the next cursor or a refresh of that bound query.
It receives no Runtime credential, arbitrary resource path, Session selector or RPC tunnel.
Authorization is captured under installation admission; accepted reads complete outside
that lock, and withdrawal refuses subsequent reads. Clients retire their local page when
installation admission, selected release, Session or Runtime connection changes.

This is a breaking development protocol change. Rebuild the shared client, Desktop, CLI
and IDE against this catalog; earlier protocol versions are refused. Existing package
releases without a view continue to describe their admitted declarative resources.

## Protocol 2026-10-07.8

`ApprovalModeResult.modes` is required: every default mode with the gate
(`pass`, `prompt` or `deny`) it applies to write, exec and network tools, taken
from the Runtime's own policy. Clients describe a mode from it instead of
restating what each mode allows.

## Protocol 2026-10-07.7

`segment.finished` carries `run`, the RunRef the segment ended with, and
`interrupts` when that Run raised them. Its former `outcome`, `metrics` and
`contextTokens` are gone, and so is `SegmentOutcome`: the Run's own status,
outcome, metrics and context footprint state the boundary, so a client
replaces its Run instead of deriving one from why the segment stopped. A
waiting Run without `interrupts` was suspended by another Run in its tree.

## Protocol 2026-10-07.6

`SkillProposal.scope` and `SkillProposalRef.scope` are `SkillProposalScope`:
`project` or `user`. A proposal never joins an installed plugin's library, so
the wire no longer admits `installation` there; a reference naming it is
refused by request validation.

## Protocol 2026-10-07.5

Every problem that can end a Run, tool call or Item now publishes its default
recovery action: the manifest's `runChannelTypes` entries carry
`recoveryAction`, and the TypeScript contract exports `RUN_PROBLEM_RECOVERY`
beside `PROBLEM_RECOVERY` for failed requests. The two differ for a type that
rides both channels: an `internal_error` request stops, while a Run it ended
can be prompted again. Clients take whether to offer a retry from these tables
instead of keeping their own list.

## Protocol 2026-10-07.4

`SessionSnapshot.session` is required: the snapshot carries the Session it was
read from, with its status resolved from the snapshot's own Runs. Clients take
Session metadata from the snapshot instead of pairing it with a separate
`sessions.get`, which could observe another moment.

## Protocol 2026-10-07.3

A resume that answers Questions commits the answers before its continuation
opens, and the continuation now publishes each answered Question as an
`item.completed` immediately after `segment.started`. A Question Item may
therefore complete twice: once unanswered when its Run parks, and once with its
answers when the Run resumes. Clients fold the second completion like any other
Item; they no longer derive answers from the command they sent or re-read the
Session to learn them.

## Protocol 2026-10-07.2

`SkillProposal.origin` is required. The Runtime records why it created every
proposal, so a proposal without a valid origin is not listed and a client never
supplies a default. Older proposal files written without an origin are not
listed; resubmit them.

## Protocol 2026-10-07.1

A Run tree's root alone carries the facts it owns for the whole tree: only a
root `RunRef` has `protocolProfile`, and only a root `ArtifactRun` has
`messageMark`; both are absent on children. Portable artifact Tool results are
keyed by their owning Item. Older Runtimes and clients are refused by the exact
protocol-version check, and older artifacts by their version.

Rebuild and deploy Runtime, CLI, Desktop/Web, IDE, and generated contract
consumers together. The timeline requires `sessions.trajectory`, and evaluation
export requires `sessions.exportTrajectory`. Bundled clients reject an older
Runtime through the existing exact protocol-version checks; an updated Runtime
likewise refuses requests declaring an older version. There is no legacy endpoint
fallback. The trajectory document has its own `schemaVersion`
(`protocol.SessionTrajectoryVersion`), independent of the Runtime protocol and
importable Session artifacts.

Model invocation and Tool attempt `callId` values preserve the executor's exact
identity: 1–256 ASCII letters, digits, dots, underscores, colons, or hyphens.
Their wire validators are generated from the same rule used for persistence.
Rebuild Runtime and contract consumers together after updating these validators;
existing stored identities need no migration or rewriting.

The protocol includes bounded `WatchSpec.paths` with advertised subscription limits,
`checkpoint_conflict` for a safely refused file restore, and
`prompt_source_too_large` for an AGENTS.md cascade that cannot be included whole.
The existing Run, Segment, Item, and command identities keep their meanings.

Portable Session artifacts carry `protocol.SessionArtifactVersion` and preserve
unresolved-effect evidence for every terminal outcome. Any other artifact version
is rejected; export again from the updated Runtime. The SQLite history representation is unchanged.
CLI attachment commands retain their prepared content with the original command
and replay guard; an older dispatched or ambiguous command whose attachment bytes
are unavailable remains unresolved and is never reconstructed from the current
path. A queued command that has never been dispatched can still prepare its input.

## Portable plugins

Runtime owns plugin installations under its data directory. `plugin.json` uses Agent
Plugins 1.0, and Flame's extension namespace is `io.github.tangerg.flame` with
`apiVersion: 1`. The complete operation schemas and error categories are generated in
[the API reference](contract/API_REFERENCE.md). `plugins` is an advertised capability.

Install from an absolute directory or ZIP path on the Runtime machine. Admission copies
regular files into a bounded, SHA-256-addressed release; it does not execute package code.
The immutable release catalog owns the declaration accepted at first admission and is the
only owner of release content: an installation names its selected and staged releases by
digest. Installation reads hydrate selected and staged catalog declarations in the same
storage transaction as installation state. Lists, MCP definitions and Skills consume that
coherent snapshot, so concurrent release reclamation cannot break a later metadata join.
Byte availability remains a live filesystem observation after the transaction. A release
is admitted once by its constructor, which validates each contribution exactly once (package admission
isolates an invalid contribution against the ones admitted before it); persistence,
installations and projections trust the admitted value. Repeated installation and cold
loading use that declaration; integrity validation does not reinterpret package
contributions. Publish different package bytes to request a new admission.
The package adapter translates the portable `mcp.json` spelling (`streamable-http`, `cwd`)
once into the MCP registry vocabulary (`streamableHttp`, `dir`) that releases and
`PluginServerDeclaration` use; connection rules belong to the MCP server owner, and a
release adds only the rules that exist because the declaration comes from a portable
package (the reserved `PLUGIN_ROOT`/`PLUGIN_DATA` variables, case-ambiguous environment
names, package-relative commands and working directories, no user information in endpoint
URLs, HTTPS outside loopback). Package bytes are public content: static headers and
environment values are never treated as secrets, so a credential reaches a server only
through a declared input. Header and `authorization` inputs are admitted only as secrets;
an environment input uses its declared `secret` flag.
The Flame extension namespace supports `apiVersion`, `inputs` and `contributes`. Any other
member, including capability `requests`, which this API version does not support, is reported as an `unknownField`
diagnostic on its `extensionField` and ignored; it is never admitted or enforced.
Relative source paths, absent or malformed mandatory manifests and unsafe or malformed archives
return `invalid_params` without creating an installation. Filesystem and storage failures
retain their own causes; component diagnostics continue to preserve valid siblings.
Release diagnostics are typed admission findings: a closed `code` (`unknownField`,
`invalidDeclaration`, `unsupportedContribution`, `componentLimit`, `invalidDependencies`,
`unavailableComponent`) plus a closed `component` reference (`manifestField`,
`extensionField`, `contribution`, `mcpServer` and `skill` carry the authored `name`;
`flameExtension`, `mcp` and `skills` carry none). They describe what admission isolated,
travel with the immutable declaration, and never decide availability.
An installation has one closed `state`: `unapproved`, `approved` or `enabled`. It starts
`unapproved`. Approve the exact selected digest, configure declared inputs, then enable it;
disable returns it to `approved`, revoke to `unapproved`. Approval always names the
selected release. Package roots are
read-only. Admission seals the staged tree before digesting it, and admission and cold
loading scan an entire release once; concurrent cold loads of one digest share that scan
while other digests load independently. Startup, configuration, reconnect and OAuth read
the current source owner before creating a connection and revalidate its package bytes:
the cached scan is reused only while every entry of the release keeps the change stamp
(inode, size, mode, modification and inode-change time) recorded when its bytes were
digested, and any added, removed or changed entry or a replaced directory sends the release
through a fresh full scan that withdraws the cache for tampered bytes and restores it for
a repaired directory. A launch accepts only a scan that began after it observed a change,
and rechecks the stamps of the proof it receives, so a scan that read bytes before they
changed can never vouch for them. An entry whose inode-change time is not strictly older
than the scan that stamped it is unsettled (the racy-index rule): a proof holding one is
not reused, and the next launch rescans. Proofs are cached within a 32 MiB budget derived
from measured per-entry size; a proof that would exceed it is not cached, and a proof an
in-flight caller holds is never evicted. Platforms without an inode-change time rescan on
every connection. A stdio launch copies from the confined verified root into private,
sealed execution content and verifies that copy against the admitted digest. Command,
arguments, environment and working directory are projected onto that content; replacing,
rewriting or reclaiming the published release cannot redirect an admitted process or its
later package reads. Connections of one digest share the content while any launch or
session holds it. A canceled preparation ends only its caller's claim; surviving callers
acquire content under their own context. This resource claim transfers once into the MCP
session ledger and is retired after process teardown, including failed handshakes, rejected or superseded
attempts, detach and shutdown. Retained source descriptors contain neither the claim nor
relocated paths and cannot launch installation stdio. Startup collects abandoned execution
copies using directory leases, preserving copies held by another Runtime. This protects
the launch from changes to published bytes; stdio still runs as trusted local code, without
an OS sandbox.
The registry resolves each server by its origin, realizing an installation
server only as far as the read requires (desired definition, dispatch authority, or launch)
and never its siblings. Ordinary tool dispatch reads admitted declarations and
current installation authority for the called server alone. Resource and Skill reads verify their selected file
against the admitted content index through a confined directory capability. `${PLUGIN_DATA}` is a private per-installation directory
retained through updates and uninstall and is never collected automatically. A release
is retained while an installation selects or stages it or a live session or pending
checkpoint depends on it; every other release, its catalog row and its directory, is
reclaimed at the admission point when a change commits and at startup. A release whose
removal fails stays admitted and is retried by the next reclamation.

Staging never switches running code. An already selected digest cannot be staged
again. Selecting a distinct staged release requires execution to be quiescent. It is one
installation transition that always returns the installation to `unapproved`: changed
code is never launched or dispatched until its exact digest is approved and enabled again.
It retains what the candidate declares unchanged: input values whose input keeps its
server, target, key and secrecy, and disabled servers and Skills that are still declared.
A secret input and an OAuth credential follow one rule: a credential follows its
recipient. For a Streamable HTTP server the recipient is the endpoint (transport, URL and
static headers, excluding configured input values), so its secrets and OAuth credential
survive a release that keeps that declaration and stop applying when it changes. For a
stdio server the recipient is the executable (release digest and server name), so its
secret inputs are dropped by every release change and must be entered again. Configuration commands require
the exact selected `digest` alongside `installationId`; a release switch rejects an
older configuration as `plugin_stale` before changing inputs or component enablement.
An omitted digest is invalid. Rebuild clients together; old unbound commands are not
upgraded or replayed against the latest release. Runtime derives quiescence
from one execution-owned dependency projection: the canonical installation ID and
release digest set of every owned executable manifest, covering package Skills and
installation MCP tools, and the same set stored relationally with each pending
execution checkpoint in the checkpoint's own transaction. Installation admission
queries only that projection by installation and never decodes a continuation, so
an unrelated or unreadable checkpoint cannot block another installation's change.
Installation changes, including the capacity check of a new install, and the
publication of an assembled execution share one short serialization point. Run
assembly, model resolution and package reads happen outside it, so a pending change
never waits for, or holds back, an assembling Run. The live MCP tool catalog is a
projection of committed installation state, not a second copy that catches up: the same
critical section that commits a change cancels the affected sources' in-flight connection
attempts and detaches them, in memory and without waiting for I/O, so no Run assembled
after the commit can freeze the superseded release's tools. Launching or dialing the
committed release happens afterwards, outside the lock. An assembly that a quiescent
change to one of its dependencies overtook is refused as `plugin_changed` on
`runs.start` and `runs.resume`: nothing was started and no interrupt was consumed, so the
person may send the same request again. Clients present it as retryable and never retry
it on their own. Revoke and disable withdraw dispatch authority immediately; an already
started effect can still finish or remain uncertain. Tool wrappers recheck current source
authority, their exact realized connection configuration and the currency of the session
that admitted them before dispatch (draft §7.5). Rotating credentials rejects old
executables even before asynchronous reconciliation starts, and a frozen executable whose
session was refused or replaced by a reconnect is rejected as "source connection is no
longer current" instead of failing on the closed transport.
Permission fingerprints bind the admitted code of one server (`release digest + server
name`), so every release change makes standing rules for its tools stale; credential
rotation, approval, revocation and sibling enablement do not erase them.
The rule-list projection observes each referenced MCP source once per request; tool
decisions and dispatch still read the current owner independently.
Permission fingerprints read desired source definitions independently of executable
availability. Missing or changed package bytes cannot hide standing rules; dispatch
still refuses those bytes through its separate admission guarantee.
Rule inspection returns display fields and computed staleness; durable authority
fingerprints and storage scope keys remain with the policy owner.
OAuth credentials are bound to the fingerprint of the OAuth target that requested them:
its MCP identity and credential recipient (for a user server its endpoint and headers; for
an installation server the recipient above). A recipient change makes them stop matching
without any invalidation step; they are removed with their MCP source.
Cold restart preserves exact dependency
checks and command replay identities. Installation commands commit desired state before connection realization.
Every installation read and command result carries `realization`, observed from its live
owners on each read and never stored: `{type: "releaseUnavailable"}` when the selected
release bytes fail verification, otherwise `{type: "available", unavailableBackends?}`
naming declared servers whose backend directory cannot be realized now. A backend whose
preparation failed therefore stays visible in every later `plugins.list` until repaired,
not only in the command that attempted it. Each connection's own outcome is the MCP
supervisor's status (below). Every installation read and command result also carries
`presentation`, the Runtime's closed decision whether the selected release's declarative
presentation contributions (themes) are shown now: `admitted` exactly when the
installation is active and its release is available, otherwise `withheld`. Clients
present admitted themes only and never rebuild that decision from `state` or
`realization`. Uninstall requires
the same quiescence as a release switch and returns `plugin_in_use` while an active or
waiting execution depends on the installation; revoke remains available to withdraw its
authority while in use. Uninstall is an acknowledgement without a result: it durably removes
admission before asynchronous connection retirement, and leaves all retirement work in the
existing Connections shutdown ownership graph.
Post-commit package preparation, reconciliation, and projection survive request
cancellation and follow the Runtime's cancellation root. A reconciliation that cannot
start settles each still-enabled source as `mcp_configuration_failed` at the connection
status owner; a removed source stays absent. The delivery endpoint
joins these calls before closing their dependencies; shutdown does not undo the
committed installation state. When shutdown preempts the post-commit realization read,
the command reports that cause even though its durable change stands.
Connection attempts own credential restoration as well as dialing. Reconfiguration
withdraws the old session and tool snapshot before restoration; restoration failure
settles the source as failed. A failed MCP server status names its closed failure
category as an inline problem type without prose: `mcp_release_unavailable`,
`mcp_backend_unavailable`, `mcp_configuration_failed` (the source refused the connection
or stored credentials could not be restored), `mcp_dial_failed` (transport, process or
handshake), `mcp_tool_discovery_failed` and `mcp_authorization_failed`. The connection
owner records the category at the failed transition, including startup admission; the
cause itself stays in traces and logs because it can carry paths, endpoints or
credentials. A connection to a disabled server, or to any server of a disabled
installation, is refused with the MCP `ErrServerDisabled` category. A refusal is a live
transition, not only a command error: when configuration, reconnect or authorization
cannot obtain an admitted configuration, a removed or disabled source is detached and any
other refusal settles as `mcp_configuration_failed`, and either way the previous session
stops serving tools. A connection dispatch or an installation reconciliation that cannot read its source owner
settles the same way, so the failure reaches status readers instead of only the log.
The connection owner admits configuration, authorization and refusal under the same lock
that advances live state. A command canceled before admission changes no session, tools,
credentials or status; a superseded launch cannot withdraw its replacement connection.
Supersession cancels obsolete work; shutdown cancels and joins every admitted attempt.
A stdio server that reports an exit status after its input closes or it is signaled has
exited, so its retirement succeeded whatever the status. Only a session that could not be
stopped, or a process-group cleanup that failed, is a retirement failure; it is logged when
it happens and still reported by Shutdown.
Canceling a wait for session retirement leaves unreported close failures with Connections
until shutdown consumes them.
OAuth target fingerprints encode the owner's exact fields and header bytes through the
shared framed hash, independently of JSON serialization. Credentials recorded with the
former JSON hash require a new OAuth authorization; no dual fingerprint reader is retained.

Portable `mcp.json` admits stdio and Streamable HTTP through the existing MCP registry,
connection supervisor, deferred tools, exposure, approval and OAuth owners.
An MCP server's identity is its origin plus the name that origin chose:
`mcpserver.ID{Origin: User | Installation(id), Name}`. Names are unique only within an
origin, so a user server and an installation server may share one. The same structured
identity is the storage key (an `mcp_sources` row with `origin`, `installation_id` and
`name`), the source half of every MCP `tool.Ref`, and the single wire shape
`MCPServerID{origin: {type, installationId?}, name}` used by `MCPServer.id`, every
server-addressed request, `ToolRef.server`, tool listings, exposure, OAuth attempts and
`mcp.changed` events. No component joins the parts into one string or parses one back.
An installation record's `Source` also binds the admitted release digest, the tool
authority of that code and the recipient its credentials follow, so none of them can be
claimed by a user record or omitted from an installation record. Model-visible tool names still project from the local name
alone; equal projections from different origins are excluded symmetrically.
Their connection configuration is changed through installation operations. User MCP CRUD
refuses these sources. Invalid server declarations are diagnosed independently.

User MCP server membership and configuration belong exclusively to the durable resource
commands: `mcp.servers.create`, `mcp.servers.update` and `mcp.servers.delete` (or the
corresponding Go binding methods). Startup only restores these resources and realizes
enabled connections. `FLAME_MCP_SERVERS` and `FLAME_MCP_<NAME>_TOKEN` no longer configure
or seed servers. Configure new servers and credentials through the existing CLI or
Desktop MCP controls, or the public Runtime binding. Previously persisted servers remain
ordinary user resources; deleting one stays effective across restart.

Typed declarations reject explicit `null`, including nested arguments, environment values
and theme colors, instead of converting it to an implicit default. Invalid contribution
lists receive their own diagnostic and do not withdraw independent contributions.
Catalog reads retain durable source membership when a release or backend directory is
unavailable, project enabled unavailable sources as failed without stale tool counts,
and preserve independent healthy sources. Desired descriptor projections never admit
connections or dispatch. Temporary execution unavailability does not remove standing
tool exposure choices. Exposure inspection and edits read durable source definitions;
reconnect and authorization reach the connection owner's launch integrity check, so a
repaired release can recover without replacing its installation.
Stdio is an explicitly trusted executable with the Runtime user's OS access, not an OS sandbox.
Only PATH, the reserved package paths, declared environment values and configured inputs
are inherited. HTTP authorization and secret headers use declared host inputs or the
existing OAuth owner. Credentials must not be embedded in portable files or UI assets.
Package entries, relative commands, working directories and resource reads share one
portable path contract; Windows reserved names and ambiguous path segments are rejected
before filesystem preparation on every host.
Reserved device stems include superscript digit aliases from the
[Win32 naming rules](https://learn.microsoft.com/en-us/windows/win32/fileio/naming-a-file).

Package Skills join the existing resolver. Explicit project Skills override user Skills,
which override package Skills. Conflicting package names are excluded rather than chosen
by installation order. Discovery, loads and frozen Run dependencies use the same source
resolution. Discovery diagnoses explicit project/user overrides and counts capacity per
selected source. Revoked or disabled package Skills become unavailable.
Discovery distinguishes package name conflicts from unavailable release bytes or
withdrawn authority. An override diagnostic can accompany its available project
or user Skill; independent installations do not share one Skill capacity budget.
Admission, discovery, detail and model loading share Scope's format and directory-name
binding. Parsing consumes exactly the verified bytes. Names must already be in Unicode
NFKC form, and the directory and frontmatter must have exactly the same name. Rename
noncanonical directories and update their frontmatter before rebuilding a package;
Runtime neither normalizes names nor keeps aliases for their former spelling.

Configuration changes use `valueChanges: { "input-id": { "type": "set", "value": "..." } }`
or an explicit `{ "type": "clear" }`. An optional empty value remains a configured
environment variable or header; clearing removes its binding. Inspection reports
`inputStates` for every declared input of the selected release: `{type: "unset"}`,
`{type: "value", value}` for a configured non-secret input, or `{type: "configured"}` for a
configured secret, including an empty one. Secret text is never copied into any
projection. Configuration is a delta: `serverChanges` and `skillChanges` map a declared
component name to `enable` or `disable`, and inputs and components a request does not
name keep their value or enablement. Required inputs cannot be cleared while their
backend is enabled. Each environment or header binding belongs either to static package
configuration or to one declared host input. Inputs cannot replace static values; HTTP
header binding identity is case-insensitive. Portable environment bindings also reject
case variants that would compete on Windows. Invalid inputs are diagnosed independently.

Themes must declare `scheme: "light"` or `"dark"` and use only the bounded color vocabulary.
The generated Go, JSON Schema, and TypeScript response checks project the domain's
color names and six-digit hexadecimal grammar before colors reach client rendering.
The appearance owner retains a rendered projection for first paint. Runtime scheme
selection reads the registered theme or system appearance, never the paint cache.
Removing, disabling or losing the release of an installation withdraws its selected theme
preference and the first-paint projection with it. Themes use the existing
Dougong Host and child lifetimes; client connection replacement retires the predecessor
before publishing its successor.

The current implementation includes Slice A/B and one optional Slice C page. A package
may declare `contributes.views` with `id`, `title`, `type: "sessionTrajectory"`, and a portable
`.html` `entry`. Admission validates UTF-8 HTML, confined resource access and the release
fingerprint. Invalid views produce diagnostics without withdrawing independent themes,
Skills or MCP declarations. Action forms, arbitrary projection queries, mutation bridges,
IDE view hosting and language integrations remain unavailable.

The [trajectory example](../examples/plugins/trajectory/plugin.json) exercises a theme,
a Skill and an isolated Session trajectory page. The public page API requires no code
execution in Runtime. Desktop's [carrier acceptance](../desktop/README.md#plugin-carrier-acceptance)
owns rendering qualification; an unavailable renderer does not change installation state.
Package limits are 128 MiB total copied bytes, 16 MiB per file, 4096 entries, 256 Skills and
128 installations. Skill documents and resources obey the existing 1 MiB Skill limits.
HTML views are limited to 16 declarations per release and 512 KiB per entry.

This is a breaking protocol/storage change. Rebuild all clients and generated contracts
together. Installation records reference immutable declarations by digest. Installation
IDs and release digests are typed values (`resourceid.InstallationID`,
`fingerprint.Digest`) parsed once at the delivery and storage boundaries; the 64-digit
lowercase SHA-256 spelling has one owner, shared by release digests and source authority
fingerprints. `mcp_sources` holds one row per server identity; the user descriptor
(`mcp_servers`), exposure, OAuth sessions and approval rules reference it by a cascading
foreign key, so deleting a user server or uninstalling a package removes every relation
about its servers. Approval rules store the tool kind, the source-local tool name and the
MCP source reference as columns rather than an encoded reference string; installation
records do not store server identities or declarations. Installation state is relational:
the installation row holds its source, selected and staged digests and its closed
`admission_state` (`CHECK`ed to `unapproved`, `approved` or `enabled`; a staged digest
differs from the selected one), and child tables hold input values and disabled
components, each removed with its installation by cascade. Open admits a data directory
that is empty or holds exactly the current tables, indexes and triggers; any other
directory, such as one with former MCP, installation, OAuth or checkpoint shapes, is
refused without modification, and no former shape is repaired in place. Pre-release
storage compatibility follows
[the repository data policy](../DEVELOPMENT.md#pre-release-data-policy). Continuation
payloads no longer carry dependency bindings. Completed historical Tool content remains generic and readable.

Stored current and historical Plans require a steps array; clearing a Plan records `[]`.
Empty text and JSON `null` are invalid Plans. Run usage and frozen capabilities use
empty text for absence; usage objects preserve reported zero, and capability objects
must declare at least one capability. Invalid stored values fail reads rather than
inventing empty state or reported usage. Valid records require no migration.

Publishing the new Runtime module and advancing CLI's released dependency is required
before an independent CLI release; workspace checks alone do not prove that release.
Installation Skill declarations expose names and descriptions. The package adapter
derives their fixed portable paths; release records and protocol declarations no
longer store another editable path beside the Skill identity. Rebuild generated
contract consumers and use a fresh pre-release data directory for this storage change.

## Open an in-process Runtime

```go
rt, err := runtime.Open(ctx, runtime.Config{
	DataDirectory:        dataDirectory,
	DefaultWorkspacePath: workspace,
})
if err != nil {
	return err
}
defer rt.Close()

session, err := rt.CreateSession(ctx, protocol.CreateSessionRequest{
	Workspace: &protocol.WorkspaceRef{Path: workspace},
}, runtime.CommandOptions{IdempotencyKey: requestID + ":session"})
if err != nil {
	return err
}
```

Hosts must close each Runtime they open. Protocol errors support `errors.Is` against public sentinel errors and `errors.As` to `protocol.ProblemError` for structured recovery information.

Repeated `Close` calls join or resume an incomplete shutdown. Once terminal
resource teardown finishes, every call returns its retained diagnostic without
repeating cleanup; a later call cannot turn a failed close into apparent success.
Shutdown cancels accepted calls and component-owned work before joining delivery.
Dependencies close only after their calls and components have returned.

## Attach without owning the Runtime

```go
client, err := runtime.Connect(ctx, runtime.RemoteConfig{
	Endpoint: "http://127.0.0.1:17171",
	Token:    token,
})
if err != nil {
	return err
}
defer client.Close()
```

`Client` uses the same typed methods and call options as `Runtime`. Its HTTP requests enter the serving Runtime's existing delivery endpoint. Closing it cancels its requests and subscriptions without shutting down the server or canceling accepted Runs. A connection failure never constructs an embedded Runtime. A caller that intends to cancel execution sends `CancelRun` explicitly.

Operation methods require a non-nil binding returned by `Open` or `Connect`. Their shared implementation no longer supports invoking an operation through a nil `*Runtime`; callers must handle constructor errors before use. `Close` remains nil-safe, and operations on a closed binding return `ErrClosed`.

`Endpoint` names a base URL, including any reverse-proxy prefix, without credentials, a query, or a fragment. Authorization travels in the bearer header. Workspace paths belong to the server's filesystem and operating system; the client does not clean them with its own platform's path rules. Client-local attachments must be read into command content before dispatch. Retries retain the original content, idempotency key, and durable namespace.

## Serve the browser application

The standalone Runtime can publish the built `desktop/frontend/dist` directory on its own HTTP origin. Set `server.webDirectory` or `FLAME_SERVER_WEBDIRECTORY` to that directory's absolute path. Startup rejects a missing distribution instead of presenting a partially configured browser endpoint.

Static assets and application navigation are public. All Runtime calls still use the generated `/v2/rpc` endpoint and its existing bearer gate; static routing never replaces protocol or health routes. The asset server confines reads to the distribution, rejects hidden files and directory listings, and keeps application HTML revalidated on reload. No token is embedded in HTML or placed in a URL.

The browser starts at its own origin and accepts the Runtime token in the connection dialog. Desktop uses its native bootstrap. Both consume the same TypeScript client from `contract/typescript/client`; Wails capabilities belong only to the desktop platform adapter.

The shared TypeScript client owns strict response decoding for JSON-RPC, event streams, and HTTP sidecars. HTTP bytes must be valid UTF-8. Microsoft `jsonc-parser` visits decoded members and strings to reject duplicate members and unpaired Unicode surrogates before native `JSON.parse` constructs values; JavaScript number and object-member semantics remain unchanged. Malformed responses are `RpcProtocolError`, which preserves unknown mutation settlement without treating invalid wire data as a recoverable connection failure. Event-stream observations cancel unread bodies before releasing their readers, including when their sole consumer returns early.

## Errors and acknowledgement certainty

Operation failures separate the stable machine `type` from their human-readable `detail`; a bare category omits redundant detail. Clients render the type once and use structured field errors, capability requirements, and active-Run references when present. Go callers retain underlying causes without parsing the display text. Explicit and inferred failures pass the same wire validation before leaving the endpoint.

Remote Go calls report malformed protocol replies through `ErrInvalidResponse`, transient connection loss through `ErrDisconnected`, and uncertain command acceptance independently through `ErrAcknowledgementUnknown`. A malformed reply can leave acceptance unknown without being safe to retry automatically. Recovery must retain the exact original command parameters, idempotency key, and namespace; the client never silently resends a command or follows an HTTP redirect.

Cancellation failures returned by the Go binding also preserve `context.Canceled` or `context.DeadlineExceeded` for `errors.Is`. Request cancellation causes are local to that invocation; wire problems and persisted replay outcomes retain their protocol category and safe details.

The standalone Runtime and CLI interpret `FLAME_HOME` as Flame's local product root. Runtime-owned state lives under `$FLAME_HOME/runtime`; the default is `~/.flame/runtime`.

## Develop

```sh
GOWORK=off go test ./...
GOWORK=off go vet ./...
GOWORK=off go build ./...
go generate ./...
```

The default suite is offline. Module rules live in [Module instructions](#module-instructions) below; current boundaries live in [`doc/ARCHITECTURE.md`](doc/ARCHITECTURE.md).

The source map follows responsibility within each ring: Run execution and input
translation live in `internal/adapter/run`, model translation in
`internal/adapter/integration/model`, and Hook management in
`internal/application/integration/hooks`. Feedback has its own Domain value and
Application recorder because its lifetime is independent of Session cleanup.
The architecture gate rejects unclassified production packages and imports.

Waiting executor checkpoints require the exact executable BuildID. A rebuilt
binary can reject them after source-only changes, including directory moves.
Finish or cancel waiting executions before replacing their owning build. This
does not require a migration of completed history or alter the public protocol.

`make run` builds and runs the development server in the foreground. Use Ctrl-C
and wait for it to return before starting a replacement. For a fresh database,
run `FLAME_HOME="$(mktemp -d)" make run`; the new product root is isolated and is
retained after shutdown. `FLAME_HOME` is exported consistently to the server.
The former `start`, `stop`, `restart`, `status`, `logs`, `reset-db`, and `fresh`
targets are removed. Runtime processes can share a data directory, so a pidfile
or port owner cannot authorize deleting its database. `make clean` removes only
the compiled binary; data cleanup remains an explicit offline operation.

## Module instructions

Flame Runtime is the product backend and sole owner of durable agent semantics. It exposes one in-process Go binding and one Runtime Protocol for CLI, Desktop, and other hosts.

Read [`../AGENTS.md`](../AGENTS.md), [`../DEVELOPMENT.md`](../DEVELOPMENT.md), and the ordered Runtime baseline in [`doc/README.md`](doc/README.md) before changing this module.

- Runtime is a product backend, not another Agent Framework. Scope owns Process, strategy, tree, provider, and effect execution contracts; Runtime adapts them to Flame product semantics.
- The module root owns the public in-process `Runtime` lifecycle and typed operation methods. `protocol` owns public request, response, event, error, version, and validation values. Do not define synonymous public models or a forwarding binding package.
- The Go binding and HTTP/JSON-RPC binding must enter the same delivery endpoint before capability admission, idempotency, invocation lifetime, Application use cases, and event or error projection.
- Domain packages own behavior-rich aggregates, value objects, invariants, and pure transitions. Aggregates validate construction, hide mutation, and expose legal domain operations; protocol, storage, configuration, provider, and projection values remain strict data. Application packages own cohesive use-case families and cross-aggregate ordering. Neither depends on protocol, storage, provider SDKs, or delivery types.
- Keep a Domain or Application package only when it protects a real aggregate vocabulary, invariant, or workflow lifecycle. Merge helper, identity, reference, and one-concept packages into their proven owner; do not replace them with one god package.
- Organize each large ring as `ring/context/package`. Context directories are non-package namespaces backed by multiple related packages; direct ring packages are limited to ring-wide mechanisms or aggregate packages that also name their context. Do not create facade parents, single-child namespaces, or deeper decorative nesting.
- Group external code by the system it translates: provider, MCP, SQLite, filesystem, Git, execution, LSP, or transport. Delete adapter-to-infra forwarding layers that own no policy or lifecycle.
- Bootstrap is the only concrete composition and shutdown owner. It constructs one delivery endpoint and one resource graph; do not maintain parallel Assembly, Host, Instance, Server, or optional-service lifecycles.
- Interfaces belong to consuming Application or delivery code and contain only invoked operations. Keep a cohesive single implementation concrete.
- Run is the product execution center. Conversation, Transcript, WorkingContext, checkpoint, stream observation, durable Item, and recovery remain distinct facts with explicit owners.
- Provider/model selection is exact and durable. Provider SDK types, credentials, endpoints, request lowering, and provider-specific errors stay behind provider adapters. Optional provider capabilities use separate narrow consumer contracts; never inflate the ordinary chat client or approximate an unavailable capability in Runtime.
- Protocol operations express product behavior, not internal functions. Catalog metadata is machine truth; generated contracts and typed Go methods are checked projections of it.
- Public API, wire, storage, and generated-contract changes are replaced completely. Migrate all in-scope consumers and delete the former shape without aliases, fallback decoding, dual persistence, or compatibility packages.
- Prefer real single-Runtime end-to-end scenarios for Goal, Plan, steer, HITL, interruption, compaction, long context, long execution, provider failure, restart, and recovery. Test concurrency only where Runtime owns concurrent lifecycle; do not invent multi-client or multi-server scenarios without a product obligation.
- Decide compaction only at an imminent model call from that complete request's token footprint. Do not trigger it from protocol message counts or Run completion. Pre-release SQLite installs the current schema directly; do not add hand-maintained epochs or a migration graph without an explicit migration requirement.

## Model invocation history

`modelInvocations.list` reads recorded provider attempts for one exact Run, newest first, with an opaque cursor and a maximum page size of 100. Child Run reads require the subagent capability, just like `runs.get`. The Go binding exposes the same operation as `ListModelInvocations`.

The invocation journal owns call identity, Segment identity, observed state, and timestamps. These records survive Run completion and restart; deletion of their Run cascades to the records.

`started` has no settlement timestamp. `completed` means Runtime accepted the complete model response. `failed` records an observed failure at the model boundary, including an invalid response or interrupted stream; it does not imply that the provider performed no work. `unknown` means execution or recovery could not establish the outcome: its `settledAt` is when that uncertainty was recorded, not a provider completion time. Consumers must not infer a measured model duration or throughput from an unknown outcome.

The timestamps come from Runtime's call lifecycle. `startedAt` is captured while reducing the start fact, before its durable commit and provider dispatch. Completion is captured after response validation and the stream projection barrier, before the completion commit. For completed and failed calls, their difference measures this Runtime lifecycle interval, including local work and waiting; it is not isolated provider latency. Neither lifecycle timestamp records the first output. Decode throughput cannot be reconstructed from this interval, token totals, or transcript timestamps.

The optional `firstOutputLatencyMillis` is measured with the monotonic clock at the streaming model boundary: from entering the provider stream to arrival of the first valid text, visible reasoning, refusal, media, or tool-call delta. It includes provider-client and network work, but excludes Run admission and the start commit. Metadata, usage, finish markers, citation attachments, and opaque reasoning state do not count. The measurement is committed with a completed or failed attempt; a stream that fails after producing output retains it. Nonstreaming calls, failures before output, unknown recovery outcomes, and historical attempts leave it absent. A measured zero is valid. It is not a token-generation rate or isolated model compute time.

The optional `usage` records provider-reported tokens for that call before Run aggregation. Missing usage means it was not reported or the attempt predates usage recording; an explicit zero remains zero. Prompt inspection is not present in this read. Aggregate Run accounting remains separate.

## Session trajectory

`sessions.trajectory` / `ListSessionTrajectory` reads one bounded page of durable
Run, model-invocation, and transcript Item observations for a Session. The default
and maximum page size is 100. `includeDescendants` defaults to false; requesting
true includes the complete Run tree and requires the subagent capability.

Entries reuse `RunRef`, `ModelInvocation`, and `Item`. Their discriminated source
and original identity identify the row; there is no additional trajectory journal
or execution state machine. Run records retain outcomes, unresolved effects,
accounting, and parent/root edges. Model records retain the exact call and Segment
identities and reported measurements. Items retain their content, tool arguments,
results, approval decision, and original timing. An Item without a recorded model
association must not be assigned to a model call by timestamp proximity. Fetch an
entry's owning Run through `runs.get` when that Run is outside the loaded page.

The order is newest occurrence first, followed by source kind and source identity
as deterministic tie breakers. Run admission, model start, and Item occurrence
supply the respective timestamps. This is an observation ordering, not a global
event sequence or proof of causality. Each entry contains its source record's
current state, so completion updates that entry without moving its page position.
Unknown model settlement remains an observation of uncertainty and supplies no
measured execution duration or inferred throughput.

A page's existence check, source selection, and record hydration share one SQLite
transaction. Its cursor is opaque and bound to the Session and descendant filter.
Pages remain bounded as history grows. Continuations do not freeze an entire
Session across subsequent writes; refresh the read after execution changes, and
use the idle Session evidence export when a coherent complete document is needed.
Completed observations survive restart for as long as their source records remain.

## Auxiliary model observation

Compaction, memory extraction and curation, skill mining, and title generation emit an `auxiliary model` OpenTelemetry span through the existing Runtime exporter. `auxiliary.operation` identifies the caller's purpose. Auxiliary calls follow their caller's cancellation and deadline; the adapter adds no wall-clock timeout. Input and output envelopes remain bounded. The span covers selection resolution, the model request, and response acceptance; its duration is not isolated provider latency. The live resolver records the exact provider/model selection, and a valid response contributes its finish reason and reported token usage. Optional cache and reasoning counts remain absent when unreported, including the distinction between absence and an explicit zero. A valid but incomplete response retains its reported usage even though its text is rejected.

Failed attempts record the `resolve`, `call`, or `response` stage and distinguish cancellation from deadline expiry. These spans do not record prompt text, output text, or raw provider errors. They are diagnostic telemetry, not durable `modelInvocations.list` records or additional Run accounting. Persistence and retention depend on the configured telemetry exporter; they do not survive restart through the invocation journal.

Auxiliary text generation uses Scope's `Client.Output` with its `Text` contract. Scope admits only naturally completed text and reasoning; refusals, media, and Tool calls cannot become summaries, memory, skill proposals, or titles by dropping their non-text parts. Runtime additionally rejects blank text and retains its resource limits and observation policy.

Title generation is nested under `run segment maintenance`, with `run.id`, `gen_ai.conversation.id`, `maintenance.operation`, and `run.parked` identifying its boundary. A parked Run can generate its initial Session title while waiting for user input; this span does not mean the Run has completed. Workspace checkpoints use the same span name and run only at a terminal boundary.

## Trajectory evidence export

`sessions.exportTrajectory` / `Runtime.ExportTrajectory` exports one JSON evaluation
document with `schemaVersion` (`protocol.SessionTrajectoryVersion`) and a collection timestamp. It includes the Session,
all root and child Runs, Items, retained conversation messages, offloaded Tool bodies,
current Plan, model attempts, Tool attempts, and matching user feedback. Child evidence
requires negotiated `subagents`; the export never silently omits it. The existing
`sessions.export` / `sessions.import` portable artifact keeps its own version and
separate import contract.

Export holds the idle Session admission and reads every source in one SQLite
transaction. An active or waiting Run returns `session_busy`; finish or cancel it
before exporting. The complete stored input is limited to 64 MiB and 100,000 source
records before bodies are loaded, and the final JSON response is independently
limited to 64 MiB. Exceeding either bound returns `export_too_large`, with no partial
document. These bounds apply to evidence export, not ordinary trajectory pagination.

The export contains all currently retained evidence, not every historical state
transition or every provider request. Compaction can replace old conversation context;
per-call prompts, provider request bodies, and auxiliary telemetry spans are absent.
Model and Tool journal records survive terminal Runs and Runtime restart until
their Run is deleted. Tool attempts preserve separate
Segments: `completed` means an observed definite result, `incomplete` includes
suspension for input, and `started` carries no known settlement. Neither these states
nor Run completion evaluates answer quality.

Feedback remains an independent append-only user signal. Matching any supplied
Session, retained Run, or retained Item reference includes the original observation;
references remain unverified and may identify records already removed. General
feedback without a matching reference is excluded. Export never manufactures a score
from execution status, reconstructs missing attempts, or fills absent usage with zero.
Run metrics include descendant accounting: do not add root and child totals together
or combine those totals with per-call usage. The document carries these limitations
alongside its evidence so downstream evaluation can preserve the distinctions.

## Background shell lifetime

Background commands remain addressable after they exit until `read_shell_output` consumes their final output. That final read reports completion and releases the shell handle and retained buffer; later reads report that the shell is absent. Compaction reminders preserve these retained handles, including commands that have finished with unread output. Reads while a command is running keep its handle available. Stopping a command preserves its unread output for the final read. Session teardown and Runtime shutdown also reclaim owned commands.

Reading or stopping a shell requires its owning Session, even when another Session knows the exact shell ID. Host teardown retains its authority to stop a Session, a shared workspace, or the complete Runtime. A call canceled before process admission cannot launch a detached command.

Shell output uses Scope's `tools/content.Content`: `stdout` is an object with `encoding` (`utf8` or `base64`) and `data`. Both foreground completion and `read_shell_output` preserve arbitrary process bytes, including output truncated inside a UTF-8 character. Background launch responses include an explicit `shell_id`; incremental reads include `shell_id`, `status`, `stdout`, and an optional `output_dropped` flag. The command transcript still renders valid text directly and represents binary output with its lossless encoded content. Consumers of raw Tool output must use this current shape; there is no string-output compatibility decoder.

## Execution lifecycle

Root and delegated Interactions, ordinary Tools, and lifetime child/process counts use Scope's unlimited cumulative quotas. Usage remains metered; completion, cancellation, model-reported blocking, and actual failures control lifecycle. Delegation depth, active-child and Tool concurrency, pending mailbox capacity, and operation-specific waits bound resource use. Execution follows its owner's lifetime.

## Scope execution settlement

Runtime uses the released Scope modules pinned in `go.mod`. Static Tool and Delegate
bindings belong to Scope Deployments through `Definition.ChildDeployments`;
Scope owns their identity, start, and restoration. Provider construction uses
Scope's `Chat`, `Messages`, `ChatCompletions`, or `Responses` entrypoint for the
selected API.

This upgrade changes Scope's Deployment and execution-tree snapshot schemas.
Complete or cancel waiting executions with their owning build before upgrading.
Older checkpoints are rejected under the existing build-identity and strict
snapshot rules. Completed product history requires no migration.

Scope alone propagates root, subtree, and owner cancellation to execution contexts. Runtime submits cancellation intent and projects Scope’s immutable termination; a late cancellation or owner deadline cannot replace an already settled failure. Provider diagnostics enrich only the model-failure stop acknowledged by Scope.

A validated complete model response and its reported usage publish under the release-owned lifetime, including the Delta barrier. Execution cancellation cannot discard that observed response or rewrite Scope's terminal outcome. Failed streams cross the same barrier and commit their validated text and visible reasoning as `incomplete` transcript Items tied to the failed model invocation. Preview loss cannot truncate this retained prefix. Partial Tool calls, opaque reasoning state, and incomplete responses never enter canonical model continuation. Both completion and failure close the preview boundary before terminal publication.

Repeated identical Tool results are allowed, including polling. Tool authorization and lifecycle-hook approvals remain enforced.

Canceling a delegated Run still cancels that child. If its in-flight model attempt returns no definite result, Scope retains the unknown Effect and fails the parent instead of delivering a normal delegate result. Runtime projects the actual Scope outcome and preserves unresolved Effect IDs separately; it does not manufacture a settlement or retry the attempt. This intentionally replaces the earlier behavior that allowed the parent to continue despite the child's unresolved Effect. Cancellation before external dispatch and terminal children with definite results remain distinct cases.

Known Tool and Delegate results, including rejected calls, are extracted through Scope's `interaction.SettledResults` at its `TreeCommitter` boundaries. Runtime atomically commits the authoritative execution-tree head, product Items, invocation journal, and content-bound result receipts. Each receipt retains its exact model-visible Tool results, including ordered content, metadata, and structured details. Settled calls retain their original round indexes even when other calls remain unresolved. The model conversation receives one ordered Tool message when the round completes. Live termination, cancellation of a parked Run, and boot recovery share the same conversation closure: already acknowledged results retain their exact output, and only unresolved calls receive an unavailable-result marker. The reducer keeps no separate conversation ledger.

Result receipts bind to the durable source message's unique sequence identity before the complete Tool message is appended. Reused provider call IDs in another round cannot adopt prior results. Compaction, rollback, and history replacement invalidate removed message references while retaining immutable publication identities for deduplication. A Run whose unresolved Tool calls lack exact receipt evidence fails terminal reconstruction explicitly; its acknowledged work is never silently rewritten as failure feedback.

Tree updates compare the previous writer and digest and require the next incarnation-local commit sequence. Checkpoint identities include the writer and sequence; Effect identities include the Effect ID and boundary kind across writers. SQLite commits the head and deduplication facts atomically. Restoration activates a new writer at sequence zero. A historical commit cannot succeed merely because tree content repeats. Repeated known results are checked against durable receipts before retired presentation metadata is accessed. Ambiguous COMMIT responses are reconciled by reading the tree and product receipts, without rerunning tools. Cancellation stops new execution and model continuation while observed settlement writes remain owned by the executor's release lifetime. Storage latency has no separate elapsed-time failure threshold: release cancels pending publication, then joins the execution workers and Scope Engine before revoking Tool resources.

Process-local fact and child-start receipts retain the pump's decision; observing a
receipt never consumes or changes that decision. Withdrawal of a queued child-start
request retains its original cause. After claim, waiting still joins the conclusive
decision even when the producer's context is canceled.

Recovery probes only validate persisted state; they never activate a writer. Waiting checkpoints retain product bindings, while the committed execution-tree head is authoritative for restoration. A checkpoint holds no copy of the root Run's model selection, capabilities, or Goal incarnation, nor of the Session's workspace or isolation; restoration reads each from its owner. This integration requires the current Scope snapshots and the current execution-tree schema. Older waiting snapshots and databases are not converted or replayed. Schema installation rejects the former execution-tree table without modifying existing data; use a fresh data directory and retain the old one separately.

Plain executor errors, lost responses, cancellation, and host failures do not prove a business outcome. They remain unknown and cannot produce normal model feedback. A concrete executor may return an explicit Scope Tool failure with its known output, including partial success; validation and authorization owners may reject calls they have not executed. Runtime's generic Tool observer never converts arbitrary errors into known failures. Approval-subject validation translates only a definite local argument refusal into a known failure, retaining effective arguments and result metadata through the same durable settlement path. Hook and authorization rewrites are validated before executable entry; arbitrary hook or authorization-store errors remain host failures. Product cancellation closes abandoned Items without inventing Tool results.

A direct Tool completion ends the Run after its ordered results commit. It adds no assistant answer and makes no further model call; a delegated direct completion supplies those same results to its parent.

Tool concurrency declarations describe the effective executable boundary. Wrappers that can change arguments through hooks, authorization, or approval continuation declare exclusive execution, because an inner resource key computed from the original arguments is no longer safe. Immutable invocation paths preserve the inner declaration; Scope remains the scheduler.

Scope owns the model-visible payload. Delegate results use `interaction.Output` directly; the former `{"reply": "..."}` envelope is removed. Runtime adapts the task input, while Scope owns the complete execution and output, including after restoration. Runtime commits assistant content and accounting at the model boundary before returning the response to Scope. Process termination closes the product Segment without another message event, reply cache, or confirmation handshake. Runtime never reconstructs Delegate output from UI text or independently formats Delegate diagnostics. Pending product metadata, including effective arguments, mutation paths, and offload references, accompanies the Scope checkpoint until its complete model round is published. Delegate admission retains its invocation metadata independently of the currently active child batch, so an earlier Delegate result survives when a later batch waits for input. Resuming that tree preserves the completed work.

Rejected calls retain their original input as `argumentsText`; parsed `arguments` remains an empty object. Transcript storage, history artifacts, CLI, and Desktop preserve this distinction, including malformed JSON. Existing records need no schema migration; previously omitted results cannot be reconstructed from the database alone. Run aborts persist the first causal diagnostic instead of replacing it with a generic internal error.

The Interaction snapshot shape changes with this contract. Waiting checkpoints from an older build must be completed before upgrade or discarded under the existing build-identity rule; no dual-schema reader is provided. Runtime uses a Scope engine backed by its SQLite TreeCommitter and restores only committed waiting checkpoints. It has no mid-run crash recovery: a restart with unfinished execution retains the existing `RunLost` policy. A lost Run's failure detail names why it could not continue: a restart outside a waiting boundary, an isolated workspace, an unavailable workspace, waiting state written by another build, waiting state that cannot be restored, or a configuration change while it waited. The probe's finer reason and cause stay in the trace. All Scope commits fence publication by Tree incarnation, commit sequence, and retained commit identity.

Waiting checkpoints declare the offloaded result IDs required by their continuation. SQLite saves these references atomically with the checkpoint, verifies Session ownership, and releases them when the checkpoint is replaced or consumed. Startup and failed-write cleanup delete only bodies held by neither a checkpoint nor an Item. Restoring an executor verifies that the declared references exactly match its pending result metadata; old checkpoints missing this ownership information are rejected under the existing recovery policy, without a compatibility reader.

Unknown-effect observations retain the Effect IDs and the first available local failure diagnostic. The RunLost record preserves these details while keeping the outcome unknown; diagnostic text is never evidence that an external operation succeeded or failed.

Portable Session artifacts preserve unresolved effects on every terminal outcome, including canceled and timed-out Runs. The artifact retains the exact source process and Effect identities, cause, reason, and detail through export and import. These are read-only historical evidence: imported Runs have no active Segment, open interrupt, or execution checkpoint, and the identities do not authorize resume or retry. Import accepts only the current artifact version; earlier development artifacts are rejected rather than treated as complete evidence.

Unknown-effect termination closes every unfinished member in Run-tree postorder, retaining the same evidence on each lost Run. Completed members keep their outcomes. Each terminal commit settles open model attempts as unknown and abandons unfinished Tool Items without inventing model-visible results; the executor is released only after the terminal publication sequence.

## Workspace observations

`runtime.subscribe` accepts exact workspace-relative `watches[].paths`; `"."` names the workspace directory. Omitted paths request only Git HEAD/index observation. Explicit paths observe ordinary external writes, atomic replacements, removal, and recreation in Git and non-Git workspaces. A directory target observes its immediate entry names and metadata; it does not recursively watch children or follow their symlinks. An opened file needs its own target to observe content changes.

Discovery publishes the subscription's total path budget, per-directory entry budget, and file hashing budget. Content up to that byte budget is hashed; larger files remain observable through size and modification metadata. Exceeding directory or registration bounds fails explicitly. Background content-observation failures, including those of the Hooks and Skills observations behind `hooks.changed` and `skills.changed`, end the subscription, and Git sampling failures retry with bounded backoff before ending it. Clients reopen the stream and revalidate their reads; this is an invalidation channel, not a durable filesystem log or atomic snapshot. Notifications retain workspace, watch, and path scope; overflow can widen them.

## Workspace search outcomes

`grep` and `glob` select either one regular file or a directory's file corpus. Selection, physical containment, ignore rules, and resource limits belong to the workspace catalog. Selecting a file never searches its siblings. Glob patterns match paths relative to a selected directory, or the basename of a selected file; returned paths remain workspace-relative. Git selection treats path names literally, including glob metacharacters.

Filesystem tool manifests require an explicit absolute workspace and share one pinned directory authority with Scope. Tool paths are absolute or workspace-relative; home shorthand (`~` or `~/...`) is rejected rather than consulting ambient process state. Read stamps, mutation protection, edits, patches, and in-process formatting continue to address that directory after a rename or pathname replacement. Path-based search and external formatter configuration require the original named workspace; detached searches fail explicitly, and unavailable formatting or LSP diagnostics are reported with the successful mutation. Multi-file patches are not transactions: their failed results and mutation metadata retain Scope's acknowledged partial effects.

The concrete local search tools return explicit failed Tool outcomes for unsuccessful queries, including invalid patterns and missing paths, so Scope commits their feedback before model continuation. Cancellation remains execution control. This guarantee applies to these non-mutating searches; the generic Tool observer still preserves unknown external outcomes and host or publication failures.

Structured diff code rows always carry `code`, including `""` for a blank line; hunk rows omit it. The Go binding represents this presence with `DiffRow.Code *string`. Go consumers must migrate string construction and access; JSON consumers retain the existing required-string contract. Missing or null code remains invalid for code rows. No persisted-data migration is needed.

Workspace diff responses require `baseline`: `head` and `mergeBase` include the exact resolved commit; `emptyTree` identifies an unborn repository. The comparison uses that resolved object, even if a branch reference moves later. Worktree mode includes untracked files; base mode compares tracked working-tree contents against the merge base. Neither mode attributes all edits to an Agent or provides an atomic snapshot of concurrent filesystem edits. Update consumers together; no stored diff migration is needed.

## Workspace file rollback

File checkpoints require physically separate workspace and checkpoint-storage
trees, including through symlink aliases. Snapshot and restore reject either
tree containing the other with `checkpoint_unavailable`, before creating or
changing checkpoint state. With Runtime's standard `<DataDirectory>/checkpoints`
layout, the data directory itself and its ancestors cannot serve as checkpointed
workspaces: Runtime durability must not be archived as project material.

`sessions.rollback` with `files` or `both` restores a Run's checkpoint for that
Session and workspace. Checkpoints archive admitted regular files and symlinks,
respect ignore rules for untracked paths, and exclude files larger than 2 MiB.
They do not represent every file in the workspace. Before restoration, Runtime
archives the currently admitted state and checks target paths against everything
left unarchived, including ignored files, oversized files, and blocking
directories. A conflict preserves the current files and history and reports
`checkpoint_conflict` after the request's recovery intent has been cleared.
Move conflicting material out of the target paths before issuing a new request.

`both` restores files before committing the history cut. Once checkout begins,
a failure can leave partial filesystem effects, so Runtime retains its durable
intent for recovery. If cleanup of a refusal's intent fails, the response remains
an internal failure with a pending recovery intent; it does not report a
definitive `checkpoint_conflict` or `checkpoint_unavailable`. Re-driving an
existing intent also retains it on refusal, since an earlier attempt may have
already changed files. Consumers must keep
the original command identity while its result is uncertain. Runtime serializes
its own writers and uses Git's protection against overwriting untracked and
ignored blockers. External filesystem writers do not share Runtime's guard;
the operation is not an atomic filesystem or filesystem-plus-history transaction.

## Steer admission

`runs.steer` and `Runtime.SteerRun` return `userItemId`, the identity reserved for that input. Success proves admission to the addressed active Segment, not model consumption. Only the matching committed user Item proves application at a model boundary. Clients reconcile by identity, including when the Item arrives before its receipt; identical text or attachments do not identify a command. A rejected steer must not silently become a new Run.

Protocol `2026-09-22` replaces the empty steer acknowledgement. Upgrade Runtime, Desktop, and CLI together. Waiting execution snapshots containing pending steer inputs now require their reserved Item identities; older pending-steer snapshots are rejected, never guessed or replayed. Finish those executions before upgrading. Completed history is unchanged.

## Command completion and configuration boundaries

Updating or manually firing a missing schedule returns `schedule_not_found`, with `refetch` recovery, consistent with other missing resources. Deleting an absent schedule remains successful and does not publish a change. Invalid schedule parameters remain `invalid_params`; stale edits remain `revision_conflict`. Schedule create/update and hook trust changes declare `workspace_unavailable` when their selected directory cannot be resolved.

A successful start commits the Run, first Segment, and opening user Item. Resume commits the accepted interrupt responses and the new Segment opening. Neither promises a provider call has completed. Cancel is a settlement barrier: success proves a canceled Run; a natural completion that wins the race returns `run_finished`. Steer admission is described above. A missing receipt is an unknown command outcome, not permission to repeat it with a new identity.

Disabling or deleting a schedule stops future occurrence claims. An already claimed occurrence retains its accepted input and may still start after that change; its Run must be canceled separately when required. Editing a schedule does not rewrite an occurrence already claimed from an earlier revision.

Calendar cron expressions use UTC unless they declare `CRON_TZ`. Creation,
editing, and recurrence use that same rule regardless of the caller's local
timezone. An expression with no reachable occurrence is invalid.

Abrupt process-exit tests exercise claim, business commit, and receipt commit separately against a temporary SQLite database. A committed receipt replays the same identity without executing again. A claim without a receipt remains unresolved, including when the business effect committed: restart and elapsed time do not prove success or failure. Keep the original key and store namespace, inspect authoritative session state, and do not issue a fresh command to bypass the reservation. There is no general automatic reconciliation across command receipts and arbitrary business or external effects.

For an existing Session, omitting both provider and model uses that Session's stored selection. An explicit pair overrides it for the Run. Global defaults do not silently replace an existing Session's selection.

Process configuration decodes YAML, defaults, and environment overrides into one
typed snapshot. Invalid boolean and integer overrides reject startup, including
settings present only in the environment. List overrides use comma-separated
entries; the resolved configuration consumes the same values that were validated.

| Change | Effective boundary |
| --- | --- |
| Utility model | Next utility invocation, including during an existing Run; an in-flight invocation retains its resolved model |
| Embedding model | Subsequent semantic searches |
| MCP configuration | Persisted first; enabled servers connect in the background, and resource status reports readiness |
| Project hook trust | Next Run opening; waiting resume is not a new Run |
| Skill archive/restore | Subsequent resolution; existing conversation context is not erased |

An unset utility role uses the Runtime composition's default model selection. It does not follow each Run's explicit main-model override. Changing the utility role does not replace the main-model deployment of a live or waiting Run. Saving configuration is not a universal live-reload guarantee. Restart recovery validates the retained model and tool deployment; it does not silently reinterpret an existing execution using arbitrary current settings.

## Discovered Skill inspection

`skills.discovered.list` returns a complete bounded `skills` catalog and `diagnostics`, replacing the former page shape. `skills.discovered.get` / `GetDiscoveredSkill` reads one named document on demand and returns its selected scope, source path, instructions, and SHA-256 revision of the exact document bytes. Upgrade consumers together; there is no legacy page adapter.

Discovery, inspection, and model loading use the same Scope resolver. Project bundles own a colliding name even when malformed; they never expose the user copy as a fallback. Invalid or oversized selected documents appear as diagnostics alongside usable entries. Filesystem, confinement, capacity, and cancellation failures remain explicit query failures. Instructions retain the existing 1 MiB document bound and source directories retain their entry bounds. Inspection describes current authored content, not proof that it was loaded into an existing model conversation.

Managed catalogs, proposal review, approval, and idle curation load verified document bytes through the same Scope format and directory-name binding used by discovery and package admission. Idle curation leaves invalid documents in place. Archive and restore serialize lifecycle moves and usage changes under one library lease; a replay does not reset subsequent activity. Malformed usage metadata fails recording and curation without replacing history or moving an active Skill. Repair the retained metadata explicitly before retrying; Runtime does not reset it to a fresh grace floor.

Inspection distinguishes invalid names (`invalid_params`), absent skills (`skill_not_found`), and invalid or oversized selected documents (`skill_unavailable`). Archive and restore also report `skill_not_found` for absent entries. Consumers should refresh a missing resource and offer document correction for an unavailable resource; public errors exclude parser diagnostics and document contents. Proposal revision failures remain `revision_conflict` because the reviewed content must be fetched again before applying it.

## MCP OAuth credentials

MCP OAuth credentials belong to one persisted authorization grant per MCP source, bound to the fingerprint of the OAuth target that requested it. Starting an explicit sign-in replaces that grant; token refresh and credential rejection can update or remove only their own grant. A changed credential recipient (a user server's endpoint or headers; an installation server's endpoint declaration, or for stdio its release) yields a different target, so an earlier credential simply never matches it; there is no trigger or write-time deletion. Descriptive metadata and Tool policy changes preserve the target. Removing the source removes its credentials by cascade. A superseded callback fails explicitly and cannot overwrite or delete replacement credentials. Pre-release databases without the current grant and target bindings require a fresh data directory; saved credentials are not converted during opening.

## Integration probes

`models.list` treats endpoint discovery as authoritative, including an empty catalog. The endpoint adapter validates every advertised identity and collapses repeated identical model IDs; the public page contains each ID once in ascending order. Endpoint failures and malformed identities return a sanitized `provider_error`, rather than `invalid_params` or a successful fallback catalog. A duplicate returned by a custom model-lister implementation violates its unique-identity port contract; tests injecting such a result do not describe raw HTTP discovery. Local registry and catalog defects remain internal failures; cancellation preserves its context identity for Go callers. Bundled metadata may enrich a discovered model but cannot change its provider/model identity.

Provider and MCP probes return a closed `outcome` without server-authored prose. `providers.test` reports `reachable`, `notConfigured`, `invalidCredentials`, `timedOut` or `failed`; `mcp.servers.test` reports `reachable`, `authorizationRequired`, `timedOut` or `failed`. Unknown integration failures are `failed`. Caller cancellation remains a call error. Clients render each outcome locally and reject any other value as a Runtime contract violation; they never branch on raw integration error strings.

## Stored JSON integrity and snapshot cost

Stored JSON uses the standard library's single-pass strict decoder. Columns require exact field names, valid UTF-8, one complete value, and no duplicate or unknown members, including inside nested values. The persisted representation is unchanged; data written by Runtime needs no migration. Manually edited records with mismatched field casing or invalid UTF-8 are rejected as corrupt rather than normalized.

`GOWORK=off go test ./internal/adapter/persistence -run '^$' -bench BenchmarkSessionMaterialSnapshot -benchmem` measures coherent SQLite material reads and application validation at increasing history sizes. It excludes protocol encoding and client rendering; evaluate those separately before changing snapshot completeness or the subscription fence.

## Scope integration

Runtime uses static `ChildDeployments`, the released provider constructors and
transport, and Scope error categories. The Dispatcher owns each Effect's replay
and capability policy; Runtime's Segment gate forwards that complete policy.
An Interaction's final Output carries either a model response or direct Tool
results, without a second completion-source field. Workspace and waiting-tree
validation errors mark a
probe as nonresumable only when their cause is `ErrExecutorStateLost`. Storage,
cancellation, and other unexpected probe errors propagate to recovery instead of
being persisted as irreversible state loss. Malformed or incompatible checkpoints
and missing execution trees still explicitly report that they cannot be resumed.

## Scope provider transport

Runtime consumes the released Scope modules pinned in `go.mod`. Provider `Call` and `Stream` use Scope's canonical streaming transport; complete calls aggregate that same validated stream. Runtime does not retain a unary provider fallback. MCP sessions use `github.com/Tangerg/go-sdk`, the same SDK as Scope MCP, so structured results preserve large integers, decimal values, and explicit empty objects through transport and Tool publication.

An MCP dial owns its temporary connecting phase for exactly its admitted lifetime.
Completion, failed registry reads, supersession, and shutdown retire that phase;
the connection pool supplies terminal status. The operation advances its phase
before notification publication. Queued notifications carry only source identity
and cannot revive a retired attempt or obscure a newer connection.
Unmapped connection or authorization states fail their read or projection with
an internal error; they never fabricate a server failure or panic during delivery.

Provider-reported token usage, durable conversation history, and model/Tool/execution telemetry are Scope contracts rather than Runtime restatements: `chat.Usage` survives whole to the protocol, the message store implements `history.Store`, and Scope's OpenTelemetry middleware instruments the provider, Tool, and execution-tree boundaries.

MCP identity follows Scope's capability chain through Tool decorators for discovery, exposure, and approval queries. Invalid MCP identity declarations reject the catalog instead of silently hiding entries or grouping them as built-ins.

`mcp.tools.list` reads the resolver's live MCP snapshot, the one each Run manifest freezes, and derives tool name conflicts from that same snapshot. Connections publish their current catalog when the resolver subscribes, then hand every changed tool set to that same sink inside the critical section that settles the connection. There is no separate startup catalog seed; connected Tool counts and the listing describe one state. Remote changes become visible after reconnect admits the replacement; listing no longer queries a separate live catalog. Invalid JSON Schemas, including unresolved references and oversized documents, now fail connection or probe admission through Scope rather than failing the next Run. Diagnostic and MCP schema projections preserve exact numeric literals and retain their existing protocol shape.

## Complete AGENTS guidance

Run guidance includes the complete discovered AGENTS.md cascade in source order, with provenance markers. The rendered cascade, including its heading and separators, must fit the 32 KiB guidance budget. An over-budget cascade fails preparation with `prompt_source_too_large`; Runtime does not silently remove ancestor instructions to retain only the most specific files. Shorten the authored documents before retrying. Discovery describes the current documents; it does not prove that a previous Run loaded them.

## Tool authority

Tool identity is a closed built-in, MCP source/tool, or A2A endpoint reference.
Model names remain presentation labels. The closed set of built-in names is owned by the
tool domain (`tool.BuiltInName`); the built-in behavior catalog is keyed by that type and a
test holds it to exactly that set, so startup performs no comparison between them.
Runtime excludes remote name collisions
before model discovery and reserves every built-in name, including unavailable tools.
Cross-server collisions never reject a connection: all competing remote identities
are excluded symmetrically. `mcp.tools.list` reports their model names and conflicting
source references, which CLI and Desktop display alongside the connected catalog.
Scope rejects a whole MCP source when names collide within that source,
before returning any executables. Preserving its unrelated tools requires an
upstream discovery API that can exclude collisions; Flame does not replace
Scope discovery or assign temporary names to work around it.
Standing allow and deny decisions live only in approval rules and retain the
source authority fingerprint. Endpoint changes make existing rules stale;
credential rotation does not. MCP exposure is configured separately through
`mcp.tools.setExposure`; `approval.setRule` owns remembered decisions.
A rule is keyed by scope, scope key, source reference, subject type, and subject value. A
source reference is a closed union: built-in name, A2A endpoint, or MCP server identity
plus remote tool name; each variant's data is reachable only through that variant. Remembering the
same key replaces both its decision and source fingerprint. Equally specific
distinct patterns still resolve to deny when they conflict.

Every pending Tool approval retains its source reference and authority fingerprint,
including one-off decisions that cannot be remembered. Restoration validates this
identity together with the call ID, model-visible name, and effective arguments;
missing identity is rejected rather than reconstructed from a label.

Remembering an approval derives an exact subject from the confirmed command or path,
including literal `*`, `?`, brackets, and backslashes. Tools without a finer subject
use an explicit `all` matcher. `approval.setRule` accepts a required typed subject:
`{type: "all"}`, `{type: "exact", value: "..."}`, or `{type: "glob", value: "..."}`.
Only an explicitly authored `glob` uses Go `path.Match` (`*` does not cross `/`).
Exact subjects outrank globs within the same scope. An
edited command or path cannot grant permission to the original command or path.
The frozen Scope input contract validates approved arguments before a rule is
saved; invalid edits produce a failed Tool result without saving a grant. Denials
remain bound to the original invocation. Standing rules never replay argument edits.
If the source changes while approval is pending, the explicit one-shot answer still
applies to that frozen invocation, but no rule is saved for the obsolete source.
Other persistence failures remain errors.

This contract intentionally breaks the former approval schema and protocol.
Use a fresh Runtime data directory when upgrading from an approval schema
without source identity or explicit subject types. Startup rejects those schemas
without modifying their data. There is no
legacy rule conversion. Update Runtime and every client together.

## Cache prefix and the request tail

A model request is ordered so that everything a provider may have cached stays byte-stable:
tool declarations, then the single System message of stable instructions (base prompt,
pinned memory, agent documents), then the conversation. Nothing that changes during a
Session precedes the conversation. Memory recalled for a prompt is a User message framed
as `<flame-context kind="recalled-memory">` that follows that prompt, so a new recall
never rewrites an earlier message; it belongs to the Root Run's opening context and is not
inherited by delegated layers. The current Goal and Plan are not part of the context Scope
adopts: the context reducer reads them from their owners for every call and composes them,
with the deferred catalog below, into that call's tail (`session-goal`, `session-plan`,
`deferred-tools`). The reducer measures that tail in the compaction budget and hands the
same messages to the model boundary for exactly that invocation, which appends them after
the conversation. A Goal or Plan change therefore alters only the tail of the next call.
Waiting checkpoints written by an earlier build carry the former context shape and are
not resumed, under the existing build-identity rule.

## Deferred tool catalog

A Run's tool declarations open the provider prompt-cache prefix, so they do not depend on
which tools are deferred. Built-in, Skill, MCP (user and installation origin) and A2A
tools withheld from the initial manifest form the Run's frozen deferred set, which the
Run's `search_tools` owns. Its declaration is static text; plugin activation, MCP
reconnects and catalog changes reach later Runs only through that set.

Each model request of a layer with deferred tools ends with one Runtime-owned User
message framed as `<flame-context kind="deferred-tools">`, the single framing Runtime
uses for the context it authors (recalled memory and retained shells use the same frame
with their own kind). It lists the deferred model names, the exact
names `select:` accepts, grouped by source in source-then-name order, without
descriptions or schemas. It is a User message because providers hoist System messages
into the instruction block ahead of the conversation. The message is a per-request
projection appended at the model boundary after Scope fixes the context it adopts, so it
never enters the Interaction context, checkpoints, durable conversation history,
compaction input or the transcript. Compaction budgets count it. Restoring a waiting Run
rebuilds the same deferred set, which the deployment configuration digest already binds,
so the catalog is the same projection after restore or compaction. A layer with no
deferred tools has neither `search_tools` nor the catalog message.
