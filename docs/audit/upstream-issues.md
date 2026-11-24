# Upstream framework issues

An entry identifies a reproducible problem in a framework contract or implementation, the versions exercised, and the smallest framework change that would address it. Flame owns its product policies, including Session and Run lifecycle, persisted history, receipts, accounting, and presentation. An integration defect in those policies belongs in Flame.

## Ownership

| Project | Framework responsibility |
| --- | --- |
| Scope | AI contracts, provider protocols, and durable agent execution. [Scope boundaries](https://github.com/Tangerg/scope/blob/2c1a6afbdaec39fcdefe76b8859bde60cced9d65/README.md#boundaries) |
| Dougong | TypeScript plugin composition, atomic host publication, resource lifetimes, and observation. [Dougong packages](https://github.com/Tangerg/dougong/blob/85ca855ada3709484bd8984d21cbe0b17108a91a/README.md) |
| Oolong | Terminal rendering, input, scene identity, streaming content, and output admission. [Oolong architecture](https://github.com/Tangerg/oolong/blob/4d49204c89a66adac6c0767457b41e139e6c7ebf/README.md#build-from-layers) |
| Frame | Plugin contracts, frozen service bindings, invocation evidence, and managed resource lifetimes. [Frame responsibilities](https://github.com/Tangerg/frame/blob/a649f2a498f0e706ce57ab092b60d71292491eb0/README.md) |

Framework changes use framework-native types and standalone fixtures. Flame translates those contracts into its product model and verifies the integration against released modules.

## Frame browser control over HTTP/1.1 (Frame v0.5.0)

Browsers open at most six HTTP/1.1 connections per origin, and the Realm's control requests share that pool with every other Runtime request.

- **Idle polling.** `ControlPump` only short-polls: each `POST /v2/frame` returns at once, so an idle Realm polls at its host-chosen interval for as long as it lives (100 ms in Flame, about ten requests per second with no work). The smallest framework change is a poll that waits, up to a bounded duration, for pending plans or fences.
- **Acquisition depends on the caller's own poll.** `Bridge.AcquirePlan` returns only after the browser pump has prepared the plan, so every pending acquisition holds a connection while it waits for another request from the same page. Under the connection limit, concurrent acquisitions starved the pump and stalled for about 80 s until the client's health timeout tore the connection down (reproduced 2026-11-24). Flame admits one acquisition per Realm at a time (`apps/web/src/features/plugins/adapters/componentModules.ts`). The smallest framework change is to deliver the grant through the control channel, or to state the one-at-a-time requirement in the browser contract.

## Frame browser Realms that end without a controlled close (Frame v0.5.0)

A browser Realm's executions and Uses are released only by the original page's `CloseProof`, which needs the execution, scope and instance identities that live in that page's Frame `Runtime`. A page that reloads, closes or crashes never sends them, and no successor document can construct them. `ResumeRealm` grants cleanup authority only to a page that still holds those objects.

- **Recovery barrier.** `sqlite` raises the admission barrier at open while any attempt or Use is unsettled, and the Frame document example keeps it closed until every Realm proves its exits. Flame previously copied that policy, so one ordinary reload followed by any Runtime restart withheld every plugin and stalled Run recovery permanently (reproduced 2026-11-24). Flame now clears the barrier after Process recovery and its own binding settlement, because a Realm from an earlier authority epoch is fenced to cleanup and cannot admit work (`runtime/internal/bootstrap/framework.go`).
- **Retained protection.** The abandoned attempts and Uses stay unsettled in the store, so `ExecutionDependencies` keeps their release content retained indefinitely.
- **Shutdown wait.** Until the next restart the Bridge still holds the abandoned Realm's executions, so Runtime shutdown waits for exits that never arrive and fails with `context deadline exceeded` after its 10 s shutdown timeout.

The smallest framework change is an explicit Realm-loss settlement: an authenticated, idempotent request through which the trusted host or a successor document holding the Realm's resume credential attests that the Realm's JavaScript heap no longer exists, after which the Bridge releases that Realm's executions and Uses with a distinct abandonment exit kind.
