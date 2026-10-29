# Skills

`flame.skills` provides the library and proposal component plus an optional
`flame.skill-learning.v1` backend. Runtime remains the sole owner of proposals,
active documents, human approval, usage observations and archive commits.

## Learning policy

After successful root Run completion, the selected backend considers a Run
complex at eight completed Tool calls. Every third complex Run in a Session may
mine a reusable procedure from at least four committed messages. A revision first
reads the actual current user Skill and keeps that name. The backend also advises
an archive check at most once per six hours, with a thirty-day inactivity threshold.
These cost controls reset when the backend restarts; they are not product history.

Mining uses a call-scoped grant for one conversation read, one current Skill read
and at most two utility-model calls. The complete model input is limited to
512 KiB and output to 4096 tokens. Refused, incomplete or failed model exchanges
cannot become drafts. Runtime confines and validates source documents, checks
proposal content and provenance, and requires an admitted source before accepting
a revision. Only the existing human approval operation publishes the proposal.
Archive advice passes through Runtime's existing provenance, usage, library-lease
and invalidation rules; human-authored Skills remain outside automatic archival.

`PLUGIN_DATA/learning.db` stores original command inputs and immutable advice
receipts. It contains no Skill or proposal library. A pending command is
unconfirmed and is never redelivered to the model, including after restart.
Reading an advice receipt does not publish a Skill. Mining and archive advice are
independent commands, so a mining failure does not prevent the archive check.

Disable or deselect this provider to stop new automatic learning. Library
inspection, proposal review and human decisions continue through the same public
Runtime operations. The former static Runtime mining and archive schedulers are
removed; no fallback worker or compatibility path remains.

Generate and package through the root `make generate` and plugin build commands.
The SDK owns the consumer and callback contracts; generated package JSON and
TypeScript files project those contracts.

## Component

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

Hosted catalogs begin with catalog navigation and search. The workspace path and
personal-library ownership guidance provide their respective scope.

Reading a named document is a child of Available Skills, not another catalog. The
catalog keeps its accepted snapshot, query and canonical reads while hidden. The
catalog begins at its first result when the user changes or clears the query;
refreshing an unchanged query preserves the current reading position. The
document header holds Back, the document name and its compact scope; the
instructions use `DocumentBody` around `RecordBody`, preserving recorded line breaks.
The document has its own read lifetime: returning or choosing another document
cancels it, a late result is ignored, and a failed refresh keeps only that
document's accepted read with an explicit label. A first failure names the document
and claims no retained content. Explicit entry focuses the document heading; a read
completing later never moves focus. Back or Escape inside the document restores the
catalog scroll and focuses the opening row, or the catalog search when a canonical
read removed that row. Escape in a nonempty catalog search clears that query in
place; otherwise Escape in the catalog belongs to the host.

Proposal confirmation captures the full inspected instructions and revision.
Escape cancels that local review and returns focus to its action before the host
handles page navigation.
Archive/restore and approve/reject use the ordinary public commands. No optimistic
business result is created. Unknown acknowledgement retains the original mutation
key and retry closure and blocks another command or catalog change until reconciled.
The Runtime client journal retains unresolved command identities across client
replacement. Repeating the same method and exact inspected input with that retained
journal reuses its original key and Runtime store namespace. A different Runtime
store refuses that original command. The component's visible pending form lasts
for its mounted lifetime; it does not create a separate receipt or journal owner.

When an accepted read removes a focused catalog row, focus returns to the catalog
search. Focus already moved to another control, including an open document, is
preserved.

The component consumes public Runtime Skills operations through `@flame/component-sdk`. Its declared namespace resource is tracked and exact rebuilds are checked. Disablement or uninstall withdraws presentation/resources and preserves canonical Skill business data.
