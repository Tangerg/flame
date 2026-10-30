# Computer use

`flame.computer` is a managed TypeScript backend and an ordinary workspace component.
It directly loads the original CUA engine from a selected official ChatGPT
application on the **Runtime host**. It provides the complete public `js`
and `js_reset` surfaces advertised by that connection, including desktop and
browser APIs, screenshots and other original MCP content. The existing Flame agent
chooses actions; the plugin does not run another agent loop.

The integration references [LCU](https://github.com/amontlabs/lcu) commit
`8389f8ff98a5d828a6b9102e92ace0de458bc862`. Flame owns application selection,
dependency validation, process launch, trusted-service integration, permission
delivery and native turn cleanup. It does not execute, import, install or configure
LCU, and does not read LCU installation records. The original CUA JavaScript,
native input, accessibility, capture and browser engines remain an explicit
external dependency supplied by the selected ChatGPT application. Their source,
binaries and instructions are not redistributed in this package.

## Installation and first use

Build and install this package through the ordinary plugin distribution path.
Installation publishes the component and contracts without launching the engine or
controlling a computer. An unconfigured installation remains inspectable.

Install and initialize the official ChatGPT application for the Runtime host's
operating-system account. Its normal engine account-state directory must exist;
the plugin does not create or replace that account's authentication state. Select the actual
application directory, such as `/Applications/ChatGPT.app` on macOS or
`/usr/lib/chatgpt` on Linux. Configure these installation inputs in Flame:

| Input                 | Meaning                                                                                                               |
| --------------------- | --------------------------------------------------------------------------------------------------------------------- |
| `cua-application`     | Absolute directory of the selected official application. Flame derives its original executable and entry.             |
| `browser-enabled`     | `true` or `false`; default `false`. Requires the official Chrome extension and original native connector.             |
| `audio-enabled`       | `true` or `false`; default `false`. Enables the original engine's optional computer-audio API.                        |
| `desktop-environment` | Secret JSON object of selected display, D-Bus, XDG and locale variables; arbitrary environment overrides are refused. |

The backend receives these as `FLAME_COMPUTER_APPLICATION`,
`FLAME_COMPUTER_BROWSER`, `FLAME_COMPUTER_AUDIO` and
`FLAME_COMPUTER_DESKTOP_ENVIRONMENT`. There is no configurable external command
or argument vector. Model tool arguments cannot select an installation, inject
environment variables, weaken the sandbox or redirect a connection.

One bootstrap owner constructs the complete child environment from the selected
application, account and permitted desktop variables. It does not inherit arbitrary
parent credentials, `NODE_OPTIONS`, engine policy or service overrides. Original
engine-owned authentication and native permissions use the selected account's
normal home; Flame neither copies credentials nor grants permissions. Secret
configuration and raw child stderr are not shown in the console or logged by the
transport. Authored prerequisite failures are actionable; child-process diagnostics
retain only bounded permission, sandbox and other failure categories.

1. Open the **Computer use** workspace page. Check the displayed Runtime hostname,
   operating system and architecture.
2. Keep the operator console outside the controlled desktop, account and browser
   surface. Use a separate operator device or an isolated target desktop.
3. Open a time-limited control lease. Only a captured human invocation can open,
   close, stop or answer permissions. A second open cannot replace a live lease.
4. The model reads `computer_context`, including the original engine instructions
   and argument schemas, before using `computer_js` or `computer_reset`.
5. Answer actual application/site/form/URL permission requests in the console.
   Only persistence scopes offered by that request are available. Closing or
   expiring the session cancels outstanding requests.
6. Stop the current operation or close the lease when finished. Closing the
   workspace page only stops that page's observations; it does not close an
   independently authorized lease.

The human console also supports reviewed JavaScript execution, explicit worker
reset, paginated original history, raw input/outcome inspection, image/audio
presentation and recovery of original uncertain Runtime commands. Stop and close
remain available while an unrelated action is running or awaiting a reply.

Manual JSON validation identifies and focuses the arguments field. Returning from
review preserves the draft; waiting or unconfirmed operations keep it selectable
and read-only. Review and permission forms use the shared form focus boundary:
retirement returns to the surviving editor, or status controls when no editor is
available, only while that form still owns focus. Pending permission actions stay
focusable without admitting another response. Captured targets, original command
identity and permission authority remain with their existing owners.

Captured instructions, schemas, arguments and original evidence use the shared
literal-source reader. Its header names the source; Ctrl/Cmd+A stays within that
reader, wrapping follows the shared page preference, and Copy receives the exact
original string. Long content scrolls beneath its actions. A fitting reader and
caption enter the Panel viewport together, while raw content stays unexecuted.

If a recorded image cannot be decoded, its captured identity and readable preview
failure replace the broken image. Original bytes remain in the recorded outcome;
this display failure does not change the operation's recorded status or affect
other images in the result.

Embedded image and audio resources keep their captured URI beside the preview as
selectable source identity. The URI does not replace their captured bytes or become
a live navigation action. Text resources retain their named source caption, using
the resource label when the URI is missing or blank. A fitting media group keeps
its URI with focused controls inside the Panel viewport.

Resource links use the first nonblank captured title, name or URI without changing
the original value. Shared prose styling makes a safe HTTP(S) destination visibly
actionable. A distinct URI stays selectable beside its label; a URI already used
as the label is shown once. Other URI schemes stay readable without acquiring a
navigation action. The original outcome and recorded status remain authoritative.

History expands the selected original record in place. Its identity, recorded
status and refresh action remain with the detail; the same disclosure returns to
the surrounding records. Rereading keeps existing evidence and focus, marks
retained evidence when a read fails, and retries only the original query.
Collapsing retires that read without changing the operation. Later responses do
not reopen it or steal focus. Paging preserves the last selection and the manual
editor draft; reading history never repeats historical actions.

## Supported deployment boundary

The direct adapter supports macOS on Apple Silicon and Linux ARM64/x86-64 with
an active X11 desktop, D-Bus, compatible glibc and working native confinement.
Windows, Intel macOS and native Wayland are refused by this adapter. This plugin
reports the host and preserves actual engine refusals. It
does not invent a working desktop, bypass OS permissions or silently change
drivers. Browser control and computer-audio APIs are available only when the
installed engine advertises them and their prerequisites have been configured.

The operator-surface separation is a deployment requirement. A `human` RPC origin
identifies the API caller; it cannot prove physical human presence when an agent
can click the same UI. The plugin refuses a new native permission for
`com.tangerg.flame`, but that defense does not revoke pre-existing grants, isolate
a browser tab, or add per-application approval to Linux. A controlled desktop
must remain separate from the human authorization surface.

Application selection checks the actual platform, architecture, manifest, original
tool declarations and executable layout. macOS additionally verifies the original
application and helper signatures and OpenAI signing identities. The captured
file generation brackets metadata/signature validation and is rechecked around
each operation. An application update retires the connection; it never substitutes
a new engine or replays an old script. Linux application provenance remains the
responsibility of the official installer; structural validation is not a vendor
signature claim.

JavaScript uses one managed minimal filesystem profile with read-only application
resources and launch guards, plus one private writable working directory. Host-home
access and network access are not granted to the kernel. On both supported
platforms, a real native confinement probe must hide the private control file,
deny control writes and permit workdir writes before the engine starts.

Every kernel launch also enters a protected Node guard that verifies actual
read/write denial at the retained private boundary before executing the original
kernel. This closes the original runtime's direct-launch fallback if a later
sandbox availability probe fails. The guard admits only the original kernel or
trusted-worker argument shape; a failed check never executes generated JavaScript.
On Linux, a separate launcher forwards the kernel to original Codex confinement
and starts only the positively identified original Sky/browser trusted worker
outside it. Unknown profiles, parent executables, scripts and service maps fail.
The original desktop service continues to own desktop behavior; no toolkit-specific
input translation is supplied.

Browser opt-in validates the original extension metadata and native registration.
It accepts the selected original host or an intact official connector copy bound
to that same application. It rejects relays and stale registrations without rewriting
profiles. Configure the official browser integration through its normal application
workflow before enabling this option. Original connection, account authentication,
agent-header capability and site approvals remain authoritative. A valid registration
does not claim that the extension is currently enabled or connected.

## Fact ownership

| Fact                                                                      | Owner                                                   |
| ------------------------------------------------------------------------- | ------------------------------------------------------- |
| Flame Session, Run, Tool approval and model loop                          | Existing Runtime and Scope                              |
| Original calling identity and uncertain command route                     | Runtime client journal and Runtime command admission    |
| Computer-control session identity, state and exclusive operation          | Behavior-rich `Session`                                 |
| Lease clock, resource acquisition, dispatch and revocation                | Application owner `Control`                             |
| Original command admission, exact input, one observed result              | `Receipts`, in the plugin's private SQLite database     |
| Selected application identity and captured dependency generation          | `Installation`                                          |
| Direct-engine composition and private execution directories               | `Cua`                                                   |
| External process group and close/join                                     | `EngineProcess`                                         |
| Durable spawn admission and retained ownership marker                     | `EngineOwnership`                                       |
| Original MCP connection, captured instructions, schemas and per-call turn | `Engine`                                                |
| Native turn admission and acknowledged cleanup                            | `NativeLifecycle`, persisted through `EngineOwnership`  |
| Kernel filesystem/network policy                                          | `sandboxState`; original native confinement enforces it |
| Per-lease confinement preparation and protected launch selection          | `prepareSandbox`, with one shared configuration decoder |
| Pending permission resolver                                               | The current call's permission bridge in `Control`       |
| Allowed application/site permissions and JavaScript heap                  | Installed original CUA runtime                          |
| Presentation, selections and unsubmitted form drafts                      | Workspace component                                     |

The component reads full bounded snapshots and queries original receipt records.
It cannot synthesize a permission, clear a backend failure or advance a command
outcome by changing local state. Instructions, schemas and contract digests are
captured from the same live connection; a tool-list change retires it instead of
rebinding old calls to a new surface.

`contracts/capability.ts` is the source of the capability descriptor, schemas and
local structural types. The ordinary contract generator produces installed
resources and component methods. There is no hand-maintained parallel descriptor.
`contracts/methods.ts` owns the pure method identity map shared by the descriptor,
backend and browser consumers. The component does not maintain another alias map
or import the Node descriptor implementation.

## Model tools and typed operations

The MCP server publishes raw tools `context`, `js` and `reset`; the standard
Runtime namespace presents them as `computer_context`, `computer_js`, and
`computer_reset`. Host-only upstream tools, including `turn_ended` and
`js_add_node_module_dir`, are never published to the model.

`computer_context` returns the current control session and worker identities,
captured surface digest, lease end, exact original instructions and `js` /
`js_reset` schemas. `computer_js` and `computer_reset` require those captured
identities and `argumentsJSON`, containing one original upstream argument object.
That object is validated against the actual captured tool schema. Numeric values
that cannot be represented by the external JavaScript MCP transport are refused
before dispatch; original admitted request evidence retains its exact JSON.

The public `flame.computer.v1` capability has these operations:

| Operation    | Kind         | Behavior                                                      |
| ------------ | ------------ | ------------------------------------------------------------- |
| `read.v1`    | query        | Current host, configuration readiness and control session     |
| `watch.v1`   | query stream | Full current snapshots, with consumer backpressure            |
| `context.v1` | query        | Current captured upstream instructions and tool schemas       |
| `open.v1`    | command      | Human-only lease acquisition and engine startup               |
| `close.v1`   | command      | Human-only exact-session revocation and close/join            |
| `stop.v1`    | command      | Human-only exact-operation interruption and engine retirement |
| `execute.v1` | command      | The same execution owner used by model tools                  |
| `reply.v1`   | command      | Human-only answer to the exact live permission request        |
| `history.v1` | query        | Bounded pages of original admission headers                   |
| `result.v1`  | query        | Original request, invocation and observed outcome             |

Method names use the `flame.computer/` prefix. Full wire schemas are in the
generated [API reference](package/io.github.tangerg.flame/contracts/API_REFERENCE.md).
Standard command and Tool receipt methods project the same original record.

## Lifecycle and failure semantics

A control lease belongs to the plugin and is not a Flame Session or Run. Each
admitted execution obtains a distinct original upstream control turn, with
plugin-generated session/turn identities and the captured original call ID.
Normal completion sends `turn_ended` with `Stop`; cancellation or an unobserved
failure sends `Interrupt`. Cleanup runs before the operation returns. These
events describe the actual plugin operation, not an inferred Run end.

On macOS the packaged Sky adapter forwards original desktop requests unchanged
and registers exact native turn admission before dispatch. Ending a turn blocks
new requests and joins every already-forwarded native request before its original
native client acknowledges the turn-ended IPC; the parent separately joins the
signed helper command. The MCP hook returning, a helper exit code of zero, or disappearance
of the child process cannot substitute for that native acknowledgement. Native
cleanup failure retains the ownership marker and prevents a replacement engine.

The connection's JavaScript heap can span successful operations. An explicit
reset runs the original `js_reset` operation after prior turn cleanup and changes
the plugin's accepted worker identity. Old captured inputs are rejected. Engine
failure, lease expiry and explicit stop retire the connection. There is no
automatic reconnect, kernel reconstruction or script replay. The original
runtime may report its own heap loss; its returned instructions and errors remain
visible rather than being rewritten into permission to retry a mutation.

The plugin admits one operation per controlled connection. Competing calls are
refused before driver dispatch; cancelling a refused or separate request cannot
cancel another operation's permission. Execution deadlines pause while a real
permission request is outstanding, but the human-authorized lease does not.

Permission requests retain their original message, schema and metadata. The
strict supported flat form vocabulary includes strings, numeric fields,
booleans, single/multiple selections and standard formats. URL-mode permissions
require an explicit user action. Unknown constraints, fabricated persistence,
stale request IDs, changed sessions and model/automatic replies are refused.
Pending promises are short-lived; they are cancelled when their call, lease or
backend ends and cannot be resurrected after a restart.

Before any external effect, SQLite commits the original command ID, method,
canonical input/digest and invocation. It reserves space for the bounded result.
A valid observed result is recorded once. Same-ID calls only observe that record.
Transport loss, process failure or cancellation after possible dispatch leaves
the outcome unconfirmed. Missing receipts never authorize re-execution.

An engine `isError` result is still a known original result. If an action result
arrives but subsequent turn cleanup fails, the result remains known while the
connection retires. A screenshot, current window state or later successful call
cannot settle an earlier unknown action.

Raw MCP content is retained once, including text, images, audio, embedded
resources, resource links, structured content and metadata. Tool recovery derives
Scope `ToolOutput` from that same record; a public Go/Scope oracle verifies its
projection against the pinned framework. Media is not replaced with base64 text.
The selected model/provider must support the actual resulting tool media; this
plugin does not bypass Runtime's final model-input capability checks.

## Process recovery and storage

`PLUGIN_DATA/computer.db` contains original commands and their outcomes. One
exclusive SQLite owner prevents independent backend processes from advancing
those records. No automatic retention expiry, deletion or replay is implemented.
Resource budgets and lease bounds are defined once in `contracts/limits.ts`.
Admission, component validation and localized lease feedback consume those bounds.
The input budget
covers the complete original evidence, including its captured invocation. Ordinary
admission always leaves one complete input/result slot for human revocation of
the current lease. Only an exact current-session close or current-operation stop
can consume that reserve; stale targets and model callers cannot. Reaching a
limit refuses other new admission while retained receipts remain queryable.
Unknown command evidence is never discarded to make room.

`PLUGIN_DATA/engine-ownership.json` is a durable process ownership marker,
created before spawning the external engine. On POSIX the engine owns a distinct
process group. Normal close sends EOF, then bounded TERM/KILL if needed, joins the
original child and checks that the group is gone before removing its marker.
An exit-probe permission denial retains uncertainty within the original bounded
wait. Only definitive absence permits release; a persistent denial retains its
diagnostic and ownership marker, and other probe failures remain explicit.
The short `.lock` file serializes marker admission and replacement.

After an abrupt backend death, a replacement refuses to start while a recorded
old process group is present. It can reclaim a committed marker only after a
read-only liveness check proves the group absent. It never signals a historical
PID, which might have been reused. An uncommitted, malformed or otherwise
unverifiable marker, or an interrupted ownership lock, requires operator
reconciliation: independently stop the old engine and confirm that input has
ceased before removing the retained ownership marker/lock. Do not delete the
SQLite database or unresolved command evidence to work around this barrier.

The marker contains the original process identity and its optional unresolved
native turn. A marker from an earlier integration that lacks this evidence is
refused; there is no compatibility reader. Retain it until the original engine and
native input have been independently reconciled. Successful close removes private
execution directories only after engine/native cleanup completes. Original receipt
content remains durable; external resource links retain the original engine's
lifetime and are not a second artifact store.

## Build and verification

From the repository root, use the normal workspace prerequisites, then:

```sh
npm run generate --workspace @flame/plugins
npm run check --workspace @flame/plugins
npm run browser:computer --workspace @flame/web
node plugins/scripts/build-packages.mjs --plugin computer --output /absolute/new/packages
node plugins/scripts/build-author-kit.mjs --plugin computer --output /absolute/new/author --offline
```

The independent author project uses its own packed public SDK dependencies and
the same contract and portable-package builders. It does not import Runtime
implementation or depend on a source checkout at activation.

Tests cover actual stdio MCP processes, original metadata and media, permission
cancellation, lease expiration, interrupted ownership, backend death with a
surviving engine, reset/close ordering, non-replaying receipt recovery, strict
permission schemas, real Runtime-client journal behavior, and the generated
component in Chromium. Fixtures never control an actual user's desktop. Real
CUA application, OS permission and native desktop qualification must run
on the intended supported host; passing a protocol fixture does not claim those
native tests passed.
