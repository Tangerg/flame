# Go SDK

This module owns Flame's public Go contracts and product-specific authoring APIs.
It consumes the independent Frame module. It has no dependency on Runtime,
products, first-party plugins or repository tooling,
including in tests and development commands.

| Package                                                                                | Owner                                                                                |
| -------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------ |
| `client`                                                                               | Remote HTTP/SSE lifecycle and generated typed operations                             |
| `protocol`                                                                             | Runtime wire values, complete tree admission and strict RPC envelopes                |
| `extension`                                                                            | Invocation scopes and installed client contribution metadata                         |
| `builtin`                                                                              | Runtime built-in identities, arguments and normalized result values                  |
| `wiregen`                                                                              | Pure schema graph and TypeScript rendering for Runtime and native bridge generators  |
| `question`                                                                             | Runtime's ordered answer space and field-specific validation                         |
| `hook`                                                                                 | Portable lifecycle events, rules and bounded command material                        |
| `plugin`                                                                               | Flame model roles, tool authoring and product receipt contracts                      |
| `plugin/backend`, `plugin/process`                                                     | Package backend process entry and the environment the Runtime binds for it           |
| `plugin/memory`                                                                        | Published Memory consumer contract                                                   |
| `plugin/skilllearning`                                                                 | Skill learning advice and bounded call-scoped source/model contracts                 |
| `plugin/codeintel`                                                                     | Versioned source transformations and diagnostic consumer contract                    |
| `plugin/modelprovider`                                                                 | Installed chat, token counting, model facts and embedding adapters                   |
| `plugin/hookprovider`                                                                  | Installed rule sources and original command outcomes                                 |
| `plugin/sqlite`                                                                        | Atomic plugin mutations and durable receipts                                         |
| `scopewire`                                                                            | Explicit portable encodings of Scope requests, responses and content                 |
| `model`                                                                                | Provider/model selection, generation values and token estimates                      |
| `model/catalog`                                                                        | Translation of Scope's released model catalog                                        |
| `identity`                                                                             | Shared resource identities, canonical digests and lexical constraints                |
| `filesystem`, `filesystem/commit`                                                      | File admission, path identity, directory resolution, leases and publication evidence |
| `pagination`                                                                           | Page limits and bounded opaque cursors                                               |
| `timezone`                                                                             | The host's IANA time zone for values recorded on a person's behalf                   |
| `capture`, `exactint`, `httpresponse`, `httporigin`, `instant`, `procgroup`, `sandbox` | Focused technical guarantees used across owners                                      |

`builtin` declares Runtime's built-in tool names, shell arguments and normalized
transcript results. Runtime produces those values and generates their schemas
from the same types; CLI consumes them for typed presentation. Scope owns the
filesystem and HTTP request contracts. Unknown tools retain complete JSON without
assigning command, path, output or exit-code semantics from field names. These
Go contracts replace the former Runtime-private declarations; rebuild consumers
together. Wire payloads and persisted transcript shapes are unchanged.

`question.Contract` is the Runtime's immutable public answer contract. Runtime
Domain admission, command validation and CLI editing use the same field, option,
whitespace, uniqueness and custom-value rules. A complete candidate contains one
values slice per field; an empty inner slice explicitly skips it. Question
lifecycle, accepted answers and durable state remain owned by Runtime. Field
kinds and limits feed protocol and Tool schema projections from this contract.

Workspace file reads return complete normalized lines and their actual served
coordinates, including whole-file requests. `ReadFileRequest.FromEnd` selects
the suffix of its range without reversing source order. Callers must use those
coordinates for continuation; a budget that cannot hold a line returns no line
instead of its former partial prefix. Rebuild Runtime and SDK consumers together.

`extension.ResultShape` owns the closed result-selector vocabulary and its
admission. A `resultShape` selector carries a typed `shape`; `toolName` and
`fileExtension` carry independent free `name` values. Runtime contract generation
projects this value union into Go validation, schema and TypeScript. Rebuild
plugin declarations and clients together; the former result-shape `name` field
is refused without conversion.

`cmd/flame-sdk` generates Flame model-role contracts and projects Flame backend
policy. Runtime's `cmd/pluginpack` translates Flame declarations into Frame releases;
Frame owns their archive format, validation and immutable content. Generic capability generation is
`go run github.com/Tangerg/flame/packages/go-sdk/cmd/flame-sdk feature --feature ...`.
Frame owns scoped SQLite transactions, Host admission, lifetimes, process and
browser executions, portable contracts and exact JSON. Flame owns its product
composition envelope and its MCP business method/setup/callback/receipt bindings;
`plugin/mcp.ProtocolJSON` generates their TypeScript transport projection. The
adapter uses Frame's `process.Protocol` extension point, without another process,
session, instance or release owner. Cancellation is the standard context contract.
Both generators use their compiled contract owners and run without source files
from another checkout.

Runtime owns execution identities, durable replay and transcript rendering.
The SDK's lexical mechanisms do not own product lifecycle policy. Plugin receipts
are encoded through `packages/go-sdk/plugin/mcp.EncodeReply`; only the business owner commits their durable
representation. Protocol consumers validate complete values with
`ValidateWireTree`; individual generated node validators are private.

`plugin.DefiniteToolFailure` projects failures only when a caller can prove no
external mutation. An existing Scope Failure already owns its kind, public
output and diagnostic cause; the SDK returns it and any boundary wrapping
unchanged. Execution cancellation remains control rather than model feedback.

`scopewire` declares Scope's complete portable transport shapes with exact JSON
numbers, base64 byte strings and explicit timestamp codecs. Scope still validates
message roles, media sources, tool payloads, options and response semantics. The
adapter checks numeric conversion before entering native model options, accepting
equivalent spellings such as `0.7` and `7e-1` while refusing input that would round
to another setting. Opaque provider metadata and tool details retain their original
JSON. Model callbacks carry `scopewire.Request` and streamed increments carry
`scopewire.ResponseDelta`; convert them to the corresponding Scope type when
entering a native model or accumulator. Rebuild consumers and regenerate contracts
together.

Use the Go version in `go.mod`. From this directory run `GOWORK=off go test ./...`.
Cross-module regeneration is coordinated from the repository root by
`make generate`. Publishers own contract identities; the SDK holds generated
consumer projections and its commands never import publisher implementations.
Runtime's generator owns the typed Runtime operations and wire constraints.

`filesystem/commit` owns the pure publication evidence used by persistence ports:
uncommitted, visible and durable outcomes retain their original I/O diagnostic.
Applications consume that value without importing filesystem operations. The
filesystem adapter remains responsible for establishing the physical evidence.
