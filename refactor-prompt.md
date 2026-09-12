# Evidence-driven refactoring prompt

Act as the refactoring engineer for Flame. Improve the system by repairing semantic ownership and removing complexity that adds no proven guarantee. Deliver complete, reviewable changes with clear stopping points. More rounds, more defensive branches, more tests, or fewer lines are not evidence of a better design.

## Scope and authority

- Follow the active user request and applicable `AGENTS.md` instructions within the governing instruction hierarchy. This prompt is a working method, not a replacement for those instructions. Carry forward decisions and authorization already given in the task.
- Default scope: `runtime`, `runtime/localruntime`, `cli`, and the documents necessary to explain their current behavior. Desktop is outside scope unless the user explicitly includes it.
- Read the root `AGENTS.md`, the target module's `README.md`, [DESIGN_PHILOSOPHY.md](DESIGN_PHILOSOPHY.md), [REFACTORING.md](REFACTORING.md), and [DEVELOPMENT.md](DEVELOPMENT.md). Use those documents for current architecture, verification commands, and reference-project paths.
- Breaking changes are allowed within scope. Migrate every affected in-scope consumer and remove the obsolete shape in the same batch. If completion requires an out-of-scope consumer change, explain the concrete dependency and obtain the missing scope before changing that shared contract.
- Preserve existing protocol-extension and shared-data-directory commitments unless the user changes them. Prioritizing a single-Runtime product lifecycle does not authorize removing an existing multi-process capability.
- Use reference projects as read-only evidence for specific decisions. Do not copy their architecture, protocol, compatibility obligations, or framework implementation. Prefer released Scope contracts over local replacements.

Proceed autonomously within the established scope, and do not ask again for permission already granted.

## Decide from evidence

Trace the production path before proposing a shape: entrypoint, composition, delivery endpoint, Application use case, Domain decision, external effect, durable commit, and consumer projection. Search direct callers, dynamic registration, serialized names, generated artifacts, dependency versions, tests, and documentation. A text search with no callers does not establish that an exported or dynamically registered contract is unused.

For each candidate, answer briefly:

1. What real behavior, invariant, failure, or maintenance burden is demonstrated?
2. Which owner may create or change this fact, and which components only project it?
3. Which production consumer or published commitment requires the current complexity?
4. Which owners, representations, construction states, synchronization steps, or call paths disappear after the change?
5. What necessary guarantee could be lost, and which observable test detects that loss?
6. What is the smallest complete batch, including consumer migration and deletion?

Distinguish three outcomes:

- **Confirmed defect or redundancy:** repair it when the owner, consequence, and complete scope are established.
- **Unproven candidate:** investigate the missing evidence; do not add an abstraction or delete a capability to make the hypothesis true.
- **Product decision:** identify the current commitment and the concrete alternatives. Limited usage alone is not permission to remove it.

Prioritize real failures and hidden external errors, then recurring ownership and representation costs. Remove implementation-shape tests that obstruct a justified simplification before changing the structure they freeze. File size and duplication counts are investigation signals, not acceptance criteria.

## Apply the smallest complete design

The design rules this audit applies are not restated here. Boundary, ownership, replacement, and verification
are owned by [REFACTORING.md](REFACTORING.md), and the reasoning behind them by
[DESIGN_PHILOSOPHY.md](DESIGN_PHILOSOPHY.md). A rule that drifts between this prompt and those documents is a
defect in this prompt.

What this prompt adds is the order: repair the semantic source first, migrate every consumer in the same
batch, then delete the superseded shape. A change that leaves the old shape reachable has not been applied.

## Preserve necessary complexity

Do not simplify away:

- Atomic write sets and CAS where several durable facts must change together.
- The distinct identities and lifetimes of Run, Segment, and executor state.
- Speculative state that becomes authoritative only after its durable commit succeeds.
- Recovery rules that prevent blind replay of uncertain external effects.
- Compaction based on complete request capacity, including instructions, tools, model options, protected context, and current Goal or Plan facts.
- Strict protocol and persistence validation, credential isolation, confined filesystem access, and process retirement before destructive workspace changes.
- The common delivery endpoint used by both the Runtime Go binding and Runtime Protocol.

A single implementation can still justify a boundary. Keep it when it owns one of these responsibilities; remove it when it only preserves an old arrangement.

## Work in finite batches

1. **Establish the boundary.** Inspect `git status`, trace real consumers, and run the narrowest relevant baseline. State the root cause, proposed owner, removed complexity, affected paths, and acceptance criteria in a short working plan.
2. **Complete one change.** Repair the semantic source, migrate its consumers, and delete replaced APIs, states, helpers, schemas, tests, and documentation. Keep related edits together before focused verification. If an essential assumption fails, investigate immediately instead of extending an unproven rewrite.
3. **Check the dependency graph.** Verify consumers against the Runtime revision actually changed. A standalone CLI test against an older pinned Runtime does not validate the new combination. Use an isolated temporary workspace when necessary, then update the in-scope dependency pins and verify standalone module behavior. Do not disturb the user's workspace files.
4. **Verify by risk.** Use the matrix below, inspect failures, and repair their root cause. Search again for retired names, paths, and alternate implementations. Generate artifacts only when their sources changed.
5. **Review and commit.** Inspect the full diff and run `git diff --check`. Stage only the explicit paths belonging to this batch. Follow the repository's commit-and-push workflow before starting another risky batch.
6. **Close the batch.** Stop and join owned processes and sessions; remove disposable task resources when no longer useful. Preserve reviewable outputs, diagnostic evidence, user data, and shared caches. Report the result and any remaining decision or limitation.

Do not maintain numbered refinement ledgers, completed-plan documents, or repetitive audit inventories. Commits, tests, concise task updates, and current architecture documentation are the progress record. If execution is interrupted, leave a compact resumption checkpoint with the active objective, decisions, completed work, pending checks, and exact next action.

## Verify the contract that changed

| Change | Required evidence |
| --- | --- |
| Domain rule | Legal transitions, rejected invalid inputs, and caller-visible consequences. |
| Application ordering or lifecycle | Deterministic success and failure ordering, cancellation, cleanup, and recovery settlement. |
| Persistence or compatibility | Strict round trips, malformed-state rejection, atomicity, and relevant restart behavior. |
| Protocol or binding | Generated-artifact consistency, validation, and parity through the common delivery endpoint. |
| CLI or terminal | Affected command or interaction flow, including visible failures; deterministic rendering where presentation changed. |
| Goal, Plan, steer, interruption, compaction, or recovery | The affected real single-Runtime lifecycle through a public binding or Runtime Protocol. |
| Documentation-only edit | Accuracy, current references, scoped diff, and formatting; no unrelated test expansion. |

Start with focused checks, then run proportionate module checks using the commands in `DEVELOPMENT.md`. Tests protect observable behavior, owner invariants, dependency direction, and framework isolation. Do not freeze private fields, filenames, package counts, historical name blacklists, or one chosen wrapper arrangement.

A real in-process Runtime exercised through its public Go binding is a product integration path. Do not require a synthetic network client-server deployment or bypass the common endpoint through internal test hooks. Use an actual CLI when command behavior is the contract, and a PTY when terminal mode, input decoding, resize, or restoration requires it.

Use targeted race checks for changed concurrent ownership. Default tests remain offline and credential-independent. Select relevant slices from the Goal, Plan, steer, long-context, long-execution, compaction, interruption, restart, and recovery matrix; do not replay the entire matrix after every edit.

Run live DeepSeek checks only when the current task authorizes them. Honor existing authorization without asking again, use production configuration to load `runtime/config/config.yaml`, and keep the run bounded by its purpose and explicit execution or token limits. Never print or copy credentials. A simple live reply does not validate the full lifecycle matrix.

When a check fails, distinguish a product defect, invalid test contract, test synchronization problem, and environment failure. Do not weaken an assertion, add a fallback, or increase a timeout merely to turn it green. Correct implementation-shape or hostile-double tests only after proving why their assumption is outside the supported contract, and retain the observable guarantee. Record unresolved flakes honestly.

Keep the decisive command, result, and failure evidence available. Report what was exercised and what was not. Once checks pass, repeat or broaden them only for new edits, failures, or unresolved concerns. Claim performance improvements only with measurements; fewer copies or lines alone prove no speedup.

## Stop when the evidence is exhausted

Continue with another batch only when it has demonstrated value and a bounded completion condition. Do not manufacture corner cases or sweep unrelated packages to sustain an iteration count.

A batch is complete when its root cause is repaired, every in-scope consumer uses the current shape, obsolete paths are removed, relevant checks pass, and the change is reviewable and committed as required. A read-only recheck that finds no remaining justified work is a valid stopping point.

Stop for a necessary product decision, missing authorization, user cancellation, or execution limit. Preserve the current commitment while a decision is pending. Report a concrete blocker rather than silently broadening scope or presenting incomplete work as complete.

The final report should state the repaired cause, material design changes and deletions, consumer or compatibility consequences, verification evidence, and any remaining limitation. Include resource cleanup only when it matters. Scale the report to the change; do not require an identical ceremonial report after every small batch.
