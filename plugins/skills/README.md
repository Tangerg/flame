# Skills

`flame.skills` implements `flame.skill-learning.v2`. Its private database owns proposal slots, review decisions and their command receipts. Runtime admits the available readers, preserves user authorization and writes approved files under the selected library root.

## Source discovery and review

The provider selects a project Skill ahead of a user Skill and a user Skill ahead of an installed Skill. Multiple installed copies without an authored override produce a diagnostic. Human catalog reads query the provider. Each Run tool group retains its original discovery command and activation in its checkpoint. Recovery reads the original receipt and resolves the chosen opaque references against admitted readers. New precedence choices apply to new Runs.

The review revision exposed to GUI, CLI and Agent is the originating command ID. Its retained input captures the content, provenance, target library, reviewed preimage and provider release. Switching the selected provider starts a separate queue. Previously reviewed work remains actionable while its original release is enabled. Disabling, replacing or uninstalling it prevents its pending commands from publishing. Forking a Session preserves the original proposal and source Session.

An approved command reserves its proposal while the host compares the active file against the captured preimage. The host records physical publication receipts in its private data directory. If it crashes after writing the approved bytes, it can reconcile them under the original permit. If the outcome is unknown and the file has since changed, the host reports that uncertainty and leaves the file in place.

## Learning policy

After successful root Run completion, the selected backend considers a Run
complex at eight completed Tool calls. Every third complex Run in a Session may
mine a reusable procedure from at least four committed messages. A revision first
reads the actual current user Skill and keeps that name. The backend also advises
an archive check at most once per six hours, with an inclusive last-activity cutoff thirty days before the captured observation time.
The plugin computes the cutoff once; the host checks it precedes the captured time and compares it with the exact activity timestamp under the library lease. A first observation starts the grace interval, including for a never-loaded skill. These cost controls reset when the backend restarts; they are not product history.

Mining uses a call-scoped grant for one conversation read, one current Skill read
and at most two utility-model calls. The complete model input is limited to
512 KiB and output to 4096 tokens. Refused, incomplete or failed model exchanges
cannot become drafts. Runtime confines and validates source documents, checks
proposal content and provenance, and requires an admitted source before accepting
a revision. Only the existing human approval operation publishes the proposal.
Archive advice passes through Runtime's existing provenance, usage, library-lease
and invalidation rules; human-authored Skills remain outside automatic archival.

`PLUGIN_DATA/learning.db` commits local decisions and receipts in one transaction. The mining command records its accepted proposal and completed model receipt in one private SQLite transaction. Its proposal revision is that mining command itself. A crash before commit exposes neither outcome. A crash after commit leaves both available even if the host never received the reply. The mining cadence retains at most 4,096 recently active sessions. A malformed model draft settles its original command as `invalid_skill_draft`. Mining and archive checks run independently. Recovery reads the original mining receipt before considering a proposal and never repeats an unconfirmed model call.

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
personal-library ownership guidance provide their respective scope. Once a read is
accepted, one summary derived from it names the catalog section and its live status:
the accepted total when every row is shown, otherwise the matching rows of that
total. Before any accepted read the section keeps its catalog title and the status
claims no count.

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
