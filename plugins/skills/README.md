# Skills component

This optional component consumes the public Runtime Skills operations through
`@flame/plugin-sdk`. It ships ordinary prebuilt ESM with no executable or own
callable contribution. Runtime owns discovery, managed documents, proposal revision
checks, publication, archiving and their command outcomes.

Client scope receives an optional frozen workspace. Available catalogs, document
inspection and proposals bind that reference; the user library is available without
it. Reads retain an accepted same-target snapshot after failure. Switching catalogs
cancels the old catalog read and clears its query and presentation. Existing Runtime
skills/plugins/file notices refresh the current catalog and any open document through
one captured subscription/watch that document navigation does not recreate.
Cancellation retires and joins the subscription and reads.

Each Available Skill is one whole-row `RecordButton` showing its name, description
and scope; activating it opens that Skill's document. Provenance belongs to the
selected reader. Personal library rows show their complete name, description and
lifecycle with Archive/Restore directly available; they have no disclosure for
repeated facts. Proposal rows expand supporting facts and proposed instructions,
and their commands stay beside the summary.

Reading a named document is a child of Available Skills, not another catalog. The
catalog keeps its accepted snapshot, query and canonical reads while hidden. The
document header holds Back, the document name and its compact scope; the
instructions use `DocumentBody` around `RecordBody`, preserving recorded line breaks.
The document has its own read lifetime: returning or choosing another document
cancels it, a late result is ignored, and a failed refresh keeps only that
document's accepted read with an explicit label. A first failure names the document
and claims no retained content. Explicit entry focuses the document heading; a read
completing later never moves focus. Back or Escape inside the document restores the
catalog scroll and focuses the opening row, or the catalog search when a canonical
read removed that row. Escape in the catalog belongs to the host.

Proposal confirmation captures the full inspected instructions and revision.
Escape cancels that local review and returns focus to its action before the host
handles page navigation.
Archive/restore and approve/reject use the ordinary public commands. No optimistic
business result is created. Unknown acknowledgement retains the original mutation
key and retry closure and blocks another command or catalog change until reconciled.
Cross-window/Runtime-generation journal recovery remains a final integration item.

When an accepted read removes a focused catalog row, focus returns to the catalog
search. Focus already moved to another control, including an open document, is
preserved.

Build `src/component.tsx` through `../build-components.mjs`; the declared namespace
resource is tracked and exact rebuilds are checked. Disablement or uninstall
withdraws presentation/resources and preserves all canonical Skill business data.
