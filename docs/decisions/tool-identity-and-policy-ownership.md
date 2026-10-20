# Tool identity and policy ownership

Runtime owns durable Tool identity and approval policy. A typed source reference
identifies a built-in, MCP installation/user server, or A2A endpoint. Model-visible
names, transcript labels and UI strings are projections; none can replace that
identity or grant authority.

The built-in descriptor catalog supplies behavior for domain-owned built-in
names. The MCP registry owns server configuration and source authority. Standing
approval rules belong to the approval use case, bound to exact source authority,
subject and scope. MCP exposure is a separate decision about visibility; it does
not authorize execution. Configuration views edit these owners through their
public operations and do not carry an independent approval flag.

Installation callbacks and command retries retain their captured source,
activation, contract and command identity. A selected release or a renamed model
label cannot retarget previously admitted work. Historical records publish the
captured evidence; fresh execution uses current explicit admission.

Domain models, wire values, persistence and clients preserve this vocabulary.
Read [Runtime architecture](../../runtime/docs/ARCHITECTURE.md) and the
[generated protocol reference](../../runtime/contract/API_REFERENCE.md) for exact
current contracts. This decision records ownership rather than another API table.
