# Memory consumer contract

This package publishes the exact callable contract required by Runtime's Memory
role, its Go data projections and caller resource envelope. It contains no backend
or private store. Runtime, independent Go implementations and generated UI
bindings consume the same published schemas.

The definition owner is `plugins/memory/contractgen`: it derives the contract from
the first-party business types and method declarations once. `go generate` in this
package publishes those schemas and the existing business-owned caller policy.
The workspace coordinator runs this before generating backend package resources.
Generated files are projections, never independently edited definitions.

Implementations parse `CapabilityJSON` through `portable.ParseCapability` from
`github.com/Tangerg/flame/qimu/portable` and bind their own codecs with
`qimumcp.ImplementMethod` from `github.com/Tangerg/flame/qimu/mcp`.
The first-party backend uses this same
path. Changing an implementation's module namespace cannot change the contract's
identity. Invalid input is refused before business execution; invalid output
remains an unconfirmed protocol failure. The backend still owns all Memory
mutations, persistence and original command receipts.

The generated caller policy publishes `SearchToolName`. Runtime supplies the
narrow embedding grant only to that remote Tool. Other Memory Tools receive the
captured workspace without model authority; they do not depend on an embedder.

Author exports carry the implementation, component and package resources. This
public contract comes from their pinned, vendored Go SDK module. Running their
`go generate` only reprojects that contract; it cannot republish a different role
under the host's existing identity. To define different callable behavior, author
and publish a distinct contract with `qimumcp.NewMethod` and
`portable.NewCapability`.

Item operations use `v2`. Snapshots project the backend-owned positive exact
`Revision`; update, review and deletion require its observed `ExpectedRevision`.
The former methods are removed. An implementation must reject stale intent and
preserve original receipts across subsequent changes. The first-party private
schema refuses revisionless directories; it does not migrate them implicitly.
