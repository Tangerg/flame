# Code Refinement Log

This file is a compact handoff index, not a duplicate command transcript. Git
history and the tests in each committed batch are the authoritative progress
record. The original 1,371-line narrative was consolidated on 2026-09-03;
completed history through Round 289 was condensed again on 2026-09-05.

## Current state

- Round 291 completes the storage-read ownership milestone. This refinement
  stream is stopped at the user's request; no further round is queued.
- Every round through the last completed table row is pushed to `origin/main`.
  Public Runtime changes are followed by their exact CLI dependency update.
- Authorized code scope remains `runtime` and `cli`. Desktop work is concurrent
  user-owned work and must not be modified by this refinement stream.
- The configured provider credential is used only through Runtime configuration.
  Its value must never be copied into this log, commands, diagnostics, or commits.

## Rounds 1–100 — consolidated history

| Rounds | Primary outcomes | Representative commit span |
| --- | --- | --- |
| 1–4 | Unified duplicated filesystem-observer lifecycle and Run event-commit construction; separated closed interrupt variants; replaced rollback boolean combinations with one closed restore scope. | `df7d909` … `c04cee1` |
| 5–14 | Made strict JSON and generated-contract behavior deterministic; rejected invalid collection restrictions, impossible conditions, unsafe selectors, projection-shape mismatches, and non-canonical JSON uniqueness comparisons. | `136ef6a` … `dcbf7bd` |
| 15–21 | Closed optional-minimum and allowed-value gaps, then split constraint rendering, validator generation, union projection, wire keyword checks, and API-reference rendering by their actual responsibilities. | `9d39742` … `b150bf2` |
| 22–25 | Separated mutation-guard and tool-observation phases, made MCP catalog lock/network ownership explicit, and replaced an unbounded recursive Skill watcher with the one-level bounded capability actually consumed. | `2072c2c` … `fe9e6f4` |
| 26–38 | Preserved observation boundary identity; unified exact observation and Skill source layers; confined project sources; rejected broken roots; counted only valid Skills; preserved Agent-document/recipe precedence, cancellation causes, and source budgets. | `cc7ed73` … `46f7937` |
| 39–49 | Made continuation and interaction recovery decisions explicit; serialized finite Run allowances; separated accounting; enforced canonical checkpoint/JSON representations; unified usage snapshots; moved usage subtraction into its domain owner; rejected restored call-count overflow. | `4188fcf` … `31f4c1a` |
| 50–75 | Confined workspace opens, listings, observations, state/config reads, plugin entries, attachment traversal, sandbox copies, and token/checkpoint reads; bounded directory traversal and fingerprints; centralized stable file/executable identities; verified complete snapshots. | `ad46b23` … `cb603af` |
| 76–85 | Bounded LSP, MCP, A2A, model, and online response frames; kept provider redirects same-origin; split Skill authoring; removed fallback clocks and unused storage timestamps; pruned expired memory replay state. | `4ea3c76` … `37a9f68` |
| 86–96 | Required exact Run execution attribution and provider/model identity at every durable, frozen, input, root-execution, and selection boundary; centralized exact validation and separated model roles from model selections. | `7472cd0` … `1ff0b21` |
| 97–100 | Made explicit Runtime configuration exclusive, retained cleanup failures, reclaimed failed Run activation, and closed failed HTTP serving without losing the initiating error. | `4691a6e` … `0b85a09` |

### Durable design decisions established by Rounds 1–100

- Runtime remains the sole semantic owner of Session, Run, Segment, Item, Goal,
  Plan, Interrupt, execution, persistence, recovery, model selection, and
  compaction. CLI adapts and presents those facts without recreating state
  machines.
- One fact has one owner, one representation, and one primary call path. Wrong
  shapes are replaced completely; aliases, compatibility fallbacks, duplicate
  state, and abandoned APIs are removed in the same batch.
- External and persistence boundaries are bounded, cancellation-aware, and
  explicit about path confinement, stable identity, complete pagination, and
  resource cleanup.
- Protocol metadata is validated before generation. Go validators, JSON Schema,
  TypeScript validators, manifests, and reference docs are generated projections
  of the same catalog.
- Model identity is the exact provider/model pair plus model-owned options.
  Credential precedence and provider-specific translation stay at the provider
  boundary.
- Closed domain choices remain closed values rather than boolean combinations or
  magic strings. Interfaces live with consumers and exist only for real boundary
  inversion or substitution.
- Complexity scores are evidence for inspection, not an instruction to split a
  coherent algorithm. Refactoring proceeds only when a distinct owner, phase,
  invariant, or deletion candidate is proven.

## Rounds 101–289 — consolidated history

The exact per-round index through 289 is preserved in
`git show 208f6b6d:CODE_REFINEMENT_LOG.md`. The ranges below record themes,
not a claim that every intermediate defense remains necessary.

| Rounds | Primary outcomes | Representative commits |
| --- | --- | --- |
| 101–109 | Preserved shutdown/error ownership, removed ambient configuration discovery, protected draft cleanup, and checked complete schedule/rule/workspace acknowledgements. | `200a4d3` … `42557a1` |
| 110–133 | Removed synonymous CLI DTOs and field-copy paths; moved resource, content, usage, model-option, and question constraints to Runtime-owned contracts. | `b0581e65`, `304f589c`, `bec9a946` |
| 134–149 | Preserved closed Runtime catalogs and exact numeric limits; validated complete wire trees before CLI projection could discard required facts. | CLI commits in the archived index |
| 150–161 | Required live and portable lifecycle timestamps, canonical tool-result identities, and consistent content/tool-name validation in generated Runtime contracts. | `bea20b5d` … `0d38d55a` |
| 162–174 | Removed repeated nested constraints and Handler-local checks; reused generated identity validation and enforced catalog order/filter continuity at CLI boundaries. | `c6a0bde9`, `f37dc362` |
| 175–188 | Made Application own total catalog order, exact identity, source precedence, and provenance for memory, workspaces, MCP, providers, Skills, recipes, documents, models, Tools, and Hooks. | `8624f0b3` … `9047eac3` |
| 189–209 | Checked bounded cursor pages and complete persisted catalogs; unified exact Session, Goal, Pending, and Run reads; removed redundant Goal/Schedule acknowledgement state. | `a2058135` … `5b130484` |
| 210–219 | Unified memory reads/curation, Knowledge cascades, archive/Plan validation, Skill catalogs, workspace inspection, and typed diagnostic Tool arguments. | `1868abc8` … `aceef0e3` |
| 220–232 | Bound recovery journals, Pending barriers, Runs, Items, Segments, and cleanup scopes to one admitted recovery tree; removed duplicate recovered-Session/checkpoint markers. | `89297db2` … `f759e265` |
| 233–243 | Replaced split expected/replacement fields with Domain-owned Run, Item, Session, Plan, Schedule, Goal, memory, and Knowledge transitions; preserved exact SQLite CAS fences. | `1ab79caf` … `772eab24` |
| 244–256 | Constructed owned compaction, Session mutation, resume, opening, barrier, cancellation, receipt, and recovery write sets before side effects; kept asynchronous handoffs isolated. | `7b02affc` … `df85ac68` |
| 257–266 | Removed unused unknown-Effect diagnostics and duplicate Start materialization; clarified execution outcomes, continuation ownership, one-shot preparation, command snapshots, and Schedule model input. | `c4540950` … `b194f5ec` |
| 267–282 | Added isolation across Session/query/Plan/conversation/watch/Hook/memory/MCP/invalidation crossings. These copies require individual production-ownership evidence during subsequent simplification. | `a97ef620` … `1069e248` |
| 283–285 | Isolated fork conversation prefixes and portable archive messages/collections from their source values. | `7b07f217` … `b8e22347` |
| 286–289 | Added storage-read copy layers and retained-result mutation tests; round 290 reassesses these against SQLite's actual fresh-result ownership. Round 289's live CLI returned `ROUND289_OK` (9,259/21 tokens; 543 ms provider duration), and invalid credentials returned `invalid_api_key` with zero usage. | `9ded61da`, `dca1eb60`, `e9357109`, `ae2736a8` |

## Recent completed rounds

| Round | Commit | Result and verification |
| --- | --- | --- |
| 290 | `d2312206` + CLI dependency update | Removed duplicate Application copies of fresh SQLite results and four mutation-only fake tests (net −168 Go lines); documented read ownership and verified old snapshots survive successor reads. No public/wire change. Focused sessions/persistence/bootstrap: 0.637/1.481/2.185 s; module/workspace matrix passed. Real CLI: `ROUND290_OK` (9,259/20 tokens, 596 ms provider duration), invalid-key zero-usage failure, cold show/list, and fork with preserved transcript and distinct identities. Temporary resources moved to Trash; history condensed to themed ranges. |
| 291 | `2c4e14b0` + CLI dependency update | Removed redundant Session/Run/Pending/Plan read copies and the second normalized-filter copy; retained validation and caller-input normalization (net −62 Go lines). No public/wire change. Module/workspace matrix passed: workspace tests 74.08 s, standalone CLI tests 39.50 s, isolated-cache Runtime/CLI lint 6.42/5.38 s. Live CLI: `ROUND291_OK` (9,259/20 tokens, 708 ms provider duration), invalid-key zero-usage failure, cold show/list, fork, and two distinct Run cursor pages; standalone snapshot matched exactly. Offline lifecycle tests cover Item reads and Pending recovery. Cache trimming delayed lint; a temporary proxy 500 was resolved using another download source. All round-owned resources moved to Trash. Stopped at this milestone; no next round. |

## Verification contract

Every completed behavioral round is accepted only after the risk-appropriate
subset of this matrix passes:

1. Format changed Go files and regenerate Runtime artifacts when applicable.
2. Run focused unit/integration tests for the changed owners and consumers.
3. Run standalone Runtime and CLI `test`, `vet`, `build`, and `staticcheck`.
4. Cross-build both modules for Linux and Windows when production Go changes.
5. Run the combined workspace Runtime/localruntime/CLI matrix.
6. Run `golangci-lint` serially for Runtime and CLI.
7. Build the real current-source CLI and execute one single-client/single-Runtime
   DeepSeek success path from a fresh `FLAME_HOME`.
8. Execute a separate fresh-home invalid-credential path and require
   `invalid_api_key` with zero input/output tokens.
9. Inspect the exact diff, commit explicit paths only, push the round, and move
   round-owned temporary resources to the system Trash.

Module checks use `GOWORK=off go test ./...`, `go vet ./...`, `go build ./...`,
`staticcheck ./...`, and `golangci-lint run --allow-parallel-runners` with the
workspace disabled. Workspace checks target `./runtime/...`,
`./runtime/localruntime/...`, and `./cli/...`. Record decisive inputs, measured
durations, failures, and extra lifecycle commands in the round row; keep raw
command output out of this index.

## Compatibility and scope

- Internal breaking changes were intentional where an invalid representation or
  duplicate owner was removed; every in-scope consumer was migrated atomically.
- Public protocol changes were regenerated and published before standalone CLI
  dependency bumps. No local `replace`, compatibility alias, fallback decoder,
  or dual persistence path remains for those changes.
- Desktop findings were not modified by this stream. Any change requiring a
  Desktop consumer remains out of scope until that module is explicitly admitted.
- No user Runtime home, shared cache, global dependency, or authorized
  configuration has been deleted or rewritten by validation cleanup.

## Remaining direction

- Further work requires fresh user direction. Possible choices, not queued
  tasks: a real-product lifecycle/terminal review, a broader evidence-backed
  simplification audit, or new product behavior. Do not continue copy-by-copy
  changes merely because a static pattern or hostile fake can be constructed.
- This milestone does not claim to revalidate every long-running live Goal,
  steer, compaction, and recovery scenario. Its bounded live checks and offline
  lifecycle coverage are recorded above.
- Keep this log compact. Add at most one concise row per completed round, then
  periodically consolidate rows into a themed range. Do not paste full command
  output, repeated validation boilerplate, credentials, or per-file narration.
