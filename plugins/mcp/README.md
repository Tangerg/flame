# MCP settings

This installed client component owns server configuration drafts, JSON import,
connection and authorization interaction, and tool visibility and auto-approval
controls. Runtime owns the server registry, connections, OAuth attempts and
approval rules. The component calls those public operations directly.

An explicitly constructed source retains drafts and original command observations
across view closes. Source retirement clears secrets and fences new work. Writes
to one exact server identity are blocked while an earlier command is unconfirmed.
Reconciliation observes its original promise and key. Sequential imports retain
the accepted prefix and original in-flight command; they never restart that
prefix after an unknown result. Accepted writes refresh read projections without
making a failed read appear to undo a successful command.

The `mcp-servers` settings intent routes the graphical client's tools shortcut to
the unique installed entry. Missing or ambiguous entries lead to plugin management.
No backend, private server registry or independent authorization store is added.

The `external-servers` backend owns the external directory, directory revisions and
interactive authorization attempts. An installation stores them in its private
`external-servers.db`. Runtime issues each launch grant for one installation and server
name. A grant identifies protected process or HTTP transport material and cannot be
redeemed for a different source. OAuth tokens, environment values, request headers,
tool authorization and execution receipts stay with Runtime.

Disabling or removing the selected connector withdraws external tools. Installed package
backends retain their own Qimu bindings. Selecting the original connector again restores
its directory. Restart marks interrupted sign-ins as unconfirmed and preserves completed attempts
for the published retention window.
