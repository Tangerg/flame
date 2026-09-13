# Flame design philosophy

Flame is one product with one durable model. Runtime owns product semantics and exposes them through an in-process Go binding and the Runtime Protocol. CLI and Desktop own presentation and interaction. Scope owns the agent framework and provider libraries.

## Why Runtime owns the durable facts

[`AGENTS.md`](AGENTS.md) states the general rule, and [`PROJECT_RULES.md`](PROJECT_RULES.md) names the exact facts Runtime owns. What that ownership buys is the reason to keep it.

A product with three surfaces — an in-process binding, a protocol, and a desktop client — can give each surface its own copy of a Run's state and reconcile them later. Flame does not, because reconciliation is where the cost lands: every added surface multiplies the pairs that can disagree, and a crash during disagreement leaves no representation that can be trusted to say what happened.

Concentrating the transitions in Runtime moves that cost once, into one state machine that persistence and recovery already have to be correct about. The surfaces keep only what they can rebuild by reading: selection, focus, drafts, rendering. Losing a surface then costs nothing a restart cannot restore.

## Repair the semantic source

A repair is complete only when the invalid state can no longer be produced. Fix the owner that admits or creates the state, migrate every in-scope consumer, and remove the former API, schema, fallback, test, and documentation in the same change.

Breaking changes are preferred to compatibility around a wrong design. This does not permit arbitrary churn: the new design must reduce the number of owners, representations, states, dependencies, or call paths.

## DDD without ceremony

Domain models own identity, invariants, legal transitions, and pure policy. Application code owns use-case ordering, transactions, cancellation, and cross-aggregate consistency. Adapters translate external systems. Delivery translates bindings and user interaction. Bootstrap constructs and shuts down the concrete graph.

These boundaries express responsibility, not a required directory matrix. A package exists only when it owns a coherent vocabulary, aggregate invariant, workflow lifecycle, external translation, or reusable technical mechanism. Related responsibilities remain as named files in one package when splitting them would add only imports and forwarding APIs.

Interfaces belong to consumers. Start with concrete types and extract the smallest interface only to reverse a dependency, isolate an external boundary, or support proven substitution.

## Behavior belongs with the fact

Prefer a behavior-rich aggregate or value object when a fact has identity, legal states, validation, or transitions. The owner validates construction, keeps mutation private, answers domain questions, and performs pure transitions through intention-revealing methods. Application code asks the owner to decide; it does not reproduce the same rules with conditionals over exported fields.

Rich does not mean that every struct needs methods. Protocol payloads, persistence records, provider requests and responses, configuration inputs, and rendering projections are data at their boundaries. They should be strict and typed, but they do not acquire fake domain behavior. I/O, clocks, cancellation, retries, transactions, and cross-aggregate ordering stay outside Domain objects.

Encapsulation is successful when invalid intermediate states disappear and callers need fewer facts to make a decision. It is not successful when getters merely hide fields, a generic manager forwards every method, or one aggregate absorbs unrelated workflows.

## One execution path

The Go binding and Runtime Protocol enter the same delivery endpoint before capability checks, idempotency, lifecycle control, Application invocation, and result projection. Runtime does not duplicate Scope's process loop, tool loop, scheduler, provider behavior, or checkpoint interpretation.

CLI commands and the terminal client consume Runtime through narrow ports. They may own drafts, selection, prompt history, queue intent, focus, rendering, and other local interaction state. They do not reconstruct durable Runtime state machines.

## Explicit composition

Construction, ownership transfer, cancellation, joining, and shutdown are explicit. Flame does not use ambient globals, service locators, dependency-injection containers, reflection registries, optional service bags, or parallel lifecycle roots.

Every goroutine and resource has one owner, a stop condition, and a join or close path. Durable state is the recovery source; process-local notifications only wake consumers so they can reread it.

## Evidence over symmetry

Architecture is judged by real consumers, invariants, failure ordering, protocol behavior, persistence, and lifecycle tests. Package counts, file counts, private field inventories, and symmetric trees are not design evidence.

Use the current module Go version and standard library first, then existing mature dependencies. Keep a wrapper only when it owns policy, translation, lifecycle, or authority. Measure before optimizing.
