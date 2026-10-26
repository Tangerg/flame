# Go SDK

This module owns Flame's public Go contracts and reusable mechanisms. It has no
dependency on Runtime, products, first-party plugins or repository tooling,
including in tests and development commands.

| Package                                                                     | Owner                                                                  |
| --------------------------------------------------------------------------- | ---------------------------------------------------------------------- |
| `client`                                                                    | Remote HTTP/SSE lifecycle and generated typed operations               |
| `protocol`                                                                  | Runtime wire values, complete tree admission and strict RPC envelopes  |
| `extension`                                                                 | Admitted operation, capability and extension-point contracts           |
| `plugin`                                                                    | Backend authoring, captured callbacks, invocation and receipt encoding |
| `plugin/memory`                                                             | Published Memory consumer contract                                     |
| `plugin/sqlite`                                                             | Atomic plugin mutations and durable receipts                           |
| `model`                                                                     | Provider/model selection, generation values and token estimates        |
| `model/catalog`                                                             | Translation of Scope's released model catalog                          |
| `identity`                                                                  | Shared resource identities, canonical digests and lexical constraints  |
| `jsonvalue`                                                                 | Immutable admitted JSON and exact structural encoding                  |
| `filesystem`                                                                | File admission, path identity, directory resolution and leases         |
| `sqlite`                                                                    | SQLite connections, cancellation and transactions                      |
| `pagination`                                                                | Page limits and bounded opaque cursors                                 |
| `contractgen`                                                               | Admitted schema model and deterministic TypeScript emission            |
| `cancellation`, `capture`, `exactint`, `httpresponse`, `instant`, `sandbox` | Focused technical guarantees used across owners                        |

`cmd/flame-sdk` is the single author command. Its `contract`, `backend-protocol`
and `archive` subcommands generate feature/model contracts, project the backend
protocol and construct release archives. Backend protocol projection reads the
protocol constants and schemas compiled into the command. Prebuilt binaries,
including builds with `-trimpath`, generate the same projection without an SDK
source checkout. The projection names only the fields consumed by the TypeScript
binding; no source scanner can publish unrelated implementation constants.

Runtime owns execution identities, durable replay and transcript rendering.
The SDK's lexical mechanisms do not own product lifecycle policy. Plugin receipts
are encoded through `EncodeReply`; only the business owner commits their durable
representation. Protocol consumers validate complete values with
`ValidateWireTree`; individual generated node validators are private.

Use the Go version in `go.mod`. From this directory run `GOWORK=off go test ./...`.
Cross-module regeneration is coordinated from the repository root by
`make generate`. Publishers own contract identities; the SDK holds generated
consumer projections and its commands never import publisher implementations.
Runtime's generator owns the typed Runtime operations and wire constraints.
