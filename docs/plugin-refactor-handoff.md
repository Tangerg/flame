# Unified plugin refactor handoff

Prepared on 2026-10-14 and updated after the 2026-10-15 standalone component author qualification.
The complete proposal/review implementation is unfinished. This document is a
starting point; current repository state remains authoritative.

## Resume here

- Worktree: `/Users/tangerg/.codex/worktrees/unified-plugins/flame`
- Branch: `codex/unified-plugins`
- Last integrated main: `7ba40c117` (Scope v0.50.0 dependency graph).
- Main integration commit: `0227ffd94`.
- Latest product commit: `ecdbe5c39` (standalone packed Usage author kit and
  explicit author-owned compiler); `d0c9afaec` removes Core Schedule identity vocabulary
  and feature imports of Core invalidation records; `325998894` (public immutable model choice and released
  catalog admission, all consumers regenerated); `419735d13` extracts public
  SQLite/receipt owners and per-database transaction authority; preceding `6e23f4d00` implements target-owned point
  availability and actual
  installed Reviews/analyzer callbacks); `18ddce955` fences same-release activation
  identity;
  `6ef5b7961` adds transitive release retention;
  preceding `6a2f30440` implements exact feature JSON and closed Selection schema;
  query-stream commit: `67e7801c1`; preceding recovery
  commit: `8e32d9c35` (observer-bound receipt retirement and unavailable original
  source recovery). Tooling commit `59c826ec6` repairs the contract lock's two
  missing optional transitive peers without changing existing locked versions.
- Last product commits (2026-10-14 session): `f59404a96` (bootstrap realignment +
  unconfirmed-command projection), `13f9eb336` (contract orphan resolution). Earlier
  this branch: `73ce66711` (two-phase startup recovery), `8ad951c7d` (embedding
  endpoint evidence), `13193c2d6` (original background resource targets).
- Last progress correction before this handoff: see the progress anchor's
  "Session close 2026-10-14" entry.
- Progress anchor: `/Users/tangerg/.codex/worktrees/unified-plugins/flame/docs/plugin-refactor-progress.md`.
- `/Users/tangerg/Desktop/flame/docs/plugin-refactor-progress.md` is an existing
  symlink to that anchor. Preserve it.

Reuse this worktree. Do not start from main, create another worktree or copy the
changes into `/Users/tangerg/Desktop/flame`: the user develops other features there.
Read this handoff, the current-stage/next/review sections of the progress anchor,
`AGENTS.md`, `PROJECT_RULES.md`, `DESIGN_PHILOSOPHY.md`, `DEVELOPMENT.md`,
`REFACTORING.md` and the relevant module README before editing.

Before each slice, inspect status and relevant current code. After a complete slice,
commit explicit owned paths, preserve integrated main at `7ba40c117`, update the existing
progress anchor, clean owned resources and report actual results. Use the last
integrated main SHA to inspect incoming main changes; `HEAD..main` can produce a
huge unrelated diff on this long-lived branch. Do not push or merge this branch into
main without further authorization.

## Objective and constraints

Implement the entire new plugin proposal and close every review item and P01-P48
requirement. Preserve one mutable owner per fact. Fix contracts at their owner,
migrate all affected consumers, and remove replaced paths in the same change.
Breaking changes are authorized; compatibility wrappers, dual writers, legacy
readers, silent fallback and provider replay are prohibited.

Development comes first. Complete connected product paths instead of repeatedly
fragmenting plumbing and expanding verification. Use meaningful focused backend
checks while developing; concentrate full/native/vet/race/architecture/dependency
and closure checks at final acceptance. Do not weaken assertions or skip failures.
Do not write or run frontend UI, E2E or screenshot tests. Desktop typechecking and
backend/shared-protocol checks are permitted. Do not run Desktop's aggregate
`npm run check`, which includes forbidden UI tests.

No production operations, credentialed/live provider calls, data deletion,
destructive Git operations, publishing or external/shared-state mutation. Preserve
the user's stash:
`WIP on main: c443a1a8 refactor(runtime): finish the json/v2 migration across delivery and tooling`.
No subagents are authorized for new work in this handoff.

## Read-only inputs

- `/Users/tangerg/Desktop/docs/Flame-Unified-Plugin-Proposal.html`
- `/Users/tangerg/Desktop/docs/Flame-Unified-Plugin-Proposal-Review-2026-10-13.md`
- `/Users/tangerg/Desktop/docs/Flame-Durable-Improvement-Measures-2026-10-12.md`
- `/Users/tangerg/Desktop/docs/Scope-Flame-Agent-Architecture-and-Upgrade-Report-2026-10-12.md`
- Original comparison draft: `/Users/tangerg/Desktop/flame/docs/flame-heterogeneous-plugins-draft-v0.2.md`

The proposal, review and original draft have not been edited. Verified SHA-256:

| Input | SHA-256 |
| --- | --- |
| Proposal | `e5f7f67ae5ba3aba2d1a560a447f82722c9443144d5ccd709956aebeae24b756` |
| Review | `cfc874a858008778443d0d68d303e76ac6d2e6ea67726f8a2fbdcd132c328360` |
| v0.2 draft | `aae40af95c4b616662398167d86e46014d2d36b4a8f2953fdb443c08f2a475ef` |

Record necessary design corrections in the worktree's design/progress documents,
not by changing these input files.

## Committed implementation to preserve

The public declaration/contribution/selection/composition foundation, installed
typed operations, Go/Protocol/CLI/TS consumers, private atomic command receipts,
original command routing, components/resources and first-party initialization are
implemented to varying levels of boundary proof. Memory/Schedules Core business
facades and older view/renderer carriers have been removed. The progress anchor
contains earlier commits and evidence; none of this closes every native workflow.

Recent connected implementation:

| Commit | Behavior |
| --- | --- |
| `2583125cd` | Independent selected unary/stream host model callbacks with exact selection, complete request and owner-supplied budgets |
| `b7c6f3c14` | Physical model source association comes from the captured admitted logical Model/Tool owner, with Session/Run/Segment/chronology validation |
| `6839e9c6e` | Original independent physical consumption joins atomic lost Run/Goal recovery; known/unknown evidence, rollback and repeated-boot exclusion |
| `0ad3d34e5` | One per-call Run consumption transition; cumulative executor/segment invoices removed |
| `086f01cc1` | Live selected/auxiliary consumption through the captured member's fact pump; persisted conversation/independent purpose and role-aware recovery |
| `8968bb1bf` | Selection/context/input preparation receives host RPC; Runtime-owned policy composition is separate from caller cross-plugin requirements; active contributor cycles are refused |
| `5f0a5030a` | Observer grants cohost canonical RPC, shared model collaborators and exact next; next retains original caller authority/cancellation while ordinary RPC uses receiver Actor |
| `f3dfbe976` | Actual selected-call regression requires the original observer-cycle refusal and avoids recursive-test hangs |
| `13193c2d6` | Retained background commands preserve original resource targets; only fresh Automatic/parentless triggers require their own installation target |

Current identities: Core Protocol `2026-10-15.2`, backend protocol `.2`, export
schema 3, Scope modules v0.50.0. Strict database admission now requires
`model_invocations.logical_purpose`; older shapes are refused without conversion
or deletion. Captured primary context and independent billing remain distinct.

## Remaining work and priority

1. Embedding results/evidence/metering is implemented through the existing source,
   live Run/Goal, query/export and recovery owners (see the progress anchor).
   Historical retention/pruning remains priority #3. Memory-owned call bounds stay
   Memory policy, as the proposal requires.
2. Complete execution-bound background provenance and original active member
   ownership. Restored caller attribution does not restore Session/Run/Segment,
   a model publisher, an expired next or stopped Scope effects. Never infer them
   from parent IDs or an ambient current Session.
3. Complete transitive availability diagnostics and evidence retention/pruning
   policy audit. `6e23f4d00` proves the actual installed Reviews/analyzer Core
   callback path, optional disable/reenable and setup refresh; target-owned point
   declarations cannot borrow another enabled installation. Backend/point/resource
   checks and the original Reviews Run/approval/CLI/recovery proof pass. Same-release activation identity is
   implemented in `18ddce955`: one session-owned proof includes ordered bindings
   and source authorities. Frozen dispatch and deployment restoration check it.
   `6ef5b7961` retains actual transitive session release sets in frozen Tools/models
   and unresolved commands. It introduces required immutable host
   `plugin_commands.activation_json` (replacing the provisional release-only column);
   old Runtime databases are refused without
   writes/conversion. Private backend formats are unchanged. Whole affected packages
   and a real bootstrap recovery selection pass; full/native/vet/race remain final. Exact Go/TS feature JSON now crosses Core as JSON strings with
   one TS codec; generated consumers, original-intent journals and components are
   migrated in `6a2f30440`. Selection schema/decode share their closed three-field
   owner; Schedules consumers are regenerated. Installed query streams now share
   the exact-contract operation policy with unary calls; queries have no command
   key/route/private receipt, while command streams retain original recovery.
   Shared TS 355 tests, whole CLI runtimebinding/terminal, focused real bootstrap
   schedules, affected Go packages, Runtime/CLI/Reviews build and SDK/portable/
   Reviews/Desktop typechecks pass. Audit every Tool,
   automatic and cross-plugin consumer for target/origin/parent/original route.
4. Public `runtime/storage/sqlite` and `runtime/plugin/sqlite` now own the shared
   implementation; old private entrances are removed and all consumers migrate.
   Full related packages, real bootstrap selection and an offline outside-repository
   public-author receipt fixture pass (`419735d13`). Public `runtime/model` and `runtime/model/catalog` now own the immutable choice
   and catalog admission (`325998894`); all private consumers and generated
   Schedules contracts migrate. Focused packages, real bootstrap (63.710s),
   external author compilation, builds and types pass. Usage now builds outside the repository from packed public artifacts and
   produces byte-identical release resources (`ecdbe5c39`). A clean reinstall
   builds as well. The SDK compiler is an explicit author-toolchain argument;
   all build callers migrate. Public artifacts are not registry publication.
   Complete independent backend author and native distribution qualification. The original P01 requires one bundled feature built from public
   artifacts, not blanket publication of host utilities or moving every backend.
   Extract further backend dependencies only for a demonstrated author need.
   `d0c9afaec` removes Core Schedule identity vocabulary and Core invalidation
   records from feature owners; full feature packages and bootstrap (34.508s) pass. Workspace
   compilation is not proof of published-module availability. Publishing is not
   authorized.
5. Run concentrated integrated acceptance and repair failures. Individually audit
   P01-P48 and every review requirement with appropriate current evidence; delete
   remaining retired APIs/schemas/examples/docs and finish with a clean worktree.

Budget ownership is already decided: proposal 10.2/10.3/11.5 assigns budgets to the
calling operation/execution. Current Run/Goal contracts have no configured universal
total-cost/step ceiling. Do not invent one from older progress wording. Preserve
actual owner-supplied limits, in-flight admission and unknown consumption without
another meter or receipt owner.

The old repeated 75% estimate was withdrawn. At the user's explicit reassessment,
overall workload was roughly estimated at 65% (60-70% range), including final
acceptance. This is not a requirement pass count and must not be carried forward
without reassessment. Prefer reporting completed paths and concrete remaining work.

## Embedding investigation: useful facts, no implementation retained

The last unfinished draft changed only
`runtime/internal/domain/modelref/endpoint.go`. It was incomplete and was withdrawn
during handoff cleanup. There is no embedding implementation commit to continue or
uncommitted patch to preserve. Re-design from the current contracts.

Confirmed facts:

- `runtime/internal/adapter/integration/model/embedding.go` constructs Scope's
  `embeddingclient.Client`, then `EmbedTexts` returns vectors and discards provider
  response metadata. Observe the actual `embedding.Model.Call` boundary before
  that projection; do not reconstruct a provider response from vectors.
- Scope v0.50.0 `embedding.Request` has `Texts` and `Options`; `embedding.Response`
  has `Outputs` and `Metadata`. Usage reports input tokens only; nil usage is
  unknown. Validate correspondence with `Response.ValidateFor(request)`.
- Request/Response have no `Clone` methods in this released embedding package.
  `embedding.Options.Clone` exists. `chat.Usage.Clone` returns a value. The withdrawn
  draft incorrectly assumed nil-safe pointer clone methods; do not reuse it.
- Current `modelref.EndpointInvocation/Outcome/Observation` are chat-specific.
  Existing SQLite source scanning, known-response decoding, price checks and
  Run/Goal recovery also assume chat. A partial embedding-only adapter would
  leave query/recovery or billing inconsistent.
- Any normalized accounting projection must preserve original embedding JSON,
  served model, unknown usage/price and admitted attempt identity. It must not
  synthesize a chat response or advance primary conversation/context.
- Persist admission before physical dispatch, retain known inner evidence under
  outer failure/cancellation, fence identity reuse and never replay unknown work.

Trace these files together before choosing the complete change:

| Concern | Starting files under the worktree |
| --- | --- |
| Provider and bootstrap | `runtime/internal/adapter/integration/model/embedding.go`; `endpoint_observation.go`; `role_resolvers.go`; `runtime/internal/bootstrap/model_environment.go` |
| Source invariant/storage | `runtime/internal/domain/modelref/endpoint.go`; `runtime/internal/infra/sqlite/model_endpoint.go`; `model_invocation.go`; `model_invocation_usage.go`; `db.go` |
| Live Run owner | `runtime/internal/adapter/executionctx/model_calls.go`; `runtime/internal/adapter/run/execution/interaction_model_calls.go`; `runtime/internal/application/agent/runs/reducer.go`; `commit.go`; `execution_fact.go`; `runtime/internal/adapter/run/segment/effects_commit.go` |
| Recovery/reporting | `runtime/internal/application/agent/runs/recovery.go`; `recovery_commit.go`; `runtime/internal/adapter/run/recovery/persistence.go`; `runtime/internal/application/agent/sessions/usage_reporter.go` |
| Public projection | `runtime/internal/delivery/model_endpoints.go`; `runtime/protocol/model_invocations.go`; generated contracts and samples |
| Memory-specific grant debt | `runtime/internal/plugins/memory/rpc/models.go`; `runtime/internal/adapter/integration/pluginservices/tool_preparation.go`; `memory.go` |

## Verification and unresolved failures

Passed before handoff: changed backend package suites, focused callback/ownership
race checks, real SQLite recovery/rollback/reopen boundaries, Runtime build,
contract generation, Runtime/bootstrap test-source compilation, CLI and independent
Reviews author compilation. After main's Scope upgrade, build/compile and focused
observer/preparation checks passed; regenerated contracts had no drift.

Most recent product slice: whole Application plugins suite 0.902s, SDK background
checks 0.719s, focused target/restart checks 1.144s, Runtime build and format/diff
checks passed. Earlier observer slice: SDK/plugins/MCP suites 1.581/28.617/1.564s;
focused race 1.839/7.932s. These are boundary/module results, not full acceptance.
Root/bootstrap test sources were compiled with `go test -c`; their TestMain was not
executed by those compile checks.

Resolved since the last handoff:

- `TestScheduleCreateRecoversAcrossSIGKILLCuts`: fixed by two-phase startup recovery
  (`73ce66711`); all seven cuts pass.
- Orphan generated definitions (RecordedUsage/PageContinuation/PageOfItem/PageQuery):
  RESOLVED (`13f9eb336`). The generator separates embedding reach from standalone
  `$ref` reach and withholds embedded-only owners from the published bundle while
  keeping them in the walked set for validators, the TS generic emitter and the
  wire-graph invariant. `ListModelEndpointsRequest` now embeds `PageQuery`.
- Operation/publication inventories, JSON vocabulary/finite-read, panic retention,
  capability gate, Run idempotency: resolved earlier this branch (see the anchor's
  acceptance sweep). `go vet ./...` clean; non-bootstrap module sweep green.

Still failing/unresolved:

- The former bootstrap failure is RESOLVED on 2026-10-15 (`8e32d9c35`). Current
  production tracing corrected the immediate old diagnosis: `BeginRoot` awaited
  initial Scope checkpoint publication while the canceled Segment pump's `finish`
  joined activation. The old host could not release its Session lease. Receipt
  waits now belong to the actual captured observer lifetime; withdrawal ends the
  wait while execution cancellation alone still preserves known facts. Original
  Schedule source availability is an explicit eligibility result, so unavailable
  sources do not prevent the existing Core ownership sweep. The original callback
  loss test, entire bootstrap twice, and final entire execution suite twice pass,
  including the original seven SIGKILL cuts and a new disabled-source cut.
- Two earlier execution sweeps saw an intermittent `resolution` cut loss. An
  unchanged HEAD execution sweep passed; no causal pre-existing classification was
  established. Diagnostic runs and the final complete `-count=2` run passed with
  original assertions preserved and all instrumentation removed. Repeat that cut
  during final concentrated acceptance; do not waive it. The progress anchor has
  the actual results and remaining acceptance concern.
- A Desktop aggregate UI check was accidentally run much earlier and interrupted;
  resulting UI failures were not proven pre-existing. Do not rerun forbidden UI
  checks or claim those failures are resolved.

Full/native/vet/global-race/dependency/cutover/review closure are not complete.
Shared Go cache cleanup once caused missing-cache/misleading missing-stdlib setup
errors; an owned temporary GOCACHE rebuild then passed the strengthened observer
check (1.957s). Repository guidance says rebuild after scheduled cache cleanup,
not investigate it as a product failure. No checks were weakened to force a pass.

## Practical continuation commands

Run from the stated worktree or owning module, never the user's main checkout.

```sh
cd /Users/tangerg/.codex/worktrees/unified-plugins/flame
git status --short
git log -5 --oneline
git log --oneline 7ba40c117..main
```

Relevant Runtime commands, selected proportionately to the actual next change:

```sh
cd /Users/tangerg/.codex/worktrees/unified-plugins/flame/runtime
go build ./...
go test ./internal/adapter/integration/model -run Embedding -count=1
go run ./cmd/contractgen --out ./contract --validators ./protocol --ts ./contract/typescript
```

Generate only after actual contract changes. Do not run full/native checks merely
because a new session started. Compile root/bootstrap tests into an owned
TemporaryDirectory when needed; ordinary `go test` there can invoke costly native
TestMain work. Preserve the managed worktree and existing necessary dependency
artifacts for continuation.
