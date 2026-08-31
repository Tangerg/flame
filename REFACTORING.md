# Flame refactoring guide

This guide applies [`DESIGN_PHILOSOPHY.md`](DESIGN_PHILOSOPHY.md) to structural changes in Runtime and CLI. Execution details live in [`DEVELOPMENT.md`](DEVELOPMENT.md).

## Establish the boundary

Before editing:

1. Read the root and nearest module `AGENTS.md`.
2. Inspect the worktree and preserve unrelated changes.
3. Trace the production entrypoint through composition, delivery, Application, Domain, adapters, persistence, and projection.
4. Search direct callers, dynamic registration, serialized names, generated artifacts, tests, and documentation.
5. Run the narrowest baseline that exercises the boundary.

Classify the problem as ownership, representation, dependency, lifecycle, transaction, protocol, persistence, provider translation, presentation, or test debt. Name the current source of truth before choosing a new shape.

## Prove a structural change

For every package, interface, wrapper, state flag, cache, or alternate API under consideration, answer:

- Which production consumer uses it?
- Which invariant, lifecycle, translation, or authority does it own?
- What externally visible capability disappears if it is removed?
- Does the replacement reduce concepts, call paths, or synchronization?
- Which focused test would fail if the change were wrong?

Merge ownerless or forwarding-only packages into their real owner. Keep strict codecs, confined filesystem capabilities, protocol boundaries, and rich aggregates when they protect a genuine responsibility, even with one implementation.

## Change the owner first

Move an invariant onto the Domain type that owns the value or transition. Move I/O ordering and cross-aggregate consistency into an Application use case. Keep SDKs, SQL, filesystems, processes, transports, and terminal frameworks outside those inner policies.

Do not introduce a wrapper to break a cycle or preserve the former shape. Reconsider the dependency direction and move the consumer interface to the consuming package.

## Replace completely

An approved breaking change must update the semantic owner, every in-scope caller, persistence and protocol encodings when affected, generated artifacts, tests, and current documentation. Delete former names, aliases, fallback readers, dual writes, old packages, and empty directories in the same batch.

Prefer responsibility-named files inside a cohesive package. Use a context namespace only when it contains several proven peer packages. Do not add facade parents, single-child namespaces, or packages named after generic roles.

## Verify by risk

- Domain changes require owner tests and caller-visible rejection.
- Application changes require exact success and failure ordering.
- Persistence changes require strict round trips and malformed-state rejection.
- Protocol changes require generation, validation, and binding parity.
- CLI changes require fresh-root command tests and captured streams.
- Terminal changes require deterministic state/render tests; use a PTY only for terminal behavior.
- Lifecycle changes require deterministic cancellation and shutdown tests.
- Structural changes require surviving consumer tests and dependency-direction checks.

After each batch, search for retired symbols and paths, run the focused check, run proportionate module checks, inspect the complete diff, and verify that no unrelated path is staged.
