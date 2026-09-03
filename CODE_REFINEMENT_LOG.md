# Code Refinement Log

This file is a compact handoff index, not a duplicate command transcript. Git
history and the tests in each committed batch are the authoritative progress
record. The original 1,371-line narrative for Rounds 1–25 was consolidated on
2026-09-03 after the refinement had passed one hundred rounds.

## Current state

- Rounds 1–136 are complete and pushed to `origin/main`; public Runtime changes
  are followed by their exact CLI dependency update.
- The current Runtime contract is `bec9a946` (`fix(runtime): require agent memory
  timestamps`).
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

## Rounds 101–136 — exact handoff

| Round | Commit | Result |
| --- | --- | --- |
| 101 | `200a4d3` | Retried standalone shutdown without hiding the primary lifecycle outcome. |
| 102 | `19eca7f` | Preserved model-selection failure identity across Runtime boundaries. |
| 103 | `c672c4c` | Kept Runtime contract-violation causes visible to CLI callers. |
| 104 | `e91c0fb` | Removed ambient Runtime configuration discovery from CLI startup. |
| 105 | `fb5d7a9` | Surfaced external-editor draft cleanup failures and withheld uncleared prompt text. |
| 106 | `b19dbae` | Verified schedule create/update acknowledgements against exact requested state and revision. |
| 107 | `83098da`, `5a31545` | Closed ApprovalRule wire shape, published the Runtime version, and made CLI validate complete unique rule catalogs. |
| 108 | `a227410` | Merged live workspace aliases at the Runtime owner and rejected repeated workspace identities at the CLI boundary. |
| 109 | `42557a1` | Rejected repeated workspace change and retained structured-diff paths at both the Runtime owner and CLI boundary. |
| 110 | `b0581e65` + CLI dependency update | Required non-empty path identities for change and structured-diff rows in every generated Runtime contract projection. |
| 111 | `1cf11854` + CLI dependency update | Required complete workspace, file, and grep output identities, then migrated CLI domain validation to the full Runtime wire shape. |
| 112 | `36d1bd54` + CLI dependency update | Removed the always-`utf-8` file-content encoding field and its duplicate CLI representation; valid text is now one documented Runtime invariant. |
| 113 | `92b928bb` + CLI dependency update | Preserved file modification instants as `time.Time` through Runtime and CLI, deferring RFC3339 formatting to the terminal presentation boundary. |
| 114 | CLI commit | Removed the unused file-entry name from CLI's presentation projection while retaining Runtime's public field for its proven Desktop consumer. |
| 115 | `4fdd4697` + CLI dependency update | Removed the file-read response's request-path echo; CLI now uses its still-owned request path for presentation. |
| 116 | `834261d3` + CLI dependency update | Removed the file-head response's request-path echo and the empty validator surface left behind by that deletion. |
| 117 | CLI commit | Removed the synonymous CLI file-line DTO and projected cloned Runtime protocol lines directly under the existing ordering invariant. |
| 118 | CLI commit | Removed the synonymous CLI grep-match DTO and projected cloned Runtime protocol matches while retaining the result aggregate invariant. |
| 119 | CLI commit | Removed the synonymous CLI diff-row DTO and projected cloned Runtime protocol rows while retaining structured-diff aggregate invariants. |
| 120 | CLI commit | Removed the synonymous CLI managed-skill DTO while retaining catalog validation, unique identity, isolation, and lifecycle acknowledgement. |
| 121 | CLI commit | Removed the synonymous CLI agent-document DTO while retaining complete-catalog validation, unique paths, and projection isolation. |
| 122 | CLI commit | Removed the synonymous CLI discovered-skill DTO and unified direct Runtime catalog cloning, validation, and identity checks. |
| 123 | CLI commit | Removed the synonymous CLI lifecycle-hook DTO while retaining wire, matcher, trust-state, and catalog isolation checks. |
| 124 | CLI commit | Removed the synonymous CLI usage-totals DTO and cloned Runtime model usage without maintaining a duplicate field list. |
| 125 | `304f589c` + CLI dependency update | Made usage bucket identity a Runtime contract invariant and removed the synonymous CLI bucket DTO. |
| 126 | `237f4484` + CLI dependency update | Made feedback content a Runtime contract invariant and removed the synonymous CLI feedback signal. |
| 127 | `bec9a946` + CLI dependency update | Required agent-memory timestamps in Runtime's Go contract and removed the synonymous CLI memory-item DTO. |
| 128 | CLI commit | Removed the synonymous CLI memory-patch DTO while preserving normalization, pointer isolation, and exact update acknowledgements. |
| 129 | CLI commit | Removed the synonymous Runtime-profile subscription-limits DTO and reused Runtime validation without touching changefeed partition behavior. |
| 130 | CLI commit | Removed the weak Runtime-profile replay-limits DTO and preserved Runtime's closed replay scope and generated validation directly. |
| 131 | CLI commit | Removed the duplicate generation-parameters DTO and its weaker validator; Run options now retain Runtime's exact value and reject repeated stop sequences. |
| 132 | CLI commit | Removed the duplicate per-model Run-usage DTO and validator while preserving CLI-owned cumulative progress, duration, equality, and isolation behavior. |
| 133 | CLI commit | Removed the duplicate question-option DTO and field-copy loop; CLI question fields now reuse Runtime validation and enforce the advertised option ceiling. |
| 134 | CLI commit | Preserved Runtime's closed Run-event and change-topic catalogs through profile negotiation, moving string conversion to text rendering only. |
| 135 | CLI commit | Derived CLI's checked revision-counter ceiling from Runtime Protocol's exact-JSON integer limit instead of duplicating the numeric contract. |
| 136 | CLI commit | Centralized the 20 MiB authored-attachment limit in the domain owner and reused it for resolution and dispatch-time revalidation. |

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

Exact commands, timings, regressions, compatibility notes, and failure evidence
remain discoverable from the corresponding commit and tests instead of being
repeated here.

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

- Start Round 136 by continuing to audit complete Runtime catalogs and CLI
  projections for aggregate
  invariants that element-level wire validation cannot express.
- Treat missing output-resource identity constraints as candidates only after the
  owning use case, protocol projection, and every in-scope consumer prove the
  required shape; do not add speculative validation.
- Keep this log compact. Add at most one concise row per completed round, then
  periodically consolidate rows into a themed range. Do not paste full command
  output, repeated validation boilerplate, credentials, or per-file narration.
