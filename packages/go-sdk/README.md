# Go SDK

This module owns Flame's public Go contracts and product-specific authoring APIs.
It consumes the independent Qimu module. It has no dependency on Runtime,
products, first-party plugins or repository tooling,
including in tests and development commands.

| Package                                                                                | Owner                                                                 |
| -------------------------------------------------------------------------------------- | --------------------------------------------------------------------- |
| `client`                                                                               | Remote HTTP/SSE lifecycle and generated typed operations              |
| `protocol`                                                                             | Runtime wire values, complete tree admission and strict RPC envelopes |
| `extension`                                                                            | Flame invocation origins and installation/call scope policy           |
| `plugin`                                                                               | Flame model roles, tool authoring and product receipt contracts       |
| `plugin/memory`                                                                        | Published Memory consumer contract                                    |
| `plugin/skilllearning`                                                                 | Skill learning advice and bounded call-scoped source/model contracts  |
| `plugin/codeintel`                                                                     | Versioned source transformations and diagnostic consumer contract     |
| `plugin/modelprovider`                                                                 | Installed chat, token counting, model facts and embedding adapters    |
| `plugin/sqlite`                                                                        | Atomic plugin mutations and durable receipts                          |
| `model`                                                                                | Provider/model selection, generation values and token estimates       |
| `model/catalog`                                                                        | Translation of Scope's released model catalog                         |
| `identity`                                                                             | Shared resource identities, canonical digests and lexical constraints |
| `filesystem`                                                                           | File admission, path identity, directory resolution and leases        |
| `pagination`                                                                           | Page limits and bounded opaque cursors                                |
| `capture`, `exactint`, `httpresponse`, `httporigin`, `instant`, `procgroup`, `sandbox` | Focused technical guarantees used across owners                       |

`cmd/flame-sdk` generates Flame model-role contracts and projects Flame backend
policy. Runtime's `cmd/pluginpack` translates Flame declarations into Qimu releases;
Qimu owns their archive format, validation and immutable content. Generic capability generation is
`go run github.com/Tangerg/flame/qimu/cmd/qimu contract --feature ...`.
Qimu owns SQLite transactions, cancellation, Portable contracts, exact JSON, MCP backend authoring, callback and
stream transport, and their Go/TypeScript protocol projection. Consumers import
those packages directly; this SDK does not re-export the old generic API.
Both generators use their compiled contract owners and run without source files
from another checkout.

Runtime owns execution identities, durable replay and transcript rendering.
The SDK's lexical mechanisms do not own product lifecycle policy. Plugin receipts
are encoded through `qimu/mcp.EncodeReply`; only the business owner commits their durable
representation. Protocol consumers validate complete values with
`ValidateWireTree`; individual generated node validators are private.

Use the Go version in `go.mod`. From this directory run `GOWORK=off go test ./...`.
Cross-module regeneration is coordinated from the repository root by
`make generate`. Publishers own contract identities; the SDK holds generated
consumer projections and its commands never import publisher implementations.
Runtime's generator owns the typed Runtime operations and wire constraints.
