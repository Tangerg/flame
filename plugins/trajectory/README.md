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
text is always shown. The page record, tool call and attention counts are one
wrapping metadata line under Recorded activity. Failed, timed-out and lost Runs are failures; canceled Runs and refused or
canceled tools remain neutral. Attention includes timed-out Runs and retains
incomplete, refused or errored tools without changing their recorded state.

The visible page summary identifies the last accepted cursor range. Refresh
rereads that page and its captured descendant scope. Newer/Older navigation
updates the summary only after acceptance and moves reading focus there while
the initiating action still owns it. Failed reads retain the accepted page and
leave their action available for retry; later focus choices remain authoritative.
Filters and counts apply to that page, without implying a total page count.
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
