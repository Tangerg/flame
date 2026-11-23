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
