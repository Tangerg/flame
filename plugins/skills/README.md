# Skills component

This optional component consumes the public Runtime Skills operations through
`@flame/plugin-sdk`. It ships ordinary prebuilt ESM with no executable or own
callable contribution. Runtime owns discovery, managed documents, proposal revision
checks, publication, archiving and their command outcomes.

Client scope receives an optional frozen workspace. Available catalogs, document
inspection and proposals bind that reference; the user library is available without
it. Reads retain an accepted same-target snapshot after failure. Catalog/document
navigation cancels old reads and clears their presentation. Existing Runtime
skills/plugins/file notices trigger owner reads through a captured subscription/watch.
Cancellation retires and joins the subscription and reads.

Proposal confirmation captures the full inspected instructions and revision.
Archive/restore and approve/reject use the ordinary public commands. No optimistic
business result is created. Unknown acknowledgement retains the original mutation
key and retry closure and blocks another command or catalog change until reconciled.
Cross-window/Runtime-generation journal recovery remains a final integration item.

Build `src/component.tsx` through `../build-components.mjs`; the declared namespace
resource is tracked and exact rebuilds are checked. Disablement or uninstall
withdraws presentation/resources and preserves all canonical Skill business data.
