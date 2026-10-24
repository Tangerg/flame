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

Implementations parse `CapabilityJSON` through `capability.Parse` and bind their
own codecs with `plugin.ImplementMethod`. The first-party backend uses this same
path. Changing an implementation's module namespace cannot change the contract's
identity. Invalid input is refused before business execution; invalid output
remains an unconfirmed protocol failure. The backend still owns all Memory
mutations, persistence and original command receipts.

Author exports carry the implementation, component and package resources. This
public contract comes from their pinned, vendored Flame module. Running their
`go generate` only reprojects that contract; it cannot republish a different role
under the host's existing identity. To define different callable behavior, author
and publish a distinct contract with `plugin.NewMethod` and `capability.New`.

The former private Runtime contract package is removed. Consumers use this public
package directly. This source move preserves the exact wire schemas and private
business data; no alias, fallback reader or data migration is introduced.
