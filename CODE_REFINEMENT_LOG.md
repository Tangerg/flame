# Code Refinement Log

This file is a compact handoff index, not a duplicate command transcript. Git
history and the tests in each committed batch are the authoritative progress
record. The original 1,371-line narrative for Rounds 1–25 was consolidated on
2026-09-03 after the refinement had passed one hundred rounds.

## Current state

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

## Rounds 101–173 — exact handoff

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
| 137 | CLI commit | Moved the 4 MiB authored-message text limit into the domain owner and reused it for command input and external-editor boundaries. |
| 138 | CLI commit | Removed the copied session-search length ceiling and delegated that public input constraint to Runtime's generated wire validator. |
| 139 | CLI commit | Reused Runtime's generated question aggregate validator so CLI questions enforce both the required field set and four-field ceiling. |
| 140 | CLI commit | Replaced partial cancellation validation with Runtime's complete generated request validator, including the reason-length contract. |
| 141 | CLI commit | Validated each projected interrupt response as a complete Runtime wire tree before resume dispatch. |
| 142 | CLI commit | Validated projected MCP create, test, and update writes against Runtime-owned identity and collection constraints before dispatch. |
| 143 | CLI commit | Replaced weaker MCP server-name checks across delete, reconnect, tool listing, and authorization start with each Runtime request's generated validator. |
| 144 | CLI commit | Removed the flattened MCP authorization-attempt DTO; ports and polling now retain the validated Runtime value while CLI adds only chronology and cross-server observation checks. |
| 145 | CLI commit | Validated MCP Server and Tool wire trees before their justified CLI form and presentation projections, centralizing Tool schema encoding in one path. |
| 146 | CLI commit | Validated complete Runtime Provider wire trees before selective CLI projection, including discarded default-embedding metadata. |
| 147 | CLI commit | Validated Usage requests and complete Runtime Usage trees before map-to-bucket projection could erase model-key constraints. |
| 148 | CLI commit | Validated complete Interrupt unions before CLI projection could erase fields belonging to another interrupt variant. |
| 149 | CLI commit | Validated complete Runtime file entries before the intentionally smaller CLI projection could erase required names. |
| 150 | `bea20b5d` + CLI dependency update | Made pending-interrupt creation time enforceably required, then rejected extra, cross-session, and wrong-root interrupt sets during CLI snapshot projection. |
| 151 | `e67e6b6d` + CLI dependency update | Moved MCP authorization creation-time presence into the generated Runtime contract and left only cross-timestamp chronology in CLI. |
| 152 | `0f351472` + CLI dependency update | Moved Schedule creation-time presence into the generated Runtime contract while retaining CLI-owned cross-field scheduling chronology. |
| 153 | `7ca66b23` + CLI dependency update | Made every Run event timestamp enforceably required in Runtime's generated contract and removed CLI's duplicate zero-time check. |
| 154 | `ad417246` + CLI dependency update | Made live Session creation/update times enforceably required and validated the complete Runtime shape before CLI projection. |
| 155 | `29adafbe` + CLI dependency update | Made Goal creation/update times enforceably required and verified the existing CLI Goal boundary rejects either missing fact. |
| 156 | `1601d420` + CLI dependency update | Required committed Plan timestamps and stopped CLI's mock Runtime from fabricating committed Plan metadata merely to validate draft steps. |
| 157 | `dc4e3331` + CLI dependency update | Required portable Session creation/update times and verified CLI import rejects either missing fact before dispatch. |
| 158 | `e93a0d73` + CLI dependency update | Required every portable terminal Run lifecycle time and verified CLI import rejects each missing fact before dispatch. |
| 159 | `6534c97a` + CLI dependency update | Published the complete portable tool-result blob contract, including canonical ID grammar and required creation time, and enforced it at CLI import. |
| 160 | `6f9c4b77` + CLI dependency update | Unified live and portable content validation so both reject blank text and non-image media types before projection or import. |
| 161 | `0d38d55a` + CLI dependency update | Required nonblank live and portable tool names from one generated rule, then collapsed four repeated CLI artifact-import rejection harnesses into one boundary helper. |
| 162 | `c6a0bde9` + CLI dependency update | Removed the import request's duplicate restatement of nested artifact Session identity and the decoder's third check while preserving the same tree-qualified rejection. |
| 163 | `4eddab70` + CLI dependency update | Removed the final production parent-to-child constraint restatement; item scope now owns its discriminator once while list requests preserve the same nested rejection. |
| 164 | `aa1a89a8` + CLI dependency update | Collapsed four Agent Memory ID shapes from redundant nonempty, length, and pattern rules to the one exact canonical pattern, preserving every rejected spelling. |
| 165 | `07a3244f` + CLI dependency update | Collapsed MCP server and remote-tool length checks into their exact bounded grammar, retained list cardinality/uniqueness, and removed the resulting unused validator helper. |
| 166 | `f37dc362` + CLI dependency update | Removed Handler-local empty-input checks already owned by the common Endpoint; one endpoint regression now proves invalid Hook, Skill, Session, and Item requests cannot reach Application while translation and domain rejection remain intact. |
| 167 | CLI commit | Replaced the sideload command protocol's copied 256-byte Session ID ceiling with Runtime's canonical optional identity validator, preserving valid opaque Unicode identities and rejecting malformed values before process admission. |
| 168 | CLI commit | Routed Goal get, clear, stop, and resume through one generated `GoalRequest` validator so every Goal entrypoint rejects non-canonical Session identities before calling Runtime. |
| 169 | CLI commit | Bound cold Session reads to validated generated requests and rejected metadata whose identity does not match the requested Session. |
| 170 | CLI commit | Rejected `runs.list` pages that violate Runtime's newest-first creation-time and Run-ID ordering contract. |
| 171 | CLI commit | Rejected `sessions.list` pages that violate Runtime's favorite, update-time, and Session-ID ordering contract. |
| 172 | CLI commit | Preserved the normalized Session query through projection and rejected pages that escape its exact Workspace filter. |
| 173 | CLI commit | Enforced Runtime's creation-time and Schedule-ID order while materializing the complete Schedule catalog across pages. |
| 174 | CLI commit | Rejected workspace-file pages that violate Runtime's directory-first, path-ascending total order, including violations across page boundaries. |
| 175 | `8624f0b3` + CLI dependency update | Moved Agent Memory management ordering from SQLite into Application, made it total with ID, published it, and rejected out-of-order CLI projections. |
| 176 | `67dd9366` + CLI dependency update | Made equal-activity Workspace order stable by canonical path and rejected missing activity time or out-of-order CLI catalogs. |
| 177 | `b8cc7719` + CLI dependency update | Replaced Git encounter order with an Application-owned, path-ascending Workspace change order and rejected out-of-order CLI projections. |
| 178 | `2a53b6ea` + CLI dependency update | Moved MCP server name ordering from SQLite into Application, published the ascending-name contract, and rejected out-of-order CLI projections. |
| 179 | `17687307` + CLI dependency update | Moved Provider ID ordering from the LLM adapter into Application, published the ascending-ID contract, and rejected out-of-order CLI projections. |
| 180 | `8ae6ce2a` + CLI dependency update | Replaced MCP connection/upstream encounter order with an Application-owned server/name tool order and rejected out-of-order CLI projections. |
| 181 | `a0359f04` + CLI dependency update | Made Skill proposals one current revision per scope/name, moved scope/name ordering from filesystem encounter into Application, and enforced both invariants in CLI. |
| 182 | `9bd2c900` + CLI dependency update | Made discovered Skill identity the precedence-resolved name, moved name ordering out of prompt-source infrastructure, and rejected shadow leaks or out-of-order CLI catalogs. |
| 183 | `1cefd3ce` + CLI dependency update | Moved managed Skill lifecycle/name ordering from filesystem traversal into Application, enforced one lifecycle per name, and rejected out-of-order CLI catalogs. |
| 184 | `04308fcc` + CLI dependency update | Made Recipe identity the precedence-resolved name, enforced it in Runtime, moved name ordering out of prompt-source infrastructure, and rejected out-of-order CLI catalogs. |
| 185 | `9a53b39c` + CLI dependency update | Enforced unique Agent-document paths and monotonic home/project-root/cwd render phases in Runtime and rejected contradictory CLI catalogs. |
| 186 | `1c6881ca` + CLI dependency update | Made Application own per-provider Model identity/order, enforced it at the CLI boundary, and removed presentation-layer sorting. |
| 187 | `fd5704cb` + CLI dependency update | Moved direct diagnostic Tool safety, canonical unique-name identity, and name order into Application and enforced the catalog at the CLI boundary. |
| 188 | `9047eac3` + CLI dependency update | Protected resolved Hook validity/provenance, trust-root containment, and global-to-project cascade order in Runtime and at the CLI boundary. |
| 189 | `a2058135` + CLI dependency update | Made Schedule Application validate bounded, valid, unique, strictly cursor-ordered store pages and isolate returned row storage before minting continuations. |
| 190 | `94fc54a2` + CLI dependency update | Made Session CatalogRead validate store-page membership, identity, order, cursor position, and overfetch before live projection, with CLI search-filter enforcement. |
| 191 | `585e53aa` + CLI dependency update | Protected Run query filters, page scope/order/cursors/overfetch, and store-slice isolation; corrected a CLI descendant-guard false positive. |
| 192 | `9a175bcb` + CLI dependency update | Protected Item page bounds/scope/order/cursors and required an exact, valid, ownership-isolated Run-ancestor forest. |
| 193 | `7552fbaf` + CLI dependency update | Protected pending-interrupt caller capabilities and bounded, valid, scoped, unique, strictly cursor-ordered store pages. |
| 194 | `cc3b1de9` + CLI dependency update | Applied the Domain Session-catalog contract to complete reads, isolated results, and routed usage aggregation through that semantic owner. |
| 195 | `1d758c44` + CLI dependency update | Made Provider aggregates self-validating and rejected corrupt, duplicate, or identity-mismatched provider registry and supported-catalog results before projection or probing. |
| 196 | `754a4db1` + CLI dependency update | Rejected invalid, duplicate, or identity-mismatched MCP registry reads across catalogs, commands, connection settlement, and tool-policy replacement. |
| 197 | `bbb08aa3` + CLI dependency update | Made advertised MCP tools and input schemas self-validating, then enforced request scope, composite uniqueness, per-server bounds, order, and slice isolation in Application. |
| 198 | `6fea3bdf` + CLI dependency update | Closed MCP connection-status state/count invariants and replaced silent duplicate-map overwrite or presenter panic with explicit Application contract errors. |
| 199 | `44c562dd` + CLI dependency update | Protected Goal point reads and startup reconciliation from invalid, mismatched, or duplicate persistence results before any lifecycle side effect. |
| 200 | `2612b288` + CLI dependency update | Removed the redundant Goal value from CAS acknowledgements so Domain-decided replacements remain the only state authority. |
| 201 | `96fe1b57` + CLI dependency update | Validated complete pending-occurrence and due-schedule worker batches before any Run dispatch or cursor claim. |
| 202 | `923e7fda` + CLI dependency update | Protected Schedule point-read identity and removed the redundant store-owned update result. |
| 203 | `9023cb0d` + CLI dependency update | Made Schedule Run requests the sole authority for the stable Session/Run identities returned after firing. |
| 204 | `d801a7a7` + CLI dependency update | Made Domain own visible Approval-rule relations, rejected out-of-scope/duplicate rules and invalid/mismatched Session reads before authorization or listing, and isolated returned storage. |
| 205 | `b3dfbcdd` + CLI dependency update | Centralized valid exact Session point reads in the Session Application entry, migrated Run/Approval/CRUD and snapshot consumers, and rejected invalid or identity-mismatched recovery/snapshot reads. |
| 206 | `5f5370a3` + CLI dependency update | Made Domain own exact Goal Current identity, routed Reader/drive/report point reads through one validated Application path, and removed prompt-layer duplicate checks. |
| 207 | `672669d2` + CLI dependency update | Centralized valid exact Pending-interrupt point reads at the Session boundary, removed duplicate Run checks, and deleted the unused list capability from the Run port and fixtures. |
| 208 | `db5f1fb9` + CLI dependency update | Centralized valid Session-scoped, duplicate-free Pending catalogs for idle admission, executor cleanup, and pagination so foreign or corrupt rows cannot drive lifecycle effects. |
| 209 | `5b130484` + CLI dependency update | Centralized valid exact, unique, admission-ordered Session Run catalogs for activity, usage, rollback/recovery, and pagination so corrupt or foreign rows cannot affect policy or history. |
| 210 | `1868abc8` + CLI dependency update | Unified pinned and searched Agent Memory behind one Application read model that rejects invalid, inactive, foreign, duplicate, or over-capacity catalogs before model context. |
| 211 | `12c83ff8` + CLI dependency update | Made Domain own visible Agent Memory edits, blocked rejected tombstone update/delete, removed bypassing SQLite writes, and validated exact management catalogs and mutation acknowledgements. |
| 212 | `076c9352` + CLI dependency update | Normalized and validated exact bounded Agent Memory curation batches, ledger/state reads, fold inputs, and watermark transitions before model use or persistence. |
| 213 | `35447fc5` + CLI dependency update | Made Domain validate Knowledge entries and Application enforce the complete ordered cascade plus exact read/update scope and content acknowledgements before prompt, protocol, or invalidation use. |
| 214 | `ded0a709` + CLI dependency update | Completed Session Snapshot validation for Conversation and Plan values, making restore/export share one aggregate gate and removing the portable restore's duplicate Plan check. |
| 215 | `2099e61f` + CLI dependency update | Validated and ownership-isolated Plan boundary reads so contradictory unrecorded steps cannot seed forks and invalid recorded values cannot reach rollback or persistence. |
| 216 | `9a68ff21` + CLI dependency update | Added Domain row validation and bounded Application gates for discovered, managed, and proposed Skill catalogs, including exact proposal submission acknowledgements and review identity checks. |
| 217 | `ff413d95` + CLI dependency update | Bounded the complete visible Approval-rule relation across Domain authorization, one-row SQLite overfetch, generated Runtime Protocol validation, and CLI consumption. |
| 218 | `78d59b77` + CLI dependency update | Unified exact workspace inspection validation across Discovery, Session views, Knowledge, and authored observation, rejecting contradictory aliases before public projection. |
| 219 | `aceef0e3` + CLI dependency update | Replaced raw diagnostic-call JSON across Delivery, Application, and the tool adapter with the canonical Tool arguments value, and moved shared Tool metadata validation into Domain. |
| 220 | `89297db2` + CLI dependency update | Isolated claimed open-invocation snapshots and validated canonical identities, UTC start times, and journal uniqueness before they can affect boot-recovery time or planning. |
| 221 | `cc46862e` + CLI dependency update | Isolated and fully validated claimed Pending barriers, enforcing exact root/Session ownership, UTC creation time, and unique root/checkpoint identity before boot-recovery planning. |
| 222 | `b457fa3c` + CLI dependency update | Admitted complete non-terminal Run catalogs before writer leasing or planning, rejecting invalid aggregates, terminal contamination, and globally duplicated Run identities through one shared validator. |
| 223 | `d3779a1d` + CLI dependency update | Admitted each complete Session transcript before time observation or planning, rejecting invalid Items, cross-Session rows, and duplicate Item identities at the recovery read boundary. |
| 224 | `40f036d4` + CLI dependency update | Enforced the Domain/SQLite invariant of one non-terminal root tree per Session at recovery catalog admission, before duplicate roots can acquire ownership or produce conflicting plans. |
| 225 | `c0b5778e` + CLI dependency update | Enforced one lifecycle state and root-owned capabilities across every active recovery-tree member before transcript reads, resumability probes, or terminal planning. |
| 226 | `34e604fa` + CLI dependency update | Bound transcript admission to its active recovery tree so every Running Item must have a live Run owner, while completed historical Items remain valid Session history. |
| 227 | `2a6d6ecf` + CLI dependency update | Bound every claimed open invocation that still names an active Run to that Run's exact Session and active Segment before recovery planning, while retaining cleanup of orphan rows for already-terminal Runs. |
| 228 | `0d634992` + CLI dependency update | Required every open Tool invocation still owned by an active Run to resolve its exact Running ToolCall Item before recovery planning; terminal-Run journal orphans remain cleanup-only records. |
| 229 | `05443025` + CLI dependency update | Bound every model and Tool invocation mutation in RecoveryCommit to the exact recovered Session ownership set, including cleanup-only rows whose Runs were already terminal. |
| 230 | `fa0f957a` + CLI dependency update | Made checkpoint-deletion Sessions the exact canonical projection of recovery-lost root trees, rejecting omissions, foreign deletion scope, and multiple lost roots for one Session. |
| 231 | `23aa1f84` + CLI dependency update | Deleted the writable recovered-Session duplicate and unconsumed preserved-checkpoint marker; callback cleanup now derives its exact Session scope from lost roots plus preserved waiting Sessions. |
| 232 | `f759e265` + CLI dependency update | Required each lost-Run Tool journal recovery to carry the matching Running Item replacement, while retaining cleanup-only journal orphans for already-terminal Runs. |
| 233 | `1ab79caf` + CLI dependency update | Replaced terminal-only lost-Run commands with exact expected→replacement aggregates across boot recovery and parked Session termination, binding invocation cleanup to the recovered active Segment and giving SQLite an exact pre-transition CAS fence. |
| 234 | `addb8d3c` + CLI dependency update | Removed the remaining terminal-only non-Event Run write path; waiting-subtree cancellation and parked Session termination now carry validated exact expected→replacement aggregates into SQLite CAS, while Event termination retains its Segment/commit fence. |
| 235 | `3188bad3` + CLI dependency update | Collapsed the two Run suspension entrypoints; every tree-barrier member is now read- and write-fenced to its exact active Segment, while the root additionally records the barrier commit receipt. |
| 236 | `3bf46d43` + CLI dependency update | Deleted the compaction-specific expected/replacement Run pair and routed Application plans, persistence validation, and SQLite watermark CAS through the canonical Run replacement value. |
| 237 | `8d646633` + CLI dependency update | Replaced the mutable Application Item pair and two-argument transcript CAS with one validated Domain-owned Item replacement value across recovery, waiting cancellation, questions, approvals, and SQLite. |
| 238 | `5be75684` + CLI dependency update | Replaced duplicate Application Session revision pairs and two-argument saves with one Domain-owned replacement that rejects cross-identity, skipped-revision, and time-regressing writes before SQLite CAS. |
| 239 | `174154b6` + CLI dependency update | Moved the duplicate Application Plan version/state pair into the Plan Domain and kept the validated optional-version advance intact through Session write-sets and SQLite CAS. |
| 240 | `11917559` + CLI dependency update | Replaced split Schedule state/revision updates with one Domain-owned management replacement that preserves identity, creation time, and the accepted-Run cursor through SQLite CAS. |
| 241 | `f3f59d2f` + CLI dependency update | Replaced split Goal state/version writes with one Domain-owned exact replacement across lifecycle, recovery, terminal accounting, invalidation, and SQLite CAS. |
| 242 | `0e7dc8ce` + CLI dependency update | Replaced the five-part Agent Memory generation call with one Domain-owned publication that binds target, expected and successor states, canonical contents, and monotonic time through SQLite CAS. |
| 243 | `772eab24` + CLI dependency update | Bound each Knowledge file CAS precondition and replacement body into one Domain-owned value while preserving workspace routing and public protocol input at their existing owners. |
| 244 | `7b02affc` + CLI dependency update | Made the conversation compaction write-set own its valid Session, isolated coordinate rewrite, and unique same-Session Run replacement set before persistence CAS. |
| 245 | `f76ff768` + CLI dependency update | Constructed Session rollback writes from one Domain boundary, admitted only the exact unknown message sentinel, and bound unique Run/checkpoint identities before side effects or persistence. |
| 246 | `d0a87b21` + CLI dependency update | Constructed Session fork writes from one normalized, ownership-isolated child projection and bound its exact parent, initial revision, parent-first Runs, and matching initial Plan replacement before persistence. |
| 247 | `4330c36b` + CLI dependency update | Constructed complete Session restore writes before process-local teardown, bound the committed Session replacement to one normalized isolated projection, and required its Plan transition to publish exactly the restored steps. |
| 248 | `5ecc5324` + CLI dependency update | Replaced the raw Session delete DTO with one canonical owned identity and rejected malformed deletion before admission, interrupt reads, process quiescing, or persistence. |
| 249 | `9cdd8fbf` + CLI dependency update | Encapsulated parked-Run terminal writes behind validated ordinary/claimed constructors, isolated every returned projection, and derived Goal accounting only from the terminal root Run. |
| 250 | `9acc010a` + CLI dependency update | Constructed Resume claims before their linearization point, deep-isolated Pending and answer projections, and exposed only owned snapshots while preserving exact consumed-state comparison. |
| 251 | `ace68a7c` + CLI dependency update | Encapsulated the tree-barrier write-set behind one validating constructor and returned deep-isolated Pending, Run-commit, message, and executor-checkpoint projections. |
| 252 | `d654dbcc` + CLI dependency update | Replaced mutable opening fields with validated admission/resume constructors, isolated every returned projection, and repaired two SQLite tests that had been short-circuited before their intended transactional failures. |
| 253 | `df3efe06` + CLI dependency update | Encapsulated waiting-subtree cancellation behind parked/resuming constructors, deep-isolated Pending, checkpoint, message, resume, opening-event, and terminal projections, and moved contradictory write-set rejection before persistence. |
| 254 | `5425b2c8` + CLI dependency update | Made authoritative execution facts private and exhaustively cloned at receipt construction and access, rejected reducer-unsupported representations early, and covered mutable model, Tool, interrupt, usage, and steer projections under race. |
| 255 | `fede4925` + CLI dependency update | Encapsulated recovery write-sets behind one validating constructor and isolated accessors, kept planner mutation private, and proved storage-side projection mutation cannot diverge committed facts from published Run, interrupt, Session, or Goal invalidations. |
| 256 | `df85ac68` + CLI dependency update | Encapsulated executor tree-interruption snapshots behind one validating constructor, deep-isolated checkpoint and nested interruption projections, and migrated the full asynchronous barrier handoff to owned accessors. |
| 257 | `c4540950` + CLI dependency update | Collapsed unknown-Effect reporting to one constructed control fact, kept executor-only Effect identities out of Application, and removed sorting, deduplication, mutable catalog state, and tests for unused diagnostics. |
| 258 | `980e995d` + CLI dependency update | Encapsulated final assistant confirmations behind role, message, and correlation validation; deep-isolated construction and access; and migrated both root raw delivery and child authoritative delivery. |
| 259 | `6702019d` + CLI dependency update | Made terminal Failure and cumulative per-model Usage constructor-owned, exposed only isolated projections, and confined executor outcome assembly to a private draft before one final SegmentEnded snapshot. |
| 260 | `49870903` + CLI dependency update | Froze waiting continuations and subtree-cancellation requests at Application/executor boundaries, deep-isolated member, checkpoint, and capability projections, and made every restore, resume, and recovery-probe entry take its own snapshot. |
| 261 | `1d50ba99` + CLI dependency update | Encapsulated prepared waiting-subtree results behind one validating constructor, isolated every returned tree projection, and made the value itself own Apply, Continue, and Discard instead of exposing its one-shot executor capability. |
| 262 | `78f89307` + CLI dependency update | Removed the duplicate Start input materializer and unused root Message/Media projections, derived prompt text from the one canonical user message, and deep-froze working context, model options, and interrupt capabilities at StageRoot. |

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

- Continue after Round 177 by auditing complete Runtime catalogs and CLI
  projections for aggregate invariants that element-level wire validation cannot
  express.
- Treat missing output-resource identity constraints as candidates only after the
  owning use case, protocol projection, and every in-scope consumer prove the
  required shape; do not add speculative validation.
- Keep this log compact. Add at most one concise row per completed round, then
  periodically consolidate rows into a themed range. Do not paste full command
  output, repeated validation boilerplate, credentials, or per-file narration.
