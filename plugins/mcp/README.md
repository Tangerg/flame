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

The `mcpServers` settings intent routes the graphical client's tools shortcut to
the unique installed entry. Missing or ambiguous entries lead to plugin management.
No backend, private server registry or independent authorization store is added.
