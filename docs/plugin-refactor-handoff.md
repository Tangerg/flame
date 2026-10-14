# Unified plugin refactor handoff

Completed on 2026-10-15 within the user's authorized implementation and
verification scope. The detailed slice history, 48 individual acceptance rows,
review reconciliation and failure records remain in
[plugin-refactor-progress.md](plugin-refactor-progress.md). Current repository
state is authoritative.

## Workspace and preservation

- Worktree: `/Users/tangerg/.codex/worktrees/unified-plugins/flame`.
- Branch: `codex/unified-plugins`; last integrated main: `7ba40c117`,
  released Scope v0.50.0. No push or merge into main was performed.
- The primary checkout `/Users/tangerg/Desktop/flame` was not edited.
- Preserve the original stash: `WIP on main: c443a1a8 refactor(runtime): finish
  the json/v2 migration across delivery and tooling`. No automatic/bare stash.
- No production/user-data initialization, copying, conversion or deletion.
- No frontend UI/E2E/screenshot execution or additions; no Desktop aggregate
  `npm run check`. The existing E2E file only migrates its import to the public
  feature client, with test logic/assertions unchanged.
- Replies remain Chinese; code/comments/errors/docs remain English. Existing
  root/module instructions continue to apply. No sub-agent delegation occurred.

## Final implementation

The original unavailable-source bootstrap failure is fixed in `8e32d9c35`.
Captured receipt observation has its actual observer lifetime; source availability
is explicit eligibility rather than a blocking `WaitSession`. Core Run opening
reconciliation continues when the original plugin is disabled/unavailable, while
private acknowledgement remains original-owner evidence without replay.

The remaining query streams, exact Go/TS feature JSON, transitive release
retention, immutable activation identity, installed point/analyzer lifecycle,
public storage/receipt/model/idempotency authoring and complete independent Usage
artifact/lifecycle qualification are implemented. Existing pending commands and
waiting executions retain original target/origin/parent/route and exact transitive
dependencies. No compatibility reader, dual business writer or provider replay.

Final product commits:

- `6b3a1d823`: generic public original-owner Tool receipts; private Schedule
  reconstruction removed, unrelated Tool/failure recovery verified.
- `9f2bac554`: Core Memory published read projections and public model port;
  generated Go consumer values are defined types, not forwarding aliases.
- `2079ee06f`, `0969e16b8`: public Tool context/text embeddings, ordinary installed
  embedding-role callbacks and distinct independent child identity admission.
- `6d3728a0c`: one Go-owned published Memory caller policy, actual grant factory
  in the Core caller, no private feature in the public Runtime build closure,
  and RED/GREEN query/embedding parent-origin correction.
- `9f3de47f0`: transitive failure paths retain their consumer context; final
  transformed Tool arguments are validated before approval/execution.
- `f8d2d9bd8`: explicit deferred read snapshots versus immediate write-sets.
  Session/Goal/export/installation/checkpoint/source reads no longer reserve a
  competing writer. Existing snapshot assertions and transaction settlement
  guarantees remain intact.
- `33696b3c2`: actual CLI composition inspection, ordering and resolution.
- `b577d3b15`, `03fb6eeb6`: active author/docs cleanup and public feature-owned
  Memory client, with Workspace consuming its own adapted application ports.

Backend owners keep Memory items/review/ledger/watermark/curation/receipts and
Schedule definitions/cursors/captured pending work/receipts. Runtime keeps accepted
Runs, execution, accounting source evidence and its original acceptance receipts.
The shared callback layer has no Memory domain or universal Memory work limit.

## Final verification

- Complete normal Runtime/localruntime/CLI/Reviews: all 174 packages pass.
  Complete bootstrap 94.535s; execution 68.930s. All original assertions and
  eight Schedule SIGKILL cuts remain present and pass.
- Full-module vet and exact Core/feature/API regeneration pass.
- Full parallel race passes all Runtime/localruntime/Reviews and 173 total package
  results, including execution 318.622s, bootstrap 399.488s and Core SQLite 34.489s.
  Its aggregate exits 1: one CLI delegated-approval recovery exceeds its unchanged
  10s deadline. This invocation is explicitly not a pass or a pre-existing failure.
- The unchanged whole failed CLI package passes race independently (67.710s).
  Three exact-case repetitions pass (39.668s). After temporary diagnostics are
  removed, complete serial CLI race passes all 33 packages, including runtimebinding
  67.414s and terminal 56.501s. No source, assertion, timeout or test selection was
  changed to obtain those passes. Future CLI race qualification uses `-p 1` to
  isolate its real Runtime fixtures; do not silently erase a failed parallel run.
- Shared contract checks: 30 files/355 tests plus types/lint/format/knip pass.
  SDK/Reviews/portable/Desktop typechecks and portable exact resources pass.
- Static dependency/layer/public-boundary/context/port/API checks pass; 95/95 API
  fact families have real consumers. No coverage exemption is added.
- Current native Wails production `.app`, strict/deep ad-hoc signature, arm64 and
  macOS 13.0 floor pass, without launch. Runtime `make build` includes all ten
  ordinary packages and current backend executables; publisher policy bytes match.
- Current packed Usage author kit builds outside the checkout. A second offline
  clean reinstall/typecheck/build reproduces every resource byte. Offline external
  Go authors compile public SDK/storage/receipts and a Runtime graph without private
  feature imports. No published registry/module release is claimed.

The first full acceptance run found genuine snapshot `SQLITE_BUSY` failures;
`f8d2d9bd8` repairs their read/write boundary. A later clean full run passes all
174 packages. The failed/terminated earlier run and failed parallel race are
preserved in the progress history, never counted as passes. No test was skipped,
weakened or given a larger case deadline to manufacture a pass.

## Scope and data boundaries

Current Core is `2026-10-15.2`, backend protocol is
`io.github.tangerg.flame/backend/2026-10-14.2`, and trajectory export schema is 3.
Rebuild affected consumers and backend/component artifacts together. Replaced
host/checkpoint/private receipt shapes are refused unchanged; preserve old data
directories and finish waiting work with its original binary. Unchanged private
formats stay with their exact owner and explicit data binding. No automatic
compatibility, migration, private copying or historical JS execution is introduced.

Run-linked evidence follows existing Run retention; Session-only endpoints follow
atomic Session deletion; unattached evidence is retained. Pending/unknown routes
never expire into replay permission. No new TTL or universal Goal ceiling is
invented.

Rendered UI/Hooks/provider identity, remote graphical Web/Wails workflows and
window-reload interaction are excluded from executed acceptance by user instruction.
Source, types and native builds are not those proofs. The earlier interrupted
prohibited UI run is not claimed resolved. Registry publication, external deployment,
pushing/merging and user-data migration require a separately authorized scope.

## Read-only references and next action

- `/Users/tangerg/Desktop/docs/Flame-Unified-Plugin-Proposal.html`.
- `/Users/tangerg/Desktop/docs/Flame-Unified-Plugin-Proposal-Review-2026-10-13.md`.

Both original inputs are unchanged. No required implementation or authorized
verification remains for this goal. Keep the reviewable Runtime and native build
outputs, clean only owned scratch resources, confirm Git/stash state, and stop.
