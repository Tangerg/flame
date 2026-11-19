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

## Scope OpenAI generation retries

**Open.** Reproduced with Scope `models/openai` and `models/protocol/openai` `v0.62.0` and `openai-go/v3` `v3.98.0`. Flame's plugin module selects these versions. [Dependency manifest](../../plugins/go.mod)

In a standalone Scope consumer, point `openai.NewResponses` or `openai.NewChatCompletions` at an `httptest.Server` with `BaseURL` set to the server URL plus `/v1`, any fixture API key, and `chat.Options{Model: "fixture-model"}`. Return HTTP 503 with `Retry-After-Ms: 1` and a JSON `error` object for every request. One `Call` with a user text message sends three POSTs to `/v1/responses` or `/v1/chat/completions` before returning the error. In Flame's installed-provider reproduction, a 503 followed by a successful Responses stream produced two POSTs under one recorded endpoint observation. The fixture measures HTTP transmissions.

The official SDK defaults to two retries. Scope's model-list request explicitly disables retries. The generation paths inherit the SDK policy, and their public configurations provide no retry setting. [OpenAI SDK retry contract](https://github.com/openai/openai-go/blob/v3.98.0/README.md#retries), [Scope SDK options](https://github.com/Tangerg/scope/blob/2c1a6afbdaec39fcdefe76b8859bde60cced9d65/models/protocol/openai/api.go#L161-L205), [Scope public OpenAI constructors](https://github.com/Tangerg/scope/blob/2c1a6afbdaec39fcdefe76b8859bde60cced9d65/models/openai/models.go#L19-L67).

Expose a generation retry policy on the Scope OpenAI bindings so a caller can explicitly select zero retries. Apply the option at SDK request construction for both protocols. The upstream regression should count one POST when zero retries is selected and verify that the original HTTP failure reaches the caller. Flame can then select the single-attempt policy and keep product accounting and receipt semantics in its own integration.

## Scope tool-call rejection classification

**Open.** Reproduced with Scope `agent` and `core` `v0.62.0` using only the public APIs. Compile an Interaction with `interaction.CallModel` wrapping a fixture model that returns one assistant Tool call with `chat.FinishReasonStop`. Start it on a `run.Runtime` with a `run.MemoryStore`. The terminal result and the stored `run.Exited` both report `external/interaction.model.tool_calls_not_completed`. [Behavior and private code](https://github.com/Tangerg/scope/blob/2c1a6afbdaec39fcdefe76b8859bde60cced9d65/agent/interaction/execution.go#L180-L188), [Scope regression](https://github.com/Tangerg/scope/blob/2c1a6afbdaec39fcdefe76b8859bde60cced9d65/agent/interaction/execution_boundary_test.go#L44-L74).

The separate missing-message rejection can be matched through `run.StepFailure(interaction.ErrInvalidModelResponse)`. The Tool-call rejection has its own persisted classification and lacks a corresponding exported code or matcher. Flame's adapter consumes the public missing-message sentinel and still compares the private Tool-call code to translate that failure. [Public sentinel](https://github.com/Tangerg/scope/blob/2c1a6afbdaec39fcdefe76b8859bde60cced9d65/agent/interaction/errors.go#L45-L51), [Step classification](https://github.com/Tangerg/scope/blob/2c1a6afbdaec39fcdefe76b8859bde60cced9d65/agent/run/step_error.go#L29-L48), [Flame mapping](../../runtime/internal/adapter/run/execution/session.go).

Expose a public code or classifier for the Tool-call rejection, with the same declaration driving the recorded failure. Preserve the failed transition and its consumed messages. A Scope regression can match the classification read back from the log. Flame then maps that classification to its own `provider_rejected` outcome.
