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

## Scope retries inside model calls (Scope v0.64.0)

A model created with `NewMessages` sent three HTTP requests during one call
against a loopback endpoint that always returned HTTP 503 (reproduced 2026-11-26
with `anthropic-sdk-go/v2` v2.12.0). The native and compatible bindings share
the same Anthropic protocol client. It inherits the SDK's two retries and its
Messages configuration provides no retry setting. Flame records evidence around
one Scope model call, so those additional requests have no separate attempts in
the Run history. [Anthropic protocol client](https://github.com/Tangerg/scope/blob/da8f279591bdb46459dcae26b15ca0b76de083f1/models/protocol/anthropic/api.go)

Loopback probes on 2026-11-27 observed three HTTP requests per call on both
429 and 503 for DeepSeek Chat Completions, Perplexity Responses and OpenAI
embeddings. OpenAI Responses with `MaxRetries` set to zero sent one request.
The DeepSeek and Perplexity builders omit the protocol's retry limit, and the
OpenAI embedding binding has no field for it. [DeepSeek chat builder](https://github.com/Tangerg/scope/blob/da8f279591bdb46459dcae26b15ca0b76de083f1/models/deepseek/chat_protocol.go)
[Perplexity Responses builder](https://github.com/Tangerg/scope/blob/da8f279591bdb46459dcae26b15ca0b76de083f1/models/perplexity/responses.go)
[OpenAI embedding configuration](https://github.com/Tangerg/scope/blob/379b392eca5226a01761810c1ff28c3c2ab26d68/models/protocol/openai/embedding.go)

Expose the transport attempt policy through the vendor chat and embedding
configurations. Flame can then select one request per recorded attempt while
each Scope binding continues to own its vendor request dialect.

## Scope child cancellation at publication (Scope Agent v0.64.0)

Flame commits a child Run together with Scope's `Started` record. A Runtime
probe held that transaction's acknowledgement after SQLite committed it
(2026-11-26). `runs.list` returned the child as running and `runs.cancel` failed
because its live Delegate binding was still being installed. Scope applies the
same `Started` record locally after the append returns. Its public cancellation
method currently requires a handle for the child process. [Scope append](https://github.com/Tangerg/scope/blob/379b392eca5226a01761810c1ff28c3c2ab26d68/agent/run/tree_runtime.go#L85-L104) [Process lookup](https://github.com/Tangerg/scope/blob/379b392eca5226a01761810c1ff28c3c2ab26d68/agent/run/runtime.go#L274-L296) [Cancellation handle](https://github.com/Tangerg/scope/blob/379b392eca5226a01761810c1ff28c3c2ab26d68/agent/run/handle.go#L146-L162)

Let the live root handle submit a descendant cancellation by Process ID. Scope's
tree queue can order the control after the in-progress append and resolve the
target there. Flame can address the child using its durable start binding and
use the Scope response as the control acknowledgement.

## A2UI schema dependency (A2UI v0.16.1)

`npm audit` reported one high-severity Zod advisory through four packages on
2026-11-27. Flame's lockfile resolves Zod 4.6.7 for its own schemas and Zod
3.25.76 for A2UI. The published `@a2ui/react` and `@a2ui/web_core` 0.16.1
packages require `^3.25.76`, and the advisory includes that entire range.
[Zod advisory](https://github.com/advisories/GHSA-mrhx-c7xc-m3w5)
[A2UI React dependencies](https://registry.npmjs.org/@a2ui%2freact/0.16.1)
[A2UI web core dependencies](https://registry.npmjs.org/@a2ui%2fweb_core/0.16.1)

Flame's interface document boundary applies limits of 512 KiB, 20,000 JSON
nodes and 48 levels before passing incoming documents to A2UI's message parser.
The advisory remains open for the dependency. A2UI can resolve it by moving its
parser and React peer range to a patched Zod release, then verifying the same
protocol messages against that parser before publishing.
