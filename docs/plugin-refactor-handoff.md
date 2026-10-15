# Unified plugin refactor handoff

## Current acceptance

The original P01-P48 audit is **35 Evidenced / 13 Partial / 0 Open**, not an
implementation percentage or a 100% claim. P13 public installed Tool proposals,
P12/P16/P18/P41 real-process proof and P48 source/API/doc cleanup are complete.
Thirteen graphical rows and all four added review cross-scenarios remain Partial.
The historical post-SDK **10s safe-steer timeout has no established cause**.
Passing repetitions do not close that item. The latest test-only qualification
also has unclassified MCP startup discovery deadlines; its failed gates remain
recorded separately from the earlier complete product pass.
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

Production Go remains `b2dcbfbba`. Later test-only slices add steer failure
diagnostics and actual installed Reviews no-Run recovery across restart,
disable/reenable, retained update/uninstall holds and replacement-target refusal.
The original private receipt recovers once; a second installation stays unchanged.
The real CLI now compares installed query and revision-conflict results with the
model Tool result through authenticated HTTP, including same-command redelivery.
Plugin test Handlers and Endpoints now share the isolated store's namespace;
the fixed conformance namespace remains confined to its matching test binding.
There is no production timeout repair or completed graphical cross-scenario.

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

The latest fixture/CLI slice is not qualified by a complete passing gate.
After correcting every custom plugin fixture, a focused Reviews/Schedules-loss/
cancel-restart race passes 29.250s; architecture passes 68.933s and bootstrap vet
passes. A complete bootstrap race fails 523.661s (147 cases pass, one fails):
`TestScheduleRunNowMissingReceiptRetainsUncertaintyWithoutCreatingRun` expires
its unchanged 10s discovery prerequisite while the original server is `connecting`.
A concurrent eight-cut execution invocation fails 152.600s: commit and malformed
expire that same initial discovery prerequisite; the other six cuts pass.
Those failures have no established cause. A temporary CPU/trace diagnostic of
the two cuts passes 37.901s and does not explain the failures. The helper now logs
public status and live goroutines after the original timeout, before cleanup.
The no-overlay complete execution/bootstrap parallel race now records execution
failure 392.755s: the edited cut expires initial discovery (helper 10.25s,
cut 10.75s); the other seven cuts pass. Its live goroutines show SDK
`Client.discover` awaiting the first `server/discover` response, before admission,
setup or Core callbacks. The proxy/backend's missing reply is not yet explained.
Bootstrap from that paired command fails 543.824s: four Memory/Schedules reply-loss
cases reach the same `Client.discover` wait. The paired command exits 1 after
551.760s (380 top-level passes, five failures, zero skips). The expanded Reviews
Tool/CLI case passes 29.58s, cancellation/restart 1.52s and lost Core-opening
recovery 5.42s; those cases do not qualify the failed packages. A temporary
startup-phase/proxy-metadata/9s-stack-and-CPU overlay then passes after 454.684s:
execution/bootstrap 333.208/435.734s, 385 top-level passes, zero skips, all eight
cuts pass (parent 52.33s). It does not reproduce unanswered discovery. Its only
9s snapshot belongs to an already-running backend, with no eligible blocked
child for native sampling. Killed/unclosed profiles are not qualified. This is
a diagnostic overlay, not a replacement for the failed unmodified-source gate;
no startup cause or production repair is established.
The representative live failure and exact candidate hashes are retained in
[MCP startup deadline evidence](plugin-refactor-mcp-startup-evidence.md).

The earlier namespace mismatch is a separate, established fixture defect:
initial plugin startup advertised the conformance namespace but reopening and
the Endpoint used the real store. A temporary negative control restoring the
mixed binding fails with `mcp_configuration_failed`, rather than remaining
`connecting`. A cancellation model stub also incorrectly treated an auxiliary
title request as a repeated primary request; that stub now distinguishes it.
Neither finding diagnoses the historical safe-steer deadline or the later
connecting deadlines. No assertion or original case deadline is weakened.

The original complete command passes on frozen `b2dcbfbba`, with no overlay,
exclusion or changed original case deadline:

```sh
go test -race -p 2 ./runtime/... ./runtime/localruntime/... ./cli/... ./examples/plugins/reviews/backend/... -count=1 -timeout=900s
```

- 174 selected packages finish: **159 tested packages pass; fifteen have no test
  files**. Race execution/bootstrap/SDK/CLI/runtimebinding/terminal pass
  276.599/330.789/9.619/70.109/40.204/41.828s, wall time 775.933s. Bootstrap
  covers its recovery/steer/schedule cases. The eight Schedule SIGKILL cuts have
  their parent test in execution; it builds and invokes bootstrap's crash helper.
  The complete command covers both packages. Bootstrap alone does not run those cuts.
- Complete normal has the same 159 passes/fifteen no-test packages. Execution/
  bootstrap/CLI/runtimebinding/terminal pass 57.079/118.847/39.523/12.150/24.648s,
  wall time 292.538s. Full-module vet passes. At this qualification, original CLI
  root/recovery/steer test files compare unchanged against `366ffff6a`; the later
  steer change retains their assertions and adds failure-only diagnostics.
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

The current chat's retained original output now recovers the historical failure
line: `steer_receipt_integration_test.go:144` (safe 15.70s, outer 26.75s). That
select waits for the second provider request after Resume succeeds with a new
Segment. Resume acceptance precedes detached activation, so this location alone
does not identify a failed activation, stalled Tool/commit or missing dispatch.
No original state/stack/profile survives for that interval.

A further fully pinned historical diagnostic runs the complete CLI root and
runtimebinding packages together at `-race -p 2 -count=6`. Overall exit is 1:
CLI passes 409.513s; runtimebinding fails 200.863s on two known delegated
Deployment refusals (2.59/2.82s). All six safe/yolo pairs pass, with safe's
second provider 168-188ms after Resume and no deadline markers. This does not
close the historical 10s failure. Retained command history shows a separate
normal execution-package RED-test build/run overlapping the original safe case;
without critical-interval scheduler/CPU evidence it is not a contention diagnosis.
An initial malformed diagnostic string fails setup, is corrected, and is not
counted as a product result. Production/tests/dependencies remain unchanged.

No deadline marker occurs in those runs. The historical post-SDK 10s timeout
remains unclassified; do not relabel this faster refusal as that timeout or claim
serial/current parallel success establishes its cause. An earlier full overlay
attempt is inconclusive because source-scanning tests read the physical tree and
not every source was frozen. Successful-cancellation diagnostic stacks from that
attempt are not timeout stacks. Failed iterations remain failed/inconclusive.

## Current stage-level investigation

A full-module Go-pinned historical diagnostic now exits 1 in 835.888s: 158 tested
packages pass, one architecture package fails, and fifteen have no tests. The
two architecture failures are diagnostic source/artifact mismatches: a virtual
helper has no physical file for the source reader, and the old API inventory
differs from the current on-disk inventory. This is not acceptance. The original
steer pair passes (safe 3.46s), with 72 stage entries and 72 returns, no deadline
marker, and the second provider reached under the original 10s assertion. Normal
post-Resume stage durations do not establish the missing historical failure.
Current qualification jobs overlap part of this diagnostic; no artificial load
or controlled contention conclusion is claimed. Detailed stage durations are
recorded in progress. The historical timeout remains unresolved.

Current new Reviews no-Run normal proof passes 12.755s; complete bootstrap race
passes 408.052s. Final no-redispatch guard assertions then pass focused race in
35.311s; the complete bootstrap result predates those extra assertions. All other
cases and production source are unchanged. A temporary receipt-to-redispatch
negative control fails as expected (exit 1, 27.621s); it is not an acceptance pass.
All eight SIGKILL cuts pass through their execution
parent (58.876s), and current complete architecture passes 31.663s. Direct static
boundary/API checks also pass. These current checks have no historical overlay.
Whole current Runtime/shared TS/native gates are not rerun for these test-only
changes; the complete product qualification above stays tied to `b2dcbfbba`.

A fully pinned `366ffff6a` overlay now observes background staging/activation,
answer delivery/proof, resumed Processes, dispatch gates, Tool executable,
tree/fact commits plus pump acknowledgement, next-model context, applied-steer
commit, Model start and the provider iterator, with Go runtime traces. Complete
historical CLI/root binding race passes 75.242/39.575s. After Resume, activation
finishes at 35.9ms, Shell executes 54.3-59.7ms, post-Tool tree commits settle by
126.1ms, context reduction ends 142.1ms, steer/Model-start commits end 146.3/148.4ms,
and the second HTTP request arrives 173.4ms after acknowledgement. All post-Resume
stages return normally. These are baseline observations, not the old failure.

Current test-only diagnostics retain phases from Start, provider-entry facts,
goroutine stacks and authoritative Run outcome/explanation when the original 10s
assertion fails. Failure inspection has its own bounded 2s read after failure;
assertions, the 10s success window and both original event loops stay unchanged.

Four temporary owner-boundary fault controls (activation, Tool executable,
steer commit and model pre-dispatch) all reproduce the same caller timeout while
the Run has already ended in 0.44-1.05s as failed or lost. The diagnostic captures
the exact retained outcome; all four commands intentionally exit 1. They prove
that the old bare timeout could hide a completed backend failure, not that one of
these injected causes occurred historically. No artificial timing/load, weaker
assertion, retry, replay or compatibility path is introduced.

Complete current CLI/root binding parallel race passes 74.927/40.261s without
overlays or exclusions, including the final diagnostics. Whole Runtime/bootstrap,
TS and native qualification is not rerun for this test-only slice. Temporary
sources, traces, binaries and control resources are removed after inspection.

The historical 10s root cause remains **unresolved**. Its exact phase/error/state
is still unavailable. Do not call the diagnosis or overall refactor complete.

## Unexecuted scenarios and risks

- P21/P26/P29-P33/P35-P36/P39-P40/P45-P46 need actual graphical interaction.
  The user prohibition remains in force; no UI tests or application launch.
- All four review cross-scenarios remain Partial: no-Run mutation response loss
  across actual UI/Runtime restart plus update/uninstall/selection; equivalent
  typed refusal through Tool/CLI/rendered UI; authenticated remote multi-resource
  ESM/module identity/connection replacement in Web and Wails; independent recovery
  while module initialization blocks the renderer.
  Real Tool/CLI query and revision-conflict equivalence now has backend process
  evidence; the graphical entrance and other cross-entrance refusal classes are
  not newly qualified by that case.
- Paid live DeepSeek is not enabled (`FLAME_LIVE_DEEPSEEK` unset). Windows/Linux/
  alternate native execution and registry/deployment publication are not qualified.
- Read-only npm audit exits 1 with eight alerts (four high/four low). All high
  entries share one unpatched advisory, GHSA-vfj7-8cjw-p6xm: StyleX PostCSS
  0.19.1 -> fast-glob 3.3.3 -> micromatch 4.0.8 -> braces 3.0.3. A bounded Node
  probe confirms stack overflow for deeply nested patterns. Current consumers
  accept only two repository-owned build globs; explicit includes disable StyleX
  dependency auto-discovery. No current untrusted-pattern path is demonstrated.
  Production-only audit reports zero high/four low and still exits 1. Retain the
  upstream build dependency risk; reopen on external-pattern admission, dependency
  changes or a fixed release. Detailed triage is in the progress document. No audit suppression,
  override, local dependency patch, downgrade or manifest/lock change occurs.
  The four low entries share GHSA-238p-pmpm-9mq7: two transitive KaTeX 0.16.47
  renderers beneath rehype-katex/micromark-extension-math. Direct KaTeX 0.19.0
  supplies CSS and does not upgrade those copies. Source tracing confirms the
  message renderer uses the affected dependency; no actual prototype-pollution
  source or rendered exploit is demonstrated. Fixed 0.18.2 is outside both latest
  integrations' `^0.16.0` range. Keep the conditional risk open; audit's suggested
  major downgrades are not a qualified fix. No UI rendering/test, dependency
  override, forced downgrade or manifest/lock change occurs. All eight alerts
  now have triage, and none is claimed remediated.
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
deadline with phase/state/stack evidence. Determine whether execution is still
live, where it stopped, and what the original owner retained; a terminal Run can
also explain the caller's wait. Do not relax assertions or limits.
The retained MCP connecting-deadline stacks stop in initial SDK discovery, before
admission/Core callbacks. A bounded phase/CPU diagnostic does not reproduce them;
obtain a failing child/proxy snapshot before making a production repair.
Do not substitute a later pass or build overlap for a cause of its failed gates.
Keep graphical criteria and dependency/live/platform/publication limits explicit.
Do not declare the whole plugin system 100%.

Read-only original inputs:

- `/Users/tangerg/Desktop/docs/Flame-Unified-Plugin-Proposal.html`.
- `/Users/tangerg/Desktop/docs/Flame-Unified-Plugin-Proposal-Review-2026-10-13.md`.

Both inputs remain outside the edited repository scope.
