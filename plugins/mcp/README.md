# MCP settings

This installed client component owns server configuration drafts, JSON import,
connection and authorization interaction, and tool visibility and auto-approval
controls. The external connector backend owns its server directory and OAuth
attempts. Runtime owns host credentials, launch permissions, live connections,
installation-bound authorization attempts and tool approval rules. The component
calls the public operations that project those owners.

An explicitly constructed source retains drafts and original command observations
across view closes. An authored draft is the only editable state of its editor;
without one, the editor presents its current accepted server, so a save that
accepts and clears the draft leaves any open editor showing the accepted values.
Source retirement clears secrets and fences new work. Writes
to one exact server identity are blocked while an earlier command is unconfirmed.
Reconciliation observes its original promise and key. Sequential imports retain
the accepted prefix and original in-flight command; they never restart that
prefix after an unknown result. Accepted writes refresh read projections without
making a failed read appear to undo a successful command.

Tool visibility and auto-approve writes own one command and feedback key per exact
server, separate from that server's configuration editor, while still locking the
same server resource, including its in-flight connection test. Their progress,
refusal and reconciliation appear in Tools.
Editing or closing the configuration draft leaves them in place, and an accepted
policy write never closes the editor or reports its draft as saved. The editor
shows Saving… only for its own submission; other work on the server makes it
read-only.

Each tool row is one focus reading unit: its identity, both permission switches,
description and any name-conflict sources. When a switch takes focus, the shared
Panel reveals the whole row if it fits the viewport; an oversized row falls back to
revealing the focused control.

A tool's description and its name-conflict notice share the row's one valid
paragraph; the notice is a block-level phrasing element on its own line rather
than a nested paragraph.

The editor's inline JSON fields, Arguments and Environment for stdio and Headers
for HTTP, keep a three-row minimum and grow or shrink with their wrapped content
in normal page flow through the shared `TextArea autosize` owner, rather than
scrolling inside a fixed three-row box. The JSON import dialog keeps its own
bounded editor: ten rows at large window sizes, capped by its grid area in a
short or zoomed window. It is a direct child of the shared dialog body; the
dialog title supplies the visible task and the textarea retains its accessible
configuration name. A one-line padded minimum keeps editing possible when the
body is shorter. The textarea keeps native internal scrolling and vertical
resizing, and the same element, value and selection survive window resizes.

After a successful JSON import, the unavailable submit action stays focusable
through the shared Button contract. It retains its current keyboard focus without
moving focus back from a later control. The completed batch cannot be submitted
again; editing the source retires its completion through the existing source owner.
The final confirmation replaces the in-progress count; partial and unresolved
imports retain their progress.

While it holds focus, Reconcile original command stays the same button through
the pending original retry. A known outcome, accepted or refused, leaves it disabled
as Original outcome confirmed beside any literal refusal; a repeated unknown keeps
it actionable. Leaving a pending or confirmed button retires its focus projection;
a later completion never reclaims focus. Only an accepted recovery whose original
editor is still mounted closes that editor; an older completion cannot close a
later editor.

Failed reads of tools, tool visibility and auto-approve rules name their object
above the publisher's literal explanation. Each failure and its Retry form one
reading unit. Tools and visibility retries reread that pair together; the rules
retry rereads only the rules. While it holds focus, the same recovery button stays
mounted through Loading and becomes Refresh after an accepted read. Leaving a
pending or accepted action resumes the normal read presentation; a later completion
never reclaims focus. A read that is ready on arrival shows no recovery action.

The `mcp-servers` settings intent routes the graphical client's tools shortcut to
the unique installed entry. Missing or ambiguous entries lead to plugin management.

The `external-servers` backend owns the external directory, directory revisions and
interactive authorization attempts. An installation stores them in its private
`external-servers.db`. Runtime issues each launch grant for one installation and server
name. A grant identifies protected process or HTTP transport material and cannot be
redeemed for a different source. OAuth tokens, environment values, request headers,
tool authorization and execution receipts stay with Runtime.

Runtime admits one versioned entry per exact server name and refuses duplicate
identities, missing revisions or grants, and replies outside the requested
selector before materializing transport grants. Authorization replies must keep
their original identity and coherent pending or terminal times; private SQL
constraints also refuse a finish before its start. This schema change requires a
fresh development directory for the connector; no conversion is performed.

Disabling or removing the selected connector withdraws external tools. Installed package
backends retain their own Frame bindings. Selecting the original connector again restores
its directory. Restart marks interrupted sign-ins as unconfirmed and preserves completed attempts
for the published retention window.
