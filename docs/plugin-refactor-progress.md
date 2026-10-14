# Plugin refactor progress

## Current stage

Handoff requested by the user on 2026-10-14. Development is stopped for continuation
in a new session. Read `docs/plugin-refactor-handoff.md` in this managed worktree
first; it summarizes the committed implementation, remaining full objective,
verification, failures and concrete entry points. The only uncommitted embedding
experiment in modelref/endpoint.go was incomplete and has been withdrawn; no
embedding implementation is delivered by that draft. Runtime go build ./... passes
on the restored committed source. Original inputs/stash are unchanged. The existing
worktree/branch and canonical anchor are retained; this goal is being paused at the
user's request after committing the handoff. No further product work is authorized
in this session until the user resumes it.

Latest completed product slice follows.

Stage 5ad (13193c2d6) removes the incorrect installation-only prerequisite from
restoration of an original dispatched plugin command. Background restoration now
checks its entire persisted invocation and exact installation/release/server before
adopting origin and parent. The operation owner's original Session/workspace target
is preserved. A merely prepared command, altered target/parent or foreign provider
cannot become a restored caller. No command is dispatched or business receipt read
by this projection; original routing and private mutation owners are unchanged.

Fresh unrecorded triggers still require Automatic origin, no invented parent and
the authenticated background contribution's own installation target. They cannot
name an arbitrary resource. This distinguishes a previously admitted resource
command from fresh background work instead of adding a target-specific exception.

Regression proof uses real SQLite command admission/dispatch/reopen, both dispatched
and observed phases, Session/workspace targets and Human/Model/Automatic origins.
It preserves original Actor/origin/parent/target and rejects prepared, retargeted and
unrecorded resource callers. Existing installation-target restoration/refusal checks
remain intact. This proves caller attribution; it does not recreate expired next,
stopped Scope effects or the original active member's model-accounting publisher.
Full execution-bound background Session/Run/Segment/member proof remains open and
must not be inferred from wire IDs, target spelling or the current Session.

Baseline restoration passed (0.825s); focused new restart/target checks passed
(1.144s). Whole application/integration/plugins passed (0.902s), SDK background
projection checks passed (0.719s), and Runtime build/changed gofmt/diff checks passed.
No concurrent ownership, protocol descriptor, public source signature or persisted
shape changed in this slice; no new race suite or generator run was needed. Full/
native/vet/global-race/closure and Schedule handshake remain concentrated final work.
No frontend UI/E2E/screenshot check was authored or executed.

Main 7ba40c117 is current; merging after the product commit reported already up to
date. Original proposal/review/draft SHA-256 values and user stash are unchanged.
All test/build/commit handles are terminal. Test-owned databases are closed; there
is no scratch artifact, background process, shared configuration or user-data change.
Preserve the managed worktree and canonical progress symlink for the full goal.
Core .31/backend .2/export schema3 and Scope v0.50.0 remain current. The previous
65-70%/75% workload estimates are withdrawn: carrying them across completed slices
without a requirement-level count was not a reliable progress measure. Report actual
completed implementation and named remaining work/evidence until P01-P48 and the
review have been individually audited. The full objective remains active and incomplete.
Current remaining development includes embedding evidence/metering, execution-bound
background provenance, retention/dependency/status and independent author qualification.
Concentrated full/native acceptance, Schedule handshake repair and closure gates remain.
Prefer completing these connected product paths over more fragmented plumbing slices;
use focused verification during development and concentrate broad checks at acceptance.
Next: actual execution-bound background provenance, embedding evidence/retention,
dependency/status/author lifecycle and full P01-P48/review/native acceptance.

## Completed

- Stage 5ad (13193c2d6): original dispatched/observed commands restore their stored
  resource targets; the former installation-only gate is removed for recorded work.
- Stage 5ad: fresh automatic triggers remain restricted to their own installation
  without invented parents; no target retargeting or new command/result owner.
- Stage 5ad: actual SQLite reopen and Session/workspace × Human/Model/Automatic
  attribution/refusal proof passes; affected Application and SDK checks/build pass.
  This does not close active execution-member restoration or native delivery.

- Stage 5ac (5f0a5030a): one call-scoped grant cohosts canonical RPC, published host
  collaborators and exact observer next; private observer-only path/limits removed.
- Stage 5ac: local-name replacement/leakage refusal, original next authority/cancel
  lifetime and active observer cycle refusal; unary/stream chunks and known evidence
  remain under original owners.
- Stage 5ac: generated variadic OpenRPC Go API and external rebuild documentation;
  SDK/composition/MCP suites and focused -race pass; Scope v0.50.0 main merged.
- Stage 5ac: merged Runtime/bootstrap/CLI/independent-author compile and focused
  observer/preparation checks pass. Strengthened original-cycle refusal check passed
  (1.957s) after isolated cache rebuilding; its earlier setup failure is retained below.

- Stage 5ab (8968bb1bf): installed selection/context/input phases receive existing
  call-scoped host RPC under exact receiver authority; success/failure withdraw it.
- Stage 5ab: Runtime-owned model pipeline admission is separated from arbitrary
  managed cross-plugin requirements while sharing exact contribution validation.
- Stage 5ab: immutable active-contributor cycle refusal and actual primary-preparation
  member publisher capture; no silent policy skipping, new ledger or inferred owner.
- Stage 5ab: installed MCP/authenticated reentry/stale/direct-call/cycle boundaries,
  affected backend suite, focused -race and Runtime build pass; main is current.

- Stage 5aa (086f01cc1): selected/auxiliary calls publish original consumption through
  the captured actual Model/Tool member; one common capture and fact pump.
- Stage 5aa: persisted conversation/independent purpose; independent outcomes cannot
  change primary stream, canonical message or context; known/unknown canceled work.
- Stage 5aa: role-aware different-model/unknown SQLite recovery with original source
  proof, rollback, unchanged primary context and second-boot exclusion.
- Stage 5aa: exact-schema refusal for the new logical-purpose column; all store ports,
  fixtures and caller signatures migrated; originals preserved without a reader alias.
- Stage 5aa: targeted publication/source/-race checks and affected backend suites pass;
  main 14fca7975 is merged in e1b297cf5 with Actor and opening-input rules retained.

- Stage 5z (0ad3d34e5): one per-call Run consumption transition; cumulative executor
  fields and terminal invoice deleted with all producers/clones/fixtures migrated.
- Stage 5z: resumed/interleaved calls, exact per-call price, original unknown price,
  duplicate completion refusal, primary context and unchanged terminal metrics proof.
- Stage 5z: bootstrap supplies the already selected installed Memory reader; actual
  Schedule recovery helper now reaches activation, with handshake acceptance open.
- Stage 5z: main ed0b6db55 merged in 63695693a; cancellation failure evidence and
  current Session edit/canonical contract tests preserved; incoming UI tests untouched.

- Stage 5y (6839e9c6e): independent physical source snapshot and immutable proof
  in atomic lost Run/Goal recovery; known/failed/unknown consumption without replay.
- Stage 5y: actual SQLite/Goal large-number/source/rollback/context/restart proof,
  primary exclusion and no repeated accrual; existing recovery checks pass.
- Stage 5y: main 67c9f498f merged in ec8b986e0; original Steer nomination/application
  evidence, metadata/installed route, lazy dispatch and public consumer checks kept.
- Stage 5y: combined Core .31 regenerated; shared/desktop typecheck, 75 shared SDK
  units and three static AST checks pass; originals/stash preserved, no UI execution.

- Stage 5x (b7c6f3c14): host-captured logical Model/Tool ownership replaces caller-ID
  inference; actual Session/Run/Segment active/chronological admission before generation.
- Stage 5x: namespace/identity protection, missing/foreign/waiting/stale-Segment
  refusal and actual callback source association/original client-context protection.
- Stage 5x: Segment projection/recovery validation, Core .30 regenerated; unchanged
  storage/backend .2/export3, main 3d897dfb6 integrated and originals/stash preserved.

- Stage 5w background fix (9fa2080f4): shared unary/stream callback senders carry
  original retained invocation; scoped projection accepts the original and refuses
  forged identity, while ordinary call/next ownership remains captured.

- Stage 5w (2583125cd): independent selected unary/stream callbacks, complete
  request/output budgets and explicit original invocation on the shared endpoint.
- Stage 5w: one general/auxiliary model capture, exact optional streaming,
  retained chains/final admission/cancel-join and existing physical evidence owner.
- Stage 5w: multiple selected routes, no Memory-wide count rule, full request/media,
  original identity/usage/price/restart/refusal/stream proof; public Go API regenerated.
- Stage 5w: main 3d897dfb6 already integrated; scoped schema gate still exposes four
  unchanged orphan definitions, retained for final correction.

- Stage 5v (2f9b4cc94): one final-request media owner; original source structural
  admission/evidence retained across Start/Resume/Steer and restored execution.
- Stage 5v: image conversion versus final rejection, unary/stream/compaction
  matrix, no unsupported provider counting and original resume media proof.
- Stage 5v: Core .29 regenerated/documented; main 3d897dfb6 integrated in
  43c0b765c, terminal unresolved-effect explanations/tests preserved.

- Stage 5u (48ecd6fd6): primary captured input chain/configuration/checkpoint/
  dependency/recovery and complete per-call request projection into Call/Stream.
- Stage 5u: final transformed media/capacity/calibration, original conversation/
  Options preserved; shared SelectedChat replaces the narrower auxiliary type.
- Stage 5u: direct/compacted repetition, refusal, exact counter boundaries and
  original waiting recovery proof; main up to date, originals/stash preserved.

- Stage 5t (6f5deb32d): public ordered input query, frozen selection/Tool/output
  authority and canonical contracts; complete auxiliary transformation integration.
- Stage 5t: repeated-call original conversation, exact order/caller, final byte/
  token/output admission, refusal/withdrawal and existing dependency lifetime.
- Stage 5t: main 0886453d9 integrated in 9535e180b with Goal explanation behavior
  and incoming tests preserved; desktop typecheck only, no UI execution.

- Stage 5s (b93555e64): frozen auxiliary token envelope and selected-model counter,
  output reservation/final context capacity admission before provider generation.
- Stage 5s: selected reasoning applied without request mutation; shared complete
  request estimator, known/unknown/count-failure semantics and counting cancel/join.
- Stage 5s: main 8bb503512 integrated in 611ff1749 with original Run admission and
  installed receipts retained; Core .28 generated contracts/samples and client gates.

- Stage 5r (c69190524): public explicit installed selection query, exact live
  binding/original caller and refusal instead of implicit discovery/fallback.
- Stage 5r: fresh Run selection before final model admission, identical selected
  executor/durable Run identity, Session preference and original input preserved.
- Stage 5r: accepted Start/Resume no-reselection and utility selection before
  provider construction; one production utility resolver and shared real fixtures.

- Stage 5q (0e538bc42): captured auxiliary exact role/model plus explicit caller
  budget; all internal resolver callers migrated without the bare-model signature.
- Stage 5q: installed context/observer order shares one use-case lifetime and
  dependency retention; final text-byte/output admission before provider I/O.
- Stage 5q: repeated-call nonaccumulation, pre-dispatch failure/withdrawal/limits,
  original request/Memory phase identity and cancel/join/release behavior verified.

- Stage 5p (41076eb45): public ordered primary model context query, exact installed
  admission/original attribution and failure before provider dispatch.
- Stage 5p: one measured/sent ephemeral prefix+tail record, authored conversation
  preserved across repeated calls, checkpoint/recovery and dependency retention.
- Stage 5p: main 414668baa integrated in fb525f9a9, one shared durable installed/
  Start/Resume/Steer command owner with no old Steer-only representation.

- Stage 5o (fce9dd9af): retained physical chat evidence is the usage report source;
  no Run aggregation fallback or duplicate primary accounting remains.
- Stage 5o: auxiliary/unattached/active/unknown coverage, exact provider/served
  model and admission day/origin/association groups, known versus exact cost.
- Stage 5o: transactional streaming scan/ownership checks, coherent refusal,
  migrated public Go/TS/CLI/component/narrow agent projection and canonical sample.

- Stage 5n (b997cb3d5): physical pricing owns the known call cost; primary
  Interaction consumes the recorded price and drops independent recomputation.
- Stage 5n: existing boot recovery atomically accrues uncommitted known physical
  evidence and settles failed logical calls; unknown remains unknown, with no replay.
- Stage 5n: original JSON/value/source CAS, immutable evidence projections and
  strict stored-usage agreement; optional cost query/export/component projection.
- Stage 5n: main 1d5d98774 integrated in da3f344df, preserving original Steer and
  installed command recovery with one current acceptance policy and journal.

- Stage 5m (9c8f667d9): physical endpoints join the existing Session trajectory
  union and idle export, with exact JSON evidence and source capacity accounting.
- Stage 5m: original Session/Run read projections, Session-only observations,
  descendant filtering, ordinary component presentation and schema 3/Core .25.
- Stage 5m: atomic Session endpoint cleanup, missing/conflicting Session admission
  refusal, actual title Session capture and storage encoding outside Domain.

- Stage 5l (c895cfb48): current Interaction known usage derives only from exact
  persisted endpoint evidence; outer missing/changed usage cannot originate billing.
- Stage 5l: one completed/failed fact advances accrual once; known source evidence
  survives missing outer response and precise caller lookup ignores ancestry paging.

- Stage 5k (7ede472a3): actual primary/auxiliary/Memory chat endpoint requests and
  full known outcomes persist beneath outer observation in the existing journal.
- Stage 5k: distinct physical identity/original ancestry, provider/model/request,
  no repeat dispatch for the same caller and unknown restart reconciliation.
- Stage 5k: public Go/TS endpoint query projects original JSON; lazy stream stop
  preserves known completion without promoting operation success.

- Stage 5j (23f6e12e7): auxiliary and Memory utility consumers freeze the public
  observer choice with caller-owned release; one Memory capture spans both calls.
- Stage 5j: active captures contribute held releases through existing admission;
  close cancels/joins before retirement and original captures reject later reuse.
- Stage 5j: public model Call/Stream callbacks preserve ancestry with per-command
  call identity; actual Memory extraction/curation have separate original children.

- Stage 5i (574a3e183): installed transparent observer chain is wired to production
  Interaction, exact provider instance, staged identity and installation retention.
- Stage 5i: original ordered observer bindings survive checkpoint projection and
  recovery; call-bound next, bounded forwarding backpressure and producer joining.
- Stage 5i: repeat/transform/withdrawal refuse and known unary evidence survives
  outer failure; no fabricated stream capability or separate command receipt owner.

- Stage 5h (c29e16107): one durable explicit ordered decoration choice with complete
  contract identity, public Go/TS management/resolve and immutable release bindings.
- Stage 5h: declared decoration dependencies now use the same order in setup;
  withdrawal/contract drift refuse, required/optional empty semantics are explicit.

- Stage 5g (9d1702458): Interaction retains complete known response and matching
  usage on outer failure in one existing failed fact/row; no successful continuation.
- Stage 5g: durable reopen, list/trajectory/export projection and evidence byte
  accounting; Core .22 and exact host schema updated with explicit old-data refusal.

- Stage 5f (06890e847): actual Memory completion uses captured public Scope model
  request/response and strict backend text consumption; the private string model
  multiplexer is deleted and embedding is an independent private method.
- Stage 5f: private method/input budget fencing, known refusal versus response-plus-
  failure, maximum escaped corpus and production maintenance checks pass.

- Stage 5e (ad60cfaa7): TS plugin mutation streams use canonical awaited chunk
  consumption and final decoding under the existing original intent journal.
- Stage 5e: per-request HTTP consumption backpressure preserves nested RPC progress;
  closure cancels/joins async consumers and missing final stays unknown without replay.
- Stage 5e: client replacement restores original binding/input/key before preparation;
  component SDK typed chunk-map stream entrance compiles, unsafe numbers are refused.

- Stage 5d (dc321990d): typed installed mutation streams share unary immutable
  binding admission and use SDK StreamValue with frozen input/options before I/O.
- Stage 5d: schema/strict chunk/final decoding, single consumption and final-after-
  source-completion; transport errors preserve their owner's known/unknown semantics.
- Stage 5d: callback streaming captures the original reference/envelope and owns
  lazy Client creation/closure; retired/missing context and external Go compile pass.

- Stage 5c (14c1003c8): public Go/Protocol installed mutation stream through the
  same delivery endpoint, original owner-response command semantics and no replay cursor.
- Stage 5c: distinct chunk/result/refusal frames and Go remote terminal admission;
  missing/malformed terminal remains unknown without automatic reconnect/resubmission.
- Stage 5c: HTTP yield acknowledgement follows successful frame writing; source
  cancellation/join, generated contracts and exact public Go signature checks pass.

- Stage 5b (b84d739b7): internal installation-qualified command streams share
  existing original route admission and durable Commands ownership with unary calls.
- Stage 5b: captured caller/target/input/provider and known final/refusal observation;
  missing receipt, stop and withdrawal retain uncertainty without replay or retargeting.
- Stage 5b: host database reopen reads the original private receipt; affected unary
  admission and SDK duplex checks pass. Public/production streaming remains open.

- Stage 5a (8d5844512): actual SDK backend stream registration and typed/export-bound
  stream invocation, original MCP final result and acknowledged Runtime callback chunks.
- Stage 5a: one in-flight chunk/demand backpressure, order/schema/byte checks, cancel/
  join and one terminal publication; partial failure remains unconfirmed without retry.
- Stage 5a: frozen input/raw metadata and real stdio executable decoration/model-next
  reentrancy with known inner response/usage; Runtime route/production integration open.

- Stage 4z (5cf6acb68): typed callback stream schemas/chunk/final identities and
  feature Go/TS/canonical/documentation generation; Core optional stream metadata.
- Stage 4z: same callback endpoint/SSE codec, shared original admission, lazy frozen
  request, terminal-after-source ordering, bounded writes and stop/cancel/source joining.
- Stage 4z: canonical Scope aggregation, captured byte budget, completed inner
  evidence plus outer failure and external executable frozen remaining-chain proof.
- Stage 4z: reject stream methods at unary Go/installed/TS admission; installed duplex
  decorators, production consumers, TS streams and durable inner observation remain open.

- Stage 4y (d33aa1482): public Go SDK complete Scope model request/response callback
  contract, caller-owned capture/admission and one-delivery explicit identity.
- Stage 4y: strict outcome schema preserves valid known inner response alongside
  outer failure, with causal cancellation/deadline and absent/zero usage distinction.
- Stage 4y: external-package subprocess, real cancellation/owner shutdown, invalid
  input/retired-reference and detached full-response evidence checks; product/stream
  and installation/TS author migration remain explicit unfinished work.

- Stage 4x (a6913d901): generic plugin-data change for every original installed
  command and authenticated backend source; remove closed first-party observer and
  business-specific topics/IDs/constraints/sample/generated exports.
- Stage 4x: installed Tool notification no longer recognizes the Schedule contract;
  private commits retain their post-change publication and no result is synthesized.
- Stage 4x: CLI/workspace/ordinary Memory/Schedules reread their business owner;
  Memory external-change observation tracks/cancels its component generation.

- Stage 4w (108e8806b): remove Core Memory capacity export/import; publish Capacity
  from the private owner and derive input/page-row width bounds from one constant.
- Stage 4w: complete CLI/Desktop traversal uses captured publisher capacity/default
  width, rejects drift/overflow/continuation failure without partial success.
- Stage 4w: regenerated canonical/Go/TS/ordinary ESM, actual backend/default-width
  contract checks and migrated existing fixture source; notifications remain open.

- Stage 4v (d3551b81a/c69a76031): remove Core Memory records/enums/constraints/
  schema/sample/exports; actual CLI/Desktop presentation owns its read vocabulary.
- Stage 4v: public publisher identity/content/vocabulary admission derived from the
  private Go owner, regenerated Go/TS/canonical JSON and ordinary Memory ESM.
- Stage 4v: complete native snapshot comparisons, visible-state/time regression and
  actual consumer/source migration; capacity/topic ownership remains explicit work.

- Stage 4u (ba903e64b): actual CLI/workspace public additions with one original
  authoring intent, captured path/content and pending-before-current-target recovery.
- Stage 4u: publisher Changed/snapshot/scope validation before settlement, private
  root/deduplication ownership and current-owner cache reread.
- Stage 4u: remove last Core Memory facade/DTO/handler/forwarder/feature; migrate
  actual source fixtures, preserve original native retries and fresh new-command binding.

- Stage 4t (7962f8f35): CLI/workspace public update for content/pin/unpin with one
  frozen item intent, nullable patch distinction and original request recovery.
- Stage 4t: public snapshot decoding before retirement; pending-first restart and
  changed-draft refusal; current-owner cache reread without cross-provider projection.
- Stage 4t: remove Core update/DTO/Go/TS/port/forwarder and generated artifacts;
  migrate actual fixture sources while preserving private business ownership.
- Stage 4s (59e723b11): CLI/workspace public Memory review with original request,
  decision, binding and receipt identity; approve/reject share one intent per item.
- Stage 4s: resolve pending before new target/catalog reads; exact/prefix recovery
  refuses ambiguity and changed decision. Actual CLI original-route/restart and
  pure recovery-policy regressions pass without UI execution.
- Stage 4s: remove Core review/DTO/global enum/Go/TS/port/forwarder and generated
  artifacts; retain private business ownership and expose actual publisher refusal.
- Stage 4r (00f2a5af): Desktop workspace Memory deletion through the generated public
  operation, stable journal-owned original intent/binding and typed acknowledgement.
- Stage 4r: remove Core deletion/DTO/Go/TS/port/forwarder and generated artifacts;
  preserve private business/storage/receipt ownership and migrate existing consumers.
- Stage 4r: factory tracking fences retired delivery creation; host refusal/decoder
  failures cannot retire original business uncertainty. Shared OperationFailure,
  exact serialized input, current-owner cache reread and one-client target capture.
- Stage 4q (0b626d89): actual CLI Memory deletion through generated public installed
  input, exact published binding and workbench-owned original request recovery.
- Stage 4q: recover full/prefix pending identity before new target/catalog reads;
  persisted restart, provider replacement and invalid acknowledgement regression
  passes. Retire the CLI Core deletion dependency; Stage 4r subsequently completes
  Desktop migration and Core deletion retirement.
- Stage 4p (fcfbfa37): actual CLI/workspace public Memory reads with one captured
  provider/contract across pages, bounds/refusals and generated publisher projections.
- Stage 4p: shared CLI exact-contract/protocol admission for real Memory/Schedules
  consumers; no reconstructed metadata or selected-provider refresh during paging.
- Stage 4p: remove Core Memory list/DTO/Go/TS/workspace wrapper/port/forwarder and
  generated artifacts; migrate existing process/contract/graphical source consumers.
  Necessary focused checks pass; final native execution and mutation cutover open.
- Stage 4o (5d928073): public CLI update/edit/enablement with original captured
  revision/patch/binding and pending-intent recovery before new catalog reads.
- Stage 4o: direct generated publisher snapshots; no Core read DTO/Enabled copy;
  workspace reset/omission and complete Go uint64 revision retain their semantics.
- Stage 4o: remove all remaining Schedule Core API/DTO/feature/error/projection
  artifacts and unused routing dependency; migrate native/terminal sources/docs.
  Focused owner/contract/type/build/source/locale checks pass; final execution open.
- Stage 4n (15f8135e): actual CLI public creation with typed publisher input,
  workspace/caller-state capture and durable original-intent reconciliation.
- Stage 4n: shared generated CLI contract projection with exported metadata and
  package-scoped names; delete adapter-local generation without contract drift.
- Stage 4n: remove Core creation API/request/Go/TS/handler/port/selected-role facade
  and exclusive cases; migrate actual process/terminal fixture sources and docs.
  Required focused compilation/source/type/build checks pass; native execution open.
- Stage 4m (5223d13f): actual CLI manual execution uses the public installed method,
  generated input/output, original durable intent and accepted Core reference.
- Stage 4m: decoded acknowledgement precedes retirement; decoder failures retain
  original intent even when shaped like a business refusal. No result store added.
- Stage 4m: delete Core run-now Go/TS/protocol/handler/port/selected-role callback
  path and obsolete fixtures; preserve backend/Core ownership. Generated Core .9,
  consumers and docs updated; focused checks pass, final native execution open.
- Stage 4l (89634015): CLI Schedule deletion calls the original public installed
  operation; durable pending intent precedes dispatch and survives client reopen.
- Stage 4l: original provider/release/contract/target/input/key recovery, replay
  store/deadline fences, binary input projection and owner-only settlement; no
  business result cache. Persistence and acknowledgement failure checks pass.
- Stage 4l: remove Core schedules.delete/Go/TS/request/port/selected-role facade;
  generated Core .8 and affected callers/docs updated. Production/source/type/build
  checks pass; full native process recovery remains final-phase work.
- Stage 4k (44baab0b/69255d53): real CLI public installed Schedule reads with one
  captured provider/contract across pages and exact published protocol refusal.
- Stage 4k: delete Core schedules.list/Go/TS/port/selected-role read forwarding;
  generate typed CLI Go from the publisher contract used by canonical JSON and TS.
- Stage 4k: preserve embedded public pagination owner generation; migrate backend
  native/source probes and remove retired frontend facade cases. Necessary focused
  checks and production compilation pass; final execution and mutation migration open.
- Stage 4j (9f30bd16): ordinary contextual Session component placement, local menu
  contribution and frozen accepted target; Trajectory owns its canonical naming form.
- Stage 4j: remove closed action declaration/enum/array, wrapper API/DTO/Go/TS/CLI
  path and private Host form/cases; existing Session revision/receipt owner retained.
- Stage 4j: Core .6/generated contracts, actual author/client/module docs and retired
  locale strings updated; focused contracts/source/type/build checks pass, final
  canonical restart/native execution remains open.
- Stage 4i (3f18d71b): public recorded Tool-result ESM and frozen SDK context; retain
  exact/shape matching, generic inspection, filtering, rows, ANSI and original evidence.
- Stage 4i: delete all remaining HTML renderer APIs/models/resources/builders and
  browser/native guests, IPC, appearance bootstrap and exclusive probes/tests/tasks.
- Stage 4i: derive mutable client Tool projection from public record types; preserve
  resource error causes/cancellation and migrate security regression to ordinary JS.
- Stage 4i: Core .5/generated consumers and public docs updated; source/type/build and
  focused checks pass, native test sources compile; final execution remains open.
- Stage 4h (d7ae9dee): public file-scoped Markdown ESM component, frozen accepted-read
  context and normalized selector admission; preserve source, ambiguity and bounds.
- Stage 4h: delete file HTML source/resource/builder and graphical carrier/read cases;
  use common exact-release modules, shared React/UI/SDK and bundled declared Marked.
- Stage 4h: Core .4 and generated consumers/docs/build migration; focused backend,
  production build/type/resource/source checks pass; full/native execution remains open.
- Stage 4g (00ea6b5f): backend-owned minute scan, fair private capture/recovery and
  original caller/target; cancel/join activation resources and independent accepted Runs.
- Stage 4g: successful setup precedes explicit idempotent activation attempt; delete
  host Dispatcher/Bootstrap scheduling join/firing port and scan/dispatch public exports.
- Stage 4g: resume original private manual command/input and atomic runNow receipt;
  actual public notification with managed contribution authority and existing resync.
  Native full lifecycle/non-selected-entry acceptance remains outstanding.

- Stage 4f (7873332f/51602f5d): explicit background declaration/setup target and
  activation-owned callback resources; Schedules read-only namespace admission and
  joined callback teardown. Its scheduling loop ownership remains open.
- Stage 4f: existing Commands owner validates retained origin/parent/target against
  original dispatched routing and authenticated Actor; fresh automatic invocation
  policy and actual HTTP refusal regression. No result store or route transition.
- Stage 4f: pure capability.Refusal replaces plugin.Failure with all callers migrated;
  setup transport DTO/identity validation and coverage-tool placement repair prior
  dependency-rule violations without changing the rule.

- Stage 4e (6e0623fb): bound editable Schedules component using generated installed
  operations, complete manual Run operation, captured revision/model/workspace and
  accepted pagination; original uncertain command handle and owner-notice reads.
- Stage 4e: remove the last business Views/HTML/template declaration path and private
  Desktop Schedule controls; migrate real component/resource and native fixture source.
- Stage 4e: source coverage includes actual CLI Go calls without operation exemptions;
  backend negative coverage rejects retired View declarations/APIs and invalid resources.
  This does not close the remaining Core-facade or background-role migration.

- Stage 4d (ca770e48): complete backend-owned manual Run operation over the existing
  private command/capture/acknowledgement and canonical Runtime Run acceptance owners;
  remove the separate prepareManual export and host two-step manual workflow.
- Stage 4d: original command/caller identity, deterministic capture lookup, immutable
  pending input and atomic private receipt completion; generated capability migration
  and real SDK/private-owner regression. Native process source migrates; final execution
  and backend background-role ownership remain open.

- Stage 4c (0c0ecad2): ordinary Skills component over canonical public reads and
  library/proposal commands, frozen target/revision confirmation, original uncertain
  command handle and owner-notice subscription with canceled/joined I/O.
- Stage 4c: delete its HTML/kind/four read APIs, private controls/curation generation/
  gateway and unused read projections; migrate backend lifecycle/curation fixtures.
- Stage 4c: shared SDK Core-error classification preserves Host class identity;
  API coverage includes the actual production component project and remains strict.

- Stage 4b (3ce298ec): common command-route consumer invalidation for public and
  feature entrances, exact contract matching, unknown-outcome refresh and removal
  of duplicate Memory/Schedules adapter publishers.
- Main integration (712801fb): retain 993eb865's original Schedule Tool recovery
  through admitted raw receipt evidence, full export/source authority and the
  existing connection waiter. Migrate its process fixtures to current receipts.
- ecda9dc8: original command conflict precedes changed Run lookup; fresh trigger
  admission uses the transaction context, and replay retains caller validation.

- Stage 4a (bdf26566): ordinary Memory component with actual own installed typed
  query/commands and human editing/review, immutable target capture and original
  uncertain-command handle; delete its HTML/carrier/private controls/duplicate query.
- Stage 4a: admitted client scope with optional Session/workspace targets, no file-
  focus dependency; preserve strict workspace focus behavior and explicit Diff narrowing.

- Stage 3z (a025360f): public client-local telemetry collaborator and one SDK row-type
  contract; actual Diagnostics ordinary ESM, owner-directed clear, detached reads and
  retired-context refusal. Generic navigation consumes the admitted diagnostics key.
- Stage 3z: delete Diagnostics view/HTML/carrier/omission format and transport-specific
  read cutoffs; preserve current telemetry owner/capacity and Runtime accounting.

- Stage 3y (0643197b): actual ordinary Trajectory component, public immutable Session
  target/dock projection and direct canonical history reads; delete its closed view,
  duplicate read entrance, HTML/carrier and obsolete GUI cases in the same change.
- Stage 3y: preserve canonical history/descendant/cursor/evidence regression source,
  exact-release asset admission/restart/disablement checks and independent history
  retention. Process source compiles; final execution is queued.

- Stage 3x (fd1a9161): canonical raw point contracts through projections, public
  Go/Protocol/CLI/TS query and actual built-in manager inspection; exact large numeric
  schema identity survives projection and request serialization. Old object shape removed.
- Stage 3x: actual HTTP resource-to-query projection derived from the route catalog,
  generated metadata and source-consumer evidence; Desktop API gate passes 109/109.

- Stage 3w (2b692d52): existing public provider APIs gain shared TS bindings and real built-in
  manager controls; declared dependencies have actual public binding inspection.
  Unknown commands retain identity; Runtime alone advances provider selection.
- Stage 3w: DataQuery provider projection/notice invalidation, joined/canceled I/O,
  generation-scoped observers and shared technical contribution join boundary.
  Localize module recovery and management copy; remove retired carrier keys.


- Stage 3v (a8d757ec): public frozen workspace/focus context, workspace scope admission and
  generic navigation metadata; actual ordinary Diff ESM over canonical Runtime reads.
- Stage 3v: delete Diff HTML/carrier/read wrapper and Host baseline preference.
  Preserve Git/path/refusal/rename/binary/truncation/restart/withdrawal/file retention
  regression source at canonical query and resource owners; final lifecycle queued.


- Stage 3u (9376b997): ordinary Usage ESM and direct canonical public query, local period/group/
  filter/refresh state, cancellation and joined I/O, preserved accepted read on failure.
- Stage 3u: remove the duplicate usage read operation/request/client binding and closed
  kind, generated contracts, HTML source/resource and Desktop carrier adapter. Preserve
  backend zero/unpriced/range/restart/uninstall accounting regression source at its
  current canonical owner. Dispatch execution passes; final lifecycle execution queued.


- Stage 3t (a3661279): component-only declaration admission and optional callable SDK context;
  ordinary Icon gallery ESM from canonical icon exports, local Hooks filtering,
  shared host imports and reproducible package-resource build.
- Stage 3t: delete static HTML/source generation, static Domain/Protocol kind and
  Desktop carrier dispatch; update current docs and retired carrier fixtures.
  Appropriate final graphical execution is not claimed.


- Stage 3s (61180b25): declared ordinary components, public resource query/subscription,
  subscription-owned URL authority, current enabled exact-release/package integrity
  checks, HTTP module/asset projection and preserved original caller context.
- Stage 3s: real Reviews workspace component registration, host shared ESM import
  map, captured installed binding, module evaluation/generation reload policy,
  rendering error boundary, observer cancellation and Dougong joining of I/O.
  Production source/build evidence only; graphical execution remains unproven.


- Stage 3r (fd9b754b): public `plugin-sdk`, typed bound query/command authoring, same-key retry
  projection, ordinary Reviews Board and prebuilt ESM artifact. Compilation/build
  evidence only; actual component loading remains open.
- Stage 3r: canonical raw contract JSON transport replaces the public call's object
  field. Go/CLI/TS consumers and current generators are migrated; large integer
  schema lexemes remain exact. Necessary generator/Delivery focused checks pass.


- Stage 3q (af081e07): pure point admission/composition, immutable installed value resolution,
  public Go/Protocol/CLI/TS query and typed consumer helper; current production and
  owning test source compile, focused contract/value checks pass.
- Stage 3q Reviews: real installed analyzer operation and separate deterministic
  producer, named optional setup dependency, consumer-owned finding admission and
  generated public contracts. Focused routing/evidence/producer checks pass;
  installed process integration remains queued.
- Stage 3q main integration (765ccacd): preserve main 7957a603 Resume acceptance/current Run
  projection, actor-scoped command keys, captured new Sessions and original-request
  verification. Adopt its accepted/retired/notRecorded vocabulary in Schedules
  and preserve catalog-derived TS policies. No retired AutomationStarts or
  ResumeClaims owner was restored.


- Stage 3p: immutable named entry bindings, owned provider-attempt waiting, real SDK setup delivery, returned resource/cancellation joining, frozen managed actor permissions and call-scoped public installed-operation callbacks; f82cad37. SDK setup lifecycle execution passes; full installed acceptance remains queued.
- Stage 3p main integration: ec674681 retains main's accepted-continuation recovery while preserving actor authority, Commands/ProviderSelections and removal of retired automation/resume-claim owners. Production builds and affected test source compilation pass.

- Stage 3o: required declaration binding checks before launch acquisition, exact live export/method admission before publication and existing-owner retirement on refusal/cancellation/panic; committed in 591d2004. Production build and owning test compilation pass; behavior execution remains queued.

- Stage 3n: admitted immutable named requirements, exact selection/collection projection, explicit optional absence, required entry-cycle paths, public requirements query and fresh cross-plugin declaration checks; committed in 990c2afd. Final behavior execution remains queued.

- Stage 3m requires admitted original caller evidence when constructing Schedule ManualRun/Claim and when scanning fresh due work. Occurrence and RunRequest snapshot/restoration preserve it; a scanner's current Invocation cannot overwrite a previously captured manual/cron request. Backend preparation/scan read it from the SDK calling-owner context, outside business input.
- Stage 3m persists cron invocation_json with the consumed cursor/occurrence in one transaction and manual captured_request_json with the queued request. Immutable SQL guards protect original evidence. Schema admission refuses former columns; data and uncertain prior command rows are not rewritten. No host copy, new result ledger or separate Run state owner was introduced.
- Stage 3m restores the request's original ID/origin/parent/target at the actual host dispatcher before binding the existing Core callback. It verifies installation association; backend dispatch compares its received context to private original work before command/Runtime I/O admission. Core Run acceptance and private dispatch-reply/queue acceptance transactions retain their existing owners.
- Schedules typed method/Schema/capability/TS/reference artifacts are regenerated from the existing source. Current protocol remains .4; the changed full capability identity strictly rejects former snapshots. Updated actual constructors, scanner/storage/dispatch/Run-origin fixtures compile. Added persistent caller/restart/immutable guard and old-format non-mutating refusal regression source; final execution is queued.

- Stage 3l adds public admitted capability.Target/Invocation values and private calling-owner propagation. Target is explicit in CallPluginRequest and PluginBinding, admitted against method scope and the installation where applicable. Origin is absent from caller-authored business DTOs and request metadata; actual adapters establish it.
- Stage 3l projects Interaction's durable logical Tool ID and member parent, serializes the captured installation target at the managed connection boundary, and exposes InvocationFromContext to backends. Old invocationId metadata and InvocationKey disappear with their real SDK/Reviews/transport fixtures and docs. Callback metadata cannot replace an already captured calling-owner context; callback credentials remain outside durable input and invocation evidence.
- Stage 3l stores full calling-owner evidence in the existing host command route and immutable SQL guard. Original lookup/dispatch/receipt/observation owners remain unchanged. Domain restoration validates identity, target scope and installation association; different target/origin/parent with the same command identity conflicts. Current host schemas refuse former routes without invocation_json rather than transforming unknown old commands.
- Core and backend/declaration protocols advance to 2026-10-13.4 for the target and Tool-context contract changes. Existing generators regenerate Core, Memory, Schedules and Reviews artifacts; all ten bundled declarations and independent Reviews require the current protocol. Typed business input/private receipts remain with their previous owners.
- New/updated backend regression source covers nested model origin/parent preservation, actual metadata delivery, forged context refusal, cross-entrance command-origin conflict, original storage restoration and incompatible host admission. Current test binaries compile; execution is deferred.

- Stage 3k adds one installed unary envelope to the ordinary operation catalog. Its extension registration resolves query versus owner-committed command policy before invoking the owner; caller kind relabeling cannot pass exact local/live contract admission. Go in-process/HTTP bindings, CLI and the shared TS RPC path consume it.
- Stage 3k exposes BindPluginMethod over existing typed plugin.Method contracts and captured PluginBinding. Typed admission remains Go-owned; the public boundary carries raw JSON only inside the envelope. Definite owner refusals preserve code/message; failure/output are exclusive and transport/unknown acknowledgement stays an error.
- Stage 3k extracts the original raw command invocation from the feature-typed wrapper. Both paths retain the same durable route, dispatch gate, exact export, receipt query and observation owner. The installed entrance checks the immutable installation/release/contribution/capability before recovery; changing a selection does not retarget an accepted command. No additional result ledger, compatibility reader or alternate transport was introduced.
- Added regression source for shared-endpoint query/command admission and actual Memory cross-entrance receipt recovery/binding conflict. These tests compile and are queued for final execution; no runtime pass is claimed.

- Stage 3j removes the closed service definitions/parser/wire enum and actual consumer routing. Release.Contributions and ContributionID admit generic contract/export bindings; shared servers may implement distinct declared contributions. Selection, distribution defaults and durable selection storage use these IDs rather than server names.
- Stage 3j binds named Memory/Schedules capabilities and validates full contracts/live exports on fresh resolution. Original command storage and immutable SQL guards retain full capability, contribution ID and export; restoration neither discovers another provider nor uses a bare-method call. Feature-specific Tool policies remain with their real consumers, matching their required contracts.
- Stage 3j moves all ten bundled Flame declarations into namespace-local files referenced by root plugin.json and adds the same metadata to the independent Reviews package. Root Agent Plugins identity and mcp.json launch configuration remain authoritative. Own-namespace unknown fields/old inline apiVersion invalidate Flame entries while portable MCP/Skill components remain independent. Current closed presentation DTO/carrier semantics still require actual component replacement.
- Stage 3j migrates builders, Runtime Go/Protocol response projections, generated contracts, typed query metadata and affected API fixtures. Core Protocol is now 2026-10-13.3; backend/capability protocol remains io.github.tangerg.flame/backend/2026-10-13.3. No old service alias, old declaration reader or old data conversion was added.

- Stage 3i removes Grant from actual Memory business DTOs, typed/generated method input schemas and host/private routing inputs. Dispatch creates/releases metadata over the original concrete session; completed receipt recovery remains Grant-free. Host and shared Memory/Schedules command stores use business_input_json and refuse former call-context input schemas without conversion.
- Stage 3i also propagates configured human Search collaborator-resolution failure, while an absent embedding role still supports keyword ranking. New-call local admission validation prevents a compacted request without required metadata from creating pending work; it never gates existing pending/completed reconciliation. The final guard and Schedules caller update are compiled; execution coverage is queued for concentrated verification.

- Stage 3h adds feature-owned typed Contracts for Memory's eight methods and Schedules' seven methods. Actual backends register those immutable methods directly; previous name-based business registration calls disappear. Host Memory list/items/search and Schedules list consume those same typed sources.
- Private receipt RegisterCommand now requires a typed command Method and derives its semantic name from that admitted owner. Its former string constructor signature is removed without an overload or alias. Nil/query registration is refused before publication; original input, mutation transaction and private receipt behavior are retained.
- Separate feature author commands emit their canonical descriptors into the existing contractgen. Generated capability/Schema/method/TS/prose are package projections; no business feature import was added to generic contractgen. Package construction copies Flame namespace resources with the executable.
- Permanent generated/source and generated/live tests compare complete method identities for actual Memory/Schedules backends and execute their typed List queries. They prove the required method contracts, not a named live capability export: BindCapability and generic installed contribution resolution are still pending.
- Complete native packages preserve every generated contract artifact byte-for-byte. Actual Memory package is 15,054,524 bytes and Schedules is 70,941,454 bytes, both below the existing 128 MiB aggregate bound. Owned temporary native outputs are removed.

- Stage 3g moves callable MethodDescriptor/MethodKind, Protocol, composition metadata and immutable Contract/Method admission to public runtime/capability. That owner validates bounded semantic identity, closed kind/composition, self-contained schemas, non-null input, required membership and canonical identity; projections copy mutable schema bytes.
- Typed SDK methods, Backend publication, Binding dispatch/receipt lookup, capability export matching and unary callbacks retain admitted pure Method objects. The transport no longer stores competing descriptor/schema/fingerprint copies or defines admitDescriptor/admitCapability/methodContract/cloneDescriptor. Public SDK aliases and former capability constructors are removed.
- Actual Reviews, Memory callbacks, original Memory/Schedules command restoration, feature generation and public Go API generation consume the new owner. Reviews regeneration preserves canonical capability, Schema, method references, TS token/data maps and prose byte-for-byte. A source API relocation alone does not change backend protocol .3 or persisted callable JSON.
- Pure-owner tests cover malformed local metadata/schema/refusal, deep-copy projection isolation, stable order/prose-independent identity, original large numeric input and concurrent structural admission. Architecture gates allow Domain/Application to consume the pure owner while still refusing SDK transport imports; its actual dependency closure contains no Runtime internal packages, MCP SDK or Scope MCP transport.
- Runtime/CLI full test/vet/build, changed-owner/generator/feature races and real Reviews original-export/committed-response loss, Memory pending command and Schedules acceptance/recovery races pass. A temporary offline module proxy containing only capability/plugin/jsoninput production source proves independent Reviews tidy/race/vet/build with GOWORK=off, no replace and no Runtime internal source/imports. It is source evidence, not a published release or full author parity.

- Stage 3f deletes Registry.Service and the sourceRegistry.Service requirement, MemoryContext and Memory/Schedules branches from the generic connection owner. Bootstrap now composes the actual feature preparation boundary rather than implementing Memory's scope/grant body. No string-based substitute or second registry was added.
- Generic tool preparation captures a call closure plus its resource release. Initial source admission precedes preparation; final source/configuration/session/cancellation admission precedes invocation. Refusal releases preparation exactly once without running the call or its post-dispatch observation. Concurrent source-read and cancellation errors preserve both causes.
- The actual feature boundary matches the executable's captured installation/release/authority/server/credential recipient against the installation owner. Read failure, withdrawal or mismatch cannot fall back to portable unprepared invocation. Memory clones original logical-call metadata, binds the captured workspace and exact callback session generation, and releases the grant after any settled or uncertain reply. Schedules invalidates owner reads only from the actual dispatched-call closure.
- Configured Memory collaborator-resolution failures now remain explicit. The owning resolver's nil,nil unconfigured-role result still permits keyword retrieval; backend optional embedding-request fallback and storage-error semantics are unchanged. The change is recorded in current Memory and Runtime documentation rather than in the read-only proposal.
- Deterministic preparation-gate races cover disablement, configuration change, session detachment, cancellation, source-read failure and invalid prepared-call output: no remote dispatch or settlement observer, exactly one resource release. Real feature transport tests prove retained metadata, Memory callback reentrancy and expiration, Schedules uncertain-reply notice and no unprepared fallback on installed-source read failure. A real managed Memory Run reads its original workspace fact with no embedding role.

- Stage 3e adds Capability as a validated immutable consumer contract: current public backend protocol, semantic reference, composition metadata, scope meaning and exact required methods. The one method/schema admission path validates local metadata; descriptors and schemas are deep-copy projections. Canonical method ordering is stable and callable fingerprints exclude prose.
- BindCapability associates named exports with existing Backend handlers after checking every required method. Publication remains single-assignment. Discovery validates the complete export against the consumer contract and the actual published method set. Dispatch gates export/capability fingerprint, membership and exact method fingerprint; associated members cannot fall back to bare invocation or receipt lookup.
- Method.Bind supplies a typed bound proxy using the existing admission/invocation/outcome algorithm. NewCapabilityBinding restores the original export/contract without discovery. Receipt queries retain that export, enter the existing private receipt owner and validate original output; no mutation/result store or new command identity is added. Shared handlers retain one semantic method/receipt namespace across exports.
- Reviews owns one capability in NewContracts, reused by server publication and contract generation. The generator now requires a capability descriptor, emits the canonical capability.generated.json and derives its TS token from that canonical fingerprint. Rewritten Schema and method references remain presentation projections. The former bare-array generation input is deleted and explicitly refused.
- A real Reviews stdio capability command commits while a probe withholds its response. After backend restart, original-export restoration reads the same receipt. A second transport probe would refuse and record any mutation resend; no such request occurs. Existing Runtime/Tool lost-response recovery now also retains the original capability export.

- Stage 3d moves captured pending → canonical Run acceptance → private acknowledgement into Schedules. The executable main package translates its pure admission port through ConnectCallback and the ordinary Runtime Client. Runtime commits captured Session/Run/Segment and its existing command acceptance atomically; the former automation API, reader/store/table and segment receipt branch are deleted.
- Actual managed session/release/source authority supplies a closed installed actor. Opening rechecks that authority in its transaction; known withdrawal remains plugin_changed, unknown storage failure retains uncertainty. Caller keys are installation-scoped and retain original identity across activation/release/configuration replacement.
- Schedules reads runs.getAcceptance with the exact original StartRunRequest before submission. Opened/retired evidence confirms the same captured identity and changed input conflicts. A pending private dispatch can reconcile its original Core acceptance; private acknowledgement and final reply commit together. Memory still refuses pending model reentry through NewlyAdmitted.
- Schedules callback refs are ephemeral MCP metadata outside admitted business input, fingerprints and persisted pending work. Private Schedules capture binds to one Runtime store namespace and refuses a replacement store. Former host/private schemas are refused without converting or deleting retained data.
- Real backend Core acknowledgement loss exposed an existing Interaction channel-close/send race. The lifetime now withdraws and joins every publication sender before closing events, including executor requests. A deterministic full-buffer regression proves blocked sender cancellation, late refusal and retained buffered events; no retry or suppressed race was added.
- Building the actual Schedules executable with the public Core Client produced 70,358,850 package bytes. Removed the former 16 MiB single-file limit while preserving the 128 MiB aggregate limit, manifest limits, entry limits and feature-resource bounds. No publisher-specific exception was added.

- Stage 3c extracts the existing RPC/SSE handler from the ordinary HTTP Server without constructing another Delivery pipeline. SDK calls compile their method set into one immutable handler; both transports use the same per-call cancellation/admission owner. Canonical RPC publication is single-assignment, rejects nil handlers/replacement, and precedes usable refs/workers.
- ConnectCallback returns the ordinary typed Runtime Client with validated loopback-only refs, activation/call headers, no proxies and no redirects. It retains existing JSON-RPC/ProblemData/SSE projection and single-message limits. Stale-call and oversized-request gates remain known pre-dispatch refusals rather than unknown command acknowledgements.
- SSE detachment cancels and joins its source before the callback owner returns. Cancellation also interrupts blocked request reads/writes; asynchronous hooks retain immutable network context/body references. Existing Memory transport and budget remain with their actual owners.
- Corrected the actual Memory callback's external-effect semantics: model completion is a command, each independent invocation carries its own identity, and the Memory-owned budget records admitted identities before provider I/O. Identical redelivery never makes another provider request, including after a lost observation; changed input returns the existing typed command conflict. This is call-lifetime fencing, not a durable provider receipt or completion of canonical model execution.
- Stage 3b replaces per-grant Memory HTTP listeners with one public SDK callback transport used by actual Memory search and maintenance. The generic MCP connection owner admits/revokes callback generations without Memory model imports. Memory owns its two-call budget and model/scope DTOs in its RPC adapter. Old Models, private HTTP codec and BindModels entry are deleted.
- Callback operations reuse immutable Go-owned Method contracts, raw structural admission and the same value/refusal envelope. Captured owner context remains authoritative; network input cannot replace target/origin/parent values. Unknown failures retain uncertainty and no retry is introduced.
- Withdrawal retains releasing calls until their handlers settle, cancels every sibling before joining and interrupts incomplete authorized request bodies. Released/foreign references are explicitly stale. A real SDK-only executable proves MCP → host callback → nested same-backend MCP without a Desktop relay.

- Read current repository instructions, design rationale, development and refactoring rules.
- Created an isolated managed checkout at the current committed main baseline; no product source changes in the shared checkout.
- Revalidated the proposal and review against current installation, delivery and dependency ownership.
- Classified existing UI/E2E checks as prohibited by the explicit task constraint; their omission must remain visible in verification reports.
- Replaced unapproved/approved/enabled installation state with installation-owned desired enablement across Domain, persistence, Delivery, generated contracts, Go/TypeScript bindings, CLI and graphical consumers. Removed approve/revoke operations, errors and current workflow instructions without compatibility aliases or decoding.
- Selection preserves desired enablement, drops secrets whose recipient changes and atomically refuses missing required configuration for an enabled candidate. Tests cover preserved preference, rejected selection, stale drafts and exact release authority.
- Same-release disable/re-enable remains available while execution holds a dependency; replacement, configuration and uninstall still require quiescence. Standing model-tool rules retain their separate source-qualified owner.
- Default distribution initialization still happens once and preserves user choices. Bounded directory enumeration now uses the actual installation capacity and preserves I/O, cancellation and close failures.
- Corrected generated TypeScript method-policy vocabulary to derive from the Delivery registry, including the existing owner-committed Run stream policy. Added a generator regression instead of another handwritten policy list.
- Extended exact-schema admission coverage for the former installation trust column, including retained-data checks. Incompatible directories remain refused rather than converted.
- Existing frontend fixtures were mechanically migrated to the removed APIs/shape and obsolete approval UI cases deleted. No new frontend UI assertions or test execution were added.
- Committed the complete first slice and merged local main without conflict. The incoming change reconciles already-accepted Interaction answer Signals as success. Integrated Runtime/CLI module checks and execution races passed.
- Fixed the existing commit-msg hook's assumption that COMMIT_EDITMSG is always relative. Linked worktree commits supply an absolute path; the hook now preserves it while using the same commitlint rules.
- Added runtime/plugin in the existing Runtime module as the one public typed MCP backend binding. Memory, Schedules, their actual host callers and the independent Reviews backend use it; former private method/result/client/tool registration implementations were removed, not wrapped.
- Unrecognized storage/commit/transport/cancellation errors remain protocol failures. Only explicitly classified business refusals become a known Failure. Memory/Schedules classifiers no longer label every unknown error internal as a business result.
- Closed method envelopes reject missing input, unknown members and malformed command identity before mutation. Replies reject ambiguous/missing outcomes and preserve an explicit nullable value separately from a missing reply.
- Centralized logical Tool metadata in the public binding. Existing Tool receipt namespaces remain unchanged; model arguments and transport IDs cannot originate that identity.
- Reviews now exposes typed list/update backend methods and its existing model Tools through the same private store. Tests prove shared receipt replay across entrances, typed revision/identity conflicts and preserved uncertainty for unexpected storage failure.
- Verified an external Reviews author with GOWORK=off, no local replace and a temporary module archive containing only the public SDK files plus module metadata, without Runtime internal source. Tidy/test/vet/build all passed in that isolated offline fixture. This proves the backend binding subset, not the full future SDK or a published release.

- Added immutable command routing in the plugin Domain and SQLite: identity, semantic method, input digest, original installation/release/server and preparation/dispatch-admission/observation evidence. Observation preserves routing and never stores a host business outcome.
- Moved installation admission into the Application owner and injected the same boundary into command routing and execution. Quiescence/reclamation derive the union of unresolved commands, live execution and pending checkpoints; no refcount or competing held set was added.
- Migrated actual Memory add/update/review/delete, Schedule create/update/delete and manual preparation from current-provider mutation calls to original-bound dispatch. After dispatch admission, redelivery only queries the original backend's receipt. Missing/malformed/unreachable evidence remains unknown; no resend or provider retargeting occurs.
- Backends now atomically persist both known success and business refusal envelopes. Refusal rolls back mutation before saving its receipt in the same transaction; unknown storage/commit errors do not fabricate a receipt. Removed the former bare-result encoding and added non-mutating refusal coverage for that private schema.
- Published the receipt lookup/fingerprint/reply primitives in the existing public backend binding. Strict request admission, nullable/missing outcome separation and zero/empty success round trips are covered.
- Real-process regressions close and reopen the host database after response/request loss. They prove retained quiescence across restart, blocked release selection/configuration/uninstall, same-release disable/re-enable recovery, missing receipt without resubmission and old identity refusal after a replacement provider is enabled. Existing Schedules captured Run acceptance/replay remains intact.
- Shared admission preserves standard cancellation plus custom cause and refuses inactive/foreign execution assemblies. Deterministic concurrency coverage proves an installation change cannot pass a pending command-binding write.
- Removed speculative lifetime-count limits from both routing and private receipts before delivery. Retained history cannot silently evict an identity or permanently exhaust normal mutation usage after 4096 commands.

- Added public NewMethod[P,R] in the existing runtime/plugin package. Semantic method identity, query/command kind, target meaning and prose are declared once; Scope derives immutable structural schemas from the owner's existing Go types. Real client/server calls use those same compiled schemas.
- Required original input members are checked before business execution, rather than re-encoding Go zero values. Output schema violations remain protocol failures, preserving effect uncertainty. Method identity mode cannot be relabeled by its caller. Descriptor mutation cannot change the active method.
- Reviews now owns both method contracts in backend/contracts.go and binds its typed entrances from those contracts. Removed separate ListMethod/UpdateMethod name declarations; Tool descriptions and input schemas derive from the same owner. Status vocabulary is shared by validation and Scope schema inference; output outcome vocabulary remains typed.
- Extended the existing Runtime contractgen with a feature-input mode. It consumes public method descriptors and reuses Scope's standard Schema derivation plus the existing Schema-to-TypeScript emitter. Generated standard Schema preserves all numeric/cross-field keywords; TypeScript remains a structural data projection. Local definition references are rewritten without provider selection, external schema loading or a custom IDL.
- Generated Reviews schema.json, methods.generated.json, API_REFERENCE.md and TypeScript method input/output maps from go generate. Backend contract tests compare generated artifact admission with the actual method schemas. The TypeScript contract fixture rejects unknown status/outcome and a missing inspected revision; it runs no UI.
- Added the actual public plugin package to go-api.json generation and the external public-binding compile proof. No new Go module/public package or dependency was added. SDK construction, raw admission, descriptor independence, invalid output, generic reference, nullable/map projection, duplicate contract and source/output collision regressions pass.

- Reviews now binds the public receipt query to its existing immutable invocation records. It validates the original method/input and private result before projecting a typed reply. No new private table, host business cache or mutation fallback was added.
- Backend restart regressions recover both original success and stored revision conflict after later review changes. Missing receipt, changed method/input, corrupt private input/result and closed storage cannot become known mutation evidence.
- Replaced the real stdio response-loss test's same-key Tool mutation recovery with public read-only receipt lookup. The original result is compared as exact JSON evidence without converting integers to floating point. The finished Run retains its original unresolved effect after Runtime restart.
- Read-only cutover review traced declaration, selected provider, invocation and client consumers. Removing the entire apiVersion:1 parser also requires migrating the actual closed page/component consumers; hiding those values in a generic declaration would retain the wrong contract.

- Added the pure public jsoninput value to own normalization, immutable JSON and fingerprint projection once across Domain and backend authors. It uses standard jsontext object reordering without floating-point canonicalization; duplicate members/invalid UTF-8 are rejected and exact number lexemes/array order are preserved. No new module or dependency was introduced.
- SDK method and Tool admission retain original JSON before typed conversion. Command.RequestInput and RequestInput(ctx) expose only that received projection; unbound contexts and constructed DTOs cannot fabricate it. Removed InputDigest, which hashed re-encoded DTOs.
- Replaced host plugin_commands.input_digest and private Memory/Schedules commands.input_digest with immutable input_json. Dispatch and lookup send the saved host input; private receipts persist received JSON and derive their digest. Original route/phase/dependency ownership and value/refusal atomicity remain unchanged.
- Reviews now stores admitted input_json and verifies the typed business input against its received projection. Its original invocation outcome remains the sole private result. Added strict private-schema admission before schema/business writes; former arguments directories are retained and refused, not interpreted as raw requests.
- Regression evidence covers struct/map/raw identity agreement, exact integers, numeric lexemes after method/Tool decoding, optional empty Tool arguments, private receipt recovery after a real DB close/reopen, changed input rejection without repeating mutation and non-mutating old-format refusal. Real installed Reviews loss recovery now uses the actual raw request without a duplicated Go DTO fixture.

- Added one public Backend assembly/publication owner in the existing plugin package. It binds typed exports, refuses duplicates and post-publication mutation, and exposes independent schema/semantic descriptions. User handlers run outside its lock. Invocation/discovery accept only the one current BackendProtocol; former per-operation MCP handlers are not registered.
- All backend Go callers decode their concrete result from that transport. Removed Memory/Schedules RegisterClient lists and connection-pool imports of those business RPC packages; the pool registers only the public transport. Removed the now-empty Method.RegisterClient forwarding API rather than retaining a compatibility wrapper.
- All actual backend methods now use the shared Go-derived structural admission. Memory identity and model selection project their existing wire representations through pure schema aliases; optional RPC fields are omitted at their adapter-owned DTOs. The Schedules model patch wire DTO translates to its Domain owner, without introducing transport tags into Domain or weakening the architecture gate.
- Schedules scan and settlement, and Memory maintenance, are declared commands rather than queries. Scan uses original-bound host command admission and private atomic receipts. Settlement retains the existing durable accepted-Run handoff identity (accept plus RunID) and private receipt, against the captured server generation.
- Memory maintenance keeps model I/O outside database transactions and preserves concurrent human edit/curation CAS semantics. It now carries command identity and original host routing; its publication-owner receipt integration remains open. A missing maintenance receipt remains unknown and never authorizes another model call or provider retargeting.
- Loss probes now match the semantic method inside the current invocation envelope while standard tools/call retains its own direction. Existing withheld-response/request, restart, known/unconfirmed and provider replacement assertions are retained.

- Private commands now represent admission without a known reply using SQL null. A prepared command records immutable method/input once before external I/O; same-identity reentry stays unconfirmed across restart. Existing atomic command handling also refuses a pending row before executing its mutation. No second table, result owner or state counter was added.
- Memory publication composes its existing CurationStore with the private receipt transaction only at Reconcile. Watermark, item fold and the original Published outcome commit or roll back together. Ledger extraction and model calls remain outside that transaction, preserving pending facts and concurrent human authority.
- Non-publication success commits its original false outcome; a completed duplicate returns the original true/false reply. Canceled/failed model work retains private pending admission and no known receipt. Later maintenance activity uses its own identity to fold retained ledger facts; it does not replace the earlier command's uncertainty.
- Private completed-only receipt databases are refused without mutation. Memory and Schedules share this actual nullable-reply format; fresh directories are required and old directories are retained separately.
- Added atomic completion rollback/immutability, pending admission restart/conflict/reentry, private publication receipt-failure and cancellation regressions. A real stdio response-loss probe witnesses committed maintenance, then a fresh backend reads and replays the original reply with exactly the original two model calls and one retained proposal.

## In progress

- Stages 5aa-5ad implement live consumption, recovery, host grants and caller attribution
  for preparation/observation. Complete one installed/native normal-terminal/failure/
  interruption/restart Run/Goal workflow, including unknown work and in-flight
  admission under the actual calling owner's resource policy.
- Schedule crash acceptance still stops at plugin handshake after the production
  Memory dependency repair. Diagnose original exchange evidence in final integrated
  acceptance; preserve all seven cut cases and original no-replay/data assertions.
- Complete execution-bound background Session/Run/Segment/member provenance and optional counter I/O admission. A phase callback cannot claim durable continuation.
- Complete embedding evidence/metering and historical retention at their existing
  owners. Preserve source-reported unknown usage/price without another billing tally.
- Complete transitive dependency status, scoped retention/release, point lifecycle,
  query streams and wide-number Go/TS parity. Existing unary/mutation-stream and
  composition boundary proofs do not close every consumer or automatic workflow.
- Audit every Tool, CLI/UI/automatic/cross-plugin consumer for exact target/origin/
  parent, original route/private receipt and lifetime. Complete independent author/
  component generation/recovery/module qualification; publication is not authorized.
- Concentrate full/native/vet/race/dependency/schema/architecture/format/deletion and
  P01-P48/review closure checks at final acceptance. No frontend UI/E2E/screenshot
  execution is authorized. Do not classify an unresolved failure as pre-existing
  without causal evidence.

## Next

Resume in a new session using this same managed worktree and the handoff document.
Preserve the full proposal/review objective and avoid carrying forward static
percentage estimates.

1. Complete embedding results/evidence/metering through existing source, live
   accounting, query/export and recovery owners; use the handoff's inspected facts.
2. Complete execution-bound background provenance and original effect ownership;
   retain canonical RPC, exact frozen continuation and one physical-source owner.
3. Complete retention/dependency/status, point/query-stream and independent author
   qualification, then concentrate final acceptance and P01-P48/review closure.
   Repair Schedule handshake and schema/architecture/format findings without
   weakening assertions or running forbidden frontend UI/E2E checks.

## Decision ledger

| Date | Decision | Reason | Rejected alternative |
| --- | --- | --- | --- |
| 2026-10-14 | Pause this session for handoff; retain the existing managed branch and committed work | The user explicitly wants to continue in a fresh session after this long conversation | Continuing automatic development, recreating from main or leaving an uncompilable partial embedding draft |
| 2026-10-14 | Withdraw carried-forward percentage estimates and report concrete completed/remaining requirements | The repeated 75% figure was not recalculated against P01-P48/review evidence and could not describe recent progress reliably | Repeating a stale estimate or inventing a higher percentage from commit counts |
| 2026-10-14 | Validate retained background commands against their complete original target/provider, regardless of target scope | Recorded Session/workspace commands were wrongly refused by an installation-only precondition | Erasing the target, special cases for individual scopes or trusting a current UI selection |
| 2026-10-14 | Restrict only fresh unrecorded triggers to Automatic/parentless/own-installation attribution | Fresh work has no original route proving a resource target or a human/model parent | Allowing unrecorded resource references or making a retained command acquire a new route |
| 2026-10-14 | Keep caller attribution distinct from active execution restoration | A persisted invocation does not recreate an expired continuation or Scope member publisher | Inferring Run/Segment from parent IDs or claiming Session/Run metering from attribution alone |
| 2026-10-14 | Cohost observer next, published host methods and canonical RPC under one call grant | Decorators need ordinary host collaborators while their outer call is pending | A private next-only endpoint, another listener or overwriting published methods |
| 2026-10-14 | Keep original caller values for next while RPC uses the receiver activation | Managed callback authority must not retarget the captured remaining chain | Giving next the observer Actor, restoring values from wire or losing network cancellation |
| 2026-10-14 | Extend active-contributor cycle refusal to installed observers | A fresh selected exchange may not recursively reenter its currently active observer | Blind recursion, silently bypassing the observer or reinterpreting next as a fresh exchange |
| 2026-10-14 | Replace OpenRPC's source signature with variadic local callbacks and regenerate its API | Local continuation composition belongs to the existing callback lifecycle owner | Old-signature compatibility wrappers or a second SDK grant framework |
| 2026-10-14 | Supply existing call-scoped host RPC during selection/context/input queries | Installed preparation code needs reentrant host collaborators under its actual receiver authority | Query identity without callbacks, activation fallback or another listener |
| 2026-10-14 | Runtime authorizes its selected model pipeline; managed direct calls retain declared-requirement checks | Global host composition is not a caller plugin's dependency declaration | Requiring every caller to declare host policy members or disabling arbitrary cross-plugin admission |
| 2026-10-14 | Reject active model-contributor cycles using an immutable captured path | Nested fresh generation must not recursively prepare its own ancestor or silently bypass policy | Blind reentry, arbitrary depth limits or skipping active contributors |
| 2026-10-14 | Capture the actual execution member publisher during primary preparation | Independent preparation work precedes primary dispatch but still belongs to that execution | Fabricating a primary logical owner or classifying active work as Session-only |
| 2026-10-14 | Honor real owner-supplied budgets without inventing a universal Run/Goal ceiling | Proposal 10.2/10.3/11.5 defines budget ownership; current Run/Goal APIs have no total-cost/step-limit contract | Treating earlier aggregate-budget progress wording as a new product requirement |
| 2026-10-14 | Common model capture publishes independent selected and auxiliary consumption through the captured execution member | Different callback facades must not create separate billing/lifetime owners | A selected-only tally, inferred current Run, a new invoice/receipt or provider replay |
| 2026-10-14 | Persist conversation/independent purpose on logical model starts | Independent completions must not close a main stream or replace context; recovery must not force their route to the main model | Presence heuristics, mutable purpose at completion or removing the primary selection guard globally |
| 2026-10-14 | Unknown admitted independent work remains unknown in live and boot accounting | Admission is evidenced even without a response; a later priced response cannot price missing work | Zero token/price substitution, losing admitted work or promotion to successful continuation |
| 2026-10-14 | Require the new logical-purpose database shape and refuse old storage unchanged | Purpose is required original evidence; the repository does not reinterpret former layouts | A default-purpose migration, nullable logical role, legacy reader or deletion of user data |
| 2026-10-14 | Run reducer accrues original model consumption once per call; segment termination has no invoice | Executor cumulative snapshots overwrite independently accrued work and introduce a second transition owner | Cumulative confirmation/arbitration, dual accounting events, terminal replacement or an old-fact adapter |
| 2026-10-14 | Unknown original usage keeps total price unknown across later known calls | A later priced report cannot establish the cost of previously admitted work | Treating the first priced report as a complete total, fabricated usage or free unknown calls |
| 2026-10-14 | Bootstrap binds the existing selected installed Memory reader explicitly | Working context has a real required pinned-memory read contract; composition omitted its implementation | Typed-nil reader, empty stub, dropping pinned memory or recreating a private business facade |
| 2026-10-14 | Lost Run/Goal recovery consumes independent physical sources through the existing atomic replacement | Primary-only recovery omitted separately admitted effects; logical completed calls already have their original projection | Fake logical Model receipts, a second invoice ledger, repricing or source replay |
| 2026-10-14 | Unknown independent attempts retain missing tokens/price and invalidate exact total cost | Admission is known; provider usage/outcome cannot be invented | Zero usage/cost substitution, dropping uncertainty, or child input recalibrating the main context |
| 2026-10-14 | Compare original JSON and billing-relevant source projections in the recovery transaction | Scope clones can normalize nil/empty containers; true usage/price changes must still refuse | Whole-struct DTO equality, trusting only altered DTO values, or weakening original evidence checks |
| 2026-10-14 | Merge main Steer evidence with original Run/installed journal and lazy registration; publish combined Core .31 | Both branches changed public contracts independently; a new combined epoch owns the merged graph | Selecting one protocol branch, metadata loss, installed route removal, eager promise execution or legacy journal conversion |
| 2026-10-14 | Keep live/normal-terminal/aggregate budget completion open after boot proof | Recovery alone does not update live totals or enforce the next external request's work budget | Claiming full accounting from the boot regression or adding live charges without changing boot classification |
| 2026-10-14 | Capture the original logical Model/Tool owner; derive Run/Segment from its admitted record | Exact caller-ID inference lost descendants and gave wire naming execution authority | Guessing from caller/parent/current Session, a parallel Run mapping, or rewriting old observations |
| 2026-10-14 | Require matching Session, active Run/Segment and chronological owner at generation admission | Old callbacks must not retarget paused/resumed work or start generation under a foreign fact | Silent Session-only fallback for a missing supplied owner, old-Segment dispatch or blind replay |
| 2026-10-14 | Protect logical namespaces and require distinct independent exchange IDs | Fresh calls must not impersonate the primary source later consumed by accounting | Reusing operation/primary/Tool IDs or trusting arbitrary wire ancestry as evidence |
| 2026-10-14 | Publish original endpoint Segment under Core .30, retain existing storage/backend/export shapes | Segment already exists as an immutable relational source; consumers need its faithful projection | Duplicate persisted association, another storage epoch or a compatibility alias |
| 2026-10-14 | Fix association before descendant totals and keep aggregate delivery open | An unassociated physical child cannot safely be charged to Run/Goal; persisted evidence is not aggregate acceptance | Billing by Session guesses or claiming total-work budget completion from generation-gate probes |
| 2026-10-14 | Shared callback senders project the original background invocation through the existing validated header | Local SDK context cannot cross the network; activation grants must restore retained caller evidence before model child admission | Missing origin/parent, accepting a backend-supplied Actor, or replacing an ordinary captured call/next owner |
| 2026-10-14 | Give each independent model exchange a new ID, selected route and complete request/output budget | Repeated captured next still names the original effect; Memory's two-call rule is private business policy | Relabeling next, automatic provider retry, global Memory call limits, text-only general requests |
| 2026-10-14 | Share one model capture and existing physical endpoint journal for fresh and auxiliary calls | Lifecycle/held dependencies and dispatch/usage/price already have owners | Duplicate capture registries, host model receipts, cached results or a second billing tally |
| 2026-10-14 | Publish exact additive selected-model callbacks beside canonical RPC; preserve wire/storage epochs | No existing wire method/record changes; new methods have their own exact identities | Adding another listener, core business API copies, artificial storage epochs or accepting a changed old method |
| 2026-10-14 | Migrate PublishRPC's variadic Go signature directly and disclose exact external method-value migration | Host collaborators need one canonical publication; no old-signature wrapper is retained | Deprecated publication alias or parallel callback startup owner |
| 2026-10-14 | Keep descendant Run/Goal accrual explicitly open despite known physical evidence | Interaction currently accounts only its exact primary caller; source persistence alone cannot prove total-work budget correctness | Claiming multi-call metering complete or introducing a competing aggregate ledger |
| 2026-10-14 | Source command admission owns structure/evidence; final prepared request owns selected-model media support | Pre-transform model checks prevented legitimate conversion and created a competing media decision | Early authored-media rejection, transformer-exists bypass flags, dropped original media or weakened final model checks |
| 2026-10-14 | Well-formed media acceptance and later model failure retain the original Run/command outcome path; Core .29 | Opening/continuation acceptance cannot claim success for a model-visible projection not yet prepared | Promoting acceptance to model success, a second refusal receipt, implicit redelivery or pretending failure timing is unchanged |
| 2026-10-14 | Canonical unsupported media cannot go to a provider counter; only its specific modality refusal allows local compaction projection | Provider counting is itself model-specific I/O and must not preempt final conversion | Counting rejected source media, swallowing arbitrary errors, estimating billing or suppressing final refusal |
| 2026-10-14 | Primary prepares one complete transformed provider request while Scope retains canonical conversation/Options | Fragment assembly after reduction could lose transformed Options or send a different request than was admitted | Persisting transformed working context, separate prepared projections, post-count mutations or implicit retries/recompaction |
| 2026-10-14 | Input order is part of the original configuration/checkpoint/dependency set | Restoration must retain the contributors accepted by that execution | Current-chain reselection, missing-field empty fallback, independent held registry or legacy checkpoint adapter |
| 2026-10-14 | SelectedChat owns the immutable selection/model/counter/token envelope for both execution kinds | Final transformed primary capacity now needs the same guarantee as auxiliary work | Duplicate capacity captures/algorithms, keeping the misleading auxiliary-only type as an alias or mandatory output for primary default policy |
| 2026-10-14 | Calibration baseline derives from complete final transformed request, actual billing from endpoint source | Pre-transform token estimates disagree with observed provider usage when messages/Options change | Keeping stale estimate 100, billing from estimates or weakening response/usage/recovery evidence |
| 2026-10-14 | Input transformation is a query before final admission, preserving captured routing, Tool contracts and caller output authority | Request/message changes are not authority over provider selection, execution, preparation/approval or another resource budget | Observer-side mutation, model-next grants, replacement Tools/model, output expansion or rewriting authored conversation |
| 2026-10-14 | Auxiliary context precedes ordered transforms, then final byte/output/token admission before observation/physical dispatch | The counted request must include every admitted change and repeated calls must retain the original source | Counting pre-transform input, adopting transformed conversation, failure fallback/retry or reselecting transform order between phases |
| 2026-10-14 | Reuse exact binding validation and existing captured dependency/lifetime owner for input contributors | Context and input share the same immutable installed export/contract guarantee | Duplicate validation rules, separate held registries or a second lifecycle owner |
| 2026-10-14 | Auxiliary capture freezes catalog token limits and preserves them/counter when its call projection is decorated | Capacity must belong to the selected model instance and remain coherent for the owning use case | Looking up new limits on every call, recapturing on decoration, independent token ledgers or byte-to-token default limits |
| 2026-10-14 | Final auxiliary context reserves output and uses the selected counter when present, else the existing request estimate | Context contributions and model-owned options must be admitted together before provider generation | Counting original input only, counter failure/zero fallback, silently dropping context or claiming estimates are exact |
| 2026-10-14 | Apply captured auxiliary reasoning before context, counting and generation | The selected effort had not been projected into the actual auxiliary request | Ignoring selected effort, request-side replacement, mutating caller-owned options or post-count option changes |
| 2026-10-14 | One complete-request estimator lives in the existing shared modeltext projection | Primary compaction and auxiliary admission must not diverge in framing/media/metadata estimates | Copying the algorithm, flattening to rendered transcript or a compatibility estimator wrapper |
| 2026-10-14 | Merge original Run metadata and installed input identity under one journal/fingerprint function; Core .28 | New main freezes Run admission metadata while installed private receipts still require their original input bytes | Dropping either recovery path, eager delivery before tracking, duplicate helpers/stores or retaining the conflicting Core version |
| 2026-10-14 | Public selection is an explicit installed query before fresh Run/auxiliary exact binding | Routing inside an already-bound model would disagree with media/capacity, construction and recovery | Post-admission provider swap, implicit discovery, selected-provider failure fallback or model-next grants at selection |
| 2026-10-14 | Session user preference and accepted Run route are distinct facts with distinct existing owners | Routing can choose a model for this execution without rewriting the user's persistent default or original command | Overwriting Session preference with automatic routing, changing command fingerprints or keeping two mutable Run selections |
| 2026-10-14 | Accepted Start retries/Resume restore the original route; selector query results require no later invocation binding | Durable Run/checkpoint already owns exact selection and a completed query has no future dependency | Reselecting after restart, retaining a fake selector execution dependency or adding a second selection store |
| 2026-10-14 | Auxiliary selection precedes provider construction with absent request when prompts are authored later | Memory captures once for two later prompts; invented empty messages would misrepresent caller input | Selecting after construction, fake input conversations or an unused preliminary utility resolver |
| 2026-10-14 | Auxiliary capture returns exact selection/model and requires the calling use case's text/output budget | Context authors need the actual model identity; contributed input must not bypass a caller's existing envelope | Response-derived identity, bare model without captured selection, implicit provider limits or observer-side expansion after admission |
| 2026-10-14 | Capture context and observers together for the full auxiliary use case | Memory's two original calls share frozen dependencies and must cancel/join before release | Re-resolving contribution order each phase, additional held registry, per-call retirement before the workflow releases its capture |
| 2026-10-14 | Public primary model selection must precede the Run/execution's accepted exact binding | Changing the provider only inside an already-bound model wrapper would contradict capacity and recovery identity | Changing endpoint identity while retaining the old capacity/calibration/selection or silently reselecting on restore |
| 2026-10-14 | Primary input contributions are ordered query material before capacity admission; original exact bindings survive recovery | Retrieval/instructions must reach the semantic admission boundary without authority over routing or authored conversation | Observer-side mutation, persisted plugin messages, replacement resolution on restore or a separate contribution fact store |
| 2026-10-14 | Extend the existing single-call prepared tail into ephemeral prefix/trailer material | Scope adopts reducer output; including plugin context there would accumulate messages and change conversation ownership | Returning ephemeral contributions as WorkingContext, independent request state, re-reading contributors after measurement |
| 2026-10-14 | Require model_contexts in retained checkpoints with explicit empty capture | Missing captured order cannot prove which contributors belonged to the accepted execution | Silent empty fallback, compatibility adapter or current-provider reselection |
| 2026-10-14 | Merge main's original RunCommand union with installed capture in the same client journal | Start/Resume/Steer and installed mutations retain their existing original request/receipt authority | Dropping either recovery path, retaining old Steer-only state/API, mixed installed/Run records or a second journal |
| 2026-10-14 | Usage reports fold retained physical endpoints once; Run accounting stays the execution/Goal projection | Finished Run-only reads omit auxiliary/global/active work and combining both sources double-counts primary calls | Run fallback, summing logical and physical usage, completed-only reports or new aggregate storage/writer |
| 2026-10-14 | Known token/price subtotals carry explicit source coverage; exact cost requires every selected attempt priced | Missing usage, price and unknown outcomes are evidence gaps, not recorded zero | Silent zero defaults, hiding unpriced/unknown attempts, presenting partial known cost as the complete bill or pricing at query time |
| 2026-10-14 | Source admission time and actual provider/served model/origin/association own grouping | Run completion/selection cannot attribute detached work or establish actual model timing | Completion-date grouping, same-name model collapse across providers, invented auxiliary Run or separate grouping data |
| 2026-10-14 | One transactional streaming source scan validates ownership before reporting | Complete reports must refuse corrupt/later evidence without materializing entire history bodies | All-body arrays, per-Session independent snapshots, partial-success reports, projection arbitration or parallel usage parsers |
| 2026-10-14 | Persist actual endpoint price beside its known response; Interaction consumes that price | Restart/current configuration cannot originate or replace the cost of an already consumed call | Independent Interaction price lookup, repricing at recovery, zero for missing prices or a separate billing store |
| 2026-10-14 | Known source evidence joins the existing lost-Run recovery transaction | A complete provider result may precede outer/Run commit; its consumption survives interruption exactly once | Model replay, semantic continuation from physical evidence, independent usage commit, success promotion or charging a settled logical call twice |
| 2026-10-14 | Canonical original admission/response values and all source projections fence recovery settlement | DTO re-encoding can change numeric evidence or spelling and usage must remain a projection of the response | Re-encoding read DTOs, omitted source CAS, competing usage/cost transitions or weak corruption checks |
| 2026-10-14 | Merge Run/Steer acceptance under one ownerRun policy while retaining installed command recovery | Main adds unary Steer acceptance to the same owner; plugin commands and streams retain their original bindings and private receipts | Keeping obsolete ownerRunStream aliases, dropping either recovery path, storing a competing business result or accepting mixed command metadata |
| 2026-10-14 | Session trajectory/export project physical endpoints from the existing model journal, including Session-only calls | Real requests and known outcomes already have one source; logical attempts cannot substitute for them | A second trajectory store, inferred Run by timestamp, decoding JSON strings through float numbers or adding duplicated usage totals |
| 2026-10-14 | Session deletion and endpoint cleanup share one transaction; new admission checks original Session existence/equality | A deleted Session cannot acquire orphan evidence and a refused delete must preserve its observations | Global deletion, Session foreign keys that change logical Run ownership, independent cleanup commits or retargeting a conflicting Session |
| 2026-10-14 | Title maintenance captures its actual Session; storage DTO owns endpoint JSON encoding | Detached maintenance still has a real Session and Domain admissions do not own storage format | Fake Run ownership, replacing inherited scope, Domain JSON tags or changing persisted envelope spelling |
| 2026-10-14 | Current Interaction bills persisted physical endpoint evidence through its existing accounting fact | Outer projections may fail or change metadata after a provider has spent resources; they cannot originate usage | Billing outer response metadata, a second metering event/writer, source read failure fallback or guessed consumption without completion |
| 2026-10-14 | Resolve the physical caller by exact SQL identity with one shared validated scanner | An ancestry page is a presentation projection and may contain other children before the actual caller | Picking a current/latest model call, limited ancestry paging for accounting or duplicating stored evidence validation |
| 2026-10-14 | Record physical endpoint admission/completion beneath observers in the existing model invocation journal | Outer failure cannot originate or erase the provider's known request/response; each real request needs its own identity | Inferring physical completion from outer success, a separate receipt/result store, overwriting logical Run outcomes or blind model replay |
| 2026-10-14 | A captured calling model identity dispatches at most one endpoint; additional requests need another admitted identity | Crash or transport uncertainty cannot authorize an untracked second provider effect | Repeating the same caller after restart, treating absence as no effect or changing the endpoint route through request options |
| 2026-10-14 | Read endpoint request/response from stored raw JSON and reconcile incomplete observations as unknown | Typed read projections must not lose provider numeric evidence or manufacture success when a process disappears | Numeric round-trip in query presentation, discarding known final-frame evidence on stop or marking unobserved calls definitely failed |
| 2026-10-14 | Auxiliary resolver returns an explicit captured model release; Memory holds it across the whole maintenance use case | A multi-call use case must keep the same observer choice and release only after callback work joins | Per-prompt re-selection, implicit closable capability, leaked captures or another maintenance replay path |
| 2026-10-14 | Active auxiliary captures register under installation admission and project held releases until actual closure | Quiescence and source retention must include non-Interaction model work without copying installation state | An unrelated installation refcount, forgetting pending in-flight calls, dropping retention before cancellation/join or retargeting closed models |
| 2026-10-14 | Public model callbacks project each admitted command as a call child with unchanged origin/parent | Separate real extraction/curation must not reuse the maintenance operation as one model invocation | Losing original origin, installation target at the model boundary or assigning separate real requests the same model invocation identity |
| 2026-10-14 | Production Interaction captures installed transparent observers at staging and checkpoints the original ordered bindings | Configuration changes must affect new executions without retargeting existing or restored next chains; held releases share execution retention | Resolving current observers after restart, ignoring observers in deployment identity/retention, first-party direct pointers or a model command receipt |
| 2026-10-14 | One unchanged next delivery belongs specifically to the transparent observer contract | The first shipped observer must preserve admitted input, exact endpoint/usage and canonical chunks; other compositions need independent identities/admission | A universal once-only decoration rule, silent request/output mutation, extra untracked provider I/O or suppressing known inner failure |
| 2026-10-14 | Inner stream production waits for outer consumer delivery, with one pending chunk | A callback SSE write can complete before the remote observer forwards it; actual consumer backpressure must govern continuation | Treating frame writing as chain consumption, unbounded queues or detached producer shutdown |
| 2026-10-14 | Store consumer-selected outer-to-inner members with the complete decoration contract in one atomic host row | Production composition needs an explicit stable order that new calls can bind without retargeting old calls | Package/load order, first-party priority, abusing single-provider selection or a parallel model-only order store |
| 2026-10-14 | Resolve named decoration requirements from the same choice; configured withdrawn members refuse the entire chain | Optional absence is distinct from silently shortening an explicitly chosen composition | Universal decoration rejection, skipping unavailable selected members, aliasing changed consumer contracts or mutating existing setup bindings |
| 2026-10-14 | Preserve known response/accounting in the single failed model fact and existing invocation row | A known inner completion must survive outer failure without becoming successful conversation or Tool continuation | Dropping usage/response, emitting another completion, adding a second evidence store or replaying the model |
| 2026-10-14 | Project full Scope evidence as JSON text and count retained bytes | Preserve model metadata and numeric lexemes across Go/TS without making client projections authoritative | Narrowed response DTO, float round-trip, omitted export capacity or compatibility conversion of former host data |
| 2026-10-14 | Memory production completion uses public Scope exchange with a utility model captured before dispatch | Preserve full known response and original selection while private Memory policy retains its real budget/receipt ownership | Private string model multiplexer, per-prompt live selection, provider replay or host-owned duplicate Memory result |
| 2026-10-14 | Separate private embedding text-array/vector exchange from model completion | Embedding is not a text generation and needs no nested string encoding | Fake model operation for vectors or claiming a vector-only private port is full public embedding parity |
| 2026-10-14 | TS streams use the existing intent journal; original contract derives unary/stream delivery | Recovery must keep original binding/input/key before current preparation without another mutable delivery mode or result store | A second stream journal, newly selected provider, chunk replay or retiring at opening response |
| 2026-10-14 | Await notification consumption per HTTP request while shared RPC dispatch remains available | Backpressure must not deadlock nested calls; cancellation must join consumers rather than queue chunks | Awaiting one consumer in the global receive loop, unbounded inbox, concurrent delivery or detached asynchronous consumer |
| 2026-10-14 | Refuse unsafe numeric stream input/evidence before accepted decoding | Current JS number construction cannot preserve those integers; refusal prevents false known output while full parity remains required | Silently rounded final evidence or claiming refusal implements full lossless cross-language parity |
| 2026-10-14 | Typed installed streams share the original binding admission and SDK StreamValue | Typed projection must preserve the same contract/identity, frozen input/options and known final evidence without owning business transitions | Independent DTO/route state, deferred reads of mutable input/metadata, early final publication or automatic resubmission |
| 2026-10-14 | Callback stream captures its original reference/envelope and lazily owns one ordinary Client | No consumption must allocate no call/socket; invoked work requires its calling-owner context and must not retarget to activation | A separate callback transport, hidden unconsumed eager client, activation fallback for invoked work or typed reclassification of transport refusals |
| 2026-10-14 | Public plugin command streams retain existing owner-response semantics with no replay cursor | Stream transport does not change the original command/private receipt owner or establish a Run | Registering a mutation as a subscription, a second command policy/store, applying Run-opening semantics or replaying chunks |
| 2026-10-14 | HTTP source yields advance only after successful SSE frame writes | Callback acknowledgements must not release backend production while downstream writing is blocked | A bridge send counted as delivery, unbounded buffering, detached producer or inferring completion from opening acknowledgement/EOF |
| 2026-10-14 | Installed mutation streams use the existing original Commands route and private receipt owner | Chunks cannot prove completion; restoration must retain exact original caller/input/provider and never repeat an unknown operation | A second stream journal/result store, chunk replay, provider retargeting or automatic resubmission |
| 2026-10-14 | Explicit backend refusal settles original stream observation; generic stream faults remain unconfirmed | Known business evidence must not be lost inside transport uncertainty | Wrapping every refusal as unknown, inferring success from partial chunks or declaring internal delivery proves public/production integration |
| 2026-10-14 | Backend stream input/final outcome stay on one original MCP request; chunks use acknowledged Runtime callback delivery | Existing two directions permit true consumer-demand backpressure without another listener or unbounded notification queue | New backend listener, standard-pipe server-originated requests, unbounded notification buffering, second final-result store or new request per model chunk |
| 2026-10-14 | Receiving reference and one in-flight chunk belong to the call/session lifecycle | Stop, late/concurrent delivery and transport loss must not detach producers or replace original evidence | Concurrent waiting handler queue, early acknowledgement, retry after partial output, duplicate terminal state or late writes to retired generations |
| 2026-10-14 | Freeze input and raw metadata before lazy stream delivery | A delayed iterator must not observe replacement callback/parent or narrowed numeric evidence | Reading mutable metadata at iteration, float64 round-trip, retargeting next to current selection or claiming SDK binding proves Runtime installation recovery |
| 2026-10-14 | Chunk and terminal result schemas are distinct parts of one immutable streaming method identity | Stream support must correspond to executable behavior and generate the same Go/TS/canonical contract | A streaming flag on unary delivery, omitted chunk identity/schema, independent handwritten DTOs or premature final success |
| 2026-10-14 | Existing callback endpoint and SSE codec own model stream framing/lifetime | Two logical directions do not justify another listener, parser, unbounded queue or client relay | Per-stream endpoint, custom SSE/JSON parser, detached producer, automatic reconnect/provider replay or treating partial output as completion |
| 2026-10-14 | Scope accumulation uses the captured response byte capacity as conservative cumulative encoded-delta budget | The aggregation needed for a final model response must stay bounded by actual caller resources | Arbitrary chunk count, unbounded accumulator, parallel aggregate parser or an undocumented claim that this conservative budget equals final serialized bytes |
| 2026-10-14 | Unary installed/component entrances refuse stream metadata before route/delivery creation | Callback streaming does not establish host-to-backend or TS transport support | Silent stream downgrade, orphan uncertain command after unsupported delivery or declaring installation/production parity from substrate tests |
| 2026-10-14 | Public model exchange consumes Scope request/response through the existing callback owner | Canonical model semantics must preserve full usage/reasoning/metadata without a second text parser or provider-selection owner | A text-only model facade, new execution/history/result engine, inferred current provider or claiming a unary descriptor streams |
| 2026-10-14 | Known valid inner response and outer failure coexist in the model result | Outer failure cannot erase completed inner evidence or imply no model work occurred | Failure-only projection that drops known response, fake success, unvalidated partial output or a second durable provider receipt |
| 2026-10-14 | Model caller retains selection/budget/identity admission and one delivery | Generic callback authoring cannot impose Memory's two-call policy or promise provider replay after lost output | Universal Memory budget, automatic provider retry, new IDs for unresolved work, private model callback wrappers labeled migrated or claiming an SDK test proves product installation composition |
| 2026-10-14 | One generic plugins.dataChanged signal covers installed business owners | First-party contract matching with closed business topics denies ordinary authors equivalent refresh and leaks business identity into Core | Closed Memory/Schedules observer, new topic per business, duplicated old/new publication, Schedule record IDs or pretending notices contain outcomes |
| 2026-10-14 | Original command dispatch/recovery owns uncertain-outcome invalidation | A lost acknowledgement may follow a commit; a notice asks for an owner read and cannot resolve uncertainty | Retiring commands on a notice, inventing success, changing original provider, another changed callback/fact owner or global Core resync for a known plugin notice |
| 2026-10-14 | Backend notification retains actor/release/server/contribution admission and component observers retain generation lifetime | Removing business topics must preserve source authority and stop retired readers | Unauthenticated publication, first-party exemptions, orphan observation, silent subscription failure or replacing accepted data on loss |
| 2026-10-14 | Memory page Capacity is a schema-bound projection of the private target owner | Complete readers need finite traversal bounds without a Core business constant or copied client capacity | Retained Core capacity export/private import, hardcoded client 512, unbounded complete traversal or a separate limits query/facade |
| 2026-10-14 | Backend management page width has one Go owner, projected into request/output schemas | Application, publisher and clients must not independently choose the business page ceiling | Copied client 100, independent DTO tag constants, widening Rows without changing owner admission or silent partial collection |
| 2026-10-14 | Captured capacity remains stable across the original paged binding | Replacement or malformed owner data cannot justify merging target facts or accepting partial results | Refreshing provider per page, changed capacity arbitration, dropping repeated/empty pages or claiming compilation proves process recovery |
| 2026-10-14 | Plugin records/enums live in their publisher and client read projections, not the Core Protocol | All actual consumers use installed operations; the remaining Core record creates business-specific substrate knowledge | Retained Core record/schema/sample, backend-private client imports, a Core alias or a new read-facade operation |
| 2026-10-14 | Publish actual ItemID pattern and existing content boundary from the Go owner | String-alias reflection discards identity pattern, and deleting Core constraints must not weaken public admission | Copying numeric/pattern constants into clients, trusting schema shape alone, weakening the invalid-ID regression or keeping retired Core constraints |
| 2026-10-14 | Native fixtures compare complete publisher snapshots | Vectors make snapshots noncomparable; the full private owner state must remain observed across restart | Comparing a truncated Core projection, dropping fields, weakening assertions or treating source compilation as process acceptance |
| 2026-10-14 | One Memory authoring intent retains original scope/path/content and resolves pending before current-target reads | A lost acknowledgement may already have committed; another target/draft cannot originate a second addition | Target-specific competing intents, current-provider replay, fresh editor prerequisite or copied result store |
| 2026-10-14 | Public additions retain the captured path; private Memory resolves its project root and owns deduplication/Changed | Host path resolution rewrites retry evidence and adds a second business boundary; the publisher already owns target semantics | Resolving a different default workspace, replacing pending input with canonicalized paths, host deduplication or inferred duplicate status |
| 2026-10-14 | Retire final Core Memory method and feature with all actual CLI/workspace consumers | Plugin business capabilities are resolved through plugins and exact publisher contracts | Retaining Core add/DTO/handler/forwarder/feature or a compatibility alias; claiming live read models/limits/topics are already eliminated |
| 2026-10-14 | Content, pin and unpin share one original item update intent | An uncertain content change cannot absorb another pin/draft command or close a newly edited form as saved | Patch-specific parallel intents, fresh catalog before recovery, merging current draft fields into pending input or replacing nil with false |
| 2026-10-14 | Decode publisher snapshot before intent retirement and reread current cache owner | Malformed or foreign output cannot confirm the original edit or overwrite another provider's projection | Core update DTO/alias, blindly trusted JSON casts, global cached-ID replacement or another copied business result store |
| 2026-10-14 | Approve and reject share one stable item review intent in each actual client | Unknown approval may already have changed or removed the pending item; opposite submission cannot replace its original outcome | Decision-specific parallel intents, fresh catalog prerequisite, another command ID or reporting a new desired choice as the old command's result |
| 2026-10-14 | Review choices belong to client presentation and the published operation, while private backend owns valid transitions | A plugin business verdict is not a universal Core enum or forwarding method | Retaining Core review/DTO/enum, private backend imports or another host-side review state machine |
| 2026-10-14 | Stable installed command intent and original routing belong to the existing TS journal | Reconstructed current-provider input fingerprints cannot recover an original binding after client replacement | Another journal/result store, current-provider retry, storing authored input, silently dropping expired uncertainty or a business Core alias |
| 2026-10-14 | Begin journal tracking before creating delivery promises and decode before settlement | Retired generations must not create orphan requests; only an observed owner refusal proves a business failure | Tracking after delivery creation, retiring on unavailable-provider/host error or treating a decoder refusal as owner evidence |
| 2026-10-14 | Require identical serialized installed input and restore routing before preparation | Private receipts bind original bytes; semantic JSON equality or edited drafts cannot authorize changed replay input | Reordered input, current contract reconstruction or new command identity after uncertain acknowledgement |
| 2026-10-14 | Reread current Memory cache ownership after deletion and capture one client for project resolution/read | An original provider's receipt and another generation's workspace cannot advance the current projection | Removing every matching cached ID across providers or resolving the target through a fresh client |
| 2026-10-14 | CLI Memory deletion reuses the existing installed-command workbench owner | Response loss and a changed provider must not turn a retry into a new business command | A fresh command ID, current-provider retry, copied receipt state or retained CLI Core deletion wrapper |
| 2026-10-14 | Resolve pending deletion identity before new target/item reads; resumed commands have no invented display target | A committed delete may have removed its item, and the current workspace may differ from the original intent | Requiring the deleted row to remain visible, resolving the current workspace before receipt recovery or refreshing inferred original target state |
| 2026-10-14 | Retire Core deletion only after its Desktop workspace consumer migrates with original recovery | The gateway still consumes Core and replaces its generation; an interface-only substitution would lose an uncertain original handle | Removing a still-used API, labeling the CLI-only cutover as full migration or submitting the uncertain intent to a replacement provider |
| 2026-10-14 | Memory pages retain one concrete public binding across continuation in every actual client | Paging belongs to that business owner; replacing selection between pages can mix facts from different stores | Retaining agentMemory.list, refreshing selected role per page, partial results on failure or selecting another enabled candidate |
| 2026-10-14 | Share exact published-contract selection after the second real CLI consumer appears | Memory and Schedules require the same protocol/contract/release refusal policy | Duplicated role arbitration, forcing advertised metadata into the current protocol or private backend imports |
| 2026-10-14 | Workspace Memory reads expose a real public context capability over the published TS contract | Existing consumers must survive Core list deletion without reaching into private context adapters | A Core query alias, a copied schema/DTO, private-context imports or deleting unrelated workspace lifecycle coverage |
| 2026-10-14 | Every Schedule entrance uses its public installed owner, with original revision and nullable patch captured once | Final Core update retained a competing entrance and enabled refresh to replace a submitted edit basis | Keeping schedules.update/DTO/feature gate, reading a new revision on retry or merging current draft values into pending input |
| 2026-10-14 | Consume the publisher snapshot directly instead of maintaining a Core Schedule read projection | The publisher already owns fields/revision/timestamps; NextRunAt owns enabled status, and a Core numeric envelope must not narrow it | Duplicate Enabled state, a replacement handwritten facade DTO or silently narrowing uint64 revisions |
| 2026-10-14 | CLI capability negotiation uses plugins and resolves the concrete Schedule contract | A plugin-owned business feature does not need a second Core feature/method namespace | Schedule-specific capability preference, unavailable-port fallback or selecting another provider for an unresolved edit |
| 2026-10-14 | Forms and adapters consume one generated publisher Go projection from the CLI contract package | Removing Core creation must not introduce a handwritten second request schema or private backend import | Retaining CreateScheduleRequest, duplicate draft structs, adapter-only aliases or generating independent metadata owners |
| 2026-10-14 | Expose generated method/contract metadata with optional package-scoped naming | A real second CLI consumer needs the same projection; package qualification already names the owner | Forwarding metadata wrappers, copying constants or requiring redundant exported prefixes |
| 2026-10-14 | One unresolved creation intent resumes its original draft before another form or provider lookup | No Schedule ID exists before creation acknowledgement; a new form must not allocate a replacement mutation after response loss | New IDs on retry, selected-role replay, empty-draft resubmission or a second success/result cache |
| 2026-10-14 | Decode the owner acknowledgement before retiring client intent | A response that cannot establish the required accepted reference must not lose its original recovery identity | Retiring first, storing an independent success result or generating a replacement command after decode failure |
| 2026-10-14 | Business refusal evidence is witnessed at the owner call, not inferred from a decoder error type | Consumer projection can fail with the same error class without proving that an accepted mutation was refused | Letting a consumer codec originate the business outcome or treating malformed output as definitive rejection |
| 2026-10-14 | Manual execution uses the generic installed callback opening and removes its Core facade | Backend capture/reconciliation and Runtime acceptance already own the transitions; the selected-role wrapper adds another entrance | Keeping schedules.runNow, another callback route, a new Run acceptance store or replay through current selection |
| 2026-10-14 | CLI pending installed commands belong to the existing workbench and retain only original intent | Client restart must not rediscover a provider or allocate a new command after acknowledgement loss; business receipts remain with their owner | A second outcome ledger, fresh command IDs, selected-role replay, automatic outcome inference or a compatibility facade |
| 2026-10-14 | Persist accepted input bytes as binary in the local intent projection | JSON formatting changes raw JSON byte layout; original request evidence must survive durable encoding exactly | Reconstructing input from current feature data, comparing only current business state or retaining two writable inputs |
| 2026-10-14 | Unavailable owners and replay-store/deadline fences retain unresolved intent | Admission failure does not prove that an earlier dispatch was refused or failed | Clearing unknown intent, silent success, retrying a replacement installation or inventing a new success record |
| 2026-10-14 | CLI Schedule catalog captures the selected installed contract once and uses public calls | Cursor continuation belongs to that concrete owner; reselecting per page can mix different business catalogs | Retaining schedules.list, importing private producer types, choosing another enabled provider or refreshing a selection mid-traversal |
| 2026-10-14 | Publisher contracts generate typed CLI Go through the same feature pipeline | Independent consumers need concrete request/output types without a handwritten second schema owner | Raw JSON as the CLI business model, duplicated structs/descriptors, float-rounding uint64 bounds or silently replacing the advertised protocol |
| 2026-10-14 | Named embedded public DTOs stay in the generated wire graph | Removing one standalone method must not erase a public embedded owner still used by other requests | A hand-written PageQuery TS type, retaining a dead API to keep generation working or deleting valid pagination checks |
| 2026-10-14 | Contextual actions are ordinary Session components; business calls use canonical operations | The plugin owns its form, while Session revision/command/receipt remain with their existing owner; local menus need no I/O | Closed action enums, a special rename RPC/CLI wrapper, private Host forms, querying a new revision to construct a menu or retargeting an open draft |
| 2026-10-14 | Capture accepted Session ID/revision/title when opening the action and retain the original uncertain mutation | Navigation and response loss cannot change the submitted business intent or fabricate success | Reading current navigation after await, changing command identity on retry, optimistic saved state or silently adopting a conflicting revision |
| 2026-10-14 | Tool renderers use ordinary components over public frozen recorded data; retire all guest machinery | Current installed presentation is not historical execution authority; the result and its Run remain with existing owners | Historical script installation, Tool replay, old readRenderer aliases, duplicate Tool projection schemas or preserving unused native/page channels |
| 2026-10-14 | Resource unavailability preserves its I/O cause and cancellation category at the public adapter | Ordinary ESM resources require the same integrity/refusal guarantee as the removed carrier | Weakening symlink/missing assertions, leaking bytes on error, replacing cancellation with unavailable or swallowing the underlying failure |
| 2026-10-14 | File renderers use ordinary components over frozen accepted reads | The read is already owned by Runtime; a carrier refresh is not another file access owner, and a component needs the same public module/lifetime contract as other placements | File HTML compatibility, live navigation retargeting, backend activation for static data or a second file store |
| 2026-10-14 | Successful setup precedes explicit once-per-session ActivationResource start | Provisional registration must not advance business work; lost activation acknowledgement cannot duplicate the loop | Starting the loop inside setup, browser wake-ups, a second host timer or another scheduler |
| 2026-10-14 | Schedules owns clock/scan/original private recovery, with host driver/export deletion | Enabled own installations must recover captures independently of provider selection or UI; manual receipt belongs to the original private command | Selected-only host scanning, public time/dispatch exports, rereading changed definitions, new identities or a competing receipt store |
| 2026-10-14 | Managed plugin notices use authenticated contribution ownership and existing resync | Unknown plugin reads must reread their owner; notices carry no results or installation claims | Business switches in generic notification, arbitrary source/installation notices, another event bus or treating invalidation as success |
| 2026-10-14 | Background activation uses the existing connection/setup lifetime and an immutable installation target | Passive role declaration needs an owned Runtime link, independent of UI/provider selection | Another activation scheduler, competing enablement state or a worker started merely to register a contribution |
| 2026-10-14 | Restore retained caller context through original Commands evidence and authenticated Actor | A backend worker must not reclassify a human/model capture or invent its original parent after process loss | Trusting arbitrary header identity, current-provider replay, a second result ledger or adapting old routes to a new contract |
| 2026-10-14 | Business refusal belongs to pure capability; setup transport owns its own DTO projection | Delivery and connection adapters cannot import outward MCP/Core wire owners; existing identity models remain the validators | Architecture exceptions, compatibility aliases or duplicated validation rules |
| 2026-10-14 | Schedules owns component templates and binds its own installation operations; remove the final business Views vocabulary | UI editing and template authoring are feature responsibilities; the loader only admits technical resources/contracts | Generic Schedule template validation, a closed business kind, private Desktop controls or retained HTML readers |
| 2026-10-14 | Source coverage distinguishes actual CLI consumers from graphical consumers | The page uses installed plugins.call while existing Core facades remain real terminal calls; source evidence must name the correct surface | Hand-listed Schedule exemptions, declaration-only inventories, dummy graphical calls or treating green coverage as complete migration |
| 2026-10-14 | Schedules owns complete manual Run capture/reconciliation in one typed runNow operation | A component must invoke its exact installation without rebuilding the host prepare/dispatch workflow or changing the captured caller between commands | Keeping prepareManual as a UI prerequisite, a feature-specific public wrapper, a second route/result ledger or new identity after lost acknowledgement |
| 2026-10-14 | Derive manual resource identities from framed target/command identity and keep execution in the existing private queue | Original capture must be locatable on private-owner recovery without mutable mappings or rereading an edited definition | Ambiguous delimiter framing, random replacement identities, a new mapping store or adapting old uncertain routes to a different contract |
| 2026-10-13 | Skills consumes canonical Core operations directly from a public client-scoped component | The existing owners already admit discovery, library and revision-based proposal decisions; personal library inspection does not require a project | Another backend/store, four read wrappers, private Host forms, a curation generation or copied DataQuery catalogs |
| 2026-10-13 | Classify Core errors through the shared Host SDK module | Bundling another RpcError constructor breaks instanceof checks on errors from the existing Runtime client | Structural guesses, copied RPC classes or feature-specific error wrappers |
| 2026-10-13 | Scan the actual owning production component project for typed API consumers | Installed ESM is loaded from release resources and has no static Desktop import | Dummy imports/calls, counting visual fixtures or adding operation exemptions |
| 2026-10-13 | Observe admitted command routing once and project exact captured contracts into existing read invalidations | Public components bypassed feature adapter notices; uncertain dispatch can leave another projection stale | Business switches in generic dispatch, duplicate feature publishers, outcome copies or treating a notice as success |
| 2026-10-13 | Recover Schedule Tool output through the current bound receipt using original raw arguments | Tool argument translation must not replace persisted evidence or lose the original export/source authority | A separate originalCreateResult RPC, restored ServiceKind dispatch, digest-only receipt storage or a duplicate connection waiter |
| 2026-10-13 | Memory components bind their own declared contribution and generated complete capability | Public UI must enter the same private business/receipt owner without a current-provider wrapper or fabricated host result | Private Desktop Memory controls, plugin readMemory envelope, copied item state, tools.invoke or UI-only backend routing |
| 2026-10-13 | Add client scope with optional frozen targets and no file-focus dependency | User Memory must remain available without a workspace; irrelevant focus changes must not retire an uncertain command observer | Requiring a workspace for user scope, private author stores/currentSession, another workspace authority or focus-driven Memory remounts |
| 2026-10-13 | Publish a uniform explicit client-telemetry collaborator and one SDK projection type owner | Ordinary Diagnostics needs local traces/logs/metrics without private state imports; the existing store remains their mutable owner | A Diagnostics branch in the loader, an ambient store/service lookup, duplicate SDK/store row DTOs, a copied observation ledger or a fake Runtime diagnostics query |
| 2026-10-13 | Remove carrier-specific row/byte omission while retaining actual telemetry buffer capacity | Ordinary in-process components no longer use the bounded HTML channel; detached reads cannot reconstruct data already evicted by the owner | A retained omission epoch/reader, arbitrary new quotas, pretending snapshots are current mutable facts or clearing Runtime accounting |
| 2026-10-13 | Project Session component targets from the existing navigation owner and call canonical trajectory directly | Session identity is captured interaction data; the old read wrapper/HTML carrier adds feature knowledge without owning history | Private Session stores in author code, an ambient currentSession lookup, a copied history store, a new RPC wrapper or a retained sessionTrajectory kind |
| 2026-10-13 | Capture descendant scope with an accepted cursor chain; retain accepted pages on failed reads | Runtime owns capability/cursor admission and durable evidence; rendering owns its local page and filters | Changing cursor scope mid-chain, advancing a failed cursor, fabricated duration/usage or component disablement deleting history |
| 2026-10-13 | Transport admitted point contracts as canonical JSON strings and parse metadata only for display | The same JavaScript numeric rounding risk as callable contracts would change exact schema identity; the Runtime point owner remains authoritative | Object/string compatibility readers, re-encoding parsed schemas, relaxed identity or another schema compiler |
| 2026-10-13 | Derive HTTP query projection from the owning endpoint catalog and credit only consumed generated resource URLs | Native module imports invoke the existing leased Core resource query; an RPC-only callsite checker cannot see that real consumer | Dummy readResource calls, hand-written query exemptions, unused metadata credit or a second resource route |
| 2026-10-13 | Keep provider choices in Runtime and project them through the existing built-in manager/DataQuery owner | Enablement and availability cannot silently select a role. Real human recovery needs visible recorded choices and explicit candidate submission | Local authoritative selection state, first-candidate defaults, inferred enablement selection or placeholder API calls |
| 2026-10-13 | Share the existing contribution join mechanism across components and management I/O; preserve original promises | New inspected reads and uncertain settings commands need the same cancellation/join owner and intact mutation identity | A second lifetime root, wrapper promises losing retry/key fields, unjoined fire-and-forget work or automatic replacement commands |
| 2026-10-13 | Host projects immutable workspace/focus context; Diff owns its local comparison mode and calls canonical workspace operations | Current workspace/focus already have owners, and only the replaced Diff carrier consumed the Host baseline preference. Ordinary authors need data and explicit public operations, not private stores | A second workspace selector, copied mutable Host stores, keeping the baseline owner/read wrapper, private author imports or opaque generic service lookup |
| 2026-10-13 | Project optional generic material keys to the existing workspace navigation consumer | File-edit navigation must find admitted providers or open its catalogue; the consumer owns that meaning while loading remains generic | A Diff branch/package-name check in the loader, preferred first-party fallback, a new routing registry or competing current selection |
| 2026-10-13 | Ordinary Usage components call the canonical public accounting query through their explicit SDK context | A trusted installed component needs no feature-specific carrier or duplicated query envelope; Runtime still owns all recorded accounting | Keeping plugins.readUsage, a dummy backend contribution, copied accounting/cache state, HTML compatibility or treating component retirement as Run cancellation |
| 2026-10-13 | Permit an absent own callable contribution for actual content-only components; keep explicitly authored references strict | Icon gallery requires shared React/UI and immutable package content, not a backend; an invented operation would add an ownerless contract | Dummy backend/callable capability, static HTML compatibility, implicit service discovery or weakening invalid-reference admission |
| 2026-10-13 | Expose the RPC owner cancellation signal to technical resource subscriptions and use it as the sole client closed fact | Clearing stream handlers during client shutdown otherwise strands lease joining; the resource observer must learn the original connection outcome | A second closed boolean, idle lease timers, suppressing cleanup errors or a separate client lifecycle root |
| 2026-10-13 | Keep resource URL authority inside an existing Core subscription and route each asset read through the public resource query | Native/browser ESM and relative assets need ordinary URLs without persistent bearer credentials; subscription cancellation supplies an existing stop/join owner | Tokens in durable data, permanent unsigned package URLs, timers/heartbeat leases, bypassing exact source/integrity checks, Blob source rewriting or a second asset/installation database |
| 2026-10-13 | Build host-provided module exports in the actual workbench graph; require reload after evaluated release/Runtime replacement | React Hooks and provider identities must refer to the actual renderer; ESM disposal cannot unload code or top-level effects | A duplicate React bundle, ambient global module lookup, runtime TSX/new Function, unbounded unique hot-reload URLs or a sandbox claim |
| 2026-10-13 | Transport canonical callable contract JSON as a string; emit its identity bytes from the existing generator | JavaScript object serialization rounds integer Schema limits and breaks exact identity. Metadata interpretation may parse a projection, but the dispatched contract retains its original bytes | Relaxed capability identity, unsafe integer rewriting, object/string compatibility readers, another schema compiler or a hand-authored TS descriptor |
| 2026-10-13 | Publish a narrow explicit component context and use existing Rolldown ESM with host React/DOM/JSX/SDK/UI externals | Reviews Board is the demonstrated author; current HTML carrier cannot supply ordinary shared Hooks or command interaction | Runtime TSX evaluation, global service lookup, copied private aliases/state, another bundler or claiming module unload from component disposal |
| 2026-10-13 | Integrate main Resume acceptance and advance Core to .5 while keeping backend .4; original-start verification uses the same capability-aware fingerprint and plugin-scoped key | Main changes the shared command acceptance projection without replacing plugin caller authority or Schedules original captured dispatch; generated clients must agree with the merged current shape | Restoring retired automation owners, dropping actor isolation/captured request checks, hand-merging generated schemas, or dual protocol readers |
| 2026-10-13 | Separate pure consumer-owned point values from callable capability contracts; reuse the existing JSONSchema compiler and public operation route | Values can name implementation or presentation data; the consumer owns their schema, applicability and composition while callable contracts own method admission | String-only point lookup, substrate Reviews enums, a new IDL/compiler, checklist data presented as an executable analyzer, or a second composition/receipt store |
| 2026-10-13 | Keep named resolved collaborators immutable in the existing session owner and use them for SDK setup and nested authority | A running entry must not change provider when mutable selection changes; setup resources and canonical callback references share its owned retirement | Re-resolving a role on every nested call, a second activation/installation ledger, SDK ambient service lookup or converting business receipt identity |
| 2026-10-13 | Merge main accepted-continuation recovery without restoring replaced automation/resume-claim owners | Preserve independent product work and its current owner while retaining unified actor/callback and original command semantics | Taking one conflict side wholesale, restoring obsolete wrappers or maintaining two acceptance owners |
| 2026-10-13 | Gate managed entry launch by mandatory declaration bindings and gate publication by exact live exports/methods | MCP initialization alone does not prove installed contribution implementation; the existing attempt/session ledger already owns cancellation and teardown | Publishing Tools first, a second activation ledger, changing persisted enablement on handshake failure or feature-specific live admission |
| 2026-10-13 | Resolve named callable dependencies from declarations and validate mandatory edges per executable entry | Metadata is available before code, and contributions sharing one process cannot evade its mandatory startup graph; optional collections must not create false activation edges | Starting code to discover requirements, ambient service lookup, fallback provider choice, or treating the resolver as full setup/lifetime acceptance |
| 2026-10-13 | Make the private Schedules captured request own immutable original trigger evidence through dispatch | The dispatcher/scanner is a later observer, not a new initiator of already admitted human/model work; original context must survive restart and uncertain acceptance | Deriving origin from wake-up time, reading mutable current UI/selection, copying trigger state into the host or arbitrating new versus stored origin |
| 2026-10-13 | Capture origin/parent from adapters and concrete target from the admitted call, then retain it in the original command route | Calling context is owner evidence, not a business argument or credential; same-command recovery cannot change it. Interaction remains the logical Tool identity owner | Caller-authored origin flags, provider ToolCall IDs as identity, implicit UI target selection, another command ledger or a retained invocationId reader |
| 2026-10-13 | Advance Core and backend/declaration contracts to .4 and regenerate real consumers | Required target and replacement Tool invocation metadata are deliberate current-contract breaks | Admitting old SDK metadata through an alias, dual protocol readers or silently reconstructing old unresolved calling context |
| 2026-10-13 | Admit installed unary query/command semantics inside the one Delivery Endpoint and reuse original command routing | One typed envelope must retain concrete consumer/provider evidence without creating another business result owner; query keys are rejected and changed captured binding conflicts | Separate feature routes, caller-authored kind flags, a host result cache or rediscovery during receipt recovery |
| 2026-10-13 | Replace closed services with independent contribution IDs and complete consumer-owned contract/export bindings | Server identity is execution placement; it cannot identify a declaration or define a business role. Real Memory/Schedules and Reviews now use the generic representation | String-renamed ServiceKind branches, contribution=server aliases, same-named export fallback or schema-only declarations |
| 2026-10-13 | Move Flame declarations to one referenced strict namespace file and remove the inline apiVersion parser | Root identity and launch configuration keep their portable owners; local contracts can be admitted before activation without executing code | Another package identity manifest, dual inline/file declaration readers, old-version interpretation or remote schema loading |
| 2026-10-13 | Capture complete capability plus contribution/export in original command routes and refuse former host tables | Reconciliation must preserve the exact endpoint even when selection/export availability changes | Rediscovering a current method, recovering through a bare export-less call, transferring receipts to another contribution or converting old uncertain evidence |
| 2026-10-13 | Prioritize development with per-slice compilation and only necessary focused checks; concentrate full/race/cross-module verification at the final stage | The user's latest instruction explicitly rejects repeated long test rounds after small implementation changes; retain regression coverage and record deferred verification honestly | Repeating full Runtime/CLI/race/package checks each round or presenting earlier passes as verification of later source |
| 2026-10-13 | Move Memory callback references to invocation metadata and name durable input business_input_json | Temporary collaborators must not become business identity or persisted credentials; exact-schema admission makes the semantic format change explicit | Ignoring Grant in hashing, scrubbing old input, dual readers/writers or a second receipt owner |
| 2026-10-13 | Check local required callback metadata only before a new private maintenance admission | A definitely unattempted invalid new call must not poison its identity; existing pending/completed work must reconcile without new permission | Validating before original receipt lookup, writing pending before known local refusal, or querying receipts through a second admission path |
| 2026-10-13 | Derive actual feature registration, host queries and package contract artifacts from one typed Method source before generic declaration cutover | A loader cannot validate consumer-required live exports without current feature metadata; creating a second schema beside registration would duplicate authority | Handwritten package schemas, schema-only scaffolding, a business-aware generic generator or retaining string registration as a feature compatibility path |
| 2026-10-13 | Treat Memory's body Grant persistence as an unresolved defect requiring an actual metadata/input/data cutover | Temporary call credentials are observation/lifecycle collaborators, not durable business identity; both current stores retain them | Ignoring Grant during hashing, silently stripping existing stored input, pretending Tool metadata migration already fixed typed maintenance or adding another receipt store |
| 2026-10-13 | Give pure callable contracts one public owner outside MCP transport, and have SDK/generation consume admitted values | Domain installation admission cannot import the outward SDK; duplicating schema/fingerprint admission would create competing contract rules | Importing transport into Domain, a private/public validator twin, type aliases or another SDK module |
| 2026-10-13 | Preserve canonical wire JSON, schema and protocol .3 during the Go source ownership break | Package relocation does not change callable semantics; real Reviews regeneration and live receipts prove the same contract | Bumping wire identity for internal/source relocation or leaving old source wrappers |
| 2026-10-13 | Keep generic MCP source authorization feature-free; compose a consumer-owned prepared call and release over the captured executable | Memory model capture and Schedules read invalidation are actual feature policies, not source-registry authority | Renaming ServiceKind to string branches in the pool, a feature registry in generic authorization, or leaving the bootstrap Memory body in place |
| 2026-10-13 | Recheck source/configuration/session/cancellation after collaborator preparation; separate resource release from dispatch-dependent observation | Preparation can block while source authority is withdrawn; rejected calls must release resources without issuing business notices | Holding global admission through I/O, validating only before preparation, or treating release as evidence of dispatch |
| 2026-10-13 | Propagate configured Memory collaborator-resolution errors while preserving normal unconfigured-role keyword retrieval | The resolver already distinguishes nil,nil absence from storage/credential/configuration failure | Logging an unexpected resolution error and pretending the role was absent |
| 2026-10-13 | Bind consumer-owned immutable callable capabilities to named exports in the existing Backend, with the whole canonical contract checked on discovery and dispatch | Generic declarations need a real required-method/export relationship; a flat method list cannot prove it | A second handler registry, discovery-only checking or omission-triggered bare-call fallback |
| 2026-10-13 | Generate canonical capability metadata separately from presentation Schema/TS data, using the same Go-owned Method source | The existing emitter rewrites local references and must not originate callable identity | Fingerprinting rewritten Schema, handwritten required-method/schema twins or retaining bare-array generator input |
| 2026-10-13 | Retain export identity on original receipt queries while keeping one semantic method/private receipt namespace for shared handlers | Export identifies routing; the same handler still owns one business fact and logical identity across entrances | Rediscovering a replacement export, export-qualified duplicate business receipts or a host result cache |
| 2026-10-13 | Extend the one runs.start opening owner with captured new Session/Run targets and delete the automation opening/receipt path | Schedules requires atomic Core acceptance with exact captured identities and durable reconciliation after material retirement | Separate sessions.create plus runs.start, a Schedules-only alias, competing automation receipts or legacy readers |
| 2026-10-13 | Scope Core logical command keys by installed actor, excluding release/activation/configuration | The actual managed session establishes source authority; replacement must not invent a new original command identity | Publisher/input-supplied actor identity or release-qualified replay keys |
| 2026-10-13 | Pass ephemeral callback references in MCP metadata, outside admitted business input | Activation/call replacement changes observation but cannot change original business identity or saved pending work | Persisting credentials/grants, adding refs to private command input or rehashing DTOs |
| 2026-10-13 | Bind Schedules private capture to one Runtime store namespace before queue creation | A fresh Core directory has no original acceptance and must not silently recreate retained private work | Rebinding captured work to whichever Runtime is currently reachable |
| 2026-10-13 | Reenter pending Schedules dispatch only through original Core acceptance reconciliation; Memory pending remains non-redeliverable | Core owns durable acceptance and private Schedules owns acknowledgement; external model effects have no equivalent receipt | Blind model retry, fabricated host completion, network I/O inside a private database transaction |
| 2026-10-13 | Retain the 128 MiB aggregate package bound and remove the 16 MiB file bound | The actual native public-Client Schedules package is 70,358,850 bytes and cannot satisfy the former per-file limit | Publisher-specific exemptions, unbounded copying or increasing aggregate capacity without evidence |
| 2026-10-13 | Close the existing session done signal before joining every Interaction sender and closing its event channel | Scope Process settlement can precede the final observed model callback; direct child requests are producers too | Closing while callbacks send, recovering send-on-closed-channel panic or dropping buffered accepted facts |
| 2026-10-13 | Publish the canonical RPC handler once in the existing callback lifecycle owner after Endpoint construction and before workers start | Recursive backend-to-host calls create a real assembly dependency knot; the listener/activation owner must own publication and admission | A mutable Endpoint holder/proxy, a callback factory reading ambient state, or a second Delivery pipeline |
| 2026-10-13 | Keep Schedules' Runtime Client adapter in its executable main package and its business admission port pure | Main is independently built and not imported by bootstrap; Application is imported by host consumers and cannot import root Runtime or Protocol | Duplicating the Core client, Protocol DTOs in Application, or moving the entire backend only to break this cycle |
| 2026-10-13 | Cancel and join the canonical SSE source before ending its enclosing callback; enforce message limits per reply/frame | Handler return alone does not prove its source settled, and an indefinite stream has no finite total response size | Fire-and-forget stream sources, buffering the full stream or a total-stream byte limit |
| 2026-10-13 | Fence each Memory model command before provider I/O, retaining its admitted input for the callback lifetime | Model requests create external effects; lost observation cannot authorize resending the original identity | Declaring completion a query, consuming budget without identity, or automatic resend |
| 2026-10-13 | Use one lazily opened Runtime-owned loopback endpoint for unary callbacks, with one activation credential and immutable call-scoped method bindings | Proposal 10.1 explicitly selects two physical transport directions; stdio is not an arbitrary reverse RPC channel and per-invocation listeners are unnecessary | Smuggling reverse calls into stdio, a listener per invocation, browser relays or a second business store |
| 2026-10-13 | Keep releasing callbacks in the lifecycle owner until joined; cancel all siblings before joining any sibling and close incomplete body reads on cancellation | Early removal loses pending work; sequential cancellation can deadlock reentrant calls and request parsing can otherwise delay retirement | Deleting the call before settlement, cancel-and-join one sibling at a time, or relying on the remote writer to finish |
| 2026-10-13 | Keep Memory budget/model/scope contracts with Memory while sharing typed callback transport publicly | The generic endpoint must not impose Memory’s two-call policy or know Memory business operations | Moving Memory policy into the public SDK or retaining the private richer HTTP path |
| 2026-10-13 | Keep external proposal and review read-only; record corrections in owning repository design documentation and this ledger | The task explicitly identifies read-only inputs; corrections must not rewrite review evidence | Editing the external inputs |
| 2026-10-13 | Give provider choice one durable composition owner, independent of enablement; preserve the selected reference when its provider becomes unavailable or is removed | An enabled candidate must not implicitly win, and absence must not silently choose another owner or transfer private data | Scanning enabled installations, exclusive enablement, publisher/load-order selection or automatic replacement |
| 2026-10-13 | Configure initial provider choices by exact distribution package/contribution and commit them with initial catalog admission | Default selection happens once without giving first-party code lasting priority; ambiguous defaults fail instead of taking the first match | Filesystem order and reselecting defaults on restart |
| 2026-10-13 | Freeze the public semantic method, query/command kind, target meaning and exact structural schemas in each original command route; derive the invocation fingerprint from that immutable value | Receipt recovery must validate the original contract without discovering a current mutation or depending on a private build revision | Name-only invocation, a competing stored checksum, current-schema receipt interpretation, or an old route reader |
| 2026-10-13 | Discover new-call metadata outside installation admission, then recheck the concrete installation/release/server before storing the route | Network I/O must not block installation changes, and a captured automatic provider cannot persist a route after withdrawal | Holding admission during RPC or persisting a stale prepared route |
| 2026-10-13 | Use one managed checkout and one canonical progress file, exposed at the requested path by a symlink | Isolate product changes from concurrent user work without creating competing progress copies | Editing product files in main or maintaining two progress ledgers |
| 2026-10-13 | Preserve the existing Go-owned contract-to-schema/TypeScript direction for the public authoring slice | The existing generator does not consume JSON Schema to produce Go; reuse one structural owner and existing generation mechanisms | Parallel handwritten Go/Schema/TS or a new custom IDL compiler |
| 2026-10-13 | Remove separate release approval; retain selected digest and desired enablement | Explicit installation/update is the trust decision; enablement remains a separate product preference | Renaming approval, automatic tool approval or a legacy state decoder |
| 2026-10-13 | Initial install remains disabled until explicitly enabled/configured; selection preserves desired enablement and refuses an enabled candidate missing retained required inputs before mutation | Keep user preference while preserving construction invariants and credential-recipient binding; disable/configure/enable is explicit when credentials cannot survive | Silently reusing executable secrets, implicitly resetting user preference or publishing invalid configuration |
| 2026-10-13 | Permit disable/re-enable of the same selected code while its execution dependency is held; retain quiescence for selection, configuration and uninstall | Recovery must be able to restore availability of the frozen release without replacing its code or inputs | Treating every availability transition as release replacement or making withdrawal irreversible while work waits |
| 2026-10-13 | Derive generated method-policy unions from registered Delivery metadata | The ownerRunStream policy already existed in main but the handwritten TypeScript union could not represent it | Adding a second manually synchronized vocabulary or dropping the registered policy |
| 2026-10-13 | Enumerate the default distribution with an owned, bounded directory reader | The required architecture gate exposed an unbounded directory read; the installation owner supplies the actual maximum | Ignoring the gate or choosing an unrelated arbitrary bound |
| 2026-10-13 | Reuse installed local development dependencies in the isolated checkout with local package links | Offline installation lacked cached packages; no source, shared config or credentialed service needed mutation | Sharing a node_modules symlink that resolves old contracts or running install scripts against shared state |
| 2026-10-13 | Preserve Git's absolute commit-message path in linked worktrees and resolve relative paths before changing directory | The existing hook blocked a valid local commit by prefixing an already-absolute path | Disabling hooks, weakening commitlint or changing shared Git configuration |
| 2026-10-13 | Publish the existing backend method binding in runtime/plugin and migrate all actual consumers together | A demonstrated public author boundary should have one implementation; independent Reviews must use the same method and Tool semantics as bundled backends | Keeping a private richer binding beside a public imitation, or creating a separate SDK module |
| 2026-10-13 | Only explicitly classified business errors are known refusals; empty classification preserves protocol failure | The former wrapper encoded every internal/commit failure as a business response, which could hide uncertainty | Treating all errors as definite failures or silently retrying a possibly committed effect |
| 2026-10-13 | Host unresolved-command evidence owns immutable routing, not mutation outcome; only the original owner's valid response discharges its dependency | No-Run commands need original provider/release retention without a competing business receipt | Client-current proxies, execution-only dependency holds, mutable provider retargeting, or host success fabrication |
| 2026-10-13 | Prove the current public backend subset through an isolated file-proxy SDK archive while its release metadata is still pending | Independent build evidence must not rely on Runtime internal source, workspace privilege or local replace; a temporary snapshot must not masquerade as a published artifact | Overwriting a cached published module, claiming an old release contains the new SDK, or publishing before the final authorization boundary |
| 2026-10-13 | Run backend, contract, failure and relevant race checks; do not write/run frontend UI, E2E or screenshot checks | Explicit task verification constraint | Running the composite frontend gate without inspecting its test components |
| 2026-10-13 | Merge current local main into the feature branch after each verified slice; do not push, publish, modify shared main history or use credentialed services during ordinary slices | The amended task explicitly requires incoming main synchronization; external/shared changes and credentials still require authorization | Deferring integration until final delivery or treating code-edit authorization as production authorization |
| 2026-10-13 | Dispatch admission is durably recorded before the network call; subsequent retries only read the original receipt | Transport loss or process death cannot prove whether the effect happened; the original binding survives availability/provider changes | Resending on absent receipt or selecting the current provider |
| 2026-10-13 | Store typed success and known refusal in the private receipt transaction; roll back refused mutation with a savepoint | A known refusal must replay the original decision while unknown commit/storage failures retain uncertainty | Bare success-only results or host business-result caching |
| 2026-10-13 | Keep historical command identities without an arbitrary lifetime-count cap or automatic eviction | Existing private receipts did not cap total historical usage; adding 4096 as a permanent product quota would break ordinary use, while eviction permits identity retargeting | Unmeasured permanent history quotas or expiration into a new provider call |
| 2026-10-13 | Use immutable Go-owned method contracts and Scope's existing schema derivation for authoring; reuse Runtime's TS emitter | Actual Reviews server, Go caller, Tool schemas and generated consumers need one structural owner | Parallel hand-maintained DTO/Schema/TS, a second SDK module or a custom IDL compiler |
| 2026-10-13 | Keep descriptor and generated files as projections; validate raw input and output at the bound method | Typed decoding can hide absent fields; artifact edits must not change a running method or create another registration path | Revalidating only a re-encoded DTO or classifying malformed output as a known refusal |
| 2026-10-13 | Give Reviews a read-only projection of its existing private invocation receipts | Recovery must not re-enter the mutation owner; the stored outcome and original arguments already supply the needed evidence | A new receipt/result table, same-key mutation used as a receipt query, or treating missing evidence as permission to resend |
| 2026-10-13 | Carry one admitted raw invocation fingerprint to private receipt owners; preserve numeric lexemes and normalize object order at the common boundary | Struct/map encoding and raw/decoded numeric encoding do not agree today; recomputing from a DTO can change identity | Map-only ordering, float64 canonicalization, backend DTO rehashing, silent same-schema hash changes or a compatibility hash fallback |
| 2026-10-13 | Persist immutable admitted JSON in host routes and private receipts; derive the digest instead of persisting a competing DTO hash | Original input must survive decoding, restart and receipt lookup; the actual input_json format change makes strict cutover enforceable | Changing hash semantics under the old schema, retaining only a re-encoded DTO, introducing a per-record epoch or keeping the former helper as an alias |
| 2026-10-13 | Share the pure public jsoninput value across Domain and SDK authors | Domain cannot depend on the MCP/Scope transport package; independent authors need the same encoding owner without Runtime internal source | Duplicated canonicalization implementations, importing transport into Domain or creating a separate SDK module |
| 2026-10-13 | Publish immutable typed exports through one public backend invocation/discovery transport | Installed operations need schema-bearing exports without connection code importing every feature's Go DTOs | Per-feature client registration, mutable published exports, former named-method aliases or a reflection/ambient service registry |
| 2026-10-13 | Keep Schedules settlement as the existing convergent accepted-Run handoff with one captured backend and stable captured RunID identity; Stage 3d carries it through Dispatch and deletes the old remote Accept entrance | Runtime's durable acceptance and the private pending queue already own recovery; an absent host dispatch record must not prevent settling that proven Run | Treating acknowledgement as another Run creation, current-provider retargeting, or replacing handoff ownership with a host business result |
| 2026-10-13 | Commit Memory maintenance receipt at its actual publication owner, with model I/O outside transactions | The new outer receipt transaction prevented concurrent human editing and rolled back required pending ledger evidence | Holding a database transaction through model calls, weakening the curation race test, or fabricating successful host maintenance evidence |
| 2026-10-13 | Persist private admission before maintenance model I/O; infer pending from absent reply and complete with publication in its existing transaction | Restart or response loss cannot authorize another model request; partial committed ledger evidence must survive while publication and receipt remain atomic | An in-memory running flag, retrying an absent receipt, a second checkpoint/result store, or rolling ledger facts back through model I/O |
| 2026-10-13 | Keep target meaning declared by the contract; capture concrete target/origin/parent at generic host admission next | Current Reviews uses installation-private facts; target metadata alone is not captured invocation authority | Pretending backend descriptors already implement installed provider/context binding |


## Review closure

Status is open until code and appropriate verification prove the requirement.

| Review item | Status | Implementation/evidence |
| --- | --- | --- |
| Original resource target for retained background commands | Stage 5ad real SQLite reopen, dispatched/observed Session/workspace targets and all origins verified; original Actor preserved and retarget/prepared/unrecorded resource callers refused; execution-member/full native proof remains open | application/integration/plugins/commands.go; commands_test.go; plugin/README.md; DESIGN_PHILOSOPHY.md |
| Observer host callbacks and original next authority | Stage 5ac installed unary/stream RPC+next, distinct receiver/caller Actors, active-cycle refusal, large usage, stop/join and source/outcome boundaries verified; stronger original refusal assertion now passed after isolated cache rebuild; full native acceptance remains open | plugin/callbacks.go/tests; mcpconnection/pool.go; pluginservices/model_observers.go; model_phases.go; model_observer_callbacks_test.go; model_endpoint_test.go; contract/go-api.json |
| Model preparation host collaborators and ownership | Stage 5ab installed selection/context/input authenticated reentry, original Actor/invocation, undeclared direct-call refusal, active-cycle refusal and stale reference boundaries verified; native/full effect composition remains open | pluginservices/model_selection.go; model_context.go; model_input.go; model_phases.go; calls.go; model_phase_callbacks_test.go; execution/interaction_context_reducer.go |
| Live independent model consumption and primary-output isolation | Stage 5aa captured-member common selected/auxiliary publication, persisted purpose and different-model/unknown recovery implemented; publication/stream/source/SQLite/-race boundaries verified; owner-policy/in-flight and full installed/native Run/Goal acceptance open | executionctx/model_calls.go; execution/interaction_model_calls.go; interaction_model.go; interaction_tool.go; pluginservices/model_capture.go; runs/reducer.go; execution_fact.go; commit.go; recovery.go; recovery_commit.go; sqlite/db.go; model_invocation.go; model_endpoint.go; recovery/persistence.go; matching regressions |
| One Run consumption owner across actual model calls and termination | Stage 5z per-call accrual, original usage/price, resume/interleaving/duplicate/context/terminal invariants implemented and verified; Stage 5aa live callback wiring implemented; full installed/native and total-work admission remain open | runs/execution_fact.go; execution_fact_commit.go; reducer_items.go; reducer_outcome.go; execution/interaction_accounting.go; interaction_session.go; interaction_durable_children.go; run/metrics.go; matching backend regressions |
| Known independent inner consumption under lost outer execution | Stage 5y source/Run/Goal atomic recovery, unknown cost, primary exclusion, rollback/context/restart proof verified; Stage 5aa live publication and role-aware recovery implemented; full normal-terminal/owner-policy admission and native acceptance open | sqlite/model_endpoint.go; application/runs/recovery.go/recovery_commit.go; adapter/run/recovery/persistence.go/model_evidence_test.go |
| Original executing Run/Segment for multiple physical model calls | Stage 5x active Model/Tool ownership, namespace/chronology/Session/wait/resume refusal and real callback source association verified; Stage 5ab preparation publisher captured; native/background/owner-policy proof remains open | executionctx/scope.go; execution/interaction_model.go; interaction_tool.go; model/endpoint_observation.go; sqlite/model_endpoint.go/tests; pluginservices/model_calls.go/tests; plugin/model_call.go; protocol/model_invocations.go/version.go |
| Multiple independent model calls, original budgets/observations and uncertainty | Stage 5w ordinary installed RPC unary/stream capabilities, three separate routes/IDs, full request budgets and physical evidence verified; total Run/Goal accounting/general remote composition/native acceptance open | runtime/plugin/model_call.go/tests; model_stream.go; callbacks.go; stream.go; pluginservices/model_calls.go/tests; model_capture.go; bootstrap/tool_environment.go/delivery.go/assemble.go |
| Cross-modality transformation and final input admission | Stage 5v source evidence/structural refusal, original Resume image, final Call/Stream/compaction conversion/refusal and counter boundary verified; full native/delegated/process acceptance open | runs/opening.go; resume.go; steering.go; ports.go; model/auxiliary.go; execution/interaction_context_reducer.go; interaction_model_input_test.go; interaction_waiting_test.go; protocol/version.go |
| Model input transformation before final admission | Stage 5t auxiliary and Stage 5u primary captured final-request/capacity/calibration/recovery verified; Stage 5v original-source/final-media admission now verified at its boundaries; Stage 5w independent callback input/budget/media handling now verified; general composition and final native acceptance open | runtime/plugin/model_input.go/tests; pluginservices/model_input.go/tests; model_context.go; auxiliary_model_observers.go |
| Auxiliary complete-input capacity and original model-owned options | Stage 5s final context/provider count/local estimate, output reservation, known/unknown/error semantics, reasoning, request equality and cancel/join verified; final native/all-workflow acceptance open | model/auxiliary.go; auxiliary_capacity_test.go; pluginservices/auxiliary_model_observers.go/tests; modeltext/request_estimate.go; maintenance/compaction_trigger.go/tests |
| Model route selection before exact binding and admission | Stage 5r public query/explicit publisher, fresh Run/auxiliary construction order, accepted Run identity and retry/Resume preservation verified; Stage 5w fresh installed per-call routes now verified; general composition/Run-Goal accounting/final native acceptance open | runtime/plugin/model_selection.go; application/integration/models/selection.go; pluginservices/model_selection.go; runs/opening.go; model/role_resolvers.go; bootstrap/assemble.go; tool_environment.go; corresponding tests |
| Primary model context contribution before admission | Stage 5p primary query/capacity/nonpersistence/waiting recovery and Stage 5q auxiliary exact capture/text/output budget/lifetime verified; Stage 5r accepted execution/auxiliary selection now verified; Stage 5s auxiliary token preflight now verified; per-call routes/general transformation/final acceptance remain open | runtime/plugin/model_context.go; pluginservices/model_context.go; execution/interaction_context_reducer.go; interaction_request_material.go; interaction_model_composition.go; interaction_checkpoint.go; installation_dependencies.go; pluginservices/auxiliary_model_observers.go; model/auxiliary.go; role_resolvers.go; corresponding tests |
| Baseline calibration and accurate old/new comparison | Resolved in planning | Current stage and decisions; external inputs preserved |
| Runtime/Scope single authority and private business fact ownership | Open; preserve throughout | runtime delivery, run execution, private plugin owners |
| Remove business-specific service/view knowledge from substrate | Generic MCP authorization/preparation, Registry.Service and closed Services candidacy removed; actual contract/export contribution cutover compiles; all business View kinds/templates/read wrappers removed in Stage 4e and all HTML renderer/guest machinery in Stage 4i and closed action wrappers in Stage 4j; Core Memory capacity removed with publisher bounds; business topics/record IDs and contract-specific notice routing removed; full Tool/dependency/streaming and final execution remain open | mcpconnection/authority.go; pool.go; pluginservices/tool_preparation.go; generic contribution contracts |
| Public operation shared by Tool, CLI, UI and other plugins | Public installed Go/Protocol/CLI/TS unary entrance and typed proxy implemented; all CLI Schedules and CLI/workspace Memory reads/human mutations use public contracts with original binding/input; all Core Memory business methods/DTOs/feature removed; Core records/enums removed and client projections relocated; Core capacity removed with published pagination bounds; generic plugin-data notifications installed for commands/background/Tool entrances; remaining Tool/all-consumer lifecycle and final process acceptance open | runtime/plugins.go; pluginservices/calls.go; CLI runtimebinding/agentmemory.go and terminal/agent_memory_management.go; desktop workspace public/memory.ts; TS client/pluginCommand.ts and mutationJournal.ts |
| Preserve origin, parent identity and concrete target | Unary/Tool/callback original context, Schedules durable trigger and authenticated background caller restoration implemented/compiled; final execution remains open | runtime/capability/invocation.go; application/operation/identity.go; delivery/handler_plugin_calls.go; plugin/invocation.go; mcpconnection/authority.go; Interaction tool; plugin_commands.go |
| Public React/TS components and shared module instances | Contextual Session naming, file-scoped Markdown, recorded Tool results, Reviews, own-bound editable Schedules, canonical editable Skills, bound editable Memory, client-local Diagnostics, Session-scoped Trajectory, workspace-scoped Diff, Usage and the component-only Icon gallery use actual ESM/resource loading; production graph exposes real host shared-module entries. Rendered Hooks/provider proof and full lifecycle acceptance remain open | plugin-sdk; Reviews Board/declaration; desktop/frontend/pluginModules.vite.mjs; componentModules.ts; InstalledComponent.tsx; Core plugin_resources.go |
| Consumer-owned collection/selection/decoration and plugin-defined points | Pure callable contract admission, exact export identity and composition metadata verified with actual Reviews; named callable dependencies and required entry-cycle checking now compile; named setup/frozen bindings and provider-attempt waiting now implemented; point value/schema/applicability/optionality/order and the Reviews analyzer consumer/producer now compile with focused checks; actual graphical point inspection and lossless raw contract transport now implemented; Stage 5h explicit ordered decoration choice and named setup required-member admission now verified; scoped execution composition, lifecycle/observed status/retention and full installed acceptance remain open | runtime/point; runtime/point_values.go; application/integration/plugins/points.go; pluginpackage/points_test.go; examples/plugins/reviews/backend/analyzers.go; examples/plugins/reviews/analyzer |
| Real callback, remote next, backpressure, cancellation and inner evidence | Unary transport and canonical Core RPC/SSE seams verified with real headless backends; captured context, protocol/refusal gates and joined subscription closure verified. Actual Schedules Core acceptance and installed source admission are now verified; actual consumer-owned Tool callback capture/retirement verified; public Scope model unary callback/full valid inner response plus outer failure and real cancellation now verified with an external-package subprocess; typed callback streaming, canonical Scope aggregate, transport backpressure/cancellation, byte budget and external frozen remaining-chain proof now pass; SDK backend register/bind and acknowledged duplex streaming now pass with a real stdio decorator/model-next executable; Internal Runtime mutation-stream original route/recovery is verified in Stage 5b; public Go/Protocol stream delivery is verified in Stage 5c; typed/callback Go streaming is verified in Stage 5d; TS mutation streaming is verified in Stage 5e; Memory production completion is migrated in Stage 5f; Stage 5g Interaction known-response/accounting survives outer failure and host reopen without continuation; Stage 5i installed transparent Interaction chain, original checkpoint bindings and held releases are implemented with real SDK HTTP boundary evidence; Stage 5j auxiliary/Memory utility capture, child identity and held-release lifetime are implemented/verified at their real boundaries; Stage 5k physical chat endpoint identity/admission/known outcome/restart and public JSON query are implemented/verified at their boundaries; Stage 5l current transparent Interaction known accounting now derives from exact source evidence; Stage 5m Session endpoint trajectory/export/deletion and title scope are implemented/verified; Stage 5n original endpoint pricing/current Interaction boot-loss accrual are implemented/verified; Stage 5o retained physical auxiliary/global reporting and coverage are implemented/verified; complete retention policy, embedding/multi-call accounting and other recovery classes, transformed-input/multiple-invocation contracts, transitive dependency lifecycle and full native acceptance remain open | runtime/plugin/callbacks.go; callbacks_test.go; client_callback_test.go; delivery/transport/http/rpc.go; stream.go; bootstrap/delivery.go; Memory rpc/models.go; plugin/model.go; model_test.go; model_external_test.go; plugin/stream.go; model_stream.go; model_stream_external_test.go; plugin/backend_stream.go; backend_stream_test.go; backend_stream_external_test.go; pluginservices/streams.go; streams_test.go; commands.go; mcpconnection/pool.go; delivery/handler_plugin_calls.go; registry.go; transport/http/stream.go; plugin_stream_test.go; client_stream.go; plugin_streams.go; plugin_streams_test.go; plugin_streams_callback_test.go; plugin_callback_caller.go; TS client/pluginStream.ts; client.ts; transports/http.ts; durableMutationJournal.ts; plugin-sdk/src/index.ts; execution/interaction_model.go; runs/reducer.go; sqlite/model_invocation.go; sqlite/model_endpoint.go; sqlite/trajectory.go; sqlite/trajectory_export.go; sqlite/session_mutation.go; persistence/trajectory_export.go; delivery/trajectory_export.go; segment/effects_finish.go; plugins/trajectory/src/component.tsx; recovery/persistence.go; runs/recovery.go; run/metrics.go; recovery/model_evidence_test.go; sessions/usage_reporter.go; delivery/handler_usage.go; sqlite/model_endpoint_test.go; cli/runtime_usage.go; plugins/usage/src/component.tsx |
| No-Run unresolved human command pins original binding and recovery evidence | Stage 4l-4o all actual CLI Schedule mutations retain original request/owner/revision before I/O; pending creation/edit/enablement precede new form/provider/revision reads. Capture/byte/store/owner/persistence/decoder checks pass. Native process replacement acceptance, remaining Memory/other entrances and cross-generation recovery open | cli/internal/application/workbench/installed_commands.go; cli/internal/application/installedcommand/command.go and command_test.go; cli/internal/delivery/terminal/schedule_management.go; runtime/internal/application/integration/plugins/commands.go; runtime/internal/infra/sqlite/plugin_commands.go; runtime/internal/bootstrap/plugin_command_recovery_test.go; examples/plugins/reviews/backend/receipts_test.go |
| One executable contract generation source | Canonical feature CAPABILITY_JSON is now emitted from the same pure identity for lossless TS calls; Go/Scope Schema/TypeScript data, canonical capability/TS token and live Reviews export matching verified through one pure owner; actual Memory/Schedules typed source/live method matching and packaged artifact generation verified; typed CLI Schedule Go projection now generated; complete author/consumer binding generation still open | runtime/internal/plugins/{memory,schedules}/rpc/contracts.go; plugins/{memory,schedules}/io.github.tangerg.flame/contracts; runtime/capability; runtime/plugin/contract.go; cmd/contractgen/feature.go; examples/plugins/reviews/backend/contracts.go; generated contracts and contract tests |
| Typed business refusal versus transport uncertainty across entrances | Pure capability.Refusal now owns typed business outcome independently of MCP; public backend evidence retained; installed refusal/unknown-ack projections implemented and receipt conflicts classified at their existing owner; final all-entrance execution acceptance open | runtime/plugin/method.go; method_test.go; Memory/Schedules classifiers; Reviews server_test.go |
| Recovery entrance independent of failed optional UI | Built-in manager provider/dependency/point inspection and localized optional-module recovery implemented; final lifecycle acceptance open | PackageManager.tsx; ProviderSelections.tsx; ContributionRequirements.tsx; PointInspection.tsx; InstalledComponent.tsx; CLI |
| Strict public protocol requirements without internal revision coupling | Backend wire and exact immutable method/capability/export contract verified; strict local declaration and real feature export binding implemented; named dependency resolution/public projection and fresh cross-plugin declaration checks now compile; managed launch/export publication gates now compile; provider waiting and setup/frozen bindings now implemented with SDK lifecycle execution; complete integrated activation, observed status/retention and all-consumer acceptance open | runtime/plugin/backend.go; binding.go; binding_test.go; public SDK handshake |
| Explicit candidate update, withdrawal, quiescence and uninstall behavior | Stage 1 enablement/update and Stage 2b unresolved-command lifecycle verified; public provider selection and actual manager implemented; final integrated acceptance open | runtime/internal/domain/integration/plugin/installation.go; runtime/internal/application/integration/plugins/installations.go; owning tests |
| Reject old host encodings without data mutation | Installation/command-route host shapes, digest-only/private bare-result and Reviews former DTO format refusal verified; full public cutover open | runtime/internal/infra/sqlite/plugins_test.go; schema_admission_test.go; managedmcp/receipts_test.go; Reviews schema_test.go; runtime/README.md |
| Preserve Memory ledger, watermark, curation and receipts | Existing private maintenance admission/publication/receipt and failure boundaries verified; public capability migration open | managedmcp/prepared.go; memory/transport/maintenance.go; prepared_test.go; memory transport tests; bootstrap/plugin_memory_maintenance_receipt_test.go |
| One Schedules wake/dispatch lifetime and retained Run acceptance | Stage 4g moves the sole loop into the backend after public setup/activation; private capture/receipt/authority/join subsets verified; full native non-selected/restart/withdrawal acceptance open | plugin/setup.go; mcpconnection/pool.go; schedules/transport/background.go; managedmcp/prepared.go; schedules/host.go; bootstrap/schedules_callback_loss_test.go; managed_services_test.go |
| Preserve package integrity, credentials, source identity and core authority checks | Open; baseline required | existing admission and policy regression checks |
| New acceptance coverage for unresolved mutation/update/provider replacement | Existing real commands verified, including database reopening and distinct installation replacement; generic/UI coverage open | bootstrap/managed_services_test.go; plugin_command_recovery_test.go; application/integration/plugins/commands_test.go |
| Standalone bundled/non-bundled author parity | Open | independent public consumer builds |
| Deletion of replaced APIs, carriers, schemas, examples and documentation | Open; audit every slice | final source and generated-contract search |

## Proposal acceptance coverage

Full P01-P48 completion remains open. Stage 1 supplies installation-state, default-initialization and incompatible-storage evidence; it does not prove the new public platform. Grouping names the evidence required, not a reduced scope.

| Cases | Required evidence | Status |
| --- | --- | --- |
| P01-P09 | Author parity, selected roles, plugin-defined points, declaration failure, provisional resources and available management | Open |
| P10-P20 | Reentrant host callback, installed model decorators, frozen ordering, admitted transformed input, command identity, stale next, preserved inner evidence, stream cancellation/backpressure and actual model identity | Unary headless callback/reentrancy/retirement and public Go Scope model callback/full valid inner result plus outer failure/cancellation subset verified; actual callback stream/aggregate/cancel/backpressure/external frozen remaining-chain subset now verified; SDK duplex/export/real stdio decorator-next subset now verified; Internal Runtime mutation-stream original route/recovery is now verified; public Go/Protocol stream delivery is now verified; typed/callback Go streaming is now verified; TS mutation streaming is now verified; Stage 5g production failed-outer known-response/accounting and host reopen are verified; Stage 5i transparent installed Interaction chain and original checkpoint/retention are implemented with HTTP boundary evidence; Stage 5j auxiliary/Memory consumers and captured lifetime are implemented/verified at their boundaries; Stage 5k physical chat endpoint admission/outcome/restart is verified at its boundaries; Stage 5l current transparent Interaction source accounting is verified; endpoint export/lifecycle, auxiliary/global/crash/multi-call accounting, transform/multiple-call contracts, transitive lifecycle and full native acceptance still open |
| P21-P28 | Shared operation entrances, receipts/conflicts, scheduling acceptance/capture, independent Run lifetime and Memory owner invariants | Actual Schedules acceptance/capture/retirement and independent Run lifetime verified; full shared installed entrances remain open |
| P29-P35 | Shared React binding, recoverable UI failure, target capture, shared resource owner, applicability, capacity and appearance owner | Open; UI/E2E execution excluded by user |
| P36-P41 | Generation safety, waiting dependencies, retained data, generic history, remote environment and headless host calls | Open |
| P42-P48 | Portable packages, strict current contracts, non-mutating old-state refusal, recovery, shutdown, diagnostics and final deletion | Open |

## Verification

- Handoff cleanup: the sole owned uncommitted modelref/endpoint.go draft was
  reversed exactly. Restored Runtime go build ./... passes. No new backend suite,
  full/native or frontend UI/E2E check was run for documentation-only handoff.
  Original proposal/review/draft hashes and user stash are verified unchanged;
  all owned build/test handles are terminal and no new scratch artifact remains.

- Stage 5ad: existing restoration baseline passed (0.825s), focused original resource
  target/reopen/refusal checks passed (1.144s). Whole application/integration/plugins
  passed (0.902s); SDK authenticated/background selected Call/Stream checks passed
  (0.719s). Runtime build, changed gofmt -l and git diff --check passed.
- Stage 5ad: no wire/storage/source API or concurrency owner changed, so no generator
  or new -race run. Full/native/vet/global-race/closure and Schedule failure remain
  final acceptance. No frontend UI/E2E/screenshot check was authored or executed.
- Stage 5ad: main 7ba40c117 is already integrated. Original input hashes/stash match;
  all handles terminal and SQLite fixtures close their owned databases after reopen.

- Stage 5ac: callback/observer baseline passed (0.819/1.973s); migrated endpoint,
  observer and auxiliary checks passed (0.686/4.991s). New SDK union and installed
  RPC/next/distinct-authority/fresh-cycle checks passed (0.795/2.673s).
- Stage 5ac: whole SDK/pluginservices/MCP passed (1.581/28.617/1.564s); focused
  -race passed (1.839/7.932s). Runtime build, contract generator, changed gofmt -l
  and diff checks passed. Generator changed only the intended Go API signature.
- Stage 5ac: main 7ba40c117 merged in 0227ffd94 with Scope v0.50.0. Merged Runtime
  build, SDK/composition checks (0.477/5.681s), Runtime/bootstrap go test -c,
  CLI/independent Reviews author compile and regeneration passed. MCP compiled
  with no tests matched by this focused selector. Compile artifacts were temporary.
- Stage 5ac: follow-up f3dfbe976 assertion check failed during shared Go cache setup
  before test execution (missing cached files, misleading missing-stdlib reports).
  Stdlib source presence is verified. Its targeted TemporaryDirectory/GOCACHE
  rebuild completed and the exact original-cycle unary/stream check passed (1.957s).
  This is an environment/setup failure followed by an actual pass, not a skipped test.
- Stage 5ac: no frontend UI/E2E/screenshot checks; full/native/vet/global-race/
  closure/Schedule acceptance remain final work. Original input hashes/stash match.

- Stage 5ab: baseline installed selection/context/input checks passed (1.752s).
- Stage 5ab: initial phase/selection/context/input/independent callback checks passed
  (2.499s); primary preparation/source checks passed (2.312s). After adding explicit
  cycle/host-composition ownership, targeted phase checks passed (2.679s).
- Stage 5ab: whole pluginservices passed (17.916s); focused callback -race passed
  (8.272s); Runtime go build ./... passed. Deferred per-contributor release then
  passed focused pluginservices/execution again (1.765s/2.706s). Changed gofmt -l
  and git diff --check are clean. All source assertions remain; none were weakened.
- Stage 5ab: main merge is already up to date at 14fca7975. Original inputs/stash
  match. Full/native/vet/global-race/closure and Schedule handshake are not rerun
  during this development slice. No frontend UI/E2E/screenshot check was run.

- Stage 5y initial recovery checks passed (adapter 1.322s; Application 0.794s).
  New source clone comparison failed correctly while diagnosing nil/empty DTO
  normalization; canonical-source and billing-projection comparison fixed it.
- Stage 5y known independent/source-change/rollback/reopen proof passed (0.789s).
  Actual Goal and unknown-attempt projection then passed (adapter 0.790s;
  Application 0.956s). Final recovery/commit selection passed (0.632s/0.972s).
- Stage 5y Runtime/bootstrap go test -c and Runtime go build ./... passed; temporary
  outputs removed. Scoped dependency/public-package/protocol/drift passed (3.275s).
  No pre-merge public wire/schema signature changed; no extra generator was needed.
- Merge ec8b986e0 includes main 67c9f498f. Resolved 16 conflicted files, preserved
  both semantic changes and regenerated merged Core .31 artifacts/samples. Invalid
  textual static-test splice repaired by retaining complete original test blocks.
- Post-merge delivery/Steer/acceptance/recovery checks passed (1.173/2.170/1.597s);
  scoped drift/version gates passed (2.483s); shared and desktop typecheck passed.
  Methods/mutation journal/plugin stream 64 tests passed (541ms); Steer recovery
  11 tests passed (591ms). Three static API-consumer AST tests passed (662ms), no UI.
- Post-hook desktop typecheck, Runtime build and Runtime/bootstrap source compile
  passed. Hook formatting/oxlint settled and its temporary backup was removed;
  original user stash unchanged. Changed format/diff checks pass; original hashes match.
- Full/native/all-workflow/vet/race, orphan schemas/older architecture/format and
  interrupted UI findings remain deferred for concentrated final acceptance. UI/
  E2E/screenshot authoring/execution remains excluded; incoming tests only preserved.

- Stage 5x initial owner fixture self-parent validation failed, corrected to a
  distinct valid parent. The original no-inference assertion is retained and
  reserved logical namespaces now have stronger explicit refusal coverage.
- Stage 5x related scoped checks passed: SQLite 1.076s, pluginservices 19.940s,
  execution 3.579s (media/auxiliary/evidence/usage cases). First recovery/context
  filter outputs said no tests to run; they were not counted as executed proofs.
- Stage 5x explicit owner/callback/SDK-background/current-recovery cases passed
  (SQLite 0.806s; pluginservices 2.071s; SDK 1.000s; recovery 0.538s). Final active/
  chronology/namespace/wait-resume/original-scope tests passed (SQLite 0.766s;
  pluginservices 2.110s; recovery 0.598s). Dedicated callback source proof 1.313s.
- Stage 5x Runtime/bootstrap go test -c and Runtime go build ./... passed; owned
  compile files removed by TemporaryDirectory. Core .30 generation and both shared/
  desktop npm run typecheck passed. Scoped dependency/public-package/protocol/
  generated-contract drift gates passed (7.468s). Changed gofmt/git diff checks pass.
- Full/native/all-workflow/vet/race and full required architecture remain deferred
  for concentrated final acceptance. Previously failed orphan-schema gate not rerun;
  no schema assertion/check weakened. No frontend UI/E2E/screenshot test authored/run.
- Main sync after b7c6f3c14: already up to date at 3d897dfb6. Original three input
  hashes and user stash unchanged. Every owned test/build/typecheck/generation handle
  completed; no owned background process or temporary compile artifact remains.

- Stage 5w background header fix: scoped selected/model-budget/captured/stopping/
  callback/auxiliary/installed-model checks passed (SDK 1.132s; pluginservices
  6.498s). Unary and stream accept the retained original background invocation and
  refuse a forged origin before handler work. A final explicit refusal-event
  assertion passed in the focused background regression (SDK 0.994s).
- Background sender diff/format checks passed. No exported signatures, Core schema,
  generated inventory or wire epoch changed in this follow-up. Real native command-
  route restoration remains a final integration obligation beyond the SDK projection
  probe. Main sync after 9fa2080f4 was already up to date at 3d897dfb6; handles settled.

- Stage 5w scoped existing/new model, auxiliary and callback checks passed
  (SDK 0.989/1.026s; pluginservices 5.449/6.469s). Reasoning string/Scope enum
  compile mismatch was corrected; an initial command ran from repository root
  rather than Runtime and was rerun from the owning module. Neither was skipped.
- Stage 5w final selected/complete-budget/selection/media/stream/owner/RPC cases
  passed (SDK 0.567s; pluginservices 3.676s). Three independent calls, distinct
  actual installed choices, same/changed ID dispatch fencing and known durable
  usage/price after outer failure/restart are checked at their existing owners.
- Stage 5w Runtime/bootstrap go test -c (temporary outputs deleted), Runtime
  go build ./... and contract generation passed. Generated diff is go-api.json;
  schemas/TS/Core/version are unchanged. Changed gofmt and git diff checks passed.
- Stage 5w scoped architecture/drift command failed only TestGeneratedSchemasResolve
  (2.597s): unchanged RecordedUsage, PageContinuation, PageOfItem and PageQuery have
  no references. This is a confirmed existing generated-schema obligation, not a
  green gate. Other selected dependency/public-package/version/drift gates reported
  no failures. Remaining older architecture failures are not revalidated this slice.
- Full/native/all-workflow/vet/race and independent process/author qualification
  remain for concentrated final checks. No frontend UI/E2E/screenshot authoring or
  execution; no frontend sources changed, so typecheck was not repeated this slice.
- Main sync after 2583125cd: already up to date at 3d897dfb6. Original three input
  hashes and user stash match the recorded baseline. All owned handles settled.

- Stage 5v original source/media and selected boundary checks passed (runs 0.514s;
  execution 1.430s). Initial no-transform compaction passed unsupported source to a
  counter; that defect was fixed with explicit modality handling, preserving the
  test assertion that neither counter nor generation receives unsupported media.
- Stage 5v valid Resume original-image and invalid-image waiting preservation,
  real original waiting transform and media matrix passed (runs 0.752s; execution
  1.507s). Original committed resume content is compared byte-for-byte; source,
  continuation, input IDs/usage/order/calibration assertions remain meaningful.
- Stage 5v broader affected Start/Steer/Resume/selection/captured/primary/auxiliary
  checks passed (runs 0.553s; execution 1.385s; model 1.489s; pluginservices 4.660s).
- Stage 5v Runtime/bootstrap go test -c, Runtime go build ./..., Core .29 generation,
  shared and desktop typecheck passed. Scoped dependency/domain/JSON-tag/protocol/
  public-package/version/schema/drift gates passed (2.500s). Full/native/vet/race/
  all-workflow acceptance is deferred; no UI/E2E/screenshot execution or authorship.
- Stage 5v main terminal-explanation changes merged without conflicts; post-merge
  desktop typecheck passed. Changed gofmt/diff/original SHA-256 passed, user stash
  retained; compile directories/owned processes settled. Backend .2/export 3 unchanged.

- Stage 5u initial source compilation passed after correcting checkpoint argument/
  restored-state wiring. Related primary context/waiting/captured/auxiliary checks
  passed (execution 2.096s; model 2.261s; pluginservices 2.941s).
- Stage 5u direct/compacted primary repetition/failure passed (execution 1.264s;
  selected capacity regressions 1.672s). Waiting original input/Options/order proof
  passed (1.813s) after deriving calibration from actual final request rather than
  its old pre-transform fixed estimate. Final primary exact/over-count boundary,
  no-generation refusal and waiting checks passed (2.554s).
- Stage 5u Runtime/bootstrap go test -c and Runtime go build ./... passed. Scoped
  dependency/domain purity/JSON-tag/protocol/public-package gates passed (0.963s).
  Full/native/vet/race/all-workflow acceptance deferred; no UI/E2E/screenshot work.
- Stage 5u changed gofmt/diff/original hashes passed; no old AuxiliaryModel/New
  constructor source remains, main up to date and user stash retained. Temporary
  compile outputs/owned handles settled; Core/backend/export versions unchanged.

- Stage 5t SDK and auxiliary/context/regression checks passed (plugin 0.555s;
  pluginservices 5.961s initial, 0.486s/2.256s after explicit selection and canonical
  absent Tool normalization). Final transformed token-capacity refusal passed
  (2.144s): the counter sees both transformations and provider generation stays zero.
- Stage 5t Runtime go build ./... passed; scoped dependency/domain purity/JSON-tag/
  protocol/public-package gates passed (0.758s). Source-only SDK/pluginservices
  compilation also passed; full native/vet/race/all-workflow suites deferred.
- Stage 5t incoming main Goal explanation changes merge cleanly; desktop typecheck
  passed. Existing incoming UI tests retained, without UI/E2E/screenshot execution
  or authorship. Core .28/backend .2/export schema 3 unchanged.
- Stage 5t changed gofmt/diff/original SHA-256 passed, user stash retained, main
  integrated and owned handles/temporary resources settled. Primary input transform
  support is expressly not yet proved by this auxiliary-only implementation.

- Stage 5s initial capacity/context/estimator regression checks passed (model
  1.458s; pluginservices 3.308s; maintenance 3.275s). Final cancellation/selected
  reasoning/Memory/role checks passed (pluginservices 3.651s; model 1.879s), and
  explicit error/cancel/negative/zero counter checks passed (1.576s).
- Stage 5s primary ModelContext/compactor checks passed (0.955s). Provider counter
  observes context and selected reasoning, and its complete request equals what
  generation receives; failed/counting-canceled calls dispatch zero generation.
- Stage 5s Runtime/bootstrap test-source compilation and Runtime build passed;
  scoped dependency/domain/JSON-tag/protocol checks passed (0.942s). Full/native/
  vet/race/full architecture are deferred to consolidated acceptance.
- Stage 5s main merge: generation, source compile/build, shared client typecheck/
  lint/changed formatting and desktop typecheck passed. 80 shared command/recovery/
  stream/journal checks passed (1.52s). Exact Run owner command checks passed
  (1.171s), protocol/version/schema rule gates passed, generated drift passed
  (4.285s). Earlier broad patterns had no Delivery/Protocol/generator matches and
  prove source compilation only, not those tests' execution.
- Stage 5s changed gofmt/diff/original hashes passed; user stash preserved and
  main merged. Temporary compile directories/owned processes settled. No UI/E2E/
  screenshot authoring or execution. Core .28, backend .2, export schema 3.

- Stage 5r: public SDK selection contract passed (0.901s); real installed selection,
  explicit absence and related context/observer checks passed (3.122s initial;
  later selected/auxiliary/context/observer subset 15.161s including linking).
- Stage 5r: all focused Start/new selection/accepted command checks passed (0.556s).
  Existing unsupported current/historical media checks retain their assertions:
  both run under the chosen model before executor validation/staging. Initial
  implementation ordering failures were repaired, not skipped or exempted.
- Stage 5r: fresh selected Run versus Session preference, detached original input,
  failure-before-staging/opening and accepted retry proof passed. Original Resume
  and utility/installed subsets passed (runs 2.493s; model 1.825s), including no
  reselection and no provider construction/fallback after selector failure.
- Stage 5r: Runtime go build ./... and Runtime/bootstrap go test -c passed.
  Scoped dependency/domain purity/JSON-tag/protocol/public-package gates passed
  (2.855s). Full/native/vet/race and complete workflow suites remain final work.
- Stage 5r: changed gofmt/diff/original hashes passed, main up to date and user stash
  retained. No UI/E2E/screenshot authoring or execution; temporary compile outputs
  and owned processes settled. Core/backend/export versions unchanged.

- Stage 5q: auxiliary/completed-text/live-role/installed-context boundary checks
  passed (model 0.975s; pluginservices 2.700s). Final role/text validation passed
  (1.529s), including exact selection and configured-role no-fallback behavior.
- Stage 5q: new context capture/budget and existing Memory maintenance/callback/
  identity/retirement checks passed (pluginservices 4.171s; Memory rpc 2.313s).
  Existing release/cancel/join assertions remain intact; failures dispatch zero
  provider calls, successful repeated calls dispatch exactly two.
- Stage 5q: title fallback and compaction checks passed (segment 0.685s;
  maintenance 2.002s). Memory maintenance fixture package source compiled; the
  chosen focused name pattern had no matching tests there, so this is compilation
  evidence only, not execution of its complete suite.
- Stage 5q: Runtime go build ./... and Runtime/bootstrap go test -c passed.
  Dependency/domain purity/JSON-tag/protocol gates passed (1.156s). Full native,
  vet/race/full architecture and full workflow suites are deferred to final checks.
- Stage 5q: gofmt/diff and original hashes passed; main up to date, user stash
  preserved, temporary compile outputs and owned processes settled. No UI/E2E/
  screenshot execution or additions this round; Stage 5p deviation remains below.

- Stage 5p: focused SDK/installed/execution contracts and related observer,
  calibration, checkpoint checks passed (0.572s/1.070s/2.968s). New waiting
  restoration assertion passed (1.231s) after accounting for the intentional
  ephemeral system prefix while preserving original user/follow-up assertions.
- Stage 5p: Runtime go build ./... and go test -c for Runtime/bootstrap passed.
  Scoped dependency/domain purity/JSON-tag/protocol/public-package gates passed
  (0.723s). Full native TestMain, vet/race and full architecture deferred.
- Stage 5p main merge: shared client typecheck, conflict-file lint/format and 76
  backend/client protocol unit checks passed in two focused runs (48 + 28).
- Stage 5p deviation: aggregate frontend npm run check inadvertently entered UI
  unit tests after typecheck/lint/format passed; interrupted with exit 130. Theme
  schema (1), workspace topic invalidation (1), Memory configuration (3) and
  Memory mutation owner (2) failures were observed in partial output; their
  origin is untriaged and the suite was not completed. No UI/E2E test authored.
  Future frontend verification uses independent static commands only.
- Stage 5p: diff checks and original SHA-256 checks passed; user stash retained;
  test/build compile temporary directories and owned processes settled.

- Stage 5o: application physical usage/window/coverage/price/ordering/overflow
  checks passed (0.773s, final 0.456s). Real SQL main+auxiliary+unattached+unknown
  report, ownership rejection and restart passed (0.654s, final 0.946s). Each
  grouping reconciles to the same known tokens/attempts; logical rows are excluded.
- Stage 5o: Protocol bounds passed (0.532s); canonical usage JSON round-trip passed
  (0.467s), and sample catalog/enum checks passed (0.496s). The catalog initially
  failed at retired unbound schedule.json; the obsolete Core artifact was deleted,
  not rebound or exempted. Scoped architecture gates passed (0.750s).
- Stage 5o: Runtime/CLI builds, Runtime/bootstrap test-source compilation, complete
  generation, shared TS/desktop typecheck, plugin type/lint/prebuilt ESM/resource
  reproduction and changed formatting passed. Initial generated receivers referenced
  retired fields; those receivers were regenerated from the replacement shapes.
  Old Live/backend/delegate fixture field accesses migrated without executing live
  provider or full native TestMain suites.
- Stage 5o: CLI binding checks passed (15.240s); the terminal check initially failed
  on intentionally changed summary text, its fixture was migrated and the exact
  presentation case passed (5.322s). No timeout or assertion was weakened. Existing
  frontend scenarios only receive the new typed wire shape; no UI/E2E/screenshot
  scenario was added or executed. Agent gateway keeps its actual narrow totals
  projection at the adapter boundary, not a second Core API or wire reader.
- Stage 5o: product fce9dd9af committed; main 1d5d98774 merge already up to date;
  immutable hashes/user stash retained; native compile/hook/resource handles
  terminal and temporary outputs removed. Six earlier architecture failures,
  unchanged Memory formatting and full/vet/race/native/all-P01-P48 acceptance
  remain final work, not claimed passed.

- Stage 5n: five known/unknown pricing/reporting restart cases include late Run
  refusal rollback and repeat recovery no-double-accrual (0.815s, then 0.818s after
  strict projection guards). Source usage-corruption and live outer-result/price
  regressions passed (1.188s SQL/1.131s execution); real installed outer-failure/
  reopen pricing passed (1.023s). Missing usage stays unreported; zero price stays
  available zero. The source CAS initially compared DTO encoding with canonical
  JSON; that defect was repaired at the stored original value boundary.
- Stage 5n: authoritative accounting and long root/delegate execution passed
  (7.201s), current application/adapter Recovery groups passed (0.761s/0.367s), and
  existing endpoint/trajectory/export groups passed (0.946s SQL/2.483s persistence/
  0.869s model). Source compile errors, fixture state typo/import syntax and one
  wrong formatting cwd were corrected; no assertion was weakened.
- Stage 5n: complete generation, Runtime/CLI builds, shared client typecheck, plugin
  type/lint/changed formatting/build/resource reproduction and scoped architecture
  dependency/domain/protocol checks (0.672s) passed. Native Runtime/bootstrap test
  sources compiled with temporary outputs removed; full TestMain execution remains
  final acceptance. No UI/E2E/screenshot tests were authored or executed.
- Stage 5n merge: main 1d5d98774 required generated/source/journal conflict resolution.
  Contracts were regenerated. Runtime build and targeted Recovery/execution/delivery
  groups passed (0.846s/0.946s/1.753s); shared TS type/lint passed, 21 Steer/journal
  checks passed (0.620s) and 71 plugin command/stream/method/request/SDK checks passed
  (1.46s). Desktop typecheck, scoped architecture gates (1.015s) and merged Runtime/
  bootstrap test-source compilation passed. Incoming UI tests were preserved only.
- Stage 5n: initial merge commit failed the conventional-message hook; corrected
  message passed with hooks enabled. Product b997cb3d5 and merge da3f344df are
  committed; main is an ancestor, original hashes/user stash remain unchanged and
  owned compilation/hook resources are terminal/removed. Six earlier architecture
  failures and unchanged Memory formatting remain open; full/vet/race/native and
  all-P01-P48 acceptance are explicitly unproven.

- Stage 5m: trajectory/Session endpoint/deletion/export groups passed (0.764s SQL,
  5.076s persistence including capacity refusal). After the storage-boundary repair,
  relevant SQL/export evidence passed again (0.587s/0.950s), with dependency/domain/
  protocol gates (0.589s). Exact wide JSON strings, failed known outcomes, foreign
  Session rejection, deletion rollback and unrelated/global retention are covered.
- Stage 5m: delivery, endpoint adapter and current Interaction source-accounting
  groups passed (1.735s/2.917s/3.638s); title/finalizer scope passed (1.356s).
  Initial enum order assertion failed and the constant ordering was corrected;
  its focused check passed (0.460s). Initial generation missed the discriminator
  enum registration; the registration was repaired and complete generation passed.
- Stage 5m: Runtime/CLI builds and Runtime/bootstrap test-source compilation passed;
  no native TestMain execution was substituted for compilation. Shared TS client
  type/lint and 42 contract tests passed (1.87s), including the schema 3 fixture.
  Plugin type/lint, exact ESM reproduction and trajectory formatting, desktop
  typecheck, gofmt/diff and immutable input hashes passed.
- Stage 5m: full architecture failed (85.161s): Domain JSON tags, operation inventory,
  JSON vocabulary, bounded directory reads, protocol sample version, orphan schemas,
  wire publication inventory and panic stack retention. The first and version
  failures are fixed and their scoped checks pass; six earlier failures remain
  open, with no weakened checks. They are final closure obligations, not evidence
  that the complete architecture gate passed.
- Stage 5m: full plugin check failed at unchanged memory/src/component.tsx formatting;
  later resource and changed-component checks passed. Full/race/vet/native checks
  remain concentrated final work. No UI/E2E/screenshot test was authored or run.
- Stage 5m: product 9c8f667d9 committed, main 6f9006f86 merge already up to date,
  originals/user stash preserved and all owned processes/temporary outputs settled.

- Stage 5l: actual persisted source with missing/changed outer result and existing
  known-failure/usage absence/zero group passed (1.244s). Duplicate sequence/
  overflow and related source regression passed (1.606s execution, 0.570s SQL).
  These prove current transparent single-call accounting, not global/multi-call
  aggregation or crash reconstruction of every projection.
- Stage 5l: Runtime/CLI builds, Runtime/bootstrap source compilation, gofmt/diff
  and original hashes passed. No generated public contract or storage format
  changed. No active failure remains; full/vet/race/native remain final work,
  with no UI/E2E/screenshot checks authored or run.
- Stage 5l: product c895cfb48 committed, main 6f9006f86 remains integrated,
  original inputs/user stash retained, owned handles/temp outputs settled.

- Stage 5k: actual installed observer/provider-once/host reopen and stored request/
  response evidence passed (2.393s), physical stream boundary passed (1.317s;
  final complete/partial/final-frame stop refinement 1.016s), restart unknown and
  legacy journal groups passed (0.515s/0.536s). Evidence is real HTTP and isolated
  SQLite; full native production acceptance remains final.
- Stage 5k: Runtime/CLI builds, Runtime/bootstrap source compilation, generation
  drift (1.830s), dependency rule (0.751s), shared TS types/lint/format, gofmt/diff
  and immutable hashes pass. Initial compile/type and overly broad Session-FK
  changes were corrected without weakening established Run-owner assertions.
  Canonical JSON equality matches the persisted wire contract. No active failure.
- Stage 5k: product 7ede472a3 and main integration 6612b6a5d retain main 6f9006f86.
  Incoming user-authored Desktop tests were preserved; none were authored/run here.
  Full/vet/race/native remain final; temp outputs/owned handles settled.

- Stage 5j: actual frozen auxiliary original-choice/close/join checks passed
  (1.425s), auxiliary/live role/resource group (1.706s), Memory complete-response
  callback group (1.121s), SDK model child ancestry/callback/stream group (0.795s).
  Actual Memory maintenance/input/receipt and distinct extraction/curation identities
  with one released capture pass (1.243s).
- Stage 5j: Runtime/CLI builds, bootstrap/run maintenance/Memory maintenance test
  source compilation, generation drift (2.113s), gofmt/diff and immutable hashes
  pass. Old fixture signature/Completion type failures corrected. A selected run
  maintenance group had no matching tests; compilation is not behavior acceptance.
  Full/vet/race/native remains final work, with no UI/E2E/screenshot checks.
- Stage 5j: product 23f6e12e7 committed, main 993eb865f remains integrated, user
  stash/original inputs preserved, temporary outputs removed and handles terminal.

- Stage 5i: final installed observer real SDK HTTP Call/Stream/stop group passed
  (2.054s), including repeat/transform refusal before provider I/O, inner known
  response/usage, outer failure, withdrawal/restore admission, actual backpressure
  and original producer join. No native first-party TestMain was executed.
- Stage 5i: captured order/identity/held release and affected checkpoint/Interaction
  failure group passed (1.201s). The failed bridge check exposed actual consumption
  ordering and was fixed; required checkpoint fixture migrated. Runtime/CLI builds,
  Runtime/bootstrap source compilation, generated drift (2.293s), gofmt/diff and
  input hashes pass. Full/race/vet/native and independent full production chain
  qualification remain final; no UI/E2E/screenshot tests added or run.
- Stage 5i: product 574a3e183 committed, main 993eb865f remains integrated, user
  stash/original inputs retained, owned handles terminal and temp outputs removed.

- Stage 5h: final explicit order/frozen projection/withdrawal/duplicate/contract drift
  and named setup required/optional tests passed (0.464s), storage reopen passed
  (0.508s), affected requirement/constructor group passed (0.664s), generated drift
  passed (2.673s). Runtime/CLI builds and changed package/services/connections/
  bootstrap test sources compile into removed temporary files. Compilation does
  not establish native execution acceptance.
- Stage 5h: shared TS types, final pure client selection/composition command group
  passed (2 cases, 231ms), changed source lint/format and Go formatting/diff passed.
  Compile/fixture failures and conditional assertion lint were corrected; no active
  failure remains. No UI/E2E/screenshot tests ran; full/vet/race/native deferred.
- Stage 5h: c29e16107 committed; main 993eb865f is already integrated; original
  input hashes and user stash remain unchanged; owned processes/files settled.

- Stage 5g: production unary/stream known-response and existing incomplete-failure
  regression passed (1.556s), SQLite reopen passed (0.277s), reducer failure group
  passed (0.703s), and known-response Tool/no-continuation regression passed (0.558s).
- Stage 5g: Runtime/CLI builds, shared TS types, go generate, final generated drift
  (1.981s), Runtime/bootstrap test-source compilation and gofmt/diff passed. One old
  bootstrap Memory callback signature was corrected after compile failure. Compiling
  native test sources does not establish native execution acceptance. Full/vet/race/
  native checks remain final; no UI/E2E/screenshot checks were added or executed.
- Stage 5g: product committed in 9d1702458, main 993eb865f remains integrated,
  original hashes and user stash unchanged; temporary compilation outputs removed.

- Stage 5f: final Memory RPC group passed (1.447s), production maintenance/input
  check passed (0.957s), public model group passed (0.504s), Runtime/CLI builds,
  formatting/diff and immutable hashes passed. Migrated test signatures and the
  invalid empty-system fixture were corrected; a confirmed blocked test process
  was stopped only after stack evidence. No active failure remains. Full/race/
  native and installed production model/decorator acceptance remain deferred.

- Stage 5e: stream/unary client contract group passed (9 cases, 258ms); affected
  existing transport/stream/mutation group passed (73 cases, 333ms); changed stream/
  client/HTTP group passed (51 cases, 301ms), final stream group passed (4 cases,
  259ms). Evidence covers awaited backpressure with nested RPC, final-only journal
  retirement, original recovery after client replacement, unsafe number refusal,
  no automatic replay and client-close consumer joining.
- Stage 5e: shared client, component SDK and Desktop source typechecks; shared lint,
  changed-source formatting, diff and immutable input hashes passed. The async
  assertion lint failure was corrected before passing lint. No UI/E2E/screenshot
  test was added or executed. Full/native/race and installed production acceptance
  remain final work; typecheck does not establish rendered author parity.

- Stage 5d: typed freeze/exact uint64/lazy single consumption and known refusal/
  invalid chunk/missing final/late failure checks passed; authenticated canonical
  HTTP callback captured-owner/retired/missing-context checks passed (source-file
  unit group 1.170s) without invoking native first-party TestMain setup.
- Stage 5d: external-module workspace compile and generated drift passed (5.244s);
  Runtime/CLI builds, Go API generation, formatting/diff and immutable input hashes
  passed. This is current-source/adapter evidence, not independent published module
  or production installed composition acceptance. Full/vet/race/native remain final.

- Stage 5c: plugin stream/refusal/identity and enum unit checks passed; real HTTP
  Go-client final/refusal/missing-terminal and existing Run-stream/source-defect/
  shutdown checks passed. HTTP blocked-write regression now proves yield waits for
  the frame write. Final focused HTTP group passed (0.947s), installed stream group
  passed (1.774s), generated drift passed (2.013s), and exact Go surface source-file
  unit passed (0.941s) without invoking native first-party TestMain setup.
- Stage 5c: Runtime/CLI builds, shared TS typecheck, go generate, formatting/diff
  and immutable input hashes passed. No active executed failure remains; full/vet/
  race/native and production all-workflow acceptance remain final-phase work.

- Stage 5b: original stream final/refusal recovery, host database reopen, missing
  receipt/no resubmission, consumer stop, original origin/input conflicts and
  withdrawal passed with the affected unary maintenance regression (1.263s).
- Stage 5b: SDK backend duplex/identity/backpressure/stop group passed (0.485s);
  Runtime/CLI builds, formatting/diff checks and immutable input hashes passed.
  Fixture compilation/constructor failures were corrected before the passing run.
  Final full/vet/race/native checks remain deferred; no frontend UI/E2E ran.

### Stage 5a SDK backend duplex stream delivery

- Passed: original command/input/export and consumer-demand backpressure, stop/join,
  partial failure/no retry, captured model-next reentrancy and real stdio executable
  inner response plus outer failure; unary/raw admission group 0.539s, final backend
  group 0.747s, metadata replacement regression 0.508s, stdio probe 0.723s.
- Passed: Runtime/CLI builds, public API generation, shared TS types, native/adapter
  source compilation into removed temporary outputs, gofmt/diff and main integration.
- Original input hashes unchanged; no owned process/output retained. No frontend
  UI/E2E/screenshot additions/execution; full/vet/race/native remain final-phase work.
- Open: Runtime installed stream admission/original route recovery, selected model/
  decorator/dependency retention, production consumers, durable inner observation,
  TS delivery and independent author/module/P01-P48 acceptance.


### Stage 4z real callback model streams and remaining-chain substrate

- Passed: lazy/frozen request, Scope chunk/final equivalence, early stop/source join,
  tentative-final rejection, failed-write source stop, completed inner plus outer
  failure, external frozen order/transformed input and stream generation/identity
  group (plugin 1.117s; generator 1.175s). Byte-budget regression 0.774s; independent
  executable stream 0.678s; installed pre-route refusal 1.048s.
- Passed: affected unary Method admission/schema regressions 0.337s, all capability
  pure contract tests 0.189s; Runtime/CLI builds and current API generation.
- Passed: shared/Desktop/plugin SDK types, Vite, source 90/90 with 108 calls and
  three sidecars/14 topics, layer 1145/3702, exact rebuilt resources, native/terminal/
  adapter source compilation, gofmt/diff, original input hashes and main integration.
- Repaired: stream literal/helper/feature-emitter references and media-type handling.
  SSE's HTTP reader owns parsing/validation. No failure is skipped or weakened.
- Not run: frontend UI/E2E/screenshot by user instruction; full/vet/race/native remain
  final-phase work. Installed duplex stream composition, production consumers,
  dependency retention, durable evidence and TS author parity remain unproven.


### Stage 4y public captured unary model foundation

- Passed: complete response/captured context/usage/reasoning/zero cache and detached
  ownership, invalid-input/retired reference, empty outcome schema and public external
  executable checks (0.680s combined); cancellation joins owner work (0.563s); final
  closed-code/schema/executable/cancellation regression group (0.736s).
- Passed: Runtime/CLI production builds, generated public Go API inventory, gofmt/
  diff checks and local main integration; original input hashes unchanged.
- Not run: frontend UI/E2E/screenshot tests by user instruction and no frontend edits.
  Full/vet/race/native are final-phase work. Existing Memory/Interaction/auxiliary
  production migration, true streaming/next/backpressure/order/inner durability and
  installation/TS/independent author parity are not established by this unary slice.


### Stage 4x generic plugin-data change cutover

- Passed: actual private command/receipt/public recovery, Tool metadata/lifetime and
  independent background notice authority (1.093s), delivery/all-resource projection
  (1.072s), invalidation identifier ownership (1.457s), private Schedule commit-notice
  policy (0.263s), current changefeed/protocol subscription (0.403s/0.436s), pure CLI
  partition/sequence-gap behavior (7.795s), four shared stream ownership cases (145ms
  suite duration). Other filtered shared cases are not run, not full-suite evidence.
- Passed: Runtime/CLI builds, shared/Desktop/plugin typechecks, Vite, source 90/90
  with 108 calls/three sidecars/14 topics, layer 1145/3702, exact ESM reproduction,
  native/terminal source compilation, gofmt/diff, input hashes and main integration.
- Repaired: old private Add fixture, unpublished public callback fixture boundary,
  duplicate topic expressions and missing imports. No failure is suppressed. Native
  source compilation is not runtime acceptance; full/vet/race/native remain final.
- No frontend UI/E2E/screenshot additions/execution. Existing fixtures migrate only
  their retired topic/shape calls. Owned build/temporary outputs are removed.


### Stage 4w publisher-owned Memory pagination bounds

- Passed: publisher bound/generated contract checks (0.550s), actual backend method/
  default-page checks (1.312s), management pagination (0.760s), CLI traversal and
  original recovery (7.256s), capacity drift/no-partial/default-width regression
  (8.525s); Runtime/CLI production builds.
- Passed: shared/Desktop/plugin typechecks, Vite, source 90/90 with 107 calls and
  three sidecars/15 topics, layer 1145/3702, rebuilt exact ESM resource reproduction.
- Passed: affected native/terminal source compilation, gofmt/diff checks, unchanged
  design input hashes, main integration and owned output cleanup. Fixture required
  Capacity and stale regenerated-resource failures are repaired, not suppressed.
- Not run: frontend UI/E2E/screenshot tests by user instruction; full/vet/race/native
  acceptance remains final-phase work. Notifications and all remaining goal work
  remain active; schema const is not a second mutable capacity owner.


### Stage 4v Core Memory read record retirement

- Passed: publisher Unicode/content/identity/generation checks (1.449s), consumer
  target/visible-state/time checks (0.623s), CLI malformed projection/identity and
  original-route restart/malformed-output checks (7.846s); Runtime/CLI builds.
- Passed: shared/Desktop types, Vite, source coverage 90/90 with 107 typed calls,
  three sidecars/15 topics, layer gate, exact Memory/other component reproduction.
- Passed: Bootstrap/terminal/protocol/delivery test source compilation into temporary
  outputs removed automatically. Initial alias-pattern, fixture array comparison,
  variable qualification and retired wire import failures are repaired.
- Passed: gofmt/diff checks, unchanged original input hashes and local main integration.
  No frontend UI/E2E/screenshot cases executed or added; final native/full/vet/race
  execution remains open. Core capacity and business topics remain known live work.


### Stage 4u final Memory Core facade cutover

- Passed: Runtime/CLI/Reviews production builds; Desktop/shared typecheck and Vite;
  source coverage 90/90, 107 typed calls, three sidecars/15 topics; layer 1145/3701.
- Passed: CLI original-provider/raw-input/command/restart/malformed-result checks
  including add (8.421s), Go terminal focused checks (8.181s), protocol/delivery
  (2.280s/3.179s); source compilation for affected native/terminal/adapter packages.
- Passed after final fixes: captured project path regression (6.058s), pure pending
  target/content fence (5.469s), CLI build, gofmt and diff checks. A preceding pure
  pending-add check also passed (5.884s). No frontend UI/E2E/screenshot execution.
- Source fixture migration preserves native original requests and captures a new
  binding only for a genuinely new command after provider replacement. Source
  compilation is not execution evidence; native/full/vet/race remain final-phase work.
- Original input hashes are unchanged; owned dist is removed; user stash preserved.
  Main 993eb865 merge is already up to date. No retired Core command is converted
  to a new public command; independent/external clients must rebuild.


### Stage 4p public Memory catalog read cutover

- Passed: CLI Memory and shared Schedule read/binding checks (7.436s), including
  original provider across pages, incompatible advertised protocol before dispatch,
  complete traversal, duplicates, invalid projections and no partial failure result.
- Passed: existing shared-client pure methods contract checks, 42 tests (458ms);
  unrelated immutable workspace/lifetime checks retain their original guarantees.
- Passed: Runtime/CLI/Reviews production Go builds, Desktop/shared types/Vite,
  source API coverage 94/94, layer closure, owned-source gofmt and diff checks.
- Passed: Bootstrap/delivery/protocol/terminal test-source compilation. Temporary
  output directories and owned Desktop dist are removed; process tests do not run.
- Fixed: unused imports, a retained target DTO/reference and an accidentally migrated
  mutation fixture field; graph checker rejected an E2E source import of a private
  adapter, so the actual reader now lives in its context public capability surface.
  All relevant failing checks are rerun successfully; no active failure remains.
- Passed: Memory publication canonical comparison unchanged; generated Go has no
  private backend import. Original proposal/review/draft hashes unchanged; main merge
  already up to date, product committed and owned outputs cleaned.
- Not run: frontend UI/E2E/screenshot checks (explicit exclusion). Existing retired
  call expressions are migrated without new cases; they are only typechecked.
  Full/vet/race/native process and credentialed live execution remain final phase.
- Breaking: Core .12 removes ListAgentMemory/agentMemory.list/list DTO and workspace
  list wrapper. External consumers must use the installed publisher query. Remaining
  mutation facades are real work; backend contract/private storage are unchanged.

### Stage 4o final Schedule Core facade removal

- Passed: CLI Schedule adapter checks (7.430s), including public operations/pages,
  workspace reset and refusal of an acknowledgement for another Schedule.
- Passed: captured revision/nullable patch/caller-pointer reuse and complete Go
  uint64 output check (6.696s). No current-provider or updated-revision lookup is
  used to construct an unresolved edit; existing intent admission remains shared.
- Passed: remaining Core Session/Plan exact revision/model contracts (0.613s) and
  workspace/problem translation checks (0.851s). Obsolete Schedule cases disappear,
  while unrelated assertions and causes remain unchanged.
- Passed: production Runtime/CLI/Reviews Go build, Desktop/shared TypeScript types,
  Vite, API coverage 95/95, layer closure and 982-key/eight-locale closure; gofmt
  and git diff --check. Bootstrap/delivery/protocol/Runtime/terminal sources compile
  only; all owned temporary binaries/directories and Desktop dist are removed.
- Fixed: retired catalogue/fixture types, a shared helper lost with its former
  Schedule test container, obsolete error copy, a misplaced error block and stale
  field assertions. All relevant failing checks are rerun to terminal success;
  no failing executed check is left active.
- Passed: proposal/review/draft hashes unchanged; main merge up to date; product
  committed. Tracked current sources have no retired update DTO/API/feature/error.
- Not run: frontend UI/E2E/screenshot checks (explicit exclusion). Retired wire/facade
  assertions are removed; remaining UI/terminal fixtures only compile. Full/vet/race,
  native process and credentialed live probes remain final concentrated obligations.
- Breaking: Core .11 removes UpdateSchedule and all Schedule Core DTO/feature/error
  projections; external consumers use plugins.call and publisher refusals. Backend
  business/private receipt/storage contracts are unchanged. No compatibility reader,
  alias, result cache or second state transition is retained.

### Stage 4n CLI public creation and shared publisher Go projection

- Passed: actual CLI operation/pagination check (7.977s) through the public creation
  method and generated publisher output. Capture check (5.908s) proves resolved CWD,
  original provider/contract/draft/model and input bytes survive caller draft/map reuse.
- Passed: Go feature projection checks (0.471s), including exported names and actual
  package-scoped generation. Previous and current canonical capability strings are
  byte-identical; no publication contract, numeric bound or business schema changes.
- Passed: Runtime/CLI/Reviews production Go builds, Desktop types/Vite, source API
  coverage 96/96, layer closure, owned-source gofmt and git diff --check.
- Passed: Bootstrap/delivery/Runtime test-source compilation; terminal compilation
  initially exposed two retired Workspace-field assertions, migrated to CWD and then
  passed. Temporary binaries/directories and owned Desktop dist are removed.
- Passed: original proposal/review/draft SHA-256 values unchanged; main merge already
  up to date. Product committed. No failing executed check remains active.
- Not run: frontend UI/E2E/screenshot checks (explicit exclusion; no such source is
  added or changed). Native process/recovery, full/vet/race and credentialed live
  execution remain for the final concentrated phase. Compilation is not execution.
- Breaking: Core .10 removes CreateSchedule/schedules.create and its request DTO;
  external consumers must use the installed operation. Generated Go projections
  move within CLI and expose public names; no aliases retained. Backend protocol,
  aggregate transitions and private storage/receipt formats are unchanged.

### Stage 4m CLI installed manual execution and Core run-now deletion

- Passed: shared installed-command owner/intent checks (0.652s), including decoder
  failure with a Refusal-shaped error retaining the original command, followed by
  the same-identity successfully decoded owner acknowledgement and retirement.
- Passed: actual CLI operation/pagination plus complete accepted-reference checks
  (6.003s). Output comes from the unchanged publisher contract, including its exact
  SessionID/RunID spelling; missing IDs, forbidden whitespace, unknown fields and
  absent acknowledgements cannot become accepted references.
- Passed: delivery missing-resource and prompt-budget owner checks (0.870s).
- Passed: shared-client pure methods/journal checks, 59 tests in two files (801ms).
  Existing generic unary recovery cases now use canonical Session fork instead of
  the deleted Schedule facade. Shared and Desktop TypeScript compilation pass.
- Passed: Runtime/CLI/Reviews production Go builds, Desktop Vite, API coverage 97/97,
  layer closure, owned-source gofmt and git diff --check. Bootstrap/delivery/Runtime/
  terminal test sources compile only; temporary binaries/directories are removed.
- Fixed: broad JSON fixture enums did not satisfy a newly typed mock promise;
  transport mocks now retain their imported raw sample type. Live helper pointer
  signatures and publisher-case output fixtures are corrected. A prefix-only ID
  assertion was a test error under the documented opaque-ID contract and is replaced
  with a real invalid value; no production rule or valid assertion is weakened.
- Passed: original proposal/review/draft hashes unchanged, main merge up to date,
  product committed and owned dist/temp outputs removed.
- Not run: frontend UI/E2E/screenshot checks (explicit exclusion; no such source was
  added or edited). Full/vet/race/native process and credentialed live probes remain
  the final concentrated phase. Compilation does not establish native recovery.
- Breaking: Core .9 removes RunScheduleNow/schedules.runNow and their transport DTOs;
  external consumers must use the installed operation. Backend protocol/storage and
  accepted Run/private receipt owners are unchanged; no compatibility alias exists.

### Stage 4l CLI installed Schedule deletion and durable original intent

- Passed: focused installed-command checks (0.651s), including durable staging
  before dispatch, acknowledgement loss/reopen, original byte/key/binding recovery,
  wrong-store and expired-promise admission fences, unavailable owner, failed writes,
  failed retirement/same-identity acknowledgement and definite owner refusal.
- Passed: real CLI adapter operation/pagination check (7.398s); terminal test source
  compilation only. Its temporary binary/directory are removed.
- Passed: production Runtime/CLI/Reviews Go builds, Desktop types/Vite, source API
  coverage 98/98 and layer closure; owned-source gofmt and git diff --check.
- Fixed: stale generated DeleteScheduleRequest receiver prevented generation until
  removed; an existing CLI integration test still called the deleted method and is
  migrated. A local JSON formatting regression changed accepted input bytes; binary
  projection preserves them across reopen. No failing check is left active.
- Passed: original proposal/review/draft SHA-256 values remain unchanged; main merge
  already up to date. Owned dist and temporary binaries removed; product committed.
- Not run: frontend UI/E2E/screenshot checks (explicit exclusion). One retired Core
  deletion-dependent HTTP case is removed; terminal fixtures are migrated, not run.
- Not run: full/vet/race/native process/offline acceptance and credentialed live probes
  (final concentrated phase; no credentialed operation authorized).
- Breaking: Core .8 removes DeleteSchedule and schedules.delete with its request and
  generated artifacts. No business/storage or backend protocol break in this slice.
  External consumers must use CallPlugin; no compatibility route is retained.

### Stage 4k CLI installed Schedule reads and Core reader deletion

- Passed: production Runtime/CLI/Reviews builds, Desktop typecheck/Vite and source/layer
  gates. Source coverage 99/99, with three sidecars/15 topics and no operation exemptions.
- Passed: feature generation/type frames/maps/time/optional/full-width integer checks
  (0.540s); named embedded pagination owner and flattened fields regression (0.841s).
  The initial PageQuery TS disappearance is repaired in the walker, without an alias
  or handwritten client shape. Schedules capability JSON is semantically unchanged.
- Passed: actual CLI public read/remaining operations and pagination, frozen provider
  across pages and duplicate-row refusal (5.935s); strict advertised-protocol refusal
  before query and read regressions (6.331s). Caller metadata/query identity stays typed.
- Passed: Bootstrap, Runtime and delivery test-source compilation into temporary
  binaries, removed afterward. Native/live backend probes now use installed reads;
  source compilation does not establish final process/restart/credentialed acceptance.
- Deleted: frontend old Core reader cases only; unrelated cursor/HITL cases remain.
  Initial compile errors from the nonexistent capability.Admit constructor and stale
  standalone PageQuery projection were repaired. An accidental unrelated Memory
  expectation edit was restored before commit; no valid assertion was weakened.
- Not run: full Go/vet/race, native/backend process execution, live credentialed probes
  and final author/deletion audit; all frontend UI/E2E/screenshot execution excluded.

### Stage 4j contextual Session components and closed action deletion

- Passed: Runtime/CLI/Reviews production builds; SDK/shared-client/bundled/Desktop
  TypeScript; targeted lint, exact component ESM reproduction, Vite and layer gate.
  Source coverage is 100/100 after removing plugins.renameSession, without exemptions;
  three sidecars and 15 existing topics remain covered.
- Passed: Session placement scopes/material/selector admission (Domain 0.602s), public
  component metadata without execution and retired action refusal (package 0.510s).
  Shared RPC original Session target/revision/key replay passes (one focused test,
  220ms); other shared-client tests were not run in this focused development check.
- Passed: Bootstrap and CLI command/binding test-source compilation into removed
  temporary binaries. Canonical Session revision/receipt/restart regression source
  now calls sessions.update and verifies component withdrawal independently; final
  native execution remains outstanding, not proven by source compilation.
- Passed: locale gate 983 keys/eight locales after removal of old action strings and
  four unused Stage 4i carrier strings. Initial inherited locale-gate failure is fixed;
  no valid assertion or check was disabled. Type iteration errors from an accidental
  extra point argument and an incorrect boundary import were repaired before commit.
- Deleted: old title-only/closed-action protocol cases and exclusive frontend Host
  form tests. No new frontend UI/E2E/screenshot tests or execution.
- Not run: full Go/vet/race and supported native/component/restart acceptance, kept
  for the final consolidated phase. Cross-generation UI command recovery remains open.

### Stage 4i public Tool results and all guest machinery deletion

- Passed: Runtime/CLI/Reviews production Go builds; SDK/UI/bundled/Desktop TypeScript;
  targeted lint, UI build, exact ESM reproduction, Vite and layer gate. API source gate
  101/101 (88 graphical, six Go headless, six materialized, one HTTP), three sidecars
  and 15 existing topics. Removed readRenderer accounts for the changed operation count.
- Passed: Domain component selectors/immutable projection (0.428s); retired HTML
  refusal, independent sibling admission, byte integrity, escaped/missing resource
  refusal and preserved cancellation (pluginpackage 0.610s). Initial migrated security
  failure exposed missing unavailability classification; production boundary repaired,
  original assertions retained. No active failure remains in these executed checks.
- Passed: unchanged dependency rule (0.792s) and CLI public API inventory (25.400s).
  Bootstrap, installation, execution and native Desktop test sources compile into
  temporary binaries, removed afterward. Source compilation does not prove native
  component execution or supported package qualification. Native linker emits macOS
  deployment-version and duplicate-library warnings under the direct compile command.
- Deleted: retired guest/frame/renderer/theme tests and probes/build entries only.
  Surviving frontend fixtures follow the component contract without new UI tests or
  execution. Generic inspection, source access and current business/receipt owners stay.
- Not run: full Go/vet/race, installed native process/lifecycle acceptance and package
  qualification (final consolidated phase); all frontend UI/E2E/screenshot work excluded.

### Stage 4h public file components and retired carrier deletion

- Passed: production Runtime/CLI/Reviews Go builds, SDK/bundled/Desktop TypeScript,
  targeted lint, exact bundled component ESM reproduction, Desktop Vite and layer
  gate; Core regeneration, formatting and diff check. API source gate remains 102/102.
- Passed: file component scope/selector admission and metadata code-nonexecution
  (pluginpackage 0.632s); detached selector projections and retired HTML refusal
  (Domain 0.456s). Existing selected-resource native test source now uses public
  component resources and compiles with Bootstrap into a removed temporary binary.
- Deleted: retired file-carrier-only frontend cases; no new UI tests or execution.
  The earlier typecheck failure was an obsolete FileSnapshot fixture and is resolved
  by deleting that retired case, not weakening a current contract assertion.
- Preserved: captured read, source fallback, matching ambiguity, shared-module/lifetime
  owners, package integrity and current admitted immutable resources. These source
  checks do not establish rendered Hooks or supported native acceptance.
- Not run: full Go/vet/race and native resource/retirement/restart execution, reserved
  for the final consolidated phase. Frontend UI/E2E/screenshot tests remain excluded.

### Stage 4g backend scheduling and host-driver deletion

- Passed: production Runtime/CLI/Reviews Go builds; Desktop Vite build and TypeScript;
  bundled TypeScript/exact component ESM checks; Core/feature/Reviews regeneration;
  formatting and git diff --check. Generated output was restored through its generator
  after an accidental formatting-only expansion, preserving reproducible artifacts.
- Passed: SDK activation-before-setup refusal, setup without business start, repeated
  activation starting once and retired lifetime refusal (plugin 0.677s); real private
  cron claim/acceptance, original caller, loop cancellation/join and original manual
  recovery/receipt after deleted definition (transport 1.605s). Exact live/generated
  five-method capability and retired scan/dispatch export assertions pass (0.755s).
- Passed: managed notice authority (human/undeclared/withdrawn refused; own enabled
  contribution publishes one outcome-free resync, 1.464s); unchanged Runtime dependency
  rule (0.721s). Source gate 102/102, including actual Go backend notification consumer;
  existing three sidecars/15 topics stay covered. No operation exemption was added.
- Passed: Bootstrap, Runtime, CLI, delivery/adapter and Reviews test-source compilation
  into temporary binaries. Existing native recovery source now follows the automatically
  activated backend; source compilation does not establish full execution acceptance.
- Repaired: migrated port fixtures and removed imports; the new activation fixture
  lacked a required method description. No valid assertion or check was disabled.
- Not run: full native restart/disabled/non-selected installation integration, full Go,
  vet and race (final consolidated phase); UI/E2E/screenshot execution/additions excluded.

### Stage 4f background activation and retained caller prerequisite

- Passed: production Runtime/CLI/Reviews builds; Desktop Vite build, Desktop/bundled
  TypeScript checks and exact ESM resources; source-consumer gate still 101/101 with
  five actual CLI-only operations. Core/feature/Reviews generation completes.
- Passed: actual authenticated HTTP invocation acceptance/refusal and setup resource
  lifetime (plugin 0.243s); original dispatched-route restoration and changed caller/
  Actor/target/refusal (0.426s); Schedules read-only namespace setup and joined resource
  closure (0.640s); private success/refusal receipt atomicity (1.079s).
- Passed: unchanged Runtime TestDependencyRule (0.720s), after fixing prior outward
  SDK/Protocol imports and relocating the unclassified tool. No rule was relaxed.
- Passed: Bootstrap, MCP connection/package adapters, Runtime and CLI test-source
  compilation; Reviews source initially exposed a local capability variable shadowing
  the migrated import, repaired in 51602f5d and then compiled successfully. Temporary
  binaries are removed. Formatting and git diff --check pass.
- Repaired failures: new fake callback tokens did not meet the existing 64-digit hex
  contract; unauthorized callback retains the existing HTTP Gone response; new backend
  ownership variables collided with server-session variables; retired imports after
  moving Refusal were removed. Assertions now reflect current real contracts.
- Not counted: an incorrectly selected empty architecture test pattern; the actual
  TestDependencyRule was subsequently run and passed. Not run: UI/E2E/screenshot
  checks (excluded), full Go/vet/race and native background execution (final phase).
  The host scan loop is still present; no full worker migration claim is made.

### Stage 4e Schedules component and final business Views deletion

- Passed: production Runtime/CLI/Reviews Go build; Desktop Vite production build;
  Desktop and bundled plugin TypeScript checks; plugin/changed Desktop lint; exact
  component ESM check; changed Go formatting and git diff --check.
- Passed: retired View declaration refusal and renderer sibling admission/content
  integrity/filesystem-escape checks (0.757s); retired readView/readSchedules method
  refusal (0.571s); pagination projection (1.021s); actual-call source coverage
  including negative test/declaration/unrelated-receiver cases (0.180s).
- Passed: Bootstrap, Runtime and CLI test-source compilation into temporary binaries;
  source gate 101/101 (89 direct graphical, five CLI-only, six materialized and one
  HTTP-projected), three sidecars, 15 topics; layers 1,160 modules/3,754 edges;
  locale completeness 990 keys in eight locales.
- Repaired failures: renderer fixture used an invalid selector and a retired unknown
  member for a sibling-admission scenario; migrate it to current renderer shape and
  retain a separate strict retired-declaration refusal. A new dispatch assertion
  required the transport.Error projection. Early generation references were removed
  before successful regeneration. No valid assertion or check was disabled.
- Not run: frontend UI/E2E/screenshot execution (excluded); full native process
  acceptance, full Go suites, vet and race (reserved for the final consolidated phase).
  Test-source compilation does not prove native restart/withdrawal behavior.

### Stage 4d complete public Schedules manual operation

- Passed: production Runtime/CLI/Reviews Go build and bundled contract/component types.
- Passed: actual SDK/backend/private-DB manual command regression (1.119s): captured
  caller/input, missing acceptance reply, failed private receipt commit rollback,
  deleted-definition retention, exact reply/replay without fresh collaborators,
  changed-input conflict and retained business refusal. Exactly one private command
  outcome exists per requested command; no inner manual dispatch receipt is created.
- Passed: generated Schedules capability equals the actual typed method owner (0.651s).
- Passed: Bootstrap and pluginservices test binaries compile and are removed by
  TemporaryDirectory. Native scenarios now use runNow and distinguish unknown missing
  receipt from committed acceptance surviving restart/withdrawal/Session deletion.
- Resolved: the first build removed an identity import still needed by cron scanning;
  it is retained. New fixture Delete result and uncertainty error references were
  corrected to the existing owner contracts; source compiles without relaxed assertions.
- Not run: full/native process/vet/race verification, queued for final concentrated
  checks. No frontend UI/E2E/screenshot tests were added or run.
- Passed: gofmt, git diff --check and unchanged original input hashes. Core .15 and
  backend .4 wire envelopes/private SQL formats are unchanged; the full Schedules
  capability identity intentionally changes and old routes remain untouched.

### Stage 4c public Skills component and retired wrappers

- Passed: current production Runtime/CLI/Reviews Go build; SDK, Runtime contract,
  bundled component and Desktop types; Runtime contract and changed source lint.
- Passed: exact prebuilt component rebuilds (Skills 9508 bytes), Vite production
  build, 103/103 API fact families, 14 API-checker regression cases, layer graph
  (1179 modules/3836 edges) and 1014-key/eight-locale consistency.
- Passed: canonical Skill reads require the Skill capability independently of
  plugin presentation, and all four retired operations are method_not_found (0.690s).
  Current exact-release authority/path checks and retired Skills kind refusal pass
  (0.743s). Bootstrap/resource/curation and owning test sources compile.
- Resolved: retired projection/visual fixture references and a mistaken wire scope
  constant initially prevented compilation; all migrated source now compiles.
  The API checker initially omitted dynamically loaded component sources. It now
  scans their real project and credits only typed calls to canonical methods.
- Resolved: runtime-contract composite check stopped at three existing assertion
  lint warnings before any test execution. Preserve equivalent assertions with an
  awaited resource rejection and recorded provider-read arguments; lint now passes.
- Not run: full/vet/race/native process/recovery checks, queued for final concentrated
  verification. Frontend UI/E2E/screenshot execution is excluded by user; obsolete
  carrier cases/fixtures are deleted, with no new graphical tests.
- Passed: gofmt, git diff --check and original input hashes. Core .15 is generated
  from the current source; private Skill storage and backend .4 are unchanged.

### Stage 4b shared invalidation and incoming-main recovery integration

- Passed: production Runtime/CLI/independent Reviews Go build on the final source.
- Passed: public Memory original receipt and missing-receipt notice regression,
  rejected origin/binding conflicts and exact-versus-same-named contract mapping.
- Passed: original Schedule historical receipt equals its normal Tool output after
  edit/delete; input conflicts, corrupt evidence and absence remain explicit.
- Passed: dispatch mutation/receipt atomicity and changed-input conflict ordering;
  private pending admission and immutable completion focused checks. The final
  three owning packages completed in 1.345s, 0.895s and 1.342s respectively.
- Passed: captured original connection authority refusal (0.692s); SDK capability/
  receipt checks (0.743s). Execution and Bootstrap test binaries compile and are
  removed by TemporaryDirectory without running TestMain/native process fixtures.
- Resolved: conflict integration initially used an obsolete typed Bind signature
  and a bare receipt lookup; the actual complete export binding now owns lookup.
  Receipt conflict classification and dispatch lookup ordering were fixed at their
  existing owners; focused checks pass without weakening assertions.
- Not run: full/vet/race/process recovery and graphical checks. Full checks remain
  queued for final concentrated verification; frontend UI/E2E is excluded by user.
- Passed: gofmt, git diff --check and unchanged original proposal/review/draft hashes.

### Stage 4a development and bound Memory component contracts

- Passed: production Runtime/CLI/Reviews Go build; SDK/package/shared Runtime TS and
  Desktop typecheck; changed lint; production Vite build and active commit hooks.
- Passed: public component build and exact reproducibility. Memory is 22766 bytes;
  Diff changes only to require/narrow focus after the public optional-target addition.
- Passed: focused Domain removed-view refusal (0.593s) and package client/Session/
  workspace scope admission (0.577s); no full suite or process claim.
- Passed: Bootstrap, Delivery dispatch and plugin Application test source compiles
  through TemporaryDirectory. Actual installed Memory query regression compares its
  backend DTO items/cursor with the selected private owner's canonical projection,
  and preserves resource/withdrawal/private-data assertions. Final execution queued.
- Passed: API consumers 107/107 (99 direct, seven materialized, one HTTP); layer graph
  1189 modules/3888 edges; locale gate 1035 keys across eight catalogs; diff review.
- Resolved: broad initial function removal accidentally removed adjacent Go plugin
  lifecycle bindings; restore them exactly from HEAD, regenerate and rebuild. Final
  Go API diff removes only ReadPluginMemory. Fixed typed ItemID/string comparison in
  compiled regression source. Retired UI copy left 17 keys; delete them across all locales.
- No new UI/E2E coverage or execution. Retired Memory UI/carrier cases are removed;
  existing RPC/visual fixture shapes migrate for compilation, including deliberately
  invalid protocol requests with explicit test-only type assertions.
- Not run: full/vet/race/installed lifecycle execution (concentrated final phase), UI,
  E2E/browser/native/screenshot checks (explicit exclusion). Unknown command recovery
  across new UI generations and public mutation invalidation remain unaccepted.
- Product bdf26566; local main already integrated. Original SHA-256 baselines unchanged.

### Stage 3z development and client telemetry contracts

- Passed: production Runtime/CLI/Reviews Go build; SDK, portable package, shared Runtime
  TS and Desktop typecheck; changed lint; production Vite build; active commit hooks.
- Passed: public SDK component build/exact rebuild. Diagnostics ESM is 3642 bytes;
  other ordinary component artifacts remain unchanged.
- Passed: two non-UI telemetry data-contract regressions (4.80s total process, including
  transformation/setup) for 301 retained logs including a 128-KiB record, deep-copy
  isolation, current-owner clear and post-retirement read/clear refusal. No DOM rendering.
- Passed: focused Domain refusal of removed clientDiagnostics/sessionTrajectory kinds
  (0.410s). Bootstrap, plugin Application and package admission test source compiles
  through TemporaryDirectory; TestMain and native/process acceptance are not executed.
- Passed: locale gate, 1052 keys across eight catalogs; public boundary gate, 1193 files;
  layer gate, 1193 modules/3909 edges; API consumers, 108/108 fact families.
- Resolved: removed native Diagnostics case left an unused helper; delete that retired
  helper and rerun lint successfully. Admitted material grammar rejects the former
  camel-case key, so declaration/navigation/fixtures use one current diagnostics key.
- No new GUI/E2E assertions or execution. Existing visual/index/navigation fixture
  metadata migrates; retired Diagnostics carrier tests/cases are deleted.
- Not run: full/vet/race/installed execution (concentrated final phase), UI, E2E,
  browser/native and screenshot checks (explicit exclusion). Shared renderer and full
  cross-generation Diagnostics execution remain unverified.
- Product a025360f; local main already integrated. Original input SHA-256 values unchanged.

### Stage 3y development and Session trajectory contracts

- Passed: production Runtime/CLI/Reviews Go build; public SDK, portable package,
  shared Runtime TS and Desktop typecheck; changed oxlint; locale gate (1054 keys,
  eight complete catalogs); production Vite build.
- Passed: actual component build and exact reproducibility check through the public
  SDK build path. Trajectory ESM is 8485 bytes; other ordinary components are unchanged.
- Passed: focused Domain old-view refusal (0.600s), package Session-scope/material
  admission (0.724s), canonical trajectory unknown-evidence/descendant gate (0.866s).
- Passed: Bootstrap and plugin Application test source compilation through TemporaryDirectory;
  no TestMain, native fixture build or process acceptance was executed.
- Passed: API source-consumer gate 108/108 (100 direct, seven materialized, one HTTP);
  retired duplicate query is absent rather than exempted. Old API/carrier search finds
  only the deliberate negative admission case for former sessionTrajectory metadata.
- Existing graphical/RPC fixture contracts migrate to current resource/Core queries;
  obsolete Trajectory carrier tests/cases are removed. No new graphical assertions,
  UI/E2E execution or screenshot checks. Canonical backend regression source remains.
- Resolved: initial Desktop typecheck found remaining retired RPC fixture calls;
  migrate them to the component resource and canonical owner, preserving withdrawal
  and history/Skill retention semantics. Initial lint path containing '..' was rejected;
  rerun from each owning module passes.
- Not run: full/vet/race/installed process acceptance (concentrated final phase), UI,
  E2E/browser/native/screenshot checks (explicit exclusion). Session replacement and
  shared renderer execution are not claimed as verified runtime behavior.
- Product 0643197b; local main already integrated. Original input hashes unchanged.

### Stage 3x development and point/resource contracts

- Passed: production Runtime/CLI/Reviews Go build; shared Runtime TS/Desktop typecheck;
  changed oxlint; locale gate (1054 keys, complete across eight catalogs); production Vite.
- Passed: focused Go point projection/query large-integer identity and former-object
  refusal (2.159s), HTTP endpoint/query catalog (0.742s), five-endpoint generator manifest
  including its exact resource projection (1.326s). These are focused checks, not full suites.
- Passed: non-UI shared-client raw point request serialization (1.81s total process);
  two HTTP source-consumer regressions (1.092s) reject unused metadata, forged identifiers
  and command projections while crediting an actual imported generated resource URL.
- Passed: integrated API consumer gate, 109/109 Desktop fact families (100 directly
  consumed, eight materialized, one consumed HTTP projection); three sidecars, 15 event
  types and four typed Tool-result shapes. This proves source coverage only.
- Resolved: locale scanner misread an inline JSX metadata expression; move the preview
  expression into its existing component local variable. No assertion or locale gate weakened.
- Passed: generated owning contracts, gofmt, diff whitespace review and active commit hooks.
- Not run: full/vet/race/cross-module lifecycle suites (concentrated final phase), UI,
  E2E/browser/native/screenshot checks (explicitly excluded). Complete installed point
  activation/order/retention and graphical execution are not claimed.
- Product fd1a9161; local main b5160b5f already integrated. Original proposal/review/draft
  SHA-256 values match the read-only baseline. No original design input was edited.

### Stage 3w development and provider client contracts

- Passed after main b5160b5f integration: production Runtime/CLI/Reviews build and
  Bootstrap/Runs/segment test source compilation. Two conflicts retain stronger
  existing discovery capacity/cleanup behavior and adopt the actual built package.
  No Bootstrap TestMain, provider real-process or crash test execution is claimed.


- Passed: shared Runtime TS and Desktop type checks; changed source lint; complete
  eight-locale check (1,046 keys); production Vite build. No backend source/schema
  changed before main synchronization, so no repeated Go execution is claimed.
- Passed focused non-UI client contract: provider list is a read; a selection captures
  original role/installation/contribution and retries its same key/input after lost
  acknowledgement and caller mutation (0.379s total). Existing GUI fixture data
  providers are migrated without new assertions or UI execution.
- Corrected verification failures: observer ownership no longer relies on an unused
  memo dependency; mount/retirement owns the controller. Locale checks exposed earlier
  component hard-coded recovery strings and retired carrier keys; translated controls
  and deletion restore the gate without rule changes.
- Failed/open: check-backend-api-consumers now names only plugins.readResource and
  plugins.resolvePoint. The real requirement/provider read/selection callers are
  recognized. Remaining semantic HTTP projection and point inspection are required.
- Not run: full/vet/race/backend integration, concentrated at final phase; graphical
  UI/E2E/screenshot/native execution, excluded. Cross-reload UI outcome recovery and
  observed dependency state/retention are still open.

### Stage 3v development and workspace component contracts

- Passed after main 9ef433b0 integration: production Runtime/CLI/Reviews build,
  Desktop/shared Runtime TS/public SDK/Reviews type checks and production Vite build.
  Shared client resource-close/Run-opening 11 tests pass in 0.600s; no React rendering.
- Passed after checker mapping repair: imported SDK helper is followed without
  crediting unused returned members, and a bound object does not count as invoking
  its methods (2 focused source-analysis tests, 0.331s). Scope remains SDK source.
- Failed/open: full check-backend-api-consumers names readResource, resolvePoint,
  resolveRequirements, listProviders and selectProvider. Genuine materialization and
  consumer work remains; rules/exclusions are unchanged and no placeholder is added.


- Passed: Core generation, production Runtime/CLI/Reviews build; public SDK/shared
  Runtime TS/portable/Desktop type checks; changed source lint, production Vite build,
  portable formatting and exact component/remaining HTML resource checks. Diff ESM
  is 6,480 bytes and shares host React/JSX/UI modules.
- Passed focused backend execution: workspace scope and material key admission,
  including absent key and invalid scope/empty/null/invalid key refusal (1.128s);
  canonical Git capability gate independent of plugin presentation plus retired
  wrapper method-not-found (0.686s). Root/Domain/Delivery/Bootstrap source compiles.
- Source review corrected ambiguous concatenated rendering keys to an encoded tuple
  and kept required context refusal synchronous inside rendering. These are source/
  compilation evidence, not graphical execution proof.
- Not run: full/vet/race/cross-module lifecycle execution, deferred to final phase;
  all UI/E2E/screenshot/native execution, excluded. Existing fixtures migrate only;
  removed tests exercise retired carrier/preference APIs, not the ESM replacement.

### Stage 3u development and canonical usage entrance

- Passed: Core generation, production Runtime/CLI/Reviews build, shared Runtime TS,
  portable and Desktop type checks, changed source lint, production Vite build and
  portable formatting. Usage ESM is 5,590 bytes; its exact rebuild and all remaining
  HTML resource rebuild checks pass.
- Passed focused backend dispatch execution: canonical usage succeeds regardless of
  plugin presentation capability and the retired entrance returns method-not-found
  (0.865s). Root/Domain/package/Delivery/Bootstrap test sources compile only.
- Corrected introduced development failures: importing the full public SDK exposes
  its existing ESNext Promise/Array library requirements; portable tsconfig now
  declares DOM/ESNext. The new regression initially accessed a concrete RPC code on
  an error interface; use the actual transport error type before asserting its code.
- Not run: full/vet/race/cross-module lifecycle execution, deferred to final phase;
  all UI/E2E/screenshot/native execution, excluded. Retired GUI tests are deleted with
  their carrier, and accounting preservation remains covered in backend source.

### Stage 3t development and component-only admission

- Passed after main ffaf30c9 integration: production Runtime/CLI/Reviews build and
  CLI runtimebinding, Run Application, workbench and terminal test source compilation.
  One API inventory conflict is resolved from the actual new reconciliation consumer.

- Passed: production Runtime/CLI/Reviews build; Core generation; shared Runtime TS,
  public SDK, portable component, Reviews and Desktop type checks; changed component/
  builder/client lint; production Vite build and Prettier for changed portable sources.
- Passed: component ESM exact rebuild (399,508 bytes, including the canonical icon
  drawings) and unchanged remaining HTML resource exact rebuild.
- Passed focused backend execution: component-only admission needs no server or
  callable and does not evaluate package code; empty/null/invalid/unknown callable
  declarations are refused (0.626s). Domain/Delivery/Bootstrap test binaries compile.
- Corrected development failures: removal left two unused PackageView imports;
  deleting those imports restores Desktop type/build. A sample-update command used
  the wrong CWD and made no edits; rerun at the worktree root updates both samples.
- Not run: full/vet/race/cross-module execution, deferred to final phase; all UI/E2E/
  screenshot/native execution, excluded. Existing GUI fixtures only migrate DTOs;
  removed assertions target the retired static carrier, not the new ESM path.

### Stage 3s development and necessary resource contract evidence

- Passed after main integration: production Core/CLI/Reviews build and execution/Runs/Bootstrap test source compilation. One preflight/publication conflict was resolved without restoring retired automation or claim owners. Resource client-close contract regression passes (86ms) after replacing duplicate closed state with the RPC owner's cancellation signal.

- Passed: production Runtime/CLI/Reviews builds, Core generation, shared Runtime TS plus public SDK/Reviews Board/Desktop type checks; changed component host lint and production Vite build. The emitted import map has 27 actual public module files, including React, DOM client, JSX, SDK and UI. No rendering occurred.
- Passed focused backend execution: resource lease through the real Endpoint subscription retains original caller, exact owner response, withdrawal refusal and cancellation/no-read (1.253s); HTTP missing capability gets Gone/no-store/nosniff while another method retains Unauthorized (0.873s).
- Passed compilation: plugin Domain/Application, package adapter, HTTP delivery and other owning test sources. The Packages fixture initially lacked the new resource port; it was migrated with an explicit unexpected-call error. Root Runtime/Delivery/CLI Runtime adapter test binaries compile. After main merge: execution, Runs and Bootstrap compile. Bootstrap TestMain is not run. Temporary test binaries are removed.
- Existing frontend test/visual fixture source types still used deleted services/inline declarations; only their DTO/manifest projections were migrated for type compilation. Assertions were not weakened; no UI tests were added or executed. Npm offline lock generation initially changed unrelated optional peers; the retained lock diff contains only the new local public SDK dependency/link.
- Fixed frontend lifecycle implementation before commit: render cancellation comes from a subscribed instance owner with Strict Mode reattachment; I/O joining remains with the contribution owner. Resource close/import failure retires the module lifetime; import/render failures are inside a shared error boundary. No duplicate mutable business owner was introduced.
- Full/race/vet, real installed callback/Board mutation/analyzer and graphical browser/Wails qualification remain deferred. Backend protocol/storage are unchanged; Core .7 requires regenerated clients. Original proposal/review/v0.2 hashes match the read-only baseline.


### Stage 3r development and canonical contract checks

- Passed: Core/Memory/Schedules/Reviews generation, production Runtime/CLI/Reviews build, shared Runtime TS typecheck, public SDK/Reviews Board typecheck and ordinary ESM build. Icons then UI built from their existing scripts; no rendering occurred. Component artifact is 9,893 bytes with shared imports retained.
- Passed necessary focused execution: canonical feature JSON preserves the uint64 bound and public endpoint query/command policy (0.646s/1.628s). After deriving directly from canonical identity bytes, the changed generator assertion also passes (0.889s).
- Passed compilation only: Runtime root/Delivery/contractgen, CLI delivery/Runtime adapter and Reviews backend test binaries. TemporaryDirectory removes every binary. Full/race/vet and installed UI/module/shared-instance tests remain deferred.
- Fixed preparation/compile failures: UI build initially lacked the required Icons artifact; after the documented Icons-first build, UI succeeds. Raw-contract change exposed old CLI adapter and shared TS request-policy object access, both migrated at their actual consumers. No assertion or check was disabled.
- Public author dependencies use existing ignored managed-worktree copies/links. No shared checkout, credential, external publication, private business data, new bundler or frontend UI/E2E/screenshot execution.


### Stage 3q development and focused contract evidence

- Passed: production `go build ./runtime/... ./cli/... ./examples/plugins/reviews/backend/...`; Core Delivery and Reviews `go generate`, including both consumer and producer analyzer projections; shared Runtime TS prettier/types; Go formatting and `git diff --check`.
- Passed necessary focused execution: pure point/applicable/required absence/order and installed value bindings (0.793s/0.810s); package local schema/value isolation and namespace confinement (0.929s); exact resolved analyzer routing plus unrelated evidence refusal (0.970s); deterministic producer snapshot/refusal behavior (1.798s). These are not actual two-process callback acceptance.
- Passed compilation only: root Runtime, plugin Domain/Application, Delivery, architecture, CLI delivery and CLI Runtime adapter test binaries. TemporaryDirectory removes all compile outputs; Bootstrap TestMain is not executed.
- Passed after main integration: production Runtime/CLI/Reviews builds, Core generation, shared Runtime TS types and persistence/segment/Runs/Delivery/Bootstrap/root test source compilation. Focused namespace/read/current material/original request/actor isolation checks pass (1.354s). Bootstrap TestMain was not executed.
- Main merge resolved twenty source/projection conflicts and regenerated projections. Core advances to .5 for required point outputs and merged command acceptance request shapes; backend remains .4. Existing Schedules/current callback fixtures were migrated, preserving their original assertions.
- Fixed: the generator initially encountered a public Descriptor basename collision and duplicate field-constraint registration; the point type is Definition and each constraint is registered once. The typed helper uses string cursor absence. The analyzer projection initially targeted a missing directory; generator ordering now creates it through the existing feature generator. The new analyzer root uses the existing extensions namespace container.
- Full/race/vet, integrated installed activation/callback/reentrancy/retention and independent released module acceptance remain deferred. No frontend UI/E2E/screenshot tests or user-data operations.


### Stage 3p development and necessary focused checks

- Passed: production Runtime/CLI/Reviews builds before and after main integration; changed Go formatting and `git diff --check`. No Core wire/API shape changed, so Core generation and TS checks were not repeated.
- Passed necessary focused execution: `go test ./runtime/plugin -run '^TestSetupProvidesNamedImmutableBindingsAndJoinsResources$' -count=1 -timeout=20s` (6.859s), covering named binding delivery, detached setup snapshots, duplicate setup refusal and success/refusal resource cleanup/cancellation. The first attempt failed because the new fixture omitted the required method description; the fixture was corrected, with no production validation or assertion weakened.
- Passed compilation only: public SDK/Runtime, Domain plugin bindings, operation actor, application plugins, connection/call adapters and infrastructure MCP. After merging main: persistence, execution, segment effects, Runs, Delivery and Bootstrap test binaries compile. Bootstrap TestMain was not executed. All temporary binaries/utilities were removed.
- Coverage retained for frozen binding input/snapshot isolation and selection changes affecting future resolution only. Full/race/vet and actual installed provider wait/setup/callback/retirement/recovery acceptance remain deferred. No frontend UI/E2E/screenshot tests were written or run.
- Main 104f76949cee2517fae2bbb0b0d943a957baa62a merged in ec674681. Five conflicts resolved around accepted continuations/remembered rules plus existing plugin actor/command/provider ownership. One incoming extra EOF blank line was removed mechanically. No shared checkout/index, remote state or private retained data was changed.
- Original proposal/review/v0.2 draft SHA-256 values remain unchanged. The shared progress path still resolves to this managed worktree document.

### Stage 3o development checks and deferred execution

- Passed: production `go build ./runtime/... ./cli/... ./examples/plugins/reviews/backend/...`; changed Go formatting; `git diff --check`. No protocol wire shape changed, so Core regeneration or TS checks were not repeated for this slice.
- Passed compilation only: public SDK, application plugins, package parser/launch adapter, MCP connection adapter and infrastructure MCP test binaries; application owner compiled again after constructor cases were added. Temporary binaries and the one-time callsite migration utility were removed. No tests were executed.
- Regression coverage retained for missing required binding before launch/resource acquisition, exact complete export-set admission, absence of published callable session after failed admission, and session/resource retirement when admission refuses or panics. Existing constructors, launch consumers and fixtures migrated directly without aliases.
- Deferred: focused behavior execution, full/vet/race checks, actual first-party/Reviews installed handshake and management independence. Metadata availability is not actual provider activation waiting or setup/frozen collaborator delivery.
- Commit 591d2004; incoming local main 78f9f08bbb26bba633bd32def0bef7cc8381a7fa already up to date. Only managed worktree code and canonical progress documentation changed; original proposal/review/draft remain read-only.

### Stage 3n development checks and deferred execution

- Passed: `go build ./runtime/... ./cli/... ./examples/plugins/reviews/backend/...`; Core owner `go generate ./internal/delivery`; changed shared TS client formatting and `tsc --noEmit`; `git diff --check`.
- Passed compilation only: application plugins, package parser and installed call adapter test binaries on final source, plus Domain/Delivery/CLI binaries earlier in this slice. Temporary binaries were removed. No tests were executed in this slice.
- Regression cases retained for exact selected binding, explicit optional absence, mandatory cycle path, optional collection cycle separation and another contribution sharing the root entry. The parser/call adapter existing tests compile against the new metadata.
- Deferred to concentrated final verification: test execution/full tests/vet/races and real installed Go/Protocol/CLI/TS, recovery and activation acceptance. No frontend UI/E2E/screenshot tests written or run.
- Local main 78f9f08bbb26bba633bd32def0bef7cc8381a7fa is already merged. Original proposal, external review and v0.2 draft SHA-256 values remain unchanged. Shared progress path still points to this canonical worktree document.

### Stage 3m development checks and deferred execution

- Passed: final production go build ./runtime/... ./cli/... ./examples/plugins/reviews/backend/...; compilation-only test binaries for Schedules Domain/Application/SQLite/transport, pluginservices, Bootstrap and affected Application Runs. Final SQLite/transport/Runs compile again after their regression and constructor updates. TemporaryDirectory removes all binaries; no test body or TestMain executed.
- Passed: existing Schedules and Core generators; regenerated full capability/Schema/TS/reference artifacts include original Invocation. Shared Runtime TypeScript client tsc --noEmit passes. Changed Go formatting and git diff --check pass.
- Passed: original proposal/review/draft SHA-256 equality; incoming main remains 78f9f08b and merge reports already up to date.
- Corrected while migrating fixtures: an automated constructor rewrite also touched one variadic database Scan; it was restored before final compilation. Added original caller evidence to the shared occurrence helper and two Application Runs fixtures. No assertion was weakened, no test execution or unrelated failure was hidden.
- Deferred: execute persistent original caller/restart/immutable guard/old-schema refusal cases, actual human/model/automatic Schedules preparation and callback dispatch, response-loss/Core acceptance/reconciliation, mismatched caller refusal and all final full/race/vet/dependency/proposal acceptance. No frontend UI/E2E/screenshot tests were added or run.

### Stage 3l development checks and deferred execution

- Passed: final production go build ./runtime/... ./cli/... ./examples/plugins/reviews/backend/...; compilation-only go test -c for SDK plugin, Application plugins, host SQLite, pluginservices, MCP connections, Interaction execution, Delivery, Bootstrap, package admission and independent Reviews backend. TemporaryDirectory removed binaries; no TestMain or test body executed.
- Passed: existing Core/Memory/Schedules generation and Reviews generation; all .4 generated wire/capability artifacts and package declarations are migrated. Shared Runtime TypeScript client tsc --noEmit passes with required Target. Changed Go formatting and git diff --check pass.
- Passed: original proposal/review/draft SHA-256 equality; source search finds no invocationId/InvocationKey reader or current .3 Runtime/plugin package declaration. Incoming main remains 78f9f08b and merge reports already up to date.
- Corrected introduced compilation/generation failures: a foreign Target validator could not be generated into protocol; constraints were moved to the containing CallPluginRequest's target fields and its obsolete generated receiver removed before owner regeneration. Removed an initially unused test import, corrected an independent Reviews package path and renamed a local export binding that shadowed the capability package. Final checks pass; no assertion was weakened.
- Deferred to concentrated final execution: SDK/backend and current model metadata delivery, callback original-context/refusal/retirement, host invocation restoration and changed origin/target/parent conflict, strict old schema/non-mutating refusal, package construction, actual public Go/Protocol/CLI/TS calls and full/race/vet/dependency/all-proposal checks. No frontend UI/E2E/screenshot tests were written or run.

### Stage 3k development checks and deferred execution

- Passed: production go build ./runtime/... ./cli/... on the final source; compilation-only go test -c for Runtime Delivery, pluginservices, the module-root Runtime, CLI command delivery and runtimebinding. TemporaryDirectory removed the binaries; no TestMain or test body executed.
- Passed: existing Core generator go generate ./internal/delivery; generated API/Go API inventory/Schema/OpenRPC/TS/node validators include the public envelope and exclusive outcome. Core Protocol remains 2026-10-13.3; backend protocol remains .3.
- Passed: shared Runtime TypeScript client tsc --noEmit, including its generated data/methods and compiled SDK fixtures; Prettier on the four changed client files, changed Go formatting and git diff --check.
- Passed: original proposal/review/draft SHA-256 equality; source-only development introduced no native distribution, runtime test process, shared configuration change, data conversion/deletion or external action.
- Corrected introduced compile failures: the first new endpoint fixture used Capabilities instead of ServerCapabilities; the constraint registration lacked its delivery import. One gofmt command initially used root-relative paths from the Runtime directory; it was rerun from the worktree root. All final compilation checks pass; no failure or assertion was suppressed.
- Deferred: execute the new shared-endpoint and actual cross-entrance Memory receipt/conflict cases, public Go/HTTP/CLI/TS boundary/refusal/restart cases, full/race/vet/dependency checks and final all-proposal acceptance. No frontend UI/E2E/screenshot test was added or run.

### Stage 3j development checks and deferred execution

- Passed: Runtime and CLI go build via go build ./runtime/... ./cli/.... No full module test/vet/race loop was run this round.
- Passed: compilation-only go test -c into owner-cleaned temporary directories for Domain plugin, Application plugins, pluginservices, host SQLite, Bootstrap and both Memory/Schedules transport test packages. After fixture migration, pluginpackage, Bootstrap, pluginservices and Schedules transport compile again. No TestMain or test body was executed by these checks.
- Passed: existing Core generator via go generate ./internal/delivery; generated API/Schema/TS/constraints reflect Contribution projections and Protocol .3. The obsolete generated PluginService validator was removed only to bootstrap the generator; the owner then regenerated it without an alias.
- Passed: strict standalone generated wire TS data compilation; node --check for both changed builders; Prettier formatting for those builders; changed Go formatting and git diff --check. An initial TS command named a nonexistent artifact; it was corrected to wire.generated.ts/wire.methods.generated.ts/wire.validate.generated.ts and passes.
- Passed: local metadata path/protocol coherence for all ten bundled declarations and independent Reviews, and original proposal/review/draft SHA-256 equality. Source search finds no ServiceKind/ServiceOwner/ServiceBinding/PluginService/AdmitService or contributes.services path.
- Fixed during compilation: old export-name validation reference, obsolete generated service receiver, two unused former feature enum imports and an unused test-support import. Fixture API rewrites were corrected before their successful compilation. No failure was labeled pre-existing and no assertion/timeout was weakened.
- Deferred to concentrated final verification: real named feature invocation/callback/response loss, original export/contract/contribution restoration, current declaration failure/portable independence, provider selection/withdrawal, package construction and source-author parity, full/race/vet/dependency checks and final public installed-operation/component acceptance. Adapted strict-namespace assertions express the intentional contract change; they have not yet executed.
- Not run or added: frontend UI/E2E/screenshot tests. No published release, credentialed operation, external mutation or user-data conversion/deletion occurred.

### Stage 3i verification and deferred final checks

- Passed before the final admission-precondition addition: targeted Memory application/domain/maintenance/RPC/private-store/transport tests, actual pluginservices, managedmcp, host SQLite and Schedules SQLite. Coverage includes matching host/private business-only input, real model callbacks, configured Search resolver failure versus normal role absence, exact old-schema refusal, original input fingerprints and private publication/receipt rollback.
- Passed before that final addition: real native Memory maintenance committed-response loss, Grant revocation/backend reconnection, Grant-free receipt recovery/completed redelivery and changed-input conflict; existing missing-owner-receipt/no-resubmit and human command response-loss regressions.
- Passed before that final addition: serial Runtime and CLI go test ./..., go vet ./..., go build ./.... These passes do not claim verification of the later local admission guard.
- Passed on final source: Runtime go build ./..., changed Go formatting and git diff --check. The no-test compilation attempt compiled managedmcp, Memory transport and pluginservices; its Bootstrap TestMain package build exposed the omitted Schedules AdmitCommand caller. The caller was migrated and the subsequent complete Runtime build passes. Do not classify this introduced compile error as pre-existing.
- Passed: complete Runtime go generate ./..., unchanged feature regeneration hashes after the intentional Memory schema change, Runtime go mod tidy with unchanged module files, standalone strict generated TS data checks and original read-only input SHA-256 equality. Core backend protocol .3 and Core operation catalog do not change; the exact Memory method input fingerprints change naturally.
- Deferred by the latest user instruction: execute the final local admission guard regression, full/race checks on the final source and final cross-module/installed-operation acceptance. Continue development without rerunning the earlier full suites in this round.
- Initial introduced test compilation error (an extra nil after chat.NewResponse's two-value return) was corrected; focused checks then pass. No assertion or timeout was weakened.
- Read-only review confirmed business input identity, metadata/session lifetime, Grant-free recovery, old-format refusal and caller/generated artifact consistency. Its fresh missing-metadata observation prompted the final local precondition guard; no new recovery reader or model I/O under the transaction was introduced.
- Not run or added: frontend UI/E2E/screenshot tests. No user-data migration/deletion, published release, credentialed or external operation occurred.

### Stage 3h verification

- Passed: final serial Runtime go test ./..., go vet ./..., go build ./...; CLI full test/vet/build. Runtime full suite includes dependency, public binding and generated Core API drift checks; no Core protocol or dependency graph changed.
- Passed: races for both feature RPC and transport packages, managedmcp and actual pluginservices. Existing private raw-input/restart, Memory human-edit/curation cancellation and Schedules acceptance/receipt behavior survive. New tests refuse nil/query private receipt registration before publication.
- Passed: bootstrap races selected by TestMemory/TestSchedule/TestMissingOwnerReceipt/TestObservedCommand, covering real native backend calls, accepted Run handoff, committed-response loss/restart and no retarget/no resend boundaries.
- Passed: actual Memory/Schedules generated/source and generated/live complete method identity matching; typed List invocation through real backends. Named capability export matching is intentionally not claimed.
- Passed: complete Runtime generation and final per-feature regeneration hash stability for all canonical capability/Schema/method/TS/prose artifacts. Standalone tsc --ignoreConfig --noEmit --strict --skipLibCheck --target ES2022 --module esnext --moduleResolution bundler over both generated data-map files; no UI execution.
- Passed: native build.mjs package construction into an owned new temporary directory, generated artifact byte equality in both compiled packages and the existing 128 MiB aggregate gate. Temporary directory removed by its owner. Prettier --check for the changed builder passes.
- Passed: Runtime go mod tidy with unchanged go.mod/go.sum; changed Go formatting, git diff --check and original proposal/review checksum equality.
- Initial full Runtime test failed TestMemoryEditSurvivesBackendAndRuntimeRestart while MCP remained connecting past its ten-second readiness bound. Isolated -count=3 passes and final serial full test/vet/build pass without code/assertion/timeout changes. Earlier checks overlapped full Runtime, focused races, CLI and native builds; load contention is a possibility, not a proven cause or a claimed pre-existing defect. Preserve the initial failure evidence; do not manufacture a pass by weakening readiness assertions.
- Read-only review found no new typed-source/receipt migration blocker. It traced existing Memory MaintenanceInput Grant into host plugin_commands.input_json and private commands.input_json; that defect is recorded above and remains open.
- Not run or added: frontend UI/E2E/screenshot tests by user instruction. No frontend application source changed. No published release, credentialed operation, production state or user data was changed.

### Stage 3g verification

- Passed baseline: go test ./plugin ./cmd/contractgen before the extraction.
- Passed: Runtime and CLI go test ./..., go vet ./..., go build ./... with the repository workspace. Runtime's full suite includes dependency direction, external public binding and generated-contract drift gates.
- Passed: go test -race ./capability ./plugin ./cmd/contractgen ./internal/adapter/integration/pluginservices ./internal/plugins/memory/rpc. Pure contract concurrent schema/projection admission, live SDK/receipt/callback paths and actual feature adapters are covered.
- Passed: focused bootstrap races for Review Backend committed response loss, Schedule model/manual/lost create/prepare/dispatch/acceptance recovery and Memory maintenance response loss; additional exact focused races cover Review Capability original export/no resend, Memory selected owner and committed reply loss, MissingOwnerReceipt and ObservedCommand no retarget.
- Passed: actual independent Reviews go test ./... and an offline public-only SDK snapshot with independent Reviews GOWORK=off tidy, go test -race ./..., vet and build. No local replace or Runtime internal source/import exists in that fixture. Temporary proxy/cache/source directories are released by their owner.
- Passed: Runtime and Reviews go generate ./...; canonical Reviews capability/Schema/methods/TS/prose SHA-256 maps agree before/after. Only generated public Go API changes, to add capability and remove old SDK definitions.
- Passed: final focused public external module admission, shared dependency rules/purity and generated-contract drift after fixture formatting; go list dependency closure for capability excludes Runtime internal/MCP SDK/Scope MCP transport.
- Passed: Runtime go mod tidy; no owning module go.mod/go.sum changes; gofmt -l over all changed Go files and git diff --check; original proposal/review/old draft checksum equality.
- Stopped/failed rather than passed: direct tidy in the real Reviews module targets its old released Runtime requirement. Go reported finding plugin/capability/jsoninput packages and querying a newer released graph. The exact owned tidy process was terminated; module files remain unchanged. The offline current-source author fixture above passes. Publishing the final SDK and advancing real released dependencies remain open; no replace or fabricated published version was committed.
- Initial introduced build issues (selector/import migration, wrong formatting cwd and package shadowing) and the stale generated API were corrected; final relevant checks pass. No assertion or dependency gate was weakened. Read-only review identified the bootstrap shadowing and stale README names, both fixed before completion.
- Not run or added: frontend UI/E2E/screenshot checks, by explicit user instruction. No frontend source or tests changed. No publishing, credentialed operation, production or user-data mutation was performed.

Stage 3f verification:

- Passed: complete Runtime and CLI go test ./..., go vet ./... and go build ./.... Runtime architecture/dependency/public-binding gates are included. No protocol catalog, public SDK type or dependency changed, so generation/tidy was not required.
- Passed: full changed MCP connection, feature preparation and toolset race suites; final MCP race rerun includes the concurrent source-read/cancellation regression discovered by read-only review. Preparation barriers use channels, not timing sleeps.
- Passed: bootstrap races for the actual managed Memory Tool with captured workspace and no embedding role; Schedules model Tool owner-write/invalidation; lost Core opening/private acknowledgement reconciliation; Memory maintenance committed-response loss without another model call. Real HTTP feature tests prove bound Memory callback reentrancy, original metadata and expiration after failed reply, and Schedules invalidation after uncertainty.
- Resolved test-fixture errors: the first gate test injected the fake registry's launch refusal instead of its dispatch read failure; corrected the fixture to fail Dispatchable and retained the no-dispatch/release assertions. The new managed Memory fixture initially named a nonexistent helper and tried reviewing an already-active human fact; migrated it to the existing installation helper and asserted its actual active-state contract. Final affected tests pass without weakening behavior assertions.
- Resolved source review finding: final source-read cancellation initially dropped the concurrent read cause. The owner now joins both and a deterministic public Tool call verifies source error, standard cancellation and custom cause together.
- Passed: git diff --check and changed Go formatting; original proposal/review SHA-256 values remain identical. All verification handles and test-owned resources are settled. Main synchronization reports already up to date at 78f9f08b; the milestone record is the sole remaining change before final clean-state verification.
- Not run: frontend UI/E2E/screenshot tests, per explicit restriction. No frontend or public SDK/client artifact changes were made. Complete component/Wails and independent public author parity remain open requirements.

Stage 3e verification:

- Passed: Runtime and CLI complete module go test ./..., go vet ./... and go build ./.... Runtime architecture/dependency gates and public Go API discovery are included; generated go-api.json reflects the new public contracts.
- Passed: full SDK/plugin and feature-generator race suites, actual Reviews complete race suite plus vet/build. Failure coverage includes incomplete or changed required contracts, missing/duplicate/late exports, forged fingerprint, nonmember invocation, bare-call/lookup bypass, deep-copy invariants, invalid publication and original receipt recovery without mutation.
- Passed: real bootstrap races for existing Runtime/Tool committed-response loss and new direct capability-command committed-response loss/restart. The guard probe records any mutation on the recovery process; no mutation is resent. Broader managed Memory/Schedules focused bootstrap race checks also pass.
- Passed: canonical Runtime and Reviews regeneration hash stability; standalone generated Reviews TypeScript contract check. Initial TypeScript invocation encountered TS5112 because TypeScript 7 requires --ignoreConfig with explicit input files; the corrected command passes without source or assertion changes.
- Passed: independent Reviews author tidy/full race/vet/build with GOWORK=off, no replace, offline file proxy, isolated private module cache and a source archive containing only public plugin/jsoninput plus module metadata. The first fixture used an unescaped mixed-case proxy path; corrected Go proxy path escaping and reran successfully. This is a source-snapshot proof, not published SDK/Core Client distribution parity.
- Passed: read-only implementation review found no concrete blocker; actual test evidence is recorded above. Changed Go formatting and git diff --check pass. Input SHA-256 values remain identical to the recorded baseline.
- Not run: frontend UI/E2E/screenshot tests, per explicit user restriction. This slice changes backend SDK/generator and Reviews data bindings only; full UI/component and Wails acceptance remains open.

Stage 3d verification:

- Passed after incoming main integration: complete Runtime and CLI go test ./..., go vet ./... and go build ./.... Complete runs/segment/approval/transcript/SQLite race suites passed, plus real Core loss/private Dispatch loss and approval/accepted-answer bootstrap races (34.157s). Incoming main's full accepted approval Resolution remains owned by its transcript Item. No merge conflict or unrelated shared-checkout edit occurred.
- Passed: complete Runtime go test ./... (119 packages), go vet ./... and go build ./...; complete CLI test/vet/build. Desktop consumer compilation passed after the existing icons/UI/frontend build prerequisites, with no UI/E2E tests. Direct Desktop build initially lacked embedded dist files; the normal build workflow supplied them. Native linker target-version warnings were emitted; this is consumer compilation, not a validated shipping Wails package.
- Passed: focused real-bootstrap -race covering Core opening response loss before private acknowledgement; retained pending; restart; deleted accepted Session; exact original reconciliation; private dispatch request/reply loss; manual replay and startup recovery; actual Memory maintenance reply loss without another model call. The old remote-Accept request-loss test was migrated to the actual Dispatch boundary: loss before submission proves no Run, loss after acceptance still proves Run survival and retired-material reconciliation.
- Passed: public target-union constraints and original-request/actor-scoped acceptance lookup after retirement; metadata changes preserve admitted business input and invalid refs are refused before execution; actor withdrawal differs from unknown storage read failures; old Schedules schema refusal preserves pending evidence.
- Passed: Runtime generation and architecture/dependency/contract drift checks; shared TypeScript client check (22 test files, 284 tests, typecheck/lint/format/knip), with no frontend UI/E2E/screenshot tests. Independent Reviews workspace race/vet/build passed.
- Passed: isolated offline GOWORK=off/no-replace public plugin/jsoninput source snapshot and independent Reviews author tidy/full-race/vet/build, without Runtime internal source. Temporary source archive, file proxy, author checkout and module cache were removed. This does not prove published module availability or standalone Core Client distribution.
- Resolved during verification: optional sessionId must not carry unconditional NonEmpty; the target union owns conditional requiredness and captured IDs remain required. A real Core-loss race exposed finish closing events while model observation still sends. The first gate incorrectly canceled owner release and broke retained terminal inspection; the full suite detected it. The final gate uses the existing done signal, preserves release semantics, covers child requests and adds deterministic blocked/late sender plus retained-inspection regression. Two superseded failing race processes were explicitly terminated after this correction; fresh complete changed-owner races/full module checks run against the final tree. The full execution race then found a test sharing its pre-restoration two-second deadline with a new continuation observation. The unchanged scenario passed three isolated race repetitions; the final test gives each observation its own two-second context and preserves every behavior assertion. The final complete execution race passed (161.376s); the final real-bootstrap loss/recovery race passed (28.826s). All other changed-owner package races pass.
- Not run: frontend UI/E2E/screenshot tests, explicitly excluded. No external publication, credentials, production operation or user-data deletion/conversion. Original proposal/review inputs remain read-only.

Stage 3c verification:

- Passed: complete Runtime and CLI go test ./..., go vet ./... and go build ./...; Runtime go generate ./... refreshed the public Go API. External-module compile and architecture/dependency checks pass. Final formatting and diff checks pass.
- Passed: full SDK, HTTP/SSE, Memory RPC/transport, generic MCP and connection-adapter race suites; focused final Core callback/publication/stream races. A real stdio executable calls the canonical query, receives a typed core refusal and protocol-version fence, opens SSE, closes observation and proves source settlement before reference retirement. Stale command and smaller call request capacity preserve known pre-dispatch refusal.
- Passed: an offline GOWORK=off/no-replace temporary source snapshot containing only plugin/jsoninput and module metadata ran its full race suite, vet and build without Runtime internal source. The fixture was removed automatically. This proves those SDK packages, not a published release or a standalone Core Client distribution.
- Resolved: the first concurrent full check reached the existing first-party backend connection wait; isolated race and subsequent full Runtime runs pass without deadline/assertion changes. Generation revealed the existing Go API test's old three-import expectation; updated it to the explicit four public imports and added the exact ConnectCallback signature assertion. Final full checks pass.
- Not run: frontend UI/E2E/screenshot tests, explicitly excluded. No graphical edits, external publication, credentials, production actions or user-data deletion/conversion. Original external proposal/review hashes remain unchanged. Actual Schedules opening/recovery and remote decoration are not claimed verified by the transport fixture.

Memory model-identity correction verification:

- Passed: complete Runtime and CLI go test ./..., go vet ./... and go build ./...; Memory RPC/transport/maintenance full -race suites; real bootstrap maintenance committed-response-loss and provider-selection -race regressions. New identity tests cover observed/lost provider results, same-input redelivery, changed-input conflict and the existing two-call budget.
- Passed: gofmt and git diff --check; incoming git merge main is already up to date at 61645c2561b174042aa157aef21cc00cfba67a86. No public SDK API/catalog changed, so generation and the independent SDK archive are not repeated for this private-owner correction.
- Not run: frontend UI/E2E/screenshot tests, explicitly excluded. No temporary resources or original design-input changes were introduced.

Stage 3b verification:

- Passed: complete Runtime and CLI go test ./..., go vet ./... and go build ./..., including architecture, dependency direction and consumer gates. Final Runtime generation/vet/build also passed after callback cancellation changes; go-api.json includes the public callback API.
- Passed: full SDK, Memory RPC/transport, generic MCP and MCP connection-adapter -race suites. They cover five generic callbacks without a Memory budget, exact contract mismatch refusal, foreign activation/call refusal, original integer/JSON and owner-context preservation, typed business refusal versus uncertainty, request/result capacities, cancellation causes/credential privacy, releasing-call retention, sibling cancellation and incomplete authorized request interruption.
- Passed: real SDK-only stdio executable performs an outer MCP request, host callback and nested same-backend MCP request without deadlock or a Desktop relay. Real bootstrap -race covers Memory maintenance committed-response loss/restart with exactly two model calls, explicit provider choice/availability and existing Schedule manual acceptance recovery.
- Passed: complete independent Reviews author tidy/test-race/vet/build with GOWORK=off, no replace, isolated cache and offline temporary file proxy containing only public plugin/jsoninput SDK source and module metadata. That same public-only SDK archive passed its full -race suite including the real callback executable. All fixture directories/proxy/archive/cache were removed; this is not a published version.
- Resolved: a new retirement test initially reassigned/closed its finish channel while handlers read it; it now owns one immutable channel and sync.Once close, and the full race suites pass. The first isolated-author attempt targeted the parent directory rather than backend/go.mod; it exited before checks, removed its temporary files and was rerun against the actual module with the whole example’s generated artifacts. No assertions or required checks were weakened.
- Not run: frontend UI/E2E/screenshot/composite frontend checks, explicitly excluded; no frontend edits. No external publication, credentialed services, production operations or user-data conversion/deletion occurred. Original external proposal/review hashes remain unchanged.

Stage 3a verification:

- Passed: complete Runtime and CLI go test ./..., go vet ./... and go build ./..., including architecture, dependency-direction and real source-consumer gates. go generate ./... updated Go/Schema/OpenRPC/TypeScript catalogs and shared request constraints. No frontend UI/E2E/screenshot tests were written or run.
- Passed: Application plugin admission and SQLite full -race suites; provider choice persists across real DB close/reopen, does not require its installation row to exist and never resolves a different role implicitly. The former host without provider selections is refused without mutation.
- Passed: real bootstrap -race for two enabled Memory candidates, absent explicit choice, wrong-role/contribution refusal without a write, provider replacement after committed-response loss, retained original command dependency/receipt after Runtime restart, isolated private facts, choice persistence and disabled-selected-provider refusal despite another enabled candidate. Existing Memory re-enable and Schedule accepted manual command recovery races pass.
- Passed: CLI provider-selection command/identity capture and refusal of a mismatched returned role/installation/contribution; focused adapter/command races pass.
- Resolved checks: initial complete runs hit managed-backend connection deadlines; focused checks and final complete runs passed without changing those bounds. Old backend tests required exclusive enablement or assumed enabling selected a provider; they now establish explicit choices while preserving identity conflict, private-data and backend-page assertions. The response-loss probe deliberately terminates its transport; the new cross-provider recovery case restores availability by reopening Runtime and retains its witnessed-commit, no-resend and original-owner assertions.
- Not run: frontend UI/E2E/screenshot/composite checks, explicitly excluded. Reviews and public-only archive checks were not repeated because their SDK/backend source did not change. No external publication, credentials, production actions or user-data conversion occurred. Generic point declarations, independent feature contracts, scoped/frozen composition, captured target/origin/parent and public components remain open.

Stage 2h verification:

- Passed: full Runtime and CLI go test ./..., go vet ./... and go build ./...; architecture/dependency checks and public consumer compile gate passed. go generate ./... refreshed the public Go API and preserved the feature artifact contracts.
- Passed: SDK, Application command admission and SQLite complete targeted -race suites. Changed method/kind/target/input/output contracts cannot enter business execution; changed prose does not alter the callable contract. Binding schema clones, missing identity, external schema refusal, malformed original receipt and preserved typed refusal are covered.
- Passed: real stdio bootstrap -race for Memory maintenance/publication loss, Memory committed loss, Schedule message loss/settlement, absent receipt and observed provider replacement. The Memory test explicitly verifies the complete original contract before loss, after host database reopening and after receipt observation. Final original-contract canonical validation was additionally checked with targeted bootstrap races and complete Runtime vet/build.
- Passed: Reviews full go test -race ./..., go vet ./... and go build ./.... An offline, isolated public-only SDK archive plus the complete Reviews author package passed GOWORK=off/no replace tidy/test-race/vet/build with no Runtime internal source. The archive is a temporary source snapshot, not a published release.
- Corrected verification gaps: two Reviews tests still used the removed per-method client registration and failed compilation; migrated them to the one transport registration without changing business assertions. Existing malformed-input tests now carry a valid contract fingerprint so they still reach the intended input/identity validation instead of passing because a new envelope field is absent. The first isolated-author attempt omitted sibling generated artifacts; copying the complete author package fixed the fixture without skipping its contract test.
- Not run: frontend UI/E2E/screenshot/composite frontend checks, explicitly excluded; no frontend changes. No publication, credentialed service, production operation or user-data conversion was performed. Generic installed declarations/selection/entrances, captured target/origin/parent, callbacks and components remain open.

Stage 2g verification:

- Passed: complete Runtime and CLI go test ./..., go vet ./... and go build ./..., including architecture/dependency gates. No public SDK/wire catalog changed, so contract generation was unnecessary.
- Passed: managed receipts/prepared admission, Memory transport and Memory SQLite complete targeted -race suites. They cover immutable admission/reply across restart, changed binding rejection, pending reentry without handlers, atomic rollback, concurrent edits, canceled model work and retained ledger facts.
- Passed: real stdio maintenance committed-response loss with backend restart and original receipt replay, using exactly two model calls. Additional bootstrap -race covers existing Memory reply loss, Schedule create/prepare/settlement message loss, absent receipt and observed provider replacement.
- Passed: completed-only, digest-only and bare-result private schema refusal, with retained evidence assertions. The actual nullable-reply shape triggers the strict cutover; no per-record epoch or compatibility reader was introduced.
- Resolved fixture migrations: canceled work cannot reuse its original ID with a fresh model grant/body; the retained-ledger recovery has a new activity ID, and the completed duplicate must return its original Published value rather than reevaluate current state. The stdio fixture resolves its temporary CWD before maintenance so later application-normalized reads address the same target. Original watermark, human authority, callback count and no-repeat assertions remain explicit.
- Not run: frontend UI/E2E/screenshot/composite frontend checks, explicitly excluded; no frontend changes. Reviews independent author checks and generation were not repeated because its source and public SDK did not change. Full generic installed operation/declaration/selection/origin/target/parent/callback/component completion remains open.

Stage 2f verification:

- Passed: Runtime and CLI complete go test ./..., go vet ./... and go build ./..., including architecture/dependency and generated-contract gates. The final SDK forwarding-method deletion was additionally regenerated and checked with SDK -race, full Runtime vet/build.
- Passed: SDK, managed receipt/model link and Memory transport full -race; Backend duplicate/frozen publication, exact current protocol, unknown export, descriptor independence, semantic kind enforcement and absence of former named RPC are covered.
- Passed: real installed Reviews, Memory lost reply, Schedule create/prepare/settlement message loss, absent receipt and observed provider replacement with targeted bootstrap -race. Schedules accepted Run recovery and Memory concurrent edit/CAS tests retain their original assertions.
- Audit correction: the independent Reviews PASS record did not prove the final Stage 2f tree after Method.RegisterClient deletion. Two stale test registrations remained there. Stage 2h removes them and proves the complete module plus a public-only offline SDK archive with GOWORK=off/no replace and no Runtime internal source. That snapshot is not a published release.
- Resolved introduced failures: generic transport made the old loss probe miss its semantic method; it now intercepts the current envelope. Opaque IDs and model selections lacked their wire schema projection, nullable optional adapter fields emitted null, and Domain tags were rejected; owner aliases and adapter DTOs fix these without weakening checks. The attempted outer maintenance receipt transaction blocked human edits; it was removed in favor of the actual publication-owner integration requirement. Generic pending-command routing was inappropriate for the proven accepted-Run settlement handoff; its original captured owner/RunID receipt contract is preserved.
- Not run: frontend UI/E2E/screenshot/composite frontend checks, explicitly excluded and no frontend or Runtime wire catalog change. Installed declaration/selection/origin/target/parent/callback/component completion and Memory maintenance receipt completion remain open.

Stage 2e verification:

- Passed: complete Runtime and CLI go test ./..., go vet ./... and go build ./..., including architecture/dependency gates and the exact public package/API inventory. No checks were disabled or assertions weakened.
- Passed: jsoninput, SDK, Application command admission and SQLite full targeted -race suites. Managed receipts full -race passed again after the restart fixture repair, including raw 1.0 recovery, changed numeric identity and exact original input persistence.
- Passed: Reviews complete go test -race ./..., go vet ./... and go build ./.... Strict refusal retains original review and invocation data. Invalid UTF-8 now fails serialized admission before the private use case; well-formed business-invalid updates retain their original refusal assertions.
- Passed: installed Reviews and Schedule manual replay -race suites, plus real Memory committed-response loss, Schedule lost create response, absent receipt without resend and observed identity after provider replacement. Existing host/backend restart, enablement and quiescence assertions remain in force.
- Passed: Runtime go generate ./... twice with zero SHA-256 drift across formal contract artifacts. go-api.json includes jsoninput and the actual SDK API changes; no Runtime wire catalog or TypeScript data contract changed.
- Passed: independent Reviews author tidy/test/vet/build with GOWORK=off, no replace, an offline temporary module archive containing only plugin/jsoninput public source and module metadata, its own generated contracts and an isolated module cache. No Runtime internal source or published module cache was substituted. The snapshot is not a published release.
- Resolved introduced fixture failures: the private restart counter schema lacked repeatable DDL/initialization and its cleanup captured a reassigned nil handle. It now uses the verified OpenSchema initialization convention and registers each actual DB handle's cleanup separately. The repaired regression and complete Runtime suite pass.
- Not run: frontend UI/E2E/screenshot/composite frontend checks, explicitly excluded; no frontend source or TypeScript wire shape changed. Existing getAcceptance consumer and published Runtime dependency gaps remain open. Generic installed-operation/target/origin/protocol handshake completion is not claimed.

Stage 2d verification:

- Passed: Reviews complete go test -race ./..., go vet ./... and go build ./.... Restart, original conflict/success, absent evidence, changed bindings, malformed private evidence and closed storage are covered.
- Passed: real stdio Reviews installation/lifecycle/response-loss suite with go test -race ./internal/bootstrap -run TestReviewBackend -count=1. Recovery now invokes only the public receipt query.
- Passed: complete Runtime and CLI go test ./..., go vet ./... and go build ./..., including their architecture/dependency gates; changed Go files gofmt -l and git diff --check.
- Confirmed by a removed temporary public-SDK caller: equivalent struct/map input has different current hashes, and raw 1.0 versus decoded 1 has different hashes. Standard jsontext object reordering makes the first encoding agree and preserves 9007199254740993 exactly; the second case requires retaining the admitted raw fingerprint rather than DTO rehashing.
- Resolved fixture mismatch: SDK Tool StructuredContent decodes numbers as json.Number, while a generic any reply uses another Go representation. The test now compares the complete raw JSON result with object-order normalization and exact numeric lexemes, preserving all original business evidence rather than weakening the result assertion.
- Not run: frontend UI/E2E/screenshot or composite frontend checks, explicitly excluded and no frontend change. No public protocol catalog/schema changed in this slice, so generation and TypeScript checks were not repeated. Generic cross-language identity and installed host entrance remain unproved; the existing frontend consumer and published dependency gaps remain open.

Stage 2c verification:

- Passed: complete Runtime and CLI go test ./..., go vet ./... and go build ./..., including architecture/dependency gates and real installed Reviews backend lifecycle tests.
- Passed: independent Reviews complete go test -race ./..., go vet ./... and go build ./...; generated artifact admission is checked against the immutable backend contracts.
- Passed: public SDK and contractgen full targeted -race suites, including raw request rejection before business execution and invalid-output uncertainty. Later generator safety/reference tests were also checked with -race and vet.
- Passed: Runtime go generate ./... and Reviews go generate ./.... A second generation preserved SHA-256 for the tracked Runtime catalog/Schema/OpenRPC/reference/API inventory and all formal Reviews artifacts.
- Passed: shared Runtime TypeScript npm run check, including typecheck, lint, format, all 284 contract tests and knip. Reviews client/contract-check.ts compiled with strict tsc; its negative assertions preserve closed status/outcome and required revision. These checks execute no frontend UI.
- Passed: an isolated independent Reviews author with its own generated package contracts built with GOWORK=off, no replace and a temporary SDK-only module archive, no Runtime internal source, offline module proxy and isolated module cache. Tidy/test/vet/build passed, including the generated-file regression. This fixture is not a published Runtime version.
- Not run: graphical UI/E2E/screenshot tests, explicitly excluded. The composite frontend gate remains unrun; its previously confirmed runs.getAcceptance consumer gap remains open. No SDK release was published or claimed available in the old dependency version.


Stage 2b verification:

- Passed: Runtime and CLI complete go test ./..., go vet ./... and go build ./..., including architecture/dependency gates. Independent Reviews complete go test -race ./..., go vet ./... and go build ./... passed.
- Passed: public SDK outside the workspace with GOWORK=off GOPROXY=off GOSUMDB=off go test ./plugin.
- Passed: focused public receipt/strict request/empty-success, atomic success/refusal rollback, immutable route/phase/history and former host/private schema tests. Real-process response-loss, absent-receipt and replacement-provider tests passed with actual host SQLite close/reopen.
- Passed: targeted -race for SDK receipt, Application commands/shared admission, private receipts, SQLite routing/schema admission, execution dependency/assembly boundaries and real Memory recovery/replacement plus Schedule manual replay. Schedule create/dispatch-loss races also passed.
- Resolved: private envelope encoding initially omitted empty Ack/zero values under JSON v2 omitempty. The shared Result uses omitzero and a zero-value regression; the formerly failing CLI lifecycle now passes. Removing speculative capacity left one Memory classifier reference; it was removed and full checks rerun successfully.
- Initial Runtime default-initialization check exceeded its existing 10-second availability deadline during the earlier full run. The unchanged check passed standalone and in the final full suite; no timeout/expectation was weakened and no unproven cause is claimed.
- Not run: frontend UI/E2E/screenshot checks, as explicitly excluded; frontend static/build checks were not repeated because this slice changes only Go/backend behavior and owning documentation. Runtime protocol catalog did not change, so go generate was unnecessary. The pre-existing frontend consumer gate and independent published SDK version remain open.

Stage 2a verification:

- Passed: public SDK and Memory/managed-model regressions, including -race; independent Reviews go test -race ./..., go vet ./... and go build ./....
- Passed: current SDK package GOWORK=off GOPROXY=off go test ./plugin. The isolated external Reviews author fixture passed GOWORK=off tidy/test/vet/build with only public SDK source and no local replace or Runtime internal source.
- Passed: real stdio Reviews installation, lost response, Runtime/backend restart, process retirement and CLI observation via go test ./internal/bootstrap -run TestReviewBackend -count=1.
- Passed: CLI full test/vet/build. Runtime's initial full suite passed all behavioral packages but exposed the intentional new public-package baseline; after updating that exact baseline and external consumer fixture, full internal/arch, vet and build passed. The final full Runtime go test ./... also passed.
- Resolved: the moved method's same-package model test callers initially referred to the deleted private functions; they now consume the public package. The public-package gate retains exact checking and dependency rejection, with plugin intentionally admitted as a transport boundary.
- Pending release graph, failed as expected: GOPROXY=off GOSUMDB=off go mod tidy in the actual Reviews module cannot find runtime/plugin in its currently named published Runtime version. Workspace and isolated current-source archive builds pass. Update published dependency metadata when the final SDK-containing Runtime version is available; no cached published module was overwritten or released in this slice.
- Not run: frontend checks in this Go/backend-only slice; no frontend files, UI/E2E/screenshot checks or new UI tests were added. The pre-existing runs.getAcceptance consumer-gate failure remains open.

- Passed before incoming main integration: runtime and CLI go test ./..., go vet ./... and go build ./.... Runtime's architecture/dependency checks run as part of its Go suite.
- Passed after incoming main integration: runtime and CLI go test ./..., go vet ./... and go build ./.... Runtime internal/arch and internal/dependency and CLI internal/arch passed with those suites.
- Passed: go generate ./... and the shared Runtime TypeScript npm run check, including 284 contract tests, types, lint, formatting and knip.
- Passed: targeted -race checks for plugin Domain, plugin Application, SQLite, package admission, execution and bootstrap. After final bounded-reader/cancellation and duplicate-case cleanup, focused distribution/default-initialization and withdrawal races also passed.
- Passed after final changes: SQLite exact-schema/former-installation-shape races, and the complete integrated execution package -race suite (167.235 seconds).
- Passed: frontend typecheck, lint, format, knip, locales, plugin message-frame contracts, circular/context/published-boundary/layer/port checks, HMR/bootstrap/style/design-system checks, tracked plugin resource rebuild verification, production bundle/CSS resolution and bundle-size gates. These static/build/contract checks do not execute the graphical UI.
- Failed, confirmed pre-existing on local main 61645c2561b174042aa157aef21cc00cfba67a86: check:api-consumers reports runs.getAcceptance has no non-test frontend consumer. Its seven checker regression tests pass. The CLI inventory now accurately identifies external reconciliation; the frontend source-consumer requirement remains open for a real use case.
- Resolved preparation/check failures: offline npm ci lacked cached dependencies; isolated copies of existing dependencies restored builds. Plugin resources initially lacked rolldown in this checkout, then passed after isolated dependency setup. Formatting drift was repaired. The pre-existing missing ownerRunStream TypeScript union and CLI API inventory entry were corrected at their owners.
- Not run: frontend UI, E2E, browser/native and screenshot checks, including the composite frontend npm run check whose test phase would violate the task. Native script fixtures were updated but their execution is not claimed.
- External proposal and review hashes still match their read-only baseline. No production/user data was opened, converted or deleted.
- Passed: changed Go files gofmt -l, git diff --check and feature commitlint using the owning frontend configuration. The managed checkout initially lacked ignored hook dispatchers; the existing dispatcher files were copied into this checkout without changing shared Git configuration.
- Resolved commit failure: the restored commit-msg hook prefixed an absolute linked-worktree message path with PWD. The corrected hook passed shell syntax validation, accepted a valid message and rejected an invalid message through both absolute and relative paths. Its temporary message fixtures were removed.

### Round cleanup

- Stage 4g: product 00ea6b5f committed and main merge is up to date. All invoked handles
  returned terminal results; temporary compile binaries and Desktop dist are removed.
  Original input hashes remain unchanged. The active worktree/dependencies are needed
  for the full goal. No external publication/shared product/user-data operation.

- Stage 4f: product 7873332f and caller/format repair 51602f5d committed; main merge
  is already up to date. All known build/generation/check/compile handles have terminal
  results. Temporary source-migration script, compile binaries and Desktop dist are
  removed. Original input hashes stay unchanged. Keep the required active worktree
  and development dependencies; no external publication/shared product mutation.

- Stage 4e: product 6e0623fb committed; main merge reports already up to date.
  All invoked build/check/compile handles returned terminal results, including the
  previously failed generator handle; the repaired generation is current. Temporary
  binaries and owned Desktop dist are removed. Hooks cleaned their temporary backup.
  Original input hashes are unchanged; no shared product source, external state or
  user data was changed. Keep the active worktree and development dependencies.

- Stage 4d: product ca770e48 committed and main already integrated. All owned
  generation/build/type/focused/source-compile/commit handles are terminal. Temporary
  binaries, backend sessions and private fixture databases are removed by their
  lifetimes. No frontend artifact, scratch script, shared source/configuration,
  external state, original input, credential or user data changed. Keep the managed
  checkout, formal contracts/regressions and dependencies for the active full goal.

- Stage 4c: product 0c0ecad2 committed and main already integrated. All owned
  generation/build/type/lint/focused/source-compile/commit handles are terminal.
  TemporaryDirectory removes test binaries and exact-rebuild probes. Owned frontend
  dist and empty retired Skills directories are removed. Keep formal ESM/contracts/
  regressions/docs and development dependencies for the active full goal. No shared
  source/configuration, original input, external state, credentials or user data changed.

- Stage 4b: formal command changes, main integration and dispatch correction are
  committed. All build/focused/source-compile handles are terminal; temporary test
  binaries and fixture processes/databases are removed by their owning lifetimes.
  No frontend artifact, scratch file, shared configuration or external state changed.
  Keep the active managed checkout and development dependencies for the full goal.

- Stage 4a: product bdf26566 committed; local main already integrated. All owned
  generation/build/typecheck/focused/test-source-compile/commit handles are terminal.
  TemporaryDirectory removes compile binaries and exact-build probes clean output.
  Owned frontend dist and empty Memory views directory are removed. Keep formal ESM,
  regressions/contracts/docs and development dependencies. No UI/native/runtime test
  process, shared configuration, external action, credential or user data changed.

- Stage 3z: product a025360f committed; local main already integrated. All owned
  generation/build/typecheck/focused/data-contract/source-compile/commit handles are
  terminal. TemporaryDirectory removes Go binaries and exact-build probes clean their
  output. Owned frontend dist/Vitest cache and empty Diagnostics views directory are
  removed. Retain formal ESM, source/contracts/regressions and dependencies. No native
  or UI process, external state, credentials, user data or shared configuration changed.

- Stage 3y: product 0643197b committed; local main already integrated. Every owned
  generation/build/typecheck/focused/source-compile/commit handle is terminal.
  TemporaryDirectory removes Go test binaries; exact ESM checks clean their temporary
  output. Owned frontend dist and empty retired Trajectory views directory are removed.
  Retain formal ESM, regression source and required dependencies. No native/runtime/UI
  process, shared configuration, external state, credentials or user data changed.

- Stage 3x: product fd1a9161 committed; local main already integrated. All owned
  generation/build/typecheck/focused/commit handles are terminal. Remove owned frontend
  dist and contract Vitest cache; temporary source probes clean their own directories.
  No runtime/native/UI test process or external state change was introduced. Retain the
  active managed checkout, formal regression source and required development dependencies.

- Stage 3w: retain formal manager/shared-client/IO/localization sources and necessary
  dependencies. Product 2b692d52 is committed and main b5160b5f integrated. Owned
  dist, temporary compile outputs and source/client test handles are terminal/removed.
  No UI/E2E/screenshot/native execution, external publication, credentials, shared
  configuration or user-data mutation. Keep managed worktree and progress symlink.


- Stage 3v: product a8d757ec and main merge/checker repair f1401198 are committed.
  Formal Diff ESM/SDK/contract/source and necessary dependencies remain. Owned
  frontend dist, exact-build temporary dirs, source-analysis probes and Go binaries
  are removed; command handles are terminal. Empty retired Diff views folder removed.
  No GUI/native/UI/E2E/screenshot execution, publication, credentials, shared config
  or user-data mutation occurred. Keep the managed worktree and progress symlink.


- Stage 3u: product/source 9376b997 is committed, main ffaf30c9 already included.
  Formal Usage ESM/source/generated contracts and necessary dependency artifacts
  remain. Owned dist, reproducibility directories and temporary Go binaries are
  removed; all command handles are terminal. Retired empty view folders are removed.
  No UI/E2E/native execution, publication, credentials or user-data mutation occurred.
  Preserve the active managed worktree and canonical progress symlink.


- Stage 3t: product a3661279 and main merge 7ff60fa4 are committed. Formal icon ESM/
  SDK/contract/source and necessary dependency artifacts are retained. Owned frontend
  dist, temporary ESM reproducibility outputs and Go test binaries are removed. All
  build/generation/typecheck/lint/focused/compile/commit handles are terminal. Keep the
  managed worktree and canonical progress symlink. No graphical test, publication,
  credentials, shared configuration or user-data mutation occurred.


- Stage 3s: retain formal module/resource/SDK/Reviews source and needed ignored dependency artifacts. Remove the owned production frontend bundle after its graph inspection. Temporary compilation outputs are removed. All generation/build/typecheck/lint/focused/compile/commit handles are terminal after final build inspection. Product is 61180b25; main d9d0c452 is merged in 06cafa4c. Preserve the active managed worktree and canonical progress symlink. No browser/native/UI test process, external state change, credentials or user-data mutation was introduced.

- Stage 3r: retain formal SDK/Board/generated ESM source and needed ignored dependency artifacts. Temporary compile outputs are removed; all owned generation/build/typecheck/focused/compile/commit handles are terminal. Product is committed in fd9b754b; main merge reports already up to date at 7957a603. Original proposal/review/v0.2 hashes remain unchanged. No browser/native/test fixture process, external state change or user-data mutation was introduced. Preserve the active worktree and progress symlink.

- Stage 3q: product committed in af081e07, incoming main 7957a603 synchronized in 765ccacd. All owned generation/build/typecheck/focused/compile/commit handles are terminal; TemporaryDirectory removes compile outputs. Empty directories from the replaced uncommitted checklist-only example are removed. No native distribution, UI test, external action, credential, shared configuration or user-data mutation was introduced. Preserve the active managed checkout and canonical progress symlink. Original input SHA-256 values match the read-only baseline.

- Stage 3m: product committed in beb2f5bc and main synchronized at 78f9f08b. Every owned compile/build/generation/typecheck/commit handle is terminal; temporary test binaries were removed. No native distribution, test process, external action, credential use, shared config change or user-data mutation was introduced. Preserve the active checkout, concurrent shared work and canonical progress symlink.

- Stage 3l: product committed in b16bfb38 and incoming main is synchronized at 78f9f08b. All generation/build/typecheck/compilation/commit handles are terminal; temporary binaries are removed, no runtime test/native distribution or background process was launched. Preserve the active worktree, concurrent shared-checkout work and canonical progress symlink. Final execution remains deferred.

- Stage 3k: product committed in 93c40ffc; all build/generation/TS/compilation/commit handles are terminal, temporary test binaries are removed and no test/native distribution was launched. Incoming main remains 78f9f08b. Preserve the active managed worktree, shared checkout changes and the canonical progress symlink. Complete execution verification is deferred under the user's development-first instruction.

- Stage 3j: every owned compile/generation/commit handle is terminal; compilation binaries were removed by TemporaryDirectory. No native distribution or test process was launched this round. Product is committed in 4f91b20c and incoming main is already up to date at 78f9f08b. Preserve all concurrent shared-checkout changes and its progress symlink. Keep the managed checkout for continued development; execution/final validation is explicitly deferred.

- Stage 3i: every owned verification/generation/commit handle is terminal. Test-owned private databases, HTTP endpoints, sessions/callbacks and native helpers were closed through their existing owners; no scratch script or persistent build output was added. Product source is committed in 0519d2ad; git merge main reports already up to date at 78f9f08b. Preserve the shared checkout's concurrent pending.go/publication.go edits and progress symlink. Keep the managed checkout for continued implementation; full/race final checks are explicitly deferred.

- Stage 3h: all owned focused/full/race/generation/TS/package-build handles are terminal. Temporary native distributions are removed; formal generated package resources are retained. Test helpers, sessions/callbacks and temporary databases are settled by existing owners. Verified source is committed in e6717fa6, incoming main remains 78f9f08bbb26bba633bd32def0bef7cc8381a7fa and original inputs retain their hashes. Keep the active managed checkout for the full goal.

- Stage 3g: all verification/commit handles are terminal; the obsolete released-graph tidy was explicitly stopped after target confirmation. Temporary independent source/proxy/cache fixtures are removed. Formal source/tests/docs remain in the active managed checkout; original design inputs retain their recorded hashes. Incoming main is 78f9f08bbb26bba633bd32def0bef7cc8381a7fa. No shared Git config, user data, temporary branch or unrelated work was changed.

- Stage 3f: all module, race and real-process verification handles are terminal. Test-owned Memory/Schedules/probe processes, HTTP transports, callbacks, sessions and private temporary databases are torn down through existing ownership. No scratch script, external/credentialed operation, user-data conversion, frontend artifact or shared configuration edit was introduced. Committed the verified source in 6b77c045 and synchronized main at 78f9f08b. All handles are terminal and process inspection found no probe/Reviews helpers; no test background work remains. Original input hashes match the baseline. Keep the active managed worktree for the full goal; this milestone record is the sole remaining change before the final clean-state check.

- Stage 3e: both independent-author fixture attempts, source archives, proxies and private module caches were removed by TemporaryDirectory, including the failed fixture. Test-owned stdio probes, backend processes, sessions and databases are torn down. No frontend build artifact, scratch script, shared configuration edit, external state change or user-data operation was introduced. All verification/commit handles are terminal, the completed slice is committed in a6161bdc and main synchronization reports already up to date at 78f9f08b. Process inspection found no live Reviews/probe helpers. Original inputs retain their baseline hashes. Keep the active managed worktree for the full goal; this milestone update is the sole remaining change before the final clean-state check.

- Stage 3d: public SDK/Reviews temporary source snapshots, file proxies, module caches and author copies have been removed by TemporaryDirectory. Tests own every helper process, callback listener and database. Owned icons/UI/frontend dist outputs created for Desktop consumer compilation have been removed; no frontend test or tracked source edit was introduced. All verification/integration/commit handles are terminal. Product commit 875375fa and incoming-main merge b13f7527 retain the completed slice; this milestone record is the only remaining change before the final clean-worktree check. Retain the active managed worktree for the full goal.

- Stage 3b: every module/race/real-process/isolated-author handle returned a terminal result. Test cleanup closes callback endpoints, connections, helper executables and databases. TemporaryDirectory removed both independent-author attempts, archives, file proxies and module caches. No scratch script, shared configuration edit, frontend artifact or background verification remains. The managed checkout is committed and clean; preserve it and its dependencies for the full goal. Original input hashes remain unchanged.

- Stage 3a: all invoked module, targeted race and fixture handles returned terminal results. Existing test cleanup closes private databases, model grants, transports and managed processes. No scratch scripts/artifacts, new worktree, shared config edits or background task checks remain. Input hashes are unchanged; retain the active managed checkout and dependencies for the full goal.

- Stage 2h: all module, architecture, targeted race and isolated-author handles are terminal. Both temporary SDK archive/author/module-cache fixtures were removed by their temporary-directory lifetime, including the failed fixture. Test cleanup owns backend processes and databases. Original proposal/review hashes remain unchanged; no shared checkout product source, user data, external state or credentialed service changed. Keep the active checkout and its development dependencies for the full goal.

- Stage 2g: all invoked backend/module/race handles returned success after fixture corrections. Existing test cleanup closes model grants, sessions, subprocesses, temporary packages and databases; no scratch scripts or build products remain. No user data, original proposal/review, shared configuration, external publication or credentialed service was changed. Keep the active checkout/dependencies for the full refactor.

- Stage 2f: backend/module/race checks and independent author fixture returned terminal success after fixes. Existing cleanup retires test processes, sessions, temporary packages and databases; TemporaryDirectory removed the public SDK proxy/archive/cache/source. No production/user data, original proposal/review, shared configuration, remote service or credential was modified. Original input hashes match the baseline. The slice is committed and incoming local main is already up to date; the active checkout/dependencies remain for the full refactor.

- Stage 2e: all verification handles returned terminal success after fixture repairs. Existing test cleanup retires process/session/database/temp-package ownership. TemporaryDirectory removed the independent author's SDK proxy/archive/cache/source. No user directory, original proposal/review, shared configuration, remote publication or credentialed service was changed. Original input hashes match the baseline. The slice is committed and incoming local main is already up to date; the active checkout and required development dependencies remain for the full refactor.

- Stage 2d: all backend/module/race checks returned success. Test-owned processes, private databases and installed binaries were retired by their existing cleanup; TemporaryDirectory removed the input-fingerprint proof source. No generated binary, UI artifact, shared configuration change or user-data operation was kept. The current input hashes still match the read-only baselines. The slice is committed and incoming local main is already up to date; keep the active checkout and required dependencies for the continuing full refactor.

- Stage 2c: all invoked verification processes returned terminal success. TemporaryDirectory removed the task-owned independent SDK proxy/checkouts/module cache. Formal generated Reviews contracts/data bindings remain as product artifacts. Task-generated Runtime TypeScript test cache was removed. The verified slice is committed and local main was already up to date; active checkout/development dependencies remain for the continuing refactor.

- Stage 2b: all invoked verification processes returned terminal success after repairs. Test-owned temporary databases/packages and helper processes were closed/removed by their owning test cleanup. No task scratch script or verification log was saved. No UI build output, shared configuration, remote publication or user-data operation was introduced. The slice was committed, incoming local main was already up to date, and the active checkout and necessary dependencies remain for the full refactor.

- Stage 2a: the temporary SDK file proxy, independent author checkout and isolated module cache were removed by their owning temporary-directory lifetime. Backend/unit/integration checks have all returned; no task check process is running. No frontend build output was generated in this slice. The worktree is committed and clean after main synchronization; the active checkout and necessary development dependencies remain for Stage 2b.
- Removed task-generated frontend/Icons/UI dist output, generated Icons/UI build sources and Runtime contract Vitest cache. Removed the task's offline-install debug log if present. All invoked checks have returned terminal success/failure; no task check process remains running.
- Kept the active managed checkout, formal code/tests/docs and isolated development dependencies for the continuing refactor. Shared product source and Git configuration were not edited. Before the next frontend verification, rebuild Icons and UI to restore their generated prerequisites.
- No remote push, publication, main-history mutation, production operation or credentialed action was performed.

## Unresolved questions

- Schedule crash acceptance still fails at activation handshake after the production
  Memory dependency repair. Preserve cut/no-replay/data assertions and inspect the
  original exchange before choosing a fix; causality is not classified as unrelated.
- Full installed/native Run/Goal live/normal-terminal/failure/interruption/restart
  accounting remains unproven. Stages 5aa/5ab/5ac are boundary implementations, not full
  delivery. Preserve unknown consumption and the original logical/physical owner.
- Complete actual owner-policy/in-flight admission and optional counting-provider
  evidence. Current contracts do not authorize inventing a universal Goal ceiling.
- Preparation phases now receive host RPC with cycle refusal and their original
  execution publisher. Observer cohosting and retained background resource attribution are implemented; execution-bound background provenance
  and full durable composition remain open; cycles cannot silently bypass policy.
- Embedding evidence, retention, transitive dependency/status/lifecycle, every
  consumer, independent author/module and all P01-P48/review acceptance remain open.

- Stage 5t/5u/5v implement public/auxiliary/primary input transformation and final
  model-media admission with original source evidence. Full real process/native/
  delegated and all-workflow acceptance remains open; bounded tests are not a full
  transformation platform completion proof.

- Primary/auxiliary context and accepted execution/use-case model selection are
  implemented. Stage 5s adds known auxiliary capacity preflight using its actual
  counter or the shared estimate. General transforms/per-call routes/multiple
  independently admitted calls exist at their boundaries; complete final acceptance remains open.
- Internal AuxiliaryResolver now requires an explicit budget and returns exact
  SelectedChat capture. All known internal consumers are migrated; no public
  plugin SDK or protocol type changed in this slice.
- The interrupted prohibited UI run exposed seven failures in partial output.
  Attribution is unproven; inspect merged feature contracts and consumers without
  authoring/running UI tests. This does not waive final integration requirements.

- Stage 4p proves actual public read compilation, CLI contracts and binding refusal,
  not the complete native selection/withdrawal/restart matrix or rendered context
  behavior. Memory notification/independent-author platform work and final mutation acceptance remain.
- All CLI Schedule Core entrances are now migrated. Native public-call process
  loss/replacement/disablement/uninstall execution, generic notification ownership,
  portable wide-number consumer behavior and independent author packaging remain
  part of the full goal; this slice does not establish them by compilation alone.
- Stage 4n completes actual public creation/capture and source migration, not the
  whole native lost-create/revocation/restart matrix. Later Stage 4o retires the
  last Schedule update method and its types; final execution remains open.
- Stage 4m migrates the actual manual entrance and preserves original request/typed
  acknowledgement ordering. The native public-call response-loss/restart/disablement
  matrix is compiled but remains unexecuted until final consolidated acceptance.
- Stage 4l proves CLI intent persistence/reopen and public adapter dispatch, not the
  complete native client-plus-Runtime response-loss/restart/replacement matrix.
  Later owning slices retire Schedule/Memory business facades; full integration remains open.
- Stage 4g implements post-setup activation, private clock/scan/original recovery,
  generic owner notices and host-driver deletion. Execute native acceptance for enabled
  non-selected installations, lost Core acknowledgement, restart/disable/replacement
  and independently accepted Runs on the final source. No second wake owner is permitted.
- All Schedule Core business facades are retired through actual consumer migration.
  Compilation does not prove all native/automatic/cross-plugin lifecycle behavior.
  Complete cross-generation command recovery acceptance remains open.

- Stage 4b implements existing Memory/Schedules consumer invalidation for public
  commands, including original receipt recovery/uncertainty, with focused evidence.
  Complete generic plugin-defined notification and final cross-generation/component
  notice consumption remain open; current feature projections do not close those.
- Verify restoration of the original Memory/Reviews mutation journal evidence after
  component/Runtime/window replacement. A retained in-render retry handle is necessary
  but does not prove recovery in a new UI generation.

- Stage 3x closes the previously missing source consumers: point/requirements/provider
  manager operations and the actual HTTP resource query projection. The 109/109 static
  gate passes. Full installed semantics, lifecycle and final source must still be
  verified; source coverage does not discharge the remaining P01-P48 obligations.


- Execute real installed Stage 3p dependency waiting/setup/callback lifetime integration on the final source. Generic Tool call preparation, complete observed dependency states and execution-owned durable retention are still implementation requirements; setup callback references must not substitute for an invoked Tool's original calling context.

- Stage 3p implements the previously missing provider waiting, named setup and frozen session binding seam; a compiled implementation plus SDK-only lifecycle test is not full installed activation/component acceptance.

- The requirements inspection query still resolves metadata without activating code. Stage 3p setup/session binding is its real execution seam; point values/applicability/ordering now have an admitted public owner and actual analyzer consumer; durable scoped composition and executing decoration remain required. Query results alone are not lifecycle evidence.

- Execute final acceptance for Stage 3m original caller retention and restoration through the actual Schedules durable request/occurrence and canonical Run callback, including response loss and restart. The private model and real dispatch path now implement it; compilation alone does not close the review requirement.

- Execute final Memory business-only input/callback/receipt regressions and the missing-metadata fresh-admission guard. Stage 3i removed Grant from the DTO and both durable inputs; this is now a verification obligation, not unfinished removal.

- Bind the verified callable capability/export owner into the installed declaration and activation lifecycle, migrate real Memory/Schedules required contracts and preserve contribution independently of server identity. Callback preparation/notifications are now outside generic source authority; bind that real feature boundary to the new declarations. Reuse existing durable Commands routing for captured contribution/export/target/origin/parent; no second route or receipt owner.

- Unary callbacks now have a public transport and real Memory consumer. Complete installed activation/dependency rendezvous must still replace remaining private feature integration; canonical Delivery binding and actual Schedules acceptance are verified, while decorators/remote next remain open.

- Extend the verified private maintenance admission/publication receipt and original host routing to public contribution bindings. Preserve the distinction between recovering a completed receipt and unknown/canceled original work; never restart its models from host routing alone.
- Extend the verified immutable input to the complete generic binding: exact contract/contribution, captured target and adapter-established origin/parent, with current declaration and activation handshake. Existing raw JSON and old-format refusal do not prove the missing installed operation/proxy/component paths.
- Extend the installed unary original-binding/retention owner with concrete target, origin/parent and declared dependencies. Stage 3j removed closed service binding; Stage 3k implements the public unary entrance, while all-consumer integration remains open.
- Finish generic call/error projections and public TypeScript client/component bindings through Reviews; current Go-owned generation supplies structural contracts and preserves typed outcome vocabulary.
- Verify the selected MCP/SDK custom method and streaming primitives using real processes.
- Select and implement actual module/resource loading for Web and Wails, including a management recovery entrance.
- Actual Schedules is now a real runs.getAcceptance reconciliation caller. The previously tracked graphical source-consumer gate remains separate; do not add a dummy client call or weaken a valid runtime contract to satisfy it.
- Select the actual SDK-containing published Runtime version for independent application module metadata after the complete public surface exists. The isolated current-source archive proof does not claim published artifact availability.

## Spillover risks

- Stage 5ac replaces the public OpenRPC source signature with variadic callbacks.
  External method-value/interface consumers require an SDK update and rebuild;
  ordinary calls remain valid. Core/backend/storage formats are unchanged. No
  published module release or external consumer deployment was performed.
- Main's Scope v0.50.0 graph is integrated and workspace compile/boundary checks
  pass. This is not qualification of old waiting checkpoints, native distributions
  or an independently published module graph; final acceptance retains those duties.

- Stage 5aa adds logical_purpose to the required database schema. Old databases are
  refused without conversion or deletion; no data migration was performed.
  Current-format same-build restart is verified, not an old-layout upgrade.

- The combined merge requires Core .31 client regeneration/rebuild. Steer fields
  use candidateUserItemId/steerInputs and accepted-versus-retired semantics; journals
  require steerItemId plus original Run metadata/installed route. Former incomplete
  entries refuse unchanged; resolve them with their original client before replacement.
  Backend .2/export3 unchanged, and provider redelivery is not a migration option.
- Unknown independent attempts now invalidate exact recovered Run/Goal cost and
  count admitted work without synthetic usage. Root context remains its own primary
  calibration. Live/normal-terminal totals remain incomplete and cannot be advertised
  as budget-enforced before the next integration removes that gap.

- Core .30 adds optional segmentId to endpoint observations. External strict
  clients require regeneration/rebuild. Backend .2 and export schema 3 unchanged;
  retained columns/JSON require no conversion. Old unassociated observations remain
  their original facts; no ancestry-based retroactive Run billing is introduced.
- Internal StartEndpointInvocation now requires an explicit logical owner argument.
  Existing in-repository consumers are migrated without an old signature. Fresh
  selected IDs may no longer reuse calling/primary/Tool identities; callers need
  distinct stable exchange IDs. No provider redelivery migration is authorized.

- Stage 5w adds exact host selected-model callback methods. External consumers
  requiring them must rebuild/current-host grant. PublishRPC method-value types
  requiring func(http.Handler) error must migrate to the variadic signature; no
  old source wrapper remains. Wire/storage epochs stay Core .29/backend .2/export3.
- Independently admitted descendant consumption is visible in physical evidence
  and retained usage reports, but its Run/Goal totals/budget enforcement remain open.
  Do not infer aggregate delivery completion from the per-call callback checks.

- Core .29 changes media refusal timing for well-formed Start/Resume/Steer input.
  External consumers must observe the accepted Run outcome; structural validation
  remains before acceptance. Original media/command identity are preserved, with
  no new storage migration or side-effect redelivery authority.

- Input-transform execution checkpoints require model_inputs and explicit empty
  capture. Former payloads cannot be silently resumed as the current chain. Internal
  SelectedChat replaces AuxiliaryModel; all known internal consumers are migrated.

- Core .28 incorporates main's required public Start/Resume identity and retained
  Run admission metadata. Old journal Run entries lacking that metadata are refused
  and preserved, without conversion/deletion; affected external clients must retain
  original keys/namespace/parameters/metadata and rebuild against the current Core.
- Auxiliary local estimates remain approximations without a provider counter;
  actual provider capacity failures retain their original failure/observation path.

- Stage 5f removes Memory's private model callback protocol and requires current
  backend/Runtime rebuild together. Private maintenance/receipt/data formats do not
  change. Interaction/auxiliary decorators, durable inner evidence and generic
  embedding parity remain open; complete response transport does not prove those.

- Stage 5e adds client.plugins.stream and bindOperations' generated chunk-map stream
  entrance. Rebuild shared client/SDK/component resources together. Async consumers
  must respect the supplied signal; close waits for their returned Promises.
  Core/backend/storage/receipt formats are unchanged. Unsafe numeric stream values
  are refused; full wide-number portability and production composition remain open.

- Stage 5d adds public PluginStreamer/PluginStreamingMethod/BindPluginStreamingMethod
  and PluginCallbackCaller.StreamPlugin. Rebuild Go authors against the generated
  public API. Core/backend/private data formats do not change. TS/query streams,
  production models/decorators and published author parity remain open requirements;
  the current workspace compiler does not establish published dependency availability.

- Stage 5c advances Core to 2026-10-14.21 and adds plugins.stream plus ephemeral
  chunk/result/refusal notification frames. Rebuild Go/TS consumers together.
  Existing unary callers retain their proper transport; TS high-level streaming,
  typed/callback proxies and actual production composition remain implementation
  requirements. Backend/storage/private receipt formats are unchanged. Transport
  backpressure/remote fixture proof does not establish full installation lifecycle.

- Public SDK backend streaming APIs require current Go module/transport support.
  Chunks use the existing call-owned callback and exact descriptor; final receipt
  ownership remains with the operation, not the SDK. A lost/partial stream is not
  replayed. Runtime generic installed/component entrances are still unary pending
  the route/recovery migration. No existing wire version/private data format changes
  or external module publication occurs in this slice.

- Core .20 adds stream schema metadata; rebuild current Go/TS clients together.
  Callback stream methods require the actual new stream API and final consumption,
  with no downgrade/retry. Aggregate byte policy conservatively counts encoded
  deltas, so callers must allocate their actual response budget accordingly. Existing
  unary identities, backend protocol/private data formats stay unchanged. Installed
  duplex/TS/production model composition and independent module publication remain open.

- New public Go model callback APIs require the current module for external authors.
  No existing Core/backend method or private data format changes; no module is
  published here. The call owner must enforce its actual budget and original model
  identity/fencing. Lost callback delivery remains uncertain; no SDK provider retry.
  Streaming/installation/TS parity and production consumers remain open.

- Core .19 removes agentMemory.changed, schedules.changed and Schedule event IDs.
  External subscribers must use plugins.dataChanged and reread their business owner;
  no legacy alias/double publication is retained. Generic plugin signals may refresh
  multiple plugin readers, but do not invalidate unrelated Core resource topics.
  Backend wire/storage/receipt formats are unchanged. Full author/lifecycle/streaming
  and all-workflow acceptance remain open; no external deployment was modified.

- Core .18 removes MaximumAgentMemoryItemsPerTarget. Published Memory pages require
  Capacity and current request/row bounds. External clients/packages must rebuild
  against current contracts; old frames have no alias or conversion. Private storage
  and receipt formats remain unchanged. Business notification migration and final
  process acceptance remain open; no externally published module is changed here.

- Core .17 removes public Memory records/enums and their wire sample/schema. External
  clients must consume the publisher contract and own their visible projections.
  Published Memory identity/content/vocabulary schemas now tighten the actual
  operation handshake; rebuild packages/Go/TS/ESM together. Private storage/receipts
  are unchanged, with no legacy conversion or provider retargeting. Core capacity
  and notification knowledge remain open until their actual consumers migrate.

- Core .16 removes the last Memory mutation and feature. External clients must
  rebuild and adopt published installed operations; unknown old Core commands
  receive no retargeting or conversion. Project additions require the caller's
  captured workspace. Core read-model/limit/topic migration and full native
  acceptance remain open, with private data/receipt formats unchanged.

- Core .12 removes a public Memory list API/request and workspace convenience read.
  In-repository callers migrate; external consumers cannot be enumerated here and
  need the installed contract. CLI/workspace materialization remains bounded by
  current target scale; future larger contracts must revisit consumer capacity.
- Core .11 removes the final Schedule method plus all public Schedule transport
  types, capability and Core error translation. All in-repository consumers migrate;
  external consumers must adopt publisher contracts/refusals. Existing pending
  installed intent retains original input and owner; no old-method translation exists.
- Core .10 removes a public Schedule creation API and its request type; external
  consumers cannot be enumerated from this checkout and need the documented
  installed call. Pending creation retains its exact draft when the original owner
  or replay promise cannot be restored; the CLI will not originate a new creation.
- Core .9 removes a public Schedule manual-execution method and both DTOs. All
  in-repository callers and generated projections migrate; external consumers
  need the published installed operation. Later slices remove Core create/update;
  no removed-method request is translated through an alias.
- Stage 4l removes a public Go/Protocol/TS Schedule deletion entrance. In-repository
  callers and generated contracts are migrated; external consumers cannot be
  enumerated from this checkout and must adopt the documented public installed call.
  Unresolved CLI intents deliberately remain visible when the original owner or
  replay guarantee cannot be restored; they cannot be resubmitted to another owner.
- Stage 4k advances Core to 2026-10-14.7 and removes schedules.list/public bindings.
  External consumers must rebuild and bind the installed publisher list operation.
  Backend wire and private storage/business formats do not change. Generated Go types
  are schema projections, not another owner; exact capability still validates I/O.
  Schedule mutations still use Core facades and require original-bound recovery during
  their next cutover. Independent backend source/publication parity remains open.
- Stage 4j advances Core to 2026-10-14.6 and removes the plugin rename wrapper/DTO,
  action enum/array/declaration and CLI subcommand; external Go/TS/CLI consumers must
  rebuild and use canonical Session editing or the public installed business entrance.
  Reinstall Trajectory at its new digest. Existing uncertain old wrapper requests are
  not converted into new sessions.update commands. Private business/storage formats
  remain unchanged; no data is deleted. Full cross-generation recovery/native execution
  remains final acceptance work rather than a claim from the focused RPC replay test.
- Stage 4i advances Core to 2026-10-14.5 and removes readRenderer, the HTML DTO/array
  and guest IPC; rebuild generated clients and reinstall Tool results. Shared UI theme
  projection/bootstrap exports are removed; external authors use ordinary provider
  context. Backend wire, private business formats and recorded outcomes stay unchanged.
  Direct native compilation warnings require supported platform packaging verification
  in the final phase. Original read-only draft still documents retired carrier history;
  it is intentionally untouched and does not govern the current implementation.
- Stage 4h advances Core to 2026-10-14.4 and adds component file scope/selector;
  rebuild all generated consumers and reinstall changed packages. Former file HTML
  metadata is refused without an alias or compatibility reader. Backend wire and
  private business data are unchanged. Tool-result carrier migration and native
  acceptance remain open; safe source inspection is a current data presentation.
- Stage 4g advances Core to .3/backend to 2026-10-14.2 and removes scan/dispatch from
  the full Schedules contract. Rebuild SDK/clients/installed resources together; original
  incompatible bindings are unavailable and never converted. Business storage format is
  unchanged. Enabled background entries each own their own installation; provider selection
  controls future human Core calls, not another installation's pending recovery. Generic
  resync is intentionally coarse and carries no business result. Source/short tests do not
  prove final native process, replacement, non-selected or window-generation behavior.

- Stage 4f advances Core to .2 and backend to 2026-10-14.1, changing complete feature
  capability identities. Rebuild packages and Go/TS/SDK clients; old captured routes
  are refused rather than converted. plugin.Failure is removed in favor of
  capability.Refusal and SetupBinding replaces Core DTO dependency in setup transport;
  repository-external Go authors must migrate these exported uses. Local source proof
  still does not establish published module availability. The old host scheduling
  loop remains explicit next work, not an alternative shipping worker implementation.

- Stage 4e advances Core to 2026-10-14.1 and removes the final business Views and two
  read operations; rebuild Runtime/CLI/TS/Host and installed package resources together.
  Renderer HTML, five CLI-consumed Schedule Core facades and the host background driver
  remain explicit migration work. The new gate credits actual CLI source calls and
  reports them separately; it does not prove installed equivalence or graphical behavior.
  Component retry handles do not by themselves prove a new-window/Runtime-generation
  recovery. Backend protocol .4 and business data formats stay unchanged.

- Stage 4d replaces prepareManual with complete runNow, changing the Schedules
  capability identity. Rebuild and select matching backend/namespace resources;
  former original bindings remain unavailable rather than being converted. Root manual
  request loss with no private receipt remains uncertainty and cannot create another
  Run. Current selected-role background recovery is retained only until the explicit
  backend-owned role cutover; full native/cross-generation acceptance is outstanding.

- Stage 4c advances Core to .15 and removes all Skills-specific read/kind/carrier
  contracts. Rebuild Runtime/CLI/TS/SDK/Host/packages together. Components should use
  shared SDK error classification to preserve the current client's class identity.
  Runtime Skill formats remain unchanged; package withdrawal preserves business data.
  Actual installed notice consumption and new-window/Runtime-generation mutation
  journal recovery still require final acceptance. The current retained in-render
  promise/key does not by itself prove cross-generation recovery.

- Stage 4b emits coarse read invalidation for every admitted Memory/Schedules
  command attempt/reconciliation, including no-change and uncertain outcomes.
  Consumers must reread their owner, not infer mutation success. Schedule notices
  no longer narrow to one ID. Current SDK receipts preserve raw original evidence
  through argument translation; new invocation still enforces the method input
  schema. Wire/storage formats are unchanged. Integrated process recovery remains
  a final-execution obligation even though its sources compile.

- Stage 4a advances Core to .14, removes Memory view/read wrappers and adds client
  component scope. Public WorkspaceContext.focus is optional for client scope; actual
  workspace focus consumers must narrow it. Rebuild SDK/Host/packages/Go/CLI/TS together.
  Backend .4 and Memory data formats stay unchanged. Stage 4b closes the existing feature notice seam; cross-generation recovery
  and final installed/graphical acceptance remain open.

- Stage 3z advances Core to .13, removes clientDiagnostics view metadata and requires
  the public ComponentContext.telemetry collaborator. Rebuild SDK clients/Host/packages
  together; no former DTO/HTML/type alias remains. Diagnostics navigation now consumes
  the valid diagnostics material key. Buffer policy and Runtime storage/accounting stay
  unchanged. Complete installed/graphical lifetime acceptance remains outstanding.

- Stage 3y advances Core to .12, removes plugins.readTrajectory and sessionTrajectory
  view metadata, and adds public Session component context/scope. Rebuild clients and
  packages together; no old HTML/object/operation reader is retained. Backend .4 and
  trajectory storage/export versions stay unchanged. Disablement withdraws component
  resources while canonical retained Session history remains available. Full installed
  and graphical lifecycle qualification remains outstanding.

- Stage 3x advances Core to .11 and changes point contract projection/request fields
  from objects to canonical JSON strings. Rebuild Go/CLI/TS clients together and retain
  the raw contract at dispatch; no old reader remains. Backend .4/business storage stay
  unchanged. The manager inspection is a read snapshot, not a persisted composition
  or activation. Final lifecycle acceptance remains queued.

- Stage 3w adds shared client methods over existing Core .10/backend .4 and changes
  graphical provider/read surfaces only. Rebuild the graphical client. Provider choice
  affects future activation; read snapshots cannot rewrite already captured execution.
  No storage conversion. Graphical/cross-reload outcome acceptance remains unexecuted.


- Stage 3v advances Core to .10, adds workspace/material component metadata and removes
  the workspaceDiff kind/read wrapper. Rebuild clients and packages. Host baseline
  preference disappears; a fresh Diff render starts in working-tree mode. Public SDK
  workspace context is immutable data, not a private store. Business/workspace formats
  and Git authority stay unchanged; graphical target replacement remains unexecuted.


- Stage 3u advances Core to .9 and deletes plugins.readUsage and the usageSummary view
  kind. Rebuild clients and package resources; use canonical usage.summary. Ordinary
  component authors need the current SDK library environment. Accounting/storage is
  unchanged. Complete graphical runtime proof remains unclaimed.


- Stage 3t advances Core to .8 and removes the static view kind. Rebuild clients and
  installed packages together. Components without an own callable expose no binding;
  authors invoking bindOperations must declare one. Icon code remains private bundled
  content while React/JSX/UI come from the host graph. No backend/storage migration.


- Stage 3s adds required components projection and resource query/subscription, advancing Core to .7. Rebuild all generated clients; backend protocol/private business formats stay unchanged. Current workspace component scope binds one own callable contribution; static/multi-contract components and remaining placements still need their real owner migration. Existing HTML carriers remain implementation debt. ESM evaluation cannot be canceled/unloaded; disablement retires supported resources and a changed evaluated generation requires window reload. Actual graphical qualification is not claimed.


- Stage 3r changes the Core call contract field to canonical JSON string and advances Core to .6. Rebuild generated Go/Protocol/CLI/TS consumers and use generated CAPABILITY_JSON; no object reader remains. Backend protocol and private business storage are unchanged. Public component SDK/Board are authoring code and a built artifact; module import mapping, graphical admission/placement, exact-release assets, retirement and full lifecycle proof remain required before claiming component delivery. Independent published dependencies remain open.


- Stage 3q adds point output arrays and the public point resolution API; regenerate and rebuild consumers. Reviews' capability changes to include analyzer execution and its optional setup dependency. Rebuild both example packages together. Integrated Core protocol is .5 and backend callable protocol remains .4; rebuild generated clients. This point slice introduces no storage format or retained data changes. Complete installed lifecycle and external published module validation remain open.


- Stage 3p requires callable backend packages to expose the new SDK setup method and to register a handler when dependencies are declared. Rebuild code/declarations together; a missing setup method/handler is refused, without an old path. Core/backend protocol remains .4 and no storage format or retained business data changes in this slice. Public SDK adds Setup/OnSetup/Close and Runtime adds the callback caller; independent published module advancement remains open. Full installed execution and changed-owner races are outstanding.

- Stage 3o intentionally refuses callable backends whose published export set or complete method contracts differ from the selected declaration. Rebuild package code/declarations together. Private Registry/Launch constructors changed with in-repository consumers migrated; no Core/backend protocol or storage format changed. Ordinary Tool-only MCP entries retain their normal handshake. Actual publication/retirement behavior still awaits final execution.

- Stage 3n adds required output arrays for contribution requirements and the public requirements query; rebuild generated consumers. Current callable contracts and Core/backend .4 remain unchanged. Fresh managed installed calls now refuse undeclared collaborators. Existing feature consumers have not all migrated to that entrance; final integration must prove declared setup and frozen dependencies. No private stored business facts are converted or deleted.

- Stage 3m deliberately changes private Schedules table/snapshot shapes (required original Invocation, firing invocation_json and manual captured_request_json). Keep former data separately and use a fresh private directory; schema admission provides no conversion or old reader. Full Schedules consumer capability identities change while current Core/backend protocol remains .4. Core acceptance and existing business receipt ownership are preserved; runtime behavior still requires final execution evidence.

- Stage 3l deliberately breaks Core/SDK/declaration contracts (.4) and host command storage (required invocation_json). Rebuild all consumers and packages and use fresh host data directories while retaining previous data separately; no old reader or uncertain-command conversion exists. Independent published Runtime module advancement is still open. Compilation does not establish actual callback/receipt acceptance or durable trigger-origin preservation.

- Stage 3k expands public Go/Protocol/CLI/TS APIs and the operation-kind vocabulary with a resolved unary extension. Rebuild generated consumers. The envelope and typed proxy compile, but origin/parent/concrete target, declared dependencies and streaming are not yet complete; do not label the full platform accepted. Independent published Runtime module metadata remains open. The actual final execution suite must validate the original route/private outcome boundaries and definite-failure projection.

- Stage 3j is a breaking root Flame declaration, Go API, Runtime Protocol and host storage change. Rebuild consumers and packages, use fresh data directories and retain old data separately; no old inline declaration or route reader exists. Generic backend metadata is admitted without activation, but full dependency waiting/cycle diagnostics and client component lifecycle remain required. Core public Protocol is 2026-10-13.3; backend Protocol .3 is unchanged. Published independent Runtime module advancement remains outside the local source proof. Current-source execution acceptance is deliberately outstanding.

- Stage 3i deliberately changes Memory Search/Maintenance input contracts and the host/shared private command format. Rebuild affected backends and consumers and use fresh data directories; exact admission refuses old formats without rewriting or deleting retained data. Existing unknown command evidence cannot be silently transferred into the new format. The internal AdmitCommand signature changes with all current Memory/Schedules callers migrated. Final guard execution/race verification remains queued; generic contribution/public operation migration remains open.

- Stage 3h changes the internal private receipt-registration Go signature and moves feature method metadata to typed factories; in-repository consumers are migrated. Callable input/output/name/kind/target shapes, backend wire .3 and storage are unchanged, while presentation descriptions become readable generated prose. No external published consumers of the internal adapter were modified. Generic contribution exports, component paths and full author parity remain open.

- Stage 3g removes the former public SDK method/capability types, enum constants, pure constructors and BackendProtocol constant without aliases. Go authors must import runtime/capability for those contracts and rebuild; all in-repository consumers are migrated. Wire .3 and saved canonical JSON remain unchanged. The independent fixture proves current source availability, while the real example still names an older published Runtime module; final publication and released dependency advancement remain required.

- Stage 3f changes only internal tool-composition/source-registry contracts and configured Memory collaborator-resolution behavior. Rebuild the Runtime; no Core/backend protocol or persisted-data schema changes are introduced. Failure reading or constructing a configured embedding collaborator now refuses that Tool call before dispatch; an absent role and backend optional embedding-request fallback remain valid. Current Services declarations/closed kind still require the forthcoming generic capability cutover.

- Stage 3e requires BackendProtocol io.github.tangerg.flame/backend/2026-10-13.3 and capability-shaped feature-generator input. External SDK callers/backend binaries require rebuild; former protocol and bare-array generator input are refused. Existing Reviews bare method/receipt callers must use the published reviews capability binding. Core Protocol and private business storage do not change in this slice. Installed declaration/token migration and published module metadata remain open.

- Stage 3d Protocol 2026-10-13.2 changes runs.start target shape and runs.getAcceptance request verification; external compiled clients and Schedules backends require rebuild. Host directories with automation_starts and private Schedules directories without runtime_target are strictly refused and retained; use fresh directories. No original user data was opened, converted or deleted. The native author package remains bounded by 128 MiB aggregate capacity.

- Stage 3b replaces Memory wire grant URLs with typed ephemeral CallbackRef values and removes private Models/BindModels APIs. In-repository producers/consumers are migrated without an old grant reader. External compiled Memory backends require rebuild. The independently verified public SDK archive is a source snapshot; published module metadata, installed callback rendezvous and canonical Delivery linkage remain final-delivery requirements.

- Stage 3a adds plugin_provider_selections and removes exclusive-enable admission. Existing data directories are refused and retained; use a fresh directory. External automation that formerly enabled a backend to select it must explicitly call plugins.selectProvider. Missing/disabled/uninstalled selections remain unavailable instead of following enabled candidates. Public Go/Protocol/CLI consumers are updated; generic declaration/point and graphical selection workflows remain open.

- Stage 2h requires the current backend protocol and exact method fingerprint on invocation. External backend binaries and SDK consumers require rebuild. Host command routes now require contract_json; former directories are refused and retained, with no conversion or old reader. The Runtime Protocol identity is independent of this backend wire change. Full installed declaration/activation protocol admission remains open.

- Stage 2g changes the shared private command schema from completed-only to pending-capable nullable reply. Existing Memory/Schedules directories are refused before conversion; retain them separately and use fresh directories. Unconfirmed original maintenance remains visible pending evidence, even if later independent maintenance folds its ledger; this is not a fabricated recovery.
- Stage 2f changes SDK registration to explicit Backend assembly/publication, removes per-method client registration/Method.RegisterClient and removes former physical per-operation RPC handlers. External backend authors/compiled callers require rebuild and migration. BackendProtocol is the current public wire identity; declaration and activation matching still need the owning cutover before API freeze.
- Stage 2e removes public plugin.InputDigest and adds unexported received input to Command; external unkeyed literals/old helper callers require migration. Host digest-only routes, Memory/Schedules digest-only receipts and Reviews re-encoded arguments directories are refused before conversion; retain them separately and use fresh directories. In-repository callers are migrated; independent published dependency metadata and exact public handshake remain open.
- New schema-bound methods tighten feature input/output admission. Params and Result now retain unexported original wire evidence, so external unkeyed literals require migration; keyed usage and real consumers are updated. Separate Reviews ListMethod/UpdateMethod exports were removed without aliases.
- Public installation/protocol/storage changes break external compiled clients. Rebuild all in-repository consumers; report any externally used SDK/module contracts.
- Protocol 2026-10-13.1 removes approve/revoke and replaces installation state with required enabled. All external compiled Runtime clients must rebuild. No externally deployed clients were inventoried or modified in this isolated task.
- Existing incompatible user data is retained separately and refused; no conversion or deletion is authorized. Stage 2b adds plugin_commands to the exact host schema and replaces private Memory/Schedules commands.result with a value/refusal reply envelope. Existing directories must remain untouched and be replaced with fresh directories for this pre-release cutover.
- Concurrent user work in main is outside this checkout. Final integration must revalidate the then-current shared branch.
- The active worktree is required work, not disposable scratch space. Remove only owned scratch artifacts between slices; retire the checkout after safe final delivery.

## Deliberately excluded scope

- Frontend UI, E2E and screenshot test execution or additions.
- Marketplace, untrusted-code sandbox, arbitrary workflow DSL, automatic compatibility, live provider migration and a second execution engine, as deferred by the proposal.
- Production operations, user-data deletion, destructive Git actions, credentialed services or publishing without the required authorization.
