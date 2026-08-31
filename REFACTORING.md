# Flame refactoring guide

This guide turns [`DESIGN_PHILOSOPHY.md`](DESIGN_PHILOSOPHY.md) into a repeatable structural method for `cli` and `runtime` changes. Repository execution and verification details live in [`DEVELOPMENT.md`](DEVELOPMENT.md).

## Establish the contract

Before editing:

1. Read the root and nearest module `AGENTS.md`
2. Inspect `git status` and identify user-owned changes
3. Read the owning architecture, decision, protocol, and persistence documents
4. Trace the real entrypoint through composition, use case, domain owner, adapter, storage, and projection
5. Search every caller, serialized name, dynamic registration, generated artifact, test, and document in the blast radius
6. Run the smallest useful baseline check and record pre-existing failures

Do not infer safety from a symbol search alone. Reflection, Cobra command registration, protocol registries, generated contracts, SQLite schemas, configuration keys, and terminal keymaps can make a path reachable without a direct call.

## Classify the problem

Classify a finding before choosing a fix:

- **Ownership**: behavior lives outside the type or use case that owns it
- **Representation**: one fact has several states, names, or encodings
- **Boundary**: an SDK, protocol, storage, or UI type leaks across layers
- **Package**: a package adds navigation and forwarding without an independent responsibility
- **Lifecycle**: several objects can start, stop, settle, or publish one resource
- **Transaction**: a use case can expose partial state or apply an external change before its durable winner
- **Protocol**: bindings disagree, validation is duplicated, or an internal concept appears on the wire
- **Provider**: identity, credential precedence, endpoint, capability, or SDK mapping is guessed downstream
- **Presentation**: terminal state, focus, layout, or input policy is scattered across render paths
- **Test debt**: tests assert implementation shape, speculative concurrency, or weak proxies instead of the product contract

Identify the source of truth and every symptom site. Change the source first. Delete symptom handling after the source cannot produce the invalid state.

## Prove simplification candidates

For a package, interface, wrapper, cache, state flag, or alternate API:

1. Find production consumers and dynamic entrypoints
2. Check exported or external compatibility obligations
3. Read the history that introduced it
4. State the invariant or lifecycle it owns
5. State the capability lost if it is removed
6. Estimate the concepts and synchronization paths removed, not only lines
7. Name the smallest test that would fail if the cut were wrong

Keep a candidate when it owns a real boundary. Delete it when the only rationale is symmetry, forwarding, test convenience, former behavior, or hypothetical substitution.

## Refactor toward rich owners

Move a rule onto a domain type when the rule describes that type's valid identity, transition, or derived meaning. Keep a free function when it constructs a value, symmetrically combines several owners, or implements a codec with no honest receiver.

Move cross-aggregate ordering into an Application use case. Keep provider calls, process execution, filesystem operations, and SQL in adapters or Infra. Do not make a model “rich” by giving it I/O.

Replace primitive bags with closed values when invalid combinations currently reach multiple callers. Prefer one constructor or parser and behavior methods over public fields plus `Validate` calls that every caller must remember.

## Shape package structure deliberately

Prefer multiple cohesive files inside one package. Merge a package into its only real owner when it has no independent vocabulary, invariant, lifecycle, external boundary, or reusable mechanism.

When a ring has become a flat catalog of independently justified packages, group them under one proven bounded-context or capability-family namespace. The parent directory contains no Go facade, the leaf packages keep their responsibilities, and shared mechanisms remain direct rather than being assigned to an arbitrary consumer. Reject single-child namespaces and nesting deeper than `ring/context/package` unless a language-enforced mechanism such as Go `internal` requires it.

Do not merge solely because a package has one consumer. A strict codec, confined filesystem capability, protocol adapter, or rich value can have one consumer and still protect a meaningful boundary.

After a merge, delete the old import path, forwarding API, mocks, architecture exceptions, and documentation. Do not create a new umbrella package to hide the same concepts.

## Replace a contract completely

For an approved breaking change:

1. Change the semantic owner
2. Change Application callers and adapters
3. Change storage and protocol encodings if the semantic shape crosses them
4. Regenerate owned artifacts
5. Migrate CLI consumers in the same batch when they are in scope
6. Delete the old name, field, method, codec, schema, fixture, and documentation
7. Search for every retired symbol and serialized spelling

Do not keep aliases, deprecated wrappers, fallback readers, dual writes, migrations, or `v2` packages unless the user explicitly requires compatibility.

## Upgrade dependencies before architecture work

For each module in scope:

1. List available updates and read upstream breaking notes for direct dependencies
2. Upgrade Scope modules to the latest published coherent release set
3. Upgrade other direct dependencies, then let `go mod tidy` choose indirect versions
4. Remove obsolete adapters and workarounds exposed by the new APIs
5. Run standalone module tests with `GOWORK=off` in addition to workspace tests
6. Commit the dependency baseline separately from behavioral refactors

Do not add `replace` directives for a published dependency. A local reference checkout is evidence, not the module source used by Flame.

## Test by risk

Choose the smallest combination that proves the changed contract:

| Change | Required evidence |
| --- | --- |
| Domain invariant | Table-driven owner tests and caller-visible rejection |
| Application transaction | Use-case test with exact durable winner and failure ordering |
| Storage shape | Strict round trip, malformed state rejection, and schema baseline |
| Protocol shape | Registry, generated artifact, strict validation, and binding parity |
| Provider mapping | Adapter contract plus a bounded live probe when credentials exist |
| Goal, Plan, steer, HITL, compaction, recovery | Real Runtime end-to-end scenario |
| TUI layout or input | Deterministic application/render test; real PTY for terminal behavior |
| Goroutine, stream, cancellation, shutdown | Deterministic lifecycle test and targeted race run |
| Package or interface simplification | Architecture guard and surviving consumer tests |

Do not add race, fuzz, multi-client, multi-server, or cross-platform matrices unless the changed boundary makes them relevant. A broad green suite cannot prove an incorrect owner.

## Run live DeepSeek scenarios safely

The development configuration at `runtime/config/config.yaml` is authorized for real tests. Treat its key as a credential even though live use is permitted.

- Load the file through production configuration code
- Do not print the config, environment, request headers, or raw key
- Bound prompts, maximum output, and retry counts
- Keep the default offline suite independent of the credential
- Cover real flows such as Tool calls, Goal continuation, Plan updates, steer, compaction, long context, cancellation, and recovery
- Record model identity, scenario, outcome, and duration without recording secrets or full private prompts

## Keep batches recoverable

One commit should repair one ownership boundary or establish one independent baseline. Before committing:

1. Run the narrow decisive test
2. Run affected module tests, vet, build, tidy, generation, and architecture checks
3. Run `git diff --check`
4. Inspect the full staged diff
5. Verify no `desktop` path is staged
6. Update stable owner documentation; leave command logs to Git
7. Commit with the root cause in the subject and push

If verification fails because the scheduled cache cleanup removed artifacts, rebuild and continue. If the baseline was already red, record that fact and do not claim the batch caused or fixed it without evidence.
