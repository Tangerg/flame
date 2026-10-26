# Go SDK

This module owns Flame's public Go contracts and mechanisms. It has no dependency
on Runtime, products, first-party plugins or repository tooling, including in its
tests and development commands.

| Package                        | Responsibility                                                         |
| ------------------------------ | ---------------------------------------------------------------------- |
| `client`                       | Remote HTTP/SSE client and generated typed operations                  |
| `protocol`, `protocol/jsonrpc` | Wire values, admission and envelope guarantees                         |
| `plugin`                       | Backend authoring, callbacks and captured invocation authority         |
| `plugin/memory`                | Published Memory contract consumed by hosts and independent providers  |
| `contractgen`                  | Admitted schema model and deterministic feature projections            |
| `cmd/contractgen`              | Feature and model-role author generation                               |
| `cmd/backendbindings`          | Go backend protocol to TypeScript projection with explicit input paths |
| `cmd/archive`                  | Safe plugin release archive construction                               |
| `storage`                      | SQLite transaction, cancellation, lock and directory mechanisms        |

Other named packages own shared values or guarantees such as identity, exact JSON,
resource paths, cancellation, bounded capture and opaque-token encoding. Runtime
owns product persistence and lifecycle transitions; sharing a mechanism does not
move product policy into this module.

Use the Go version in `go.mod`. From this directory run `GOWORK=off go test ./...`.
Cross-module regeneration is coordinated from the repository root by `make generate`:
publishers own contract identities, and this SDK contains their generated consumer
projections. SDK commands do not import those publisher implementations. Generated
wire methods and validators are projected by Runtime's Host generator.
