# Unified plugin refactor handoff

## Current acceptance

The original P01-P48 audit is **35 Evidenced / 13 Partial / 0 Open**, not an
implementation percentage or a 100% claim. P13 public installed Tool proposals,
P12/P16/P18/P41 real-process proof and P48 source/API/doc cleanup are complete.
Thirteen graphical rows and all four added review cross-scenarios remain Partial.
The historical post-SDK **10s safe-steer timeout has no established cause**.
Passing repetitions and the current full parallel gate do not close that item.
Detailed evidence and each row live in
[plugin-refactor-progress.md](plugin-refactor-progress.md).

## Workspace and preservation

- Reuse `/Users/tangerg/.codex/worktrees/unified-plugins/flame`, branch
  `codex/unified-plugins`; integrated main remains `7ba40c117` (Scope v0.50.0).
- Do not edit the primary checkout `/Users/tangerg/Desktop/flame`, recreate from
  main, push, merge main or mutate stash. The retained stash is
  `WIP on main: c443a1a8 refactor(runtime): finish the json/v2 migration across delivery and tooling`.
- No user/production data initialization, copying, conversion or deletion occurs.
- No frontend UI/E2E/screenshot tests are added or run. No Desktop aggregate
  `npm run check` is run. Source/type/native build checks do not qualify rendering.
- Replies are Chinese; code/comments/errors/docs are English. Existing root/module
  instructions apply. No sub-agent delegation occurs.
- Stage explicit paths. Hooks run with `lint-staged --no-stash
  --no-hide-partially-staged` and commitlint before HUSKY=0 commits; no bare stash.

## Current product changes

Frozen current Go source is `b2dcbfbba`; later changes are documentation only.

- `6a88bd45a`: public `ToolProposalContract`/`ToolProposalMethod` queries use the
  existing installed composition. Only arguments may transform; execution owns
  Tool identity, schema and approval. Bindings, activation and transitive dependencies
  freeze. Final approval/execution/history agree across actual stdio/Reviews restart,
  with one transformation and no replay. Internal `CallComposition` replaces the
  model-only name, without an alias. Waiting recovery also preserves unreported
  provider usage and later known breakdowns without inventing reported zero/price.
- `ef5a9a0f3`: P12 installs two stdio observers and observes exact nested order,
  frozen in-flight configuration and a reversed next Run. P16 uses a real public-SDK
  command decorator and SQLite receipt: inner mutation commits once, outer failure
  stays unknown, original receipt survives redelivery/reopen and stale next fails.
  P18 exits an installed process after acknowledged output, retaining a chunk and
  uncertainty, with original-receipt-only recovery after explicit reconnect.
  P41 builds/runs the real CLI over authenticated HTTP through installed Schedules
  into Core Run acceptance and same-command redelivery, without a page.
- The CLI path exposes a production metadata defect: absent nested HTTP headers
  inherited parent command key/namespace/cursor. Every request now replaces those
  values even when empty. Core error encoding also refuses to let a nested
  transport error bypass the Delivery-owned problem vocabulary; RED/GREEN tests pass.
- `ac5c3ece6`: actual disabled and failed stdio contributors across restart no
  longer turn captured waiting probes into startup failure. Connecting sources
  defer; definite unavailable/stale captures become explicit lost state; inconclusive
  I/O remains an error without changing facts. Pre-engine discard closes only
  acquired resources. No replacement chain or unapproved mutation executes.
- `b2dcbfbba`: current Runtime/Reviews/CLI guidance replaces stale cutover claims,
  the old design draft is labeled superseded, public API inventory includes P13,
  and the process fixture uses JSON/v2. Strict architecture/generation/static closure
  passes. Retired API strings remain only in explicit refusal tests/removal prose.

Earlier product roots remain relevant: `366ffff6a` compiles immutable model
contracts once; `981c4c915` captures/restores MCP authority separately for every
Tool group, without discovering additional authority for a waiting Deployment.
Other completed contract/dependency/retention/model/feature ownership work remains
recorded in progress rather than duplicated here.

## Actual current qualification

The original complete command passes on frozen `b2dcbfbba`, with no overlay,
exclusion or changed original case deadline:

```sh
go test -race -p 2 ./runtime/... ./runtime/localruntime/... ./cli/... ./examples/plugins/reviews/backend/... -count=1 -timeout=900s
```

- 174 selected packages finish: **159 tested packages pass; fifteen have no test
  files**. Race execution/bootstrap/SDK/CLI/runtimebinding/terminal pass
  276.599/330.789/9.619/70.109/40.204/41.828s, wall time 775.933s. Whole bootstrap
  includes all eight Schedule SIGKILL cuts and existing recovery/steer/schedule cases.
- Complete normal has the same 159 passes/fifteen no-test packages. Execution/
  bootstrap/CLI/runtimebinding/terminal pass 57.079/118.847/39.523/12.150/24.648s,
  wall time 292.538s. Full-module vet passes. Original CLI root/recovery/steer
  test files compare unchanged against `366ffff6a`.
- Core/Memory/Schedules/API regeneration and architecture pass. Shared TS check
  passes 30 files/355 tests plus type/lint/format/dead-code checks. Desktop,
  component SDK, Reviews and portable component types pass. Direct layer,
  published-boundary, context, port and API checks pass: 95/95 fact families,
  112 typed sites, three sidecars, fourteen events and four result readers.
- Runtime `make build` ships ten manifests and both backend executables. Current
  Wails native Go/CGo/package, strict/deep ad-hoc signature, arm64 and macOS 13.0
  floor pass. Its frontend task is up-to-date and reused; this is not a newly run
  Vite build. The duplicate `-lobjc` warning remains. No application is launched.
- Current independent Usage public archives reproduce bundled resources and pass
  clean offline reinstall/typecheck/build. Current independent Reviews plus the
  Tool-proposal executable/command-next receipt case pass offline tidy/race/vet/build,
  GOWORK=off, no replace and isolated module cache. Author source imports public
  APIs only. The module archive includes required internal implementation and a
  separate localruntime artifact; a public-directories-only archive is insufficient.
  No published module cache is overwritten and no registry release is claimed.

## Safe-steer findings and remaining limit

The old delegated deadline has measured immutable-schema compilation evidence and
an independent deterministic Deployment reconstruction repair. That does not by
itself explain every later safe-steer timeout.

A fully pinned historical Go overlay at `366ffff6a` maps all 2,426 Go files;
new Go files are excluded. SQL/module files have no diff through current source.
Only timestamps and deadline-only inspection are added; original assertions and
10s contexts stay unchanged. Eight repetitions reproduce one **safe** failure in
2.78s: steer accepted at 0.815s, waiting snapshot at 1.020s, then
`configurationChanged`, `exact root Deployment does not match` and
`interrupt_not_open`. This proves safe restoration also suffers the original MCP
catalog reconstruction defect. The identical current-source diagnostic passes
eight safe/yolo repetitions (53.615s), matching the existing per-group repair.

No deadline marker occurs in those runs. The historical post-SDK 10s timeout
remains unclassified; do not relabel this faster refusal as that timeout or claim
serial/current parallel success establishes its cause. An earlier full overlay
attempt is inconclusive because source-scanning tests read the physical tree and
not every source was frozen. Successful-cancellation diagnostic stacks from that
attempt are not timeout stacks. Failed iterations remain failed/inconclusive.

## Unexecuted scenarios and risks

- P21/P26/P29-P33/P35-P36/P39-P40/P45-P46 need actual graphical interaction.
  The user prohibition remains in force; no UI tests or application launch.
- All four review cross-scenarios remain Partial: no-Run mutation response loss
  across actual UI/Runtime restart plus update/uninstall/selection; equivalent
  typed refusal through Tool/CLI/rendered UI; authenticated remote multi-resource
  ESM/module identity/connection replacement in Web and Wails; independent recovery
  while module initialization blocks the renderer.
- Paid live DeepSeek is not enabled (`FLAME_LIVE_DEEPSEEK` unset). Windows/Linux/
  alternate native execution and registry/deployment publication are not qualified.
- Read-only npm audit exits 1 with eight alerts (four high/four low): the high
  chain includes StyleX PostCSS/fast-glob/micromatch/braces, with low math/KaTeX
  alerts. Exposure/remediation is unqualified; dependency manifests/locks are
  unchanged. Build success does not close these alerts.
- Core is `2026-10-15.2`, backend is
  `io.github.tangerg.flame/backend/2026-10-14.2`, export schema is 3. Private
  checkpoints require per-group `mcp_manifests` and `tool_proposals`. Preserve old
  data and finish incompatible waiting work with its original binary. There is
  no compatibility reader, automatic migration, private copying or provider replay.
- Runtime owns accepted Runs, execution and accounting. Backends own private
  Memory/Schedules/Reviews facts and receipts. Run-linked evidence follows its
  Run, Session-only evidence follows atomic Session deletion, unattached evidence
  remains retained. Pending unknown routes never expire into replay permission.

## Cleanup and next

Temporary overlays, source snapshots, author archives/caches/kits and diagnostic
resources are removed after inspection; formal code/docs and current ignored
Runtime/native build outputs remain. All verification processes finish. Main and
stash are preserved. No push, merge, external publication or user-data change.

Next substantive unresolved work is to capture a real historical-class safe-steer
deadline with phase/state/stack evidence, without relaxing assertions or limits.
Keep graphical criteria and dependency/live/platform/publication limits explicit.
Do not declare the whole plugin system 100%.

Read-only original inputs:

- `/Users/tangerg/Desktop/docs/Flame-Unified-Plugin-Proposal.html`.
- `/Users/tangerg/Desktop/docs/Flame-Unified-Plugin-Proposal-Review-2026-10-13.md`.

Both inputs remain outside the edited repository scope.
