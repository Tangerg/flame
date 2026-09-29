# Protocol proposal review: core streaming and side-channel API

**Review date:** 2026-09-23 · **Reviewed tree:** Flame `main` @ `f19c64e2`

> **Status.** This is a review record, not part of the architecture baseline.
> [`README.md`](README.md) states that competitive research and retrospectives belong in Git
> history rather than the active documentation set. Keep this file only as long as its
> verdicts are driving decisions; delete it once the accepted items are implemented and the
> rejected ones are settled.

## 1. What this document is

Two external analysis reports compared Flame against OpenCode and Codex and produced a
combined list of proposals for the Runtime Protocol and its side-channel API. This document
consolidates those proposals with an independent review of each one against the actual
Flame implementation.

For every proposal it records: the original intent, a verdict, the code evidence behind the
verdict, the cost of acting, and a recommendation. It also records the corrections that
review produced, the findings the source reports missed, and the reference-project facts
that survived verification.

The source reports themselves are not checked in. Every proposal they raised is captured
here; nothing was dropped for brevity.

## 2. Evidence base and method

### 2.1 Trees examined

| Project | Ref used for verification | Notes |
| --- | --- | --- |
| Flame | `main` @ `f19c64e2` | The reviewed tree; also the commit the source reports pinned |
| OpenCode | `origin/v2` @ `68b28bdb98`, `dev` @ `45719acebb` | The v2 rewrite lives on `origin/v2`; `dev` has a different architecture with different file names |
| Codex | `main` @ `c44deff7b1` | — |

The source reports pinned OpenCode `d56ce743` (on `origin/v2`, 2026-09-22) and Codex
`8a3c4ea3`. Both exist and their cited files exist at those commits. An earlier pass of this
review wrongly concluded the OpenCode citations were unreproducible; that pass was reading
`dev`, where the v2 files do not exist. **OpenCode conclusions must be checked against
`origin/v2`, never `dev`.**

### 2.2 Method

Each proposal was reduced to a question that code can answer — "can a client learn X", "does
Y exist", "what does Z return" — and then traced through the protocol contract, the
delivery handler, the application owner, and the adapter. Where the answer depended on a
reference project's behavior, the claim was re-read in that project's own source rather than
taken from the report.

### 2.3 Not verified

- Whether Scope exposes any provider-retry callback. This decides the feasibility of the
  retry-preview proposal (§4.8) and was not checked.
- Any load measurement. No session-size, snapshot-latency, or directory-enumeration
  benchmark was run. Every scale statement below is an argument from code structure, not a
  measurement.
- Desktop runtime behavior. Desktop code was read, not executed; no UI reproduction was
  attempted, and Desktop is outside the current change boundary in any case.

### 2.4 Verdict taxonomy

| Verdict | Meaning |
| --- | --- |
| **Already owned** | Implemented; the proposal restates existing behavior |
| **Real gap** | The contract or implementation genuinely cannot answer the question |
| **Premise absent** | The proposal assumes a structure Flame does not have |
| **Deliberate** | Code or comments document the current behavior as a chosen trade-off |
| **Net-new surface** | Not a gap in existing behavior; a new product capability |

## 3. Verdict summary

Thirty-six proposals. Eight are real gaps; four of those are worth acting on now.

| # | Proposal | Verdict | Act? |
| --- | --- | --- | --- |
| 4.1 | Observable steer consumption | Real gap (narrow) | **Now** |
| 4.2 | Input queue | Premise absent | No |
| 4.3 | Keep strict command fingerprint | Already owned | — |
| 4.4 | Declare completion-payload coverage | No such payload today | Rule only |
| 4.5 | Windowed session bootstrap | Real gap | Measure first |
| 4.6 | Rebuildable command receipts | Real gap | **Now** |
| 4.7 | Bounded active preview snapshot | Real gap, low value | No |
| 4.8 | Structured retry preview | Unverified feasibility | Defer |
| 4.9 | Workflow / group execution | Conditional | No |
| 4.10 | Per-interaction identity inside one Item | Premise absent | No |
| 4.11 | Non-blocking questions, partial continuation | Conditional | No |
| 4.12 | Recovery observability | Real gap | **Now** (with 5.10) |
| 4.13 | Desktop README wording | Out of scope | No |
| 4.14 | Long-term event log | Rejected, with evidence | No |
| 4.15 | Cross-process run continuation | Conditional | No |
| 4.16 | Large attachments as resources | See 5.17 | — |
| 5.1 | Watch ordinary external file edits | Real gap | With Desktop |
| 5.2 | Consume existing invalidation scope | Desktop-side | Out of scope |
| 5.3 | Run-scoped diff and change attribution | Real gap | Conditional |
| 5.4 | Desktop rename summary | Out of scope | No |
| 5.5 | Auditable restore coverage | Real gap | **Now** |
| 5.6 | Skill proposal target CAS | Deliberate | No |
| 5.7 | Run context provenance | Real gap, medium value | Later |
| 5.8 | Effective config with origins | Premise absent | No |
| 5.9 | Optimistic concurrency for provider/MCP | Real gap, no consumer | No |
| 5.10 | Directory listing pagination cost | Real gap, measurable | **Measure now** |
| 5.11 | Search coverage summary | Low value | No |
| 5.12 | Media and binary reads | Net-new surface | No |
| 5.13 | Incremental search sessions | Net-new surface | No |
| 5.14 | MCP resource previews | Net-new surface | No |
| 5.15 | Standalone process resource | Net-new surface | No |
| 5.16 | First-class LSP API | Net-new surface | No |
| 5.17 | Ranged retrieval of offloaded results | Real gap | Later |
| 5.18 | Worktree / environment identity | Conditional | No |
| 5.19 | Additional error categories | Already sufficient | No |
| 5.20 | Capability declaration for new surfaces | Already owned | — |

## 4. Core execution flow proposals

### 4.1 Observable steer consumption — real gap, far narrower than proposed

**Proposed:** make accepted / applied / not-applied queryable for appended input, following
OpenCode's separation of durable admission from delivery.

**Verified behavior.** Three of the four paths already explain themselves:

- Rejection is explicit. `submitSteer` reserves the Item ID, registers the pending steer, and
  returns an error when the engine does not accept the signal
  (`internal/adapter/agentexec/interaction_control.go:54-108`).
- Application is visible. At the model boundary `commitAppliedInputs` emits
  `SteerMessagesApplied`, which commits the transcript Item the client is already holding an
  ID for (`internal/adapter/agentexec/interaction_control.go:110-166`,
  `internal/application/agent/runs/execution_fact.go:424-438`,
  `internal/application/agent/runs/execution_fact_commit.go:124-129`).
- Park and resume preserve it. Pending steers are encoded into the executor checkpoint and
  restored (`internal/adapter/agentexec/interaction_checkpoint.go:229-253`, `internal/adapter/agentexec/interaction_restore.go:92`).

The response contract already says acceptance is not consumption
(`protocol/runs.go:289-307`).

**The gap.** When a Segment ends terminally — cancel, failure — with entries still in
`pendingSteers`, that state dies with the session. No fact is produced. The client holds a
`userItemId` for an Item that will never exist and receives no explanation.

**Cost.** Small, and it needs **no protocol addition**: the Item ID is already reserved, so
committing that Item in a failed or canceled state reuses `item.completed` plus `error`.
Every existing consumer benefits without a binding change.

**Recommendation:** do this. Scope it to the negative fact only. Do not import OpenCode's
inbox lifecycle to solve a one-path problem.

### 4.2 Input queue — premise absent

**Proposed:** queue further tasks for after the current one, with editable, cancelable,
reorderable entries.

`runs.start` refuses a second root with `session_has_active_run`
(`contract/API_REFERENCE.md:26`). That refusal is deliberate: it hands the client an explicit
choice between steer, resume, and cancel instead of silently deciding. A queue changes that
product behavior, and no usage evidence shows users batching tasks.

**Recommendation:** no. Revisit only with real usage data, and then as an operation of its
own, never by loosening `runs.start`.

### 4.3 Keep the strict command fingerprint — already owned

Flame binds an idempotency key to `method + typed parameters` and rejects a key rebound to a
different operation (`internal/delivery/replay.go:68-79`). This is stricter than OpenCode's
first-admission-wins reconciliation and should stay. No action.

### 4.4 Declare completion-payload coverage — rule, not work

Codex's `turn/completed` carries only a trailing summary or nothing, flagged by `itemsView`
(`codex-rs/app-server/src/bespoke_event_handling.rs:1328-1329`). A client that treats it as a
full timeline loses already-rendered items.

Flame has no equivalent aggregate payload: `segment.finished` carries outcome, metrics, and
context tokens only (`protocol/events.go:22-58`). There is nothing to fix.

**Recommendation:** keep as a design constraint for any future summary, window, or
subtree-result payload — state coverage explicitly; never let a consumer infer it from an
empty array.

### 4.5 Windowed session bootstrap — real gap, cost overestimated

`SessionSnapshot` carries complete `Items`, `Runs`, and `Interrupts` arrays with no window
(`protocol/sessions.go:66-72`), read from one coherent storage snapshot
(`internal/application/agent/sessions/material_snapshot.go:16-28`). Cold open cost grows with
history.

The source reports described this as needing new pagination. It does not: `items.list`
already exists with a scope union, order, and cursor (`protocol/items.go:18-74`), and the
generated method index already lists it as a component of `sessions.snapshot`
(`contract/API_REFERENCE.md:17`).

**The real cost is in CLI.** `ListItems`, `ListInterrupts`, and `GetPlan` are currently
classified `materializedByAggregate` — CLI obtains them through the snapshot instead of
calling them (`cli/internal/adapter/runtimebinding/runtime_api_inventory_test.go:16-37`). A
windowed snapshot forces CLI to page for real. Neither report mentions this migration.

**Recommendation:** measure first (§4.12, §5.10). If bootstrap cost is real, window the
history while keeping the complete active tree, all pending interrupts, and the current
plan and goal, and preserve the existing snapshot/tail fence.

### 4.6 Rebuildable command receipts — real gap

The delivery path is claim → execute → persist receipt. When `completeDetached` fails after
the business effect committed, the outcome is held in memory, logged, and reported as
`idempotency_in_progress` (`internal/delivery/replay.go:108-127`). A hard crash at that point
leaves the key claimed — so no second execution — but the original result unknown forever.

**Recommendation:** do this, limited to `runs.start` and `runs.resume`. Persist a correlation
from command identity to the business resource it produced, so a retry can rebuild the
response when the effect is findable, re-admit when it provably did not happen, and keep
returning unknown otherwise. Do not weaken the existing protection by expiring pending
claims.

### 4.7 Bounded active preview snapshot — real gap, low value

`segment.progress` and `item.delta` are non-replayable by construction
(`internal/application/agent/runs/projection_event.go:159-163`), and the journal drops
non-replayable events for a lagging consumer while never dropping replayable ones
(`internal/application/agent/runs/journal.go:412-440`). A cold recovery therefore cannot restore in-flight preview text.

This is the intended layering — durable facts recover, previews do not.

**Recommendation:** no, unless refresh-during-generation becomes a measured complaint.

### 4.8 Structured retry preview — feasibility unverified

The protocol has no representation of an in-progress provider retry; `retryAfterSeconds`
exists only on `ProblemData` for idempotency backoff. OpenCode surfaces scheduled retries and
Codex marks errors `willRetry`.

Whether Flame can surface anything depends on whether Scope reports its retries at all,
which this review did not check.

**Recommendation:** defer. Check the Scope surface before designing anything. If added, it
must remain a preview that never advances a terminal outcome.

### 4.9 Workflow and group execution — conditional

Flame already owns the run tree: parent, root, spawning item, tree-wide cancellation, and
waiting-child cancellation that re-shapes pending state. What a DAG would add is node
identity, attempt identity, dependency edges, per-branch waiting, and result composition.

**Recommendation:** no, until a concrete multi-strategy product flow exists. Adding a
`strategy` value and more events does not produce those semantics.

### 4.10 Per-interaction identity inside one Item — premise absent

`Interrupt` is keyed by `ItemID` (`protocol/runs.go:501-506`). Codex needs multiple approval
IDs under one command item; Flame's model is one approval per tool call.

**Recommendation:** no. Revisit only if one Item must carry several independently answerable
decisions.

### 4.11 Non-blocking questions and partial continuation — conditional

`PendingInterruptSet` is consumed atomically: a resume validates and consumes the whole set,
and a page never splits one (`protocol/runs.go:508-523`). That is what makes continuation
atomic.

**Recommendation:** no. If branch-independent waiting is ever required, it changes durable
pending ownership, checkpoints, partial consumption, root aggregation, and recovery
together — not a flag.

### 4.12 Recovery observability — real gap, cheap

Nothing counts replay versus cold recovery, why a replay window was refused, snapshot size,
or fence wait time. The protocol's recovery vocabulary is already precise
(`internal/delivery/problems.go:37-52`), so the classification exists; only the counting does
not.

**Recommendation:** do this alongside §5.10. It is the instrument that decides §4.5.

### 4.13 Desktop README wording — out of scope

The "stateless pure compute unit" phrasing appears in a bullet about authentication,
accounts, and multi-tenancy (`desktop/README.md:120`). The wording is loose but the claim is
about user identity, not persistence. Desktop is outside the active change boundary.

### 4.14 Long-term event log — rejected, with evidence

OpenCode's durable event table is opt-in: `const persist = options?.persist ?? false`
(`origin/v2:packages/core/src/bus.ts:203`). The project cited as precedent for permanent
event storage does not keep event rows by default.

**Recommendation:** no. Flame's durable facts plus a bounded in-process journal already cover
cold recovery and short disconnects. A second recovery path would need its own retention,
ownership, and rebuild semantics.

### 4.15 Cross-process run continuation — conditional

Recovery deliberately keeps only complete waiting trees with compatible checkpoints and
marks everything else lost. Continuing live execution across a restart requires settling
external side effects, which is a separate design.

### 4.16 Large attachments as resources

Merged into §5.17.

## 5. Side-channel API proposals

### 5.1 Watch ordinary external file edits — real gap

Three observation paths exist and none covers an ordinary source file edited outside Flame:

- `GitWatcher` watches repository metadata and `refs/heads`
  (`internal/adapter/workspace/watch.go:45-82`); a change produces a resync scoped to
  `files.changed` (`internal/delivery/workspace_stream.go:479-519`).
- `AuthoredWatcher` registers named product targets only — knowledge, hooks, skills, recipes
  (`internal/adapter/workspace/authored_watch.go:71-99`).
- The only `files.changed` event carrying paths originates from the agent's own writes
  (`internal/adapter/run/segment/workspace.go:29` →
  `internal/delivery/runtime_events.go:9-17`).

So an open file or diff panel can stay stale after an external save until something else
triggers a read.

**Recommendation:** worth doing, but half the benefit is on the consumer side (§5.2), which
is outside the current boundary. If Runtime acts alone, prefer letting a subscription declare
the paths it is displaying over recursive whole-tree watching, and define atomic-save,
rename, unavailable-watcher, and overflow behavior explicitly.

### 5.2 Consume the invalidation scope that already exists — Desktop-side

The wire event already carries workspace and paths
(`internal/delivery/runtime_events.go:9-17`). The Desktop consumer models only
`type/sequence/sessionIds/topics`
(`desktop/frontend/src/plugins/builtin/workspace/domain/eventInvalidation.ts:55-60`), so one
file change invalidates whole query categories.

**Recommendation:** out of scope here. Record it as the canonical example of a protocol that
already answers a question its consumer does not ask.

### 5.3 Run-scoped diff and change attribution — real gap, cost overestimated

`workspace.diff.get` compares the current worktree against HEAD or the merge base and takes
no run identity (`protocol/workspace_diff.go:3-27`). It cannot answer "what did this run
change".

The reports treated this as a new changeset model. It is cheaper than that: the checkpoint
store already commits and tags the working tree per run
(`internal/infra/git/checkpoint/snapshot.go:26-60`), so run-boundary before/after anchors
exist today.

**Caveat that makes this conditional:** checkpoints exclude ignored files and anything over
their budgets (`internal/infra/git/checkpoint/snapshot.go:18-22`). A run-scoped diff derived from them inherits
that coverage, so it must ship with §5.5's coverage reporting or it will present a partial
diff as complete.

**Recommendation:** viable, after §5.5. Keep the current worktree aggregate and any
run-scoped view as clearly distinct questions; do not add `runId` to the existing request and
call the result attribution.

### 5.4 Desktop rename summary — out of scope

Renames are preserved in the protocol (`protocol/workspace_diff.go:72-90`); the Desktop
summary projection discards `previousPath`.

### 5.5 Auditable restore coverage — real gap

`sessions.rollback` restores history, files, or both, files first and atomically
(`protocol/sessions.go:113-130`), but the response carries only the session and the dropped
runs (`protocol/sessions.go:132-153`). It never says which files were restored, which were
never in the checkpoint, or whether the tree changed after review. The checkpoint layer has
that information — per-file budgets and total budgets are enforced there
(`internal/infra/git/checkpoint/snapshot.go:18-22`), and restore resets to the run's tag without cleaning
untracked files (`internal/infra/git/checkpoint/restore.go:8-62`).

**Recommendation:** do this. Report the restored set, the excluded set with reasons, and what
files versus history each completed. It is the cheapest way to make an irreversible-feeling
operation explainable, and §5.3 depends on the same vocabulary.

### 5.6 Skill proposal target CAS — deliberate

`ProposalRef.Revision` is content-addressed over the proposal, and `revision_conflict` fires
when the proposal changed. There is no base revision on the active skill being replaced —
and that is documented as the design: a single archive slot, no per-version history, and a
proposal whose target has vanished simply installs as the current version
(`internal/infra/filesystem/skillauthoring/proposals.go:187-193`).

**Recommendation:** no code change. If any UI says "apply this revision to the version you
reviewed", fix the wording instead.

### 5.7 Run context provenance — real gap, medium value

`modelInvocations.list` records call, run, segment, state, usage, and latency
(`protocol/model_invocations.go:26-40`). Nothing records which skill, recipe, or knowledge
revision entered a given run's context, or which configuration was resolved.

**Recommendation:** later. It becomes worth doing when "why didn't it use my skill" turns
into a real support question. Reuse the resolution results Runtime already computes rather
than re-reading current files and inferring the past.

### 5.8 Effective configuration with origins and layers — premise absent

Codex can report an effective value, its origins, and whether a write was overridden. Flame
has no layered configuration to explain: across all 88 methods there is no config or settings
operation; configuration is typed per domain — providers, hooks, knowledge, approval,
schedules, models (`contract/API_REFERENCE.md:14-101`).

**Recommendation:** no. Importing origins and layers would mean inventing the layering first.

### 5.9 Optimistic concurrency for provider and MCP updates — real gap, no consumer

Neither update carries an expected revision, so two windows editing the same field from stale
forms silently last-write-wins. Idempotency prevents duplicate intent, not conflicting edits.

**Recommendation:** no, until multi-window or collaborative editing is a product requirement.
Other domains that need it already have it (`revision_conflict` on sessions, schedules,
knowledge, skill proposals).

### 5.10 Directory listing pagination cost — real gap, measurable

`Files.List` enumerates the whole (optionally recursive) listing on every page, then filters,
sorts, and slices at the cursor anchor
(`internal/application/workspace/file_reads.go:192-221`). Paging a large tree repeats the
enumeration per page.

This is the one item in either report that can be quantified immediately.

**Recommendation:** measure enumeration count, first-page latency, and peak memory on a large
repository before choosing between on-demand expansion, a short-lived snapshot, or an index.
Do not redesign the cursor contract, which is sound.

### 5.11 Search coverage summary — low value

Full-text search already bounds query size, match count, returned material, and total scanned
corpus, and fails rather than reporting a partial total as complete. A coverage summary would
only help a user asking why a specific oversized file was skipped.

### 5.12 Media and binary reads — net-new surface

`workspace.files.read` serves UTF-8 text windows and rejects anything else with
`unsupported_mime` (`contract/API_REFERENCE.md:43`). Image and PDF preview is a new product
capability, not a defect. If added, keep the text window contract intact and give binary
content its own operation with declared MIME, size, and range.

### 5.13 Incremental search sessions — net-new surface

Codex stops server-side work for superseded queries. At single-repository scale, request
cancellation plus client generations is sufficient.

### 5.14 MCP resource previews — net-new surface

There is no MCP resource operation in the method set. Note the property worth preserving if
one is ever added: management listing, connected tool counts, and execution publication
already share one admitted Scope snapshot, so the settings page and the model cannot disagree
about what exists.

### 5.15 Standalone process resource — net-new surface

The public direct-tool surface is deliberately limited to safe diagnostics (`tools.list`,
`tools.invoke`). A user-facing terminal or command resource is a product decision with its
own ownership, permission, output-offset, and retention contracts.

### 5.16 First-class LSP API — net-new surface

`FeatureLSP` is declared (`protocol/features.go:17-34`) and used by agent tooling; there is no
editor-facing LSP operation and no current need for one.

### 5.17 Ranged retrieval of offloaded results — real gap

Large tool results are offloaded durably, and the transcript keeps a preview string that the
protocol projects as the tool result
(`internal/delivery/presenter_item.go:155-163`, validated in
`internal/application/agent/sessions/snapshot_validation.go:38-50`). This correctly bounds
snapshot and page size.

The gap is retrieval: there is no operation that reads the full offloaded body. A client that
wants the complete output must export the entire session.

**Recommendation:** later, and build it on the existing artifact storage — a reference with
owner, size, truncation, and retention, plus bounded reads. Do not create a second copy of
the body.

### 5.18 Worktree and environment identity — conditional

Workspace, project root, relocation, and isolation exist. Stable environment identity matters
only when several checkouts or execution environments run concurrently.

### 5.19 Additional error categories — already sufficient

The `ProblemData` union covers more than twenty stable categories with per-type required
fields, paired with an explicit recovery vocabulary
(`contract/API_REFERENCE.md:144-162`, `internal/delivery/problems.go:37-52`). New categories
should arrive with the capabilities that need them.

### 5.20 Capability declaration for new surfaces — already owned

Feature flags, client opt-in, and run-protocol requirements are already distinguished, and
capabilities that shape authoritative run content are frozen at run creation
(`protocol/features.go:17-34`).

## 6. Findings the source reports missed

**6.1 A new protocol method carries a fixed CLI tax.** Every Runtime method must declare a
concrete consumer or a documented exclusion, enforced by test
(`cli/internal/adapter/runtimebinding/runtime_api_inventory_test.go:16-37`). This alone argues
against most of the net-new-surface proposals.

**6.2 Windowing the snapshot moves work into CLI.** Three aggregate-materialized methods
(`GetPlan`, `ListInterrupts`, `ListItems`) would have to become real calls. §4.5 is not a
Runtime-only change.

**6.3 Two proposals reuse anchors that already exist.** `items.list` (§4.5) and per-run
checkpoint tags (§5.3) mean both were costed an order of magnitude too high, because both
reports described them from the shape of the reference projects rather than from Flame.

## 7. Corrections to the source reports

| Report claim | Correction |
| --- | --- |
| Applied steer input is not observable | The applied path commits the Item whose ID the client already holds; only the terminal-end-without-application path is silent (§4.1) |
| Windowed bootstrap requires new history pagination | `items.list` exists with a cursor and is already documented as a snapshot component (§4.5) |
| Run-scoped change views require a new changeset model | Per-run checkpoint commits and tags already provide the anchors (§5.3) |
| Skill proposals should carry a target base revision | Full replacement with a single archive slot is documented as deliberate (§5.6) |
| Flame should explain effective configuration and its origins | Flame has no configuration layering to explain (§5.8) |
| Desktop README misstates Runtime as stateless | The sentence is about authentication and user identity (§4.13) |

## 8. Reference-project facts that survived verification

Kept because they are load-bearing for the verdicts above, each re-read in the project's own
source:

- **OpenCode separates admission from consumption.** A durable pending row carries a delivery
  policy of `steer` or `queue` and an admitted sequence; promotion is a separate step
  (`origin/v2:packages/core/src/session/inbox.ts:44-48,230-267`,
  `origin/v2:packages/core/src/session/sql.ts:108-120`). Notably the table has no status
  column: pending means the row exists, and its disappearance is explained either by
  promotion producing a message or by explicit cancellation. That is the design lesson for
  §4.1 — no state machine required.
- **OpenCode does not persist events by default** (`origin/v2:packages/core/src/bus.ts:203`),
  which is the evidence against §4.14.
- **Codex completion payloads are partial by design**
  (`codex-rs/app-server/src/bespoke_event_handling.rs:1328-1329`), which is the rule kept in
  §4.4.
- **Codex host file operations are not agent-sandboxed** — every call passes `sandbox: None`
  (`codex-rs/app-server/src/request_processors/fs_processor.rs:71-189`). This is a trusted-host
  assumption, not a model Flame should copy into any remote file surface.

## 9. Recommended sequence

**Act now — all inside Runtime, no protocol additions, no CLI migration:**

1. **§4.1** Emit the negative fact when accepted steer input is never applied.
2. **§5.5** Report restore coverage from `sessions.rollback`.
3. **§4.6** Rebuild command receipts for `runs.start` and `runs.resume`.
4. **§5.10 + §4.12** Measure directory pagination cost and recovery paths. These are the
   inputs to every remaining scale decision.

**Conditional, in this order, once the measurements exist:**

5. **§5.3** Run-scoped change view, built on checkpoint anchors, after §5.5 defines coverage.
6. **§4.5** Windowed session bootstrap, including the CLI paging migration (§6.2).
7. **§5.1 + §5.2** External file observation, planned together with the consumer.

**Not doing:** the remaining twenty-six proposals, for the reasons recorded per item —
premise absent, deliberate trade-off, net-new product surface, or contradicted by the
evidence in the very project cited as precedent.

## 10. Open questions before acting

1. Does Scope report provider retries to its caller? Decides §4.8.
2. What is the actual bootstrap cost of a long session — bytes, parse time, fence wait?
   Decides §4.5 and the priority of §5.17.
3. Is "what did this run change" a user question or a reviewer question? The answer selects
   between §5.3's checkpoint-anchored view and a narrower per-tool write record.
