# Session investigation

This optional package supplies a public Session trajectory view and a contextual
`sessionAction` component for naming an investigation. Both declare Session scope
and share one namespace-local prebuilt ESM module. The package has no backend.

The Session row menu offers the action using local admitted metadata. Opening it
captures that row's accepted ID, revision and title as `context.sessionTarget`.
Navigation and refreshed row data cannot replace the open target. The action does
not fetch another revision while constructing its menu or silently overwrite a conflict.

The component owns its form and calls the canonical public `sessions.update` command,
the same Session owner used by built-in editing and `flame session rename`. It submits
an explicit title only, with the inspected revision. Unknown acknowledgement retains
the original mutation handle/key and offers same-command reconciliation; a new edit
is blocked until settlement. Accepted data is shown only from the canonical response.
Close and reopen for another edit or a fresh inspected revision.

While saving, the title stays read-only and focusable and the action says
`Saving title…`. Same-command reconciliation says `Checking original command…`.
Both refuse repeat activation without removing their control from the tab order.
A definitive refusal keeps the exact draft editable and respects later focus;
an unknown outcome keeps the draft read-only until the original command settles.

Disablement withdraws menu registration and ends supported component resources;
closing observation does not retract an accepted Core command. Its durable receipt
remains owned by the canonical Session delivery operation. The former closed action
operation/declaration, special Core/CLI wrapper and private Host form are removed.

Rebuild with `node plugins/scripts/build-components.mjs`; use `--check` for exact ESM
reproduction. Reinstall/select changed package bytes. React, SDK and UI come from
actual shared Host modules; code replacement can require a window reload. No guest
or historical script installation path exists.

The trajectory distinguishes logical model calls from physical provider endpoints.
Endpoints retain request and known response JSON as strings, preserve original
caller/parent/origin, and may belong to a Session without a Run. The Session-only
filter includes them; unknown outcome and failure remain visible. Counts are record
counts, not additive billing totals.

Status tones project recorded severity independently of the attention filter and
counts. Settled neutral states read as quiet summary metadata; running, waiting,
warning and failure states keep a toned badge beside the title. The recorded state
text is always shown. The accepted page number, matching/page record count, tool
call and attention counts share one wrapping status line above the records.
There is no separate loaded-count introduction or duplicate activity heading.
Failed, timed-out and lost Runs are failures; canceled Runs and refused or
canceled tools remain neutral. Attention includes timed-out Runs and retains
incomplete, refused or errored tools without changing their recorded state.

The visible page summary identifies the last accepted cursor range. Refresh
rereads that page and its captured descendant scope. Newer/Older navigation
updates the summary only after acceptance and moves reading focus there while
the initiating action still owns it. Failed reads retain the accepted page and
leave their action available for retry; later focus choices remain authoritative.
Filters and counts apply to that page, without implying a total page count.
An empty result offers Clear filters for the active text, type and Run selection.
That local action restores the search field's focus and keeps the accepted page,
cursor and descendant scope without rereading. A failed read and accepted-page
warning remain until the existing read owner succeeds. Empty pages are described
as pages rather than implying that the whole Session has no records.
An explicit Run filter survives page changes and refreshes, including pages
without that Run. Its current selection stays available in the control; an
empty match does not silently widen the scope to All Runs.

Run summaries retain their full accepted Run ID alongside provider/model, state
and time. Summary metadata groups identities, timestamps and approval decisions
as separate wrapping values; long identities retain complete wrapping text.
Expanded Run facts name that identity once and retain the recorded root, parent
and delegating item relationships.

Message text, non-redacted reasoning and compaction summaries use literal recorded
prose. Message content keeps its text and image order. When a readable body is
present, complete JSON remains available under Recorded evidence; structured
records keep primary JSON evidence. Both projections use the same redacted
evidence, preserving unknown fields while omitting redacted reasoning text.
Recorded-evidence code actions and accepted-copy feedback use the component's
active locale. Shared UI retains the literal reader and wrap preference; changing
an action label does not rewrite the captured evidence.

Recorded message images retain their bounded inline thumbnail and open the
original captured bytes in the shared image reader. Zoom, panning and dismissal
do not change the accepted page, its filters or the recorded message. Closing
returns to the image trigger; hiding the owning view ends that open capture.
A decoder failure shows the recorded image identity with a local unavailable
notice. The original bytes remain under Recorded evidence, the recorded status
does not change, and adjacent images stay independent. A replacement source
gets its own decode result; reading does not retry a completed operation.

Page search uses the same redacted evidence as reading. It excludes only the data
bytes of canonical user/agent message image blocks, retaining their metadata.
Ordinary `data` properties in Tool arguments, results and extensions remain
searchable, including nested arrays and image-shaped business values. This local
search projection never changes accepted records or their complete raw evidence;
Run/type selection, page identity and disclosure/focus owners remain unchanged.

Search encodes accepted records with `@qimu/sdk/json`'s `stringifyJSON`; raw
evidence uses its shared two-space `prettyJSON` formatter. Opaque Tool arguments
and results retain wide integers, exact decimals, exponent literals and negative
zero as JSON numbers. Neither projection converts them to JavaScript numbers or
quoted strings. The existing Host-shared JSON module owns their encoding and
formatting; rebuild/select the matching package resource with that Host SDK.
