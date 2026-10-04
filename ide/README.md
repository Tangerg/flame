# Flame IDE

This VS Code extension attaches to an existing Flame Runtime. It uses the same
Runtime-owned TypeScript client as Desktop and Web. Runtime owns Sessions, Runs,
interrupts, execution and workspace writes; the extension owns editor context,
selection, native prompts and read-only presentation.

## Build and install

Use Node 22.12 or newer. From the repository root:

```sh
npm ci --prefix runtime/contract/typescript
npm ci --prefix ide
npm run check --prefix runtime/contract/typescript
npm run package --prefix ide
```

Install the resulting `ide/flame-ide-0.1.0.vsix` with **Extensions: Install from
VSIX**. The extension runs in the workspace extension host, including a remote
VS Code workspace. An endpoint such as `127.0.0.1` therefore refers to that
extension host. It does not refer to a different machine displaying VS Code.

Start Runtime separately, then run **Flame: Connect to Runtime**. The extension
never discovers a process by port ownership, starts an embedded Runtime, kills a
shared Runtime, or falls back to a different execution target. Connection checks
the exact protocol version and Runtime instance across discovery. A mismatch is
an explicit connection failure.

Enter the Runtime address and its local token. The token lives in VS Code
SecretStorage under the exact normalized address. HTTP(S) addresses reject URL
credentials, query strings and fragments. The shared HTTP client refuses
redirects. No token is stored in settings, URLs, prompts, or command records.

## Native workflow

Select a Session from **Flame Sessions** in the Explorer or use **Flame: Create
Session**. Workspace choices and entered workspace paths are resolved on the
Runtime host. The editor's current directory is never substituted for a Runtime
workspace.

- **Send Prompt** starts a Run in the selected Session using Runtime's configured
  provider and model.
- **Send Editor Selection** captures the complete source document, URI, document
  version, language, dirty flag and zero-based UTF-16 selection range before
  opening the prompt. The input is a source snapshot, not a filesystem authority
  or an instruction for Runtime to overwrite an editor buffer.
- **Respond to Waiting Run** presents exact pending approvals and questions.
  Approval arguments are visible before the native confirmation. Runtime claims
  the wait, so another client resolving it first produces a typed refusal.
- **Cancel Run** explicitly requests cancellation of the chosen Runtime Run.
  Disconnecting, closing the window, or replacing a Session only closes local
  observation; it never implies Run cancellation.
- **Review Runtime Changes** opens Runtime's unified patch in a read-only native
  document. **Open Runtime File** reads through the selected Runtime workspace.
  Truncated previews are identified explicitly.
- **Compare Submitted Buffer with Editor** opens a native side-by-side diff of
  the exact submitted source snapshot and the current editor version. Both panes
  are immutable snapshots. The extension never applies WorkspaceEdit or writes
  Runtime changes into dirty editor buffers.

The output channel projects durable Items. Token previews are not negotiated.
An active Run is attached with the atomic `runs.subscribe(snapshot: true)`
snapshot and its successor event tail. Session changes and pending waits refresh
through the Runtime subscription. A Segment change during attachment causes a new
authoritative read. Waiting and finished Runs refresh their durable snapshot before
observation ends. Selecting another Session retires the previous observation before
the replacement is displayed. If observation fails, **Refresh Session**
reattaches from authoritative state; **Connect to Runtime** replaces a failed
connection and restores global notifications.

## Unresolved commands

Before dispatch, the shared SDK freezes and validates each mutation's exact
parameters and saves them with its durable replay identity in one immutable
record. The extension supplies atomic file storage under VS Code's private
extension directory. The SDK alone creates the idempotency key, records its
original age and namespace, authorizes retries, and removes settled records.
This covers Session creation, Run start, wait response and cancellation.

A lost acknowledgement retains that record. After reconnecting, **Retry
Unresolved Command** lists saved commands and replays the one explicitly chosen.
It never rereads an editor buffer, substitutes a new workspace path or invents a
new key. Independent extension hosts publish different records atomically;
settlement removes only the addressed UUID, and two clients recovering the same
UUID rely on Runtime's durable idempotency admission.

Retry refuses a different Runtime store, a missing identity, or an expired
retention window. It preserves the original creation time across reconnects and
honors the shorter of the original deadline and the Runtime's current retention
measured from that creation time. A refused retry retains the saved command for
inspection. Read-only Session inspection remains available. The client does not
infer execution failure from lost transport, restart a lost Run, or silently
resend an expired command as new work. Saved source text stays local to extension
storage until definitive settlement removes its record.

The former IDE-owned command records are rejected. Resolve outstanding commands
with the version that created them before upgrading. The new SDK does not infer
their original creation time, replace their identity, or migrate them into a
fresh replay window.

## Development boundary

`src/connection.ts` owns construction, negotiation, prepared command dispatch and
local shutdown. `src/commandStore.ts` implements opaque, atomic file storage for
the shared SDK's prepared mutation journal.
`src/observation.ts` owns snapshot-before-tail observation, without a duplicate
Run state machine. `src/extension.ts` translates VS Code UI and document APIs.
Transport, generated wire validation, mutation replay and stream admission live
in `runtime/contract/typescript/client`; do not introduce an IDE protocol fork.

`npm run check` typechecks, lints, formats, tests and bundles the extension.
Offline tests exercise real localhost HTTP negotiation/replay, multiple command
record owners, saved source provenance and observation retirement. Packaging
validates the VSIX manifest and bundled artifact. Interactive VS Code behavior
requires opening the extension in an Extension Development Host.

## Runtime plugins

Configuration JSON requires the inspected release's `digest` alongside `installationId`.
Stale requests are refused by Runtime; replay preserves their original release binding.

The command palette provides **Inspect Runtime Plugins** and **Manage Runtime Plugin**.
Management uses exact Runtime request JSON and the existing
prepared-command journal. Source paths belong to the connected Runtime. Results open as
JSON documents. Plugin action invocation and HTML hosting are unavailable in this slice;
installed tools use the existing Runtime execution and approval path. See
[the Runtime contract](../runtime/README.md#portable-plugins).
