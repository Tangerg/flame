# Tool identity and policy ownership

**Status:** Implementation plan for review. It describes a breaking repair of the current Runtime and is not a description of shipped behavior.  
**Date:** 2026-10-02.  
**Baseline:** `Tangerg/flame@1083cab5794d6bcc252bdd27dac4b23ecb8a5940`.  
**Scope:** Runtime domain, application, adapters, SQLite storage, Runtime Protocol, the generated TypeScript client, Desktop settings, and CLI approval commands.

This plan repairs who owns three facts: which tool a call targets, whether that call may skip its approval prompt, and whether a tool is hidden from the model. It stands on its own and is worth doing without plugins. The heterogeneous-plugin draft (`flame-heterogeneous-plugins-draft-v0.2.md`) depends on it and does not restate it.

The defects below were found by reading the baseline code and schema. None has been reproduced by a test yet. Step 1 reproduces them before any production change.

## 1. Facts and target owners

| Fact | Owner after this change | Projections that may not advance it |
| --- | --- | --- |
| Which tool a call targets | The tool's source, expressed as one typed tool reference | Model-visible name, transcript item name, UI label |
| Model-visible tool name | Derived from the tool reference by one projection function | Provider declarations, search results |
| Built-in tool behavior (safety class, presentation, outcome) | Built-in descriptor catalog, keyed by the domain-owned `tool.BuiltInName` set; a test, not startup, holds it to exactly that set | Interpreter and presenter results |
| Standing approval decision | Approval policy (`application/agent/approvals`), keyed by tool reference, subject, and scope, and bound to the source's authority fingerprint | Approval settings views, CLI listings, MCP settings toggles |
| Source authority fingerprint | The tool's source owner: the MCP registry for MCP servers, configuration for A2A agents | Fingerprint stored with a rule at the time it is remembered |
| MCP tool exposure (disabled tools) | MCP application use case, in a relation separate from the server record | Run manifests, discovery results |
| MCP connection descriptor, secrets, enablement | MCP registry use case, as today | `MCPServer` read model |

## 2. Current defects

### D1. Policy identity is a lossy projection

MCP tools reach the model as `sanitize(server + "_" + tool)`, truncated to 64 bytes (`internal/domain/integration/mcpserver/tool_name.go:19`). Its own comment says that two `(server, tool)` pairs can collapse to one name. Three policy paths nevertheless key on that string:

- Built-in descriptors are looked up by name (`internal/adapter/toolset/descriptors.go:79`, called from `interpreter.go:34,45,92`, `presentation.go:51,194`, `registry.go:56`, and `run/execution/interaction_tool.go:97,387,580`).
- `approval.Query.Tool` and `approval.RememberRequest.Tool` are plain strings (`internal/domain/run/approval/policy.go:119-136`). Rule matching is exact string equality (`rule.go`, `rule.Tool != q.Tool`), and the rule ID derives from that string.
- Storage holds `approval_rules.tool TEXT` with no reference to any source (`internal/infra/sqlite/db.go:533-540`). The wire `ApprovalRule.tool` is documented as "tool name" (`protocol/approval.go:11`).

Consequence: suppose a user remembers "always allow" for `github_create_issue`, deletes the `github` server, and creates a new `github` server pointing at another endpoint. The rule still applies. Policy rows (`mcp_server_tool_policies`) cascade with their server, but approval rules do not.

### D2. Two owners decide whether a prompt may be skipped

`ToolCallPlan.ResolvePromptShortcuts(standing, autoApproved)` (`internal/domain/run/approval/callgate.go:96`) combines two sources:

- a remembered rule from the approval policy;
- `AutoApproveTools` stored on `mcpserver.Server` and read through `ToolPolicyState.ToolAutoApproved` (`internal/application/integration/mcp/tool_policy_state.go:38`).

The function's precedence order (remembered rule first, then auto-approve) is the arbitration rule that shows two owners advance one fact. The auto-approve value also crosses a string-typed seam: `MCPToolAutoApproved func(server, tool string) bool` is built in `internal/bootstrap/assembly_composition.go:309-318` and called from `run/execution/interaction_tool.go:375-382`. That seam re-parses names that were already typed.

### D3. A user decision lives inside the connection record

`mcpserver.Server.ToolPolicy` (`internal/domain/integration/mcpserver/server.go`) mixes exposure (`disabled`) and approval (`autoApproved`) with the connection descriptor. The MCP update use case edits both through one patch (`internal/application/integration/mcp/servers.go:372-379`), and delivery parses both from one request (`internal/delivery/mcp_request.go:61-93`). Approval is the approval policy's fact, so the record owns something that is not its own.

### D4. Built-in names are not reserved

A user MCP server named `read` that advertises `skill_resource` projects to `read_skill_resource`, which is a built-in name (`internal/domain/run/tool/tool.go:54`). The Scope registry rejects duplicate names (`scope/core/tool/registry.go:50-59`). What that rejection does to manifest construction for the Run has not been traced. In either case a remote server can affect which name resolves to which behavior. Step 1 determines the actual outcome.

## 3. Target model

### 3.1 Tool reference

`domain/run/tool` owns tool vocabulary and gains one closed reference type. It may import `domain/integration/mcpserver` for `ServerName` and `RemoteToolName`. `mcpserver` imports no domain packages, so the dependency stays acyclic, and `domain/run/approval` already depends on `domain/run/tool`.

```go
// Ref identifies the source of one tool. The model-visible name is derived
// from it and is never parsed back into a Ref.
type Ref struct {
	kind   refKind // builtIn | mcp | a2a
	name   string  // built-in name or A2A endpoint name
	server mcpserver.ServerName
	remote mcpserver.RemoteToolName
}

func BuiltIn(name string) (Ref, error)
func MCP(server mcpserver.ServerName, remote mcpserver.RemoteToolName) (Ref, error)
func A2A(endpoint string) (Ref, error)
```

The shape is illustrative. The required properties are: constructors validate, the zero value is invalid, values are comparable for map keys, and one canonical text form exists for storage and wire. Each constructor accepts exactly what its source owner guarantees. For example, `BuiltIn` accepts only names present in the built-in catalog.

`mcpserver.ToolRef` is deleted, and its uses move to `tool.Ref`. `run/execution`, `adapter/toolset`, and `infra/integration/mcp` share one resolver in `adapter/toolset` that maps an executable tool contract to its `Ref`. It wraps the existing `mcp.IdentifyTool` and the built-in and A2A sources. No caller derives a reference from a name.

### 3.2 Projection

`Ref.ModelName()` is the only projection. It keeps today's output exactly: built-in names unchanged, `sanitize(server + "_" + remote)` truncated to 64 bytes for MCP, and the Scope-assigned name for A2A. Existing model-visible names, user hook matchers (globs over names, `internal/domain/integration/hooks/hook.go:192-197`), and historical transcripts therefore stay valid.

When the manifest is built:

- A non-built-in reference whose projection equals a built-in name is excluded, with a diagnostic naming the reference.
- If two non-built-in references project to the same name, both are excluded, and the diagnostic names both. Neither wins by load order or source kind.
- Exclusion affects only those tools. It does not fail the Run's manifest.

Hooks keep matching model-visible names. A hook can deny, rewrite within its contract, or force a prompt, but it cannot waive one (`internal/application/integration/hooks/runner.go:51-55`, `callgate.go:17`). Its name-based matching therefore cannot widen authority, and it is not part of this repair.

### 3.3 Standing approval

The approval policy is the sole owner of standing decisions. A rule key is `(scope, scope key, tool reference, subject)`, and the rule ID derives from that key. Each rule also stores the source authority fingerprint that was current when the rule was remembered.

- **MCP fingerprint:** the MCP registry computes it from the transport and the endpoint descriptor: URL for Streamable HTTP; command, arguments, and directory for stdio. Secrets are excluded, so rotating a token for the same endpoint does not revoke approvals.
- **A2A fingerprint:** configuration computes it from the card URL and the allowed RPC origins.
- **Built-in fingerprint:** a built-in has none. Its authority is the Runtime binary itself.

`Decide` matches a rule only when its fingerprint equals the source's current fingerprint. A mismatched rule is reported as stale in the read model and never matches, so the call prompts again. This replaces the cross-owner "invalidate in the same transaction" coupling. The approval policy never needs to observe registry writes or configuration reloads, and a file-configured A2A agent is handled by the same rule. Rules for a removed MCP server are also deleted by a foreign-key cascade, which keeps storage bounded. Correctness does not depend on that cascade.

`ResolvePromptShortcuts` loses its `autoApproved` parameter. `ToolAuthorizationRequest.AutoApproved`, `InteractionExecutorConfig.MCPToolAutoApproved`, the session field `mcpToolAutoApproved`, `ToolPolicyState.ToolAutoApproved`, and the bootstrap closure are deleted. An MCP "auto-approve this tool" control in a client writes a global, whole-tool allow rule through the approval operations.

`ToolAuthorizationRequest` carries the `tool.Ref` for policy and the model-visible name for the prompt. `ApprovalPrompt.ToolName` stays a name, because it records what the user was shown. Recovery revalidates a pending prompt against the rebuilt request by reference as well as by name.

### 3.4 MCP exposure

`mcpserver.Server` loses `ToolPolicy`. The MCP application context keeps owning exposure as a separate user-owned relation: `mcp_tool_exposure(server_name, tool_name)` with a foreign-key cascade to `mcp_servers`. Its value set is "disabled". The `ToolPolicyDecision` enum, the `autoApproved` decision, and `ServerToolPolicy` are deleted. The `ToolPolicyState` snapshot keeps only exposure and is renamed after it. The resolver's disabled-tool check (`internal/adapter/toolset/resolver.go:213-230`) keeps its current behavior, but its key type becomes `tool.Ref`.

### 3.5 Out of scope

- **Origin-qualified MCP server identity.** It was deferred until a second writer of server records existed. Plugin installations are that writer: `Ref`'s MCP variant now holds `mcpserver.ID`, an origin plus a local name, and the plugin work owns that identity.
- **Exposure for built-in and A2A tools.** No consumer exists.
- **Rewriting historical transcript items, artifacts, or trajectory exports.** They keep the names they recorded.

## 4. Contract changes

Every row is breaking. Nothing keeps the former field, method, parameter, or type as an alias.

| Contract | Before | After |
| --- | --- | --- |
| `mcpserver.ToolRef` | Domain type keyed by server and remote name | Deleted; replaced by `tool.Ref` |
| Built-in descriptor lookup | `descriptorFor(name string)` | Lookup by built-in `tool.Ref`; interpreter and presenter take a reference |
| `approval.Query`, `RememberRequest`, `Rule` | `Tool string` | `Tool tool.Ref` plus the source fingerprint |
| `approval_rules` table | `tool TEXT` | Canonical reference columns, `source_fingerprint`, an MCP server foreign key with cascade, and an ID derived from the new key |
| `ResolvePromptShortcuts` | `(standing, autoApproved bool)` | `(standing)` |
| `mcpserver.Server.ToolPolicy`, `ServerToolPolicy`, `ToolPolicyDecision` | Exposure and auto-approve on the record | Deleted; exposure lives in its own relation |
| `mcp_server_tool_policies` table | `decision IN (disabled, autoApproved)` | Replaced by `mcp_tool_exposure` |
| Wire `ApprovalRule.tool` | Name string | Closed tool-reference union, plus a read-only `modelName`, a stale flag, and display fields |
| Wire `MCPServer.disabledTools`, `autoApproveTools`, and the matching update fields | Present | Removed; MCP gains exposure operations, and auto-approve is set through approval operations |
| Approval forget and list operations | Identify rules by an ID derived from the name | Identify rules by the new ID |

Wire method and field names are provisional. The Runtime catalog owns them, and `go generate ./...` regenerates `protocol/wire_constraints.generated.go` and the TypeScript contract.

## 5. Persisted-data migration

Follow the existing pattern in `internal/infra/sqlite/db.go`: inside the schema transaction, detect the old shape structurally (`pragma_table_info`) and convert it once. Do not introduce a schema version counter.

**`approval_rules`.** For each old row:

1. If `tool` is a built-in name, use the built-in reference with no fingerprint.
2. Otherwise, it may be an MCP name. It maps to `(server, remote)` only when all of the following hold:
   - exactly one stored server name `s` has `sanitize(s) + "_"` as a prefix of `tool`;
   - `tool` is shorter than 64 bytes, so truncation was not involved;
   - the remainder is a valid `RemoteToolName`.

   The remainder is taken literally as the remote name. This cannot broaden authority: if the true remote name differed before sanitizing, the migrated rule matches nothing, and the call prompts again. The fingerprint is the server's current fingerprint.
3. Otherwise, the row is deleted. This includes A2A names, because A2A configuration is not visible inside the storage transaction. The row is counted in a migration report that is logged and exposed through the approval read model's diagnostics, and the next call prompts again.

**`mcp_server_tool_policies`.** `disabled` rows move to `mcp_tool_exposure`. Each `autoApproved` row becomes a global, whole-tool allow rule for its MCP reference, with the server's current fingerprint. A remembered deny rule for the same tool still exists separately. It is not overwritten, and the next step makes the conflict explicit.

**Conflicts after conversion.** The rule ID derives from `(scope, scope key, tool reference, subject)` and excludes the decision, as in the baseline. Remembering the same key again therefore replaces the decision and refreshes the fingerprint; it never leaves an allow and a deny side by side. When an auto-approve row converts onto a key that already holds a remembered rule, the remembered rule is kept. This matches today's precedence, in which a remembered rule outranks auto-approve.

The old table and columns are dropped in the same transaction.

## 6. Consumers to update

| Area | Files located by search (verify each when implementing) |
| --- | --- |
| Domain | `domain/run/tool`, `domain/run/approval` (`policy.go`, `rule.go`, `callgate.go`), `domain/integration/mcpserver` (`tool_name.go`, `tool_policy.go`, `server_tool_policy.go`, `server.go`) |
| Application | `application/agent/approvals/runtime_policy.go`, `application/integration/mcp` (`servers.go`, `tool_policy_state.go`, `views.go`) |
| Adapters | `adapter/toolset` (`descriptors.go`, `interpreter.go`, `presentation.go`, `registry.go`, `resolver.go`, `build.go`, `discovery.go`), `adapter/run/execution` (`interaction_tool.go`, `interaction_tools.go`, `interaction_tool_policy.go`, `interaction_session.go`, `interaction_executor.go`), `adapter/integration/mcpconnection` |
| Infrastructure | `infra/integration/mcp` (`tools.go`, `tool_identity.go`), `infra/sqlite` (`db.go`, `approval.go`, `mcpserver.go`) |
| Bootstrap | `bootstrap/assembly_composition.go` (delete `MCPToolAutoApproved`) |
| Delivery and protocol | `delivery/mcp_request.go`, `delivery/mcp_projection.go`, approval delivery, `protocol/approval.go`, `protocol/mcp.go`, generated constraints, root binding files `approval.go` and `mcp.go` |
| TypeScript contract | `runtime/contract/typescript` (regenerated) |
| Desktop | `settings/mcp-servers` (`ToolControls.tsx`, `ServerForm.tsx`, `mcpServerDraft.ts`, `mcpServerInput.ts`, `mcpServerQueries.ts`, `runtimeMcpServerGateway.ts`, `runtimeMcpServerProjection.ts`), `settings/approvals` (`RulesRow.tsx`, `approvalConfig.ts`), `agent` approval policy files, `rpc/samples.test.ts` |
| CLI | `internal/delivery/cmd/approvals.go`, `internal/adapter/runtimebinding/catalogs.go`, `internal/delivery/terminal` approval readers, `internal/runtimefixture/approvals.go` |
| IDE | No direct references found; rerun its check after the contract changes |
| Documentation | Runtime README and architecture sections that describe approval rules and MCP tool policy |

## 7. Steps

Each step leaves the tree building and its tests passing. Steps 2–5 are one migration batch: they may be separate commits, but none is released alone.

1. **Reproduce.** Add failing tests against the baseline for:
   - O01: a remembered allow survives deleting a server and recreating it under the same name with a different endpoint;
   - O02: D4's built-in-name collision, documenting the actual current outcome;
   - O03: two references projecting to one name;
   - O04: the auto-approve and remembered-rule precedence path.

   Use the bootstrap and approval test layers that already exercise MCP policy (`bootstrap/mcp_policy_test.go`, `bootstrap/approval_recovery_test.go`).
2. **Reference and projection.** Add `tool.Ref`, the resolver, and `ModelName`. Move descriptor lookup and manifest construction to references, with reserved built-in names and symmetric exclusion. Delete `mcpserver.ToolRef`.
3. **Approval ownership.** Re-key approval rules, add fingerprints and the stale state, and remove the auto-approve input and every seam that carried it.
4. **Exposure relation.** Remove `ToolPolicy` from `mcpserver.Server`, and add the exposure relation and its operations.
5. **Storage, wire, and clients.** Run the migration from Section 5. Change the protocol, regenerate, and update Desktop, CLI, and documentation. Delete the superseded tests and the obsolete fields.
6. **Verify.** All tests from Step 1 now assert the target behavior, plus the cases in Section 8.

## 8. Acceptance

| ID | Scenario | Observable result |
| --- | --- | --- |
| O01 | Remember allow, delete the server, recreate it with the same name and a different endpoint | The next call prompts. |
| O02 | MCP projection equals a built-in name | That MCP tool is excluded with a diagnostic; the built-in is unaffected; the Run starts. |
| O03 | Two MCP references project to one name | Both are excluded and the diagnostic names both; other tools are unaffected. |
| O04 | Auto-approve set from MCP settings | A global allow rule appears in the approval read model; no other path skips the prompt. |
| O05 | MCP endpoint changed without deleting the server | Rules for that server's tools become stale and do not match. |
| O06 | Token rotated on the same endpoint | Rules remain valid. |
| O07 | A2A card URL changed in configuration | Rules for that agent become stale. |
| O08 | Migration of ambiguous, truncated, or A2A rule rows | Rows deleted and counted in the report; no guessed reference. |
| O09 | Migration of auto-approve and disabled rows | Converted to an allow rule and an exposure row respectively; a deny rule for the same tool still wins. |
| O10 | Pending approval restored after restart | Revalidated by reference and name; a mismatch refuses the response. |
| O11 | Existing hook matcher on an MCP tool name | Matches the same tools as before. |

## 9. Verification commands

From `DEVELOPMENT.md`:

```sh
(cd runtime && go generate ./...)
(cd runtime && GOWORK=off go test ./... && GOWORK=off go vet ./... && GOWORK=off go build ./...)
(cd runtime/contract/typescript && npm run check)
(cd desktop/frontend && npm run check)
(cd ide && npm run check)
(cd cli && GOWORK=off go test ./... && GOWORK=off go vet ./... && GOWORK=off go build ./...)
git diff --check
```

## 10. Open decisions

| Decision | Recommendation | Evidence needed |
| --- | --- | --- |
| How Scope's A2A tool set names its tools | Use that name as the A2A projection and the configured endpoint name as the reference | Read `scope/a2a` at the pinned version |
| What D4 currently does to a Run | Whatever it is, the target is exclusion without failing the Run | Step 1 test O02 |
| Wire shape of the tool-reference union | `{ "type": "builtIn" \| "mcp" \| "a2a", ... }` following existing closed-union conventions | Runtime catalog review |
| Where the migration report is surfaced | Approval read-model diagnostics plus one startup log line | Check existing diagnostic conventions in the approval read model |
