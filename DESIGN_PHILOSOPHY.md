# Flame design philosophy

Flame is one product with one durable model. Runtime owns product semantics and exposes them through an in-process Go binding and the Runtime Protocol. CLI and Desktop own presentation and interaction. Scope owns the agent framework and provider libraries.

## One fact, one owner

Session, Run, Segment, Item, Goal, Plan, Interrupt, execution, persistence, recovery, provider selection, and compaction advance only in Runtime. Storage records, protocol values, CLI state, and Desktop state are projections of those facts.

When a fact appears in several representations, identify the representation that may change it. Every other representation may encode, cache, or render it, but cannot create a competing transition.

## Repair the semantic source

A repair is complete only when the invalid state can no longer be produced. Fix the owner that admits or creates the state, migrate every in-scope consumer, and remove the former API, schema, fallback, test, and documentation in the same change.

Breaking changes are preferred to compatibility around a wrong design. This does not permit arbitrary churn: the new design must reduce the number of owners, representations, states, dependencies, or call paths.

## DDD without ceremony

Domain models own identity, invariants, legal transitions, and pure policy. Application code owns use-case ordering, transactions, cancellation, and cross-aggregate consistency. Adapters translate external systems. Delivery translates bindings and user interaction. Bootstrap constructs and shuts down the concrete graph.

These boundaries express responsibility, not a required directory matrix. A package exists only when it owns a coherent vocabulary, aggregate invariant, workflow lifecycle, external translation, or reusable technical mechanism. Related responsibilities remain as named files in one package when splitting them would add only imports and forwarding APIs.

Interfaces belong to consumers. Start with concrete types and extract the smallest interface only to reverse a dependency, isolate an external boundary, or support proven substitution.

## One execution path

The Go binding and Runtime Protocol enter the same delivery endpoint before capability checks, idempotency, lifecycle control, Application invocation, and result projection. Runtime does not duplicate Scope's process loop, tool loop, scheduler, provider behavior, or checkpoint interpretation.

CLI commands and the terminal client consume Runtime through narrow ports. They may own drafts, selection, prompt history, queue intent, focus, rendering, and other local interaction state. They do not reconstruct durable Runtime state machines.

## Explicit composition

Construction, ownership transfer, cancellation, joining, and shutdown are explicit. Flame does not use ambient globals, service locators, dependency-injection containers, reflection registries, optional service bags, or parallel lifecycle roots.

Every goroutine and resource has one owner, a stop condition, and a join or close path. Durable state is the recovery source; process-local notifications only wake consumers so they can reread it.

## Evidence over symmetry

Architecture is judged by real consumers, invariants, failure ordering, protocol behavior, persistence, and lifecycle tests. Package counts, file counts, private field inventories, and symmetric trees are not design evidence.

Use the standard library first, then existing mature dependencies. Keep a wrapper only when it owns policy, translation, lifecycle, or authority. Measure before optimizing.
