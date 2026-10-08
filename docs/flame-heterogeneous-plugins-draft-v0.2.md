# Flame Heterogeneous Plugins

## Design proposal v0.2

**Status:** Draft for architecture review. Not an implementation contract or a description of shipped behavior.  
**Date:** 2026-10-01.  
**Product baseline:** `Tangerg/flame@1083cab5794d6bcc252bdd27dac4b23ecb8a5940`.  
**Audience:** Runtime, Scope, Desktop/Web, CLI, IDE, and plugin SDK maintainers.  
**Document language:** English.  
**Canonical review artifact:** This Markdown document. The PDF is a generated reading copy.

This proposal consolidates the heterogeneous-plugin design discussed for Flame: a Go product runtime, multiple clients, JavaScript graphical views, portable packages, and shared tool semantics. It specifies the recommended architecture, the boundaries needed for a first implementation, and the conditions under which later capabilities may be added.

The proposal is deliberately separate from Flame's current architecture baseline. It does not authorize repository changes, dependency upgrades, installation of third-party code, or migration of user data. Once a design slice is implemented and verified, only its current behavior belongs in the owning architecture document and generated contract. This draft must not become a competing API reference or a permanent implementation-progress ledger. [F4] [F6]

**Interpretation.** “Must” and “must not” state proposed acceptance requirements within this draft, not claims about current implementation. “Recommended” names the selected design direction. “Deferred” means excluded from the initial implementation; the text states a trigger for reconsideration. All new method names, capability names, JSON fields inside the Flame extension namespace, and example identifiers are provisional. Existing Runtime APIs retain their current definitions until an explicit replacement is implemented.

---

## Implementation reference

The first platform phase uses the production namespace `io.github.tangerg.flame`.
References to `org.example.flame` below remain illustrative proposal examples.
[Runtime's plugin documentation](../runtime/README.md#portable-plugins) owns the
implemented behavior and migration requirements; generated contracts own exact API shapes.
First-party capability extraction remains a separate phase.

## Contents

- [1. Decision summary](#1-decision-summary)
- [2. Evidence, scope, and current boundaries](#2-evidence-scope-and-current-boundaries)
- [3. Terminology and single-writer ownership](#3-terminology-and-single-writer-ownership)
- [4. Architecture and execution boundaries](#4-architecture-and-execution-boundaries)
- [5. Portable package format and Flame extensions](#5-portable-package-format-and-flame-extensions)
- [6. Installation, releases, and activation](#6-installation-releases-and-activation)
- [7. Capability catalog and contribution resolution](#7-capability-catalog-and-contribution-resolution)
- [8. Invocation semantics and delegated authority](#8-invocation-semantics-and-delegated-authority)
- [9. MCP integration profile](#9-mcp-integration-profile)
- [10. Strongly host-bound tools](#10-strongly-host-bound-tools)
- [11. Runtime Protocol and shared client contracts](#11-runtime-protocol-and-shared-client-contracts)
- [12. Desktop, Web, CLI, and IDE behavior](#12-desktop-web-cli-and-ide-behavior)
- [13. UI resource delivery and bridge security](#13-ui-resource-delivery-and-bridge-security)
- [14. Threat model and grants](#14-threat-model-and-grants)
- [15. Updates, uninstallation, and compatibility](#15-updates-uninstallation-and-compatibility)
- [16. Durable state, history, and recovery](#16-durable-state-history-and-recovery)
- [17. Hooks, observers, and controlled continuation](#17-hooks-observers-and-controlled-continuation)
- [18. Resource bounds, performance, and observability](#18-resource-bounds-performance-and-observability)
- [19. Migration boundaries for existing Flame capabilities](#19-migration-boundaries-for-existing-flame-capabilities)
- [20. End-to-end behavior and acceptance](#20-end-to-end-behavior-and-acceptance)
- [21. Bounded open decisions and implementation slices](#21-bounded-open-decisions-and-implementation-slices)
- [22. Architectural consequences](#22-architectural-consequences)
- [Appendix A. Review questions](#appendix-a-review-questions)
- [Appendix B. Sources and provenance](#appendix-b-sources-and-provenance)

## 1. Decision summary

### 1.1 Recommended architecture

Flame should provide one plugin product model with several execution and presentation adapters. It should not require all extension code to share one language, one process, one user interface, or one transport.

Portable packages describe capabilities and resources. Runtime owns installation admission, authorization, active backend resources, and product operations. Scope remains the owner of agent execution and the reusable tool contract. Desktop/Web projects accepted view contributions into the existing Dougong host. CLI and IDE consume the same Runtime-owned capabilities through their own interactions.

The central constraint is **one behavior, one semantic owner**, not **one behavior, one mandatory IPC hop**. A local Go implementation and an MCP-backed implementation can use the same invocation policy without duplicating business logic. Conversely, two MCP services can violate ownership if both independently advance the same Plan or Run.

| Decision | Recommended outcome | Consequence |
| --- | --- | --- |
| Package format | Adopt Agent Plugins 1.0.0 for portable Skills and MCP declarations. | Flame-specific capabilities stay in one explicit extension namespace. |
| Tool abstraction | Retain the released Scope tool and execution contracts. | Do not build a second tool framework inside Flame. |
| Tool identity | Key every tool policy by a typed, source-qualified tool reference; model-visible names become pure projections. | Breaking repair of approval rules, built-in descriptors, and MCP tool policy before any third-party tool is admitted. |
| MCP server records | One server registry and one connection supervisor; each record names the owner of its descriptor. | Installation-provided servers are not written through user MCP management, and no second MCP pool exists. |
| Standing tool decisions | Approval and exposure decisions have one owner, separate from the connection descriptor. | Per-server `autoApproveTools` and `disabledTools` leave the server record; remembered approval no longer merges with a second auto-approve source. |
| External backend transport | Prefer MCP over stdio or Streamable HTTP, selected by deployment. | Go, JavaScript, Python, and other server implementations can participate. |
| First-party execution | Permit trusted in-process implementations where they protect existing guarantees. | Complete stdio conversion is not a prerequisite for plugin support. |
| Graphical composition | Reuse Dougong and the existing product extension points. | No second frontend plugin kernel or competing disposer stack. |
| Third-party views | Support bounded declarative contributions and an isolated Web UI surface. | Third-party code does not receive the main React tree, raw Runtime client, or Wails bindings. |
| Product state | Keep Session, Run, Segment, Item, Goal, Plan, Interrupt, and recovery in Runtime. | A plugin requests transitions; it does not publish authoritative replacements. |
| Installation versus activation | Model desired configuration, backend activation, and client realization separately. | A failed Desktop view does not disable working tools for CLI. |
| Updates | Stage releases and refuse unsafe activation while dependencies remain in use. | Initial support does not promise arbitrary live code replacement. |
| Security | Separate provenance, approval of an exact release digest, and enforceable isolation; defer capability requests and grants until a host-brokered operation consumes them. | A valid package, signature, or tool annotation never grants authority by itself. |
| Compatibility | Keep portable format, plugin API, UI bridge, and core protocol versions distinct. | Support is explicit; no guessing from client names or package version strings. |
| Delivery scope | Repair tool identity and policy ownership first, then admit external packages through existing owners. | Existing first-party tools are not moved behind a process boundary to demonstrate the platform; Memory and scheduling are not extraction targets. |

These decisions preserve the repository's product ownership, explicit composition, consumer-shaped interfaces, and preference for existing released dependencies. They do not import Cordis, Effect, an ambient service locator, or a general-purpose dependency-injection container into Go. [F1] [F2] [F3]

### 1.2 What this proposal does not promise

The first implementation does not include a public marketplace, transparent hot replacement of active execution dependencies, a universal UI description language, arbitrary plugin-to-plugin imports, cross-runtime distributed scheduling, automatic conversion of React pages to CLI interfaces, or a new language runtime embedded in Flame.

It also does not promise that native subprocesses are safe merely because they use stdio. A subprocess operating with the user's OS privileges can act outside the broker unless an effective OS boundary restricts it. Initial native-plugin support must expose that trust model honestly.

### 1.3 Required user-visible outcome

A user installs one approved plugin release into one Runtime. Its tools become discoverable through that Runtime. Its optional view appears on capable clients. Desktop and Web can display the same packaged UI without maintaining the plugin's backend state. CLI can perform the underlying operation and read structured output when such an operation exists. IDE can offer native commands or, later, a compatible Webview. Closing any view does not cancel accepted Runtime execution.

A theme-only plugin can have no backend. A backend-only plugin can have no view. A plugin that genuinely requires a visual interaction can declare that requirement and be explicitly unavailable in a headless context. The product must not fabricate equivalence where none exists.

## 2. Evidence, scope, and current boundaries

### 2.1 Repository constraints that affect the proposal

The repository gives Runtime sole authority over durable agent-product semantics. Go and remote bindings enter one delivery endpoint. Scope provides released framework and provider libraries. Domain owners are behavior-rich and deterministic; application code coordinates I/O and lifecycle; adapters translate external systems; Bootstrap owns construction and shutdown. Repository documentation is English. [F1] [F2] [F3]

The development rules prohibit ownerless forwarding packages, copied framework implementations, speculative topologies, and temporary audit inventories in the active repository baseline. The draft therefore describes necessary responsibilities rather than imposing a directory matrix. Its migration gates are acceptance conditions, not a record of completed work. [F4] [F5] [F6]

### 2.2 Relevant current implementation

| Current fact | Design implication |
| --- | --- |
| The graphical application already has a Dougong host, typed extension points, and built-in feature plugins. | Extend its trust boundary and inputs; do not replace its composition system. |
| Workspace and settings contributions contain React component types. | These are local implementation contracts, not serializable backend descriptors. |
| Desktop/Web and IDE use the Runtime-owned TypeScript client. | Extend that client once rather than introducing a protocol fork per surface. |
| Client capabilities distinguish presentation preferences from requirements of an authoritative Run profile. | View degradation must not silently weaken execution requirements. |
| Scope MCP exposes registration and discovery adapters around Scope tools. | Reuse these adapters and identify semantic gaps before migrating tools. |
| Scope's referenced remote-result adapter returns an incomplete-result error when more input is required. | A multi-round-trip bridge needs execution integration; it is not enabled merely by rendering a dialog. |
| Filesystem execution retains a directory authority, and some mutations expose prospective paths. | A path string sent to a child process is not an equivalent authority. |
| `search_tools`, questions, and delegated agents interact with execution-owned state. | They are not independent stateless functions merely because the model invokes them as tools. |
| The MCP registry is user-owned and keyed by a bare server `Name`. One `mcpserver.Server` record carries the connection descriptor, secrets, enablement, and per-tool `ToolPolicy`. | Package-provided servers have no owner in this model. Adding them as ordinary rows would let user MCP management and installation both advance the same record. |
| MCP tools reach the model as `<server>_<tool>`, sanitized and truncated to 64 bytes. Different `ToolRef` pairs can collapse to one name. | The model-visible name is a lossy projection and cannot serve as a policy identity. |
| Built-in safety descriptors and remembered approval rules (`approval_rules.tool`) are keyed by that model-visible name. Remembered rules have no binding to the server that produced the tool. | A replaced or newly installed server whose projected name matches inherits standing approval. The policy identity must be repaired before third-party tools exist. |
| A call may skip its prompt through either a remembered rule or the MCP server's `AutoApproveTools`. `ResolvePromptShortcuts` merges the two by precedence. | Two owners advance the same fact. The precedence rule is the symptom; repair the ownership instead of extending it to plugin grants. |
| MCP tools already enter the Run's deferred set behind `search_tools`. | Plugin tools reuse deferred exposure; activation does not rewrite the tool declaration block that opens the provider prompt-cache prefix. |

The first three facts are documented by the graphical architecture and IDE contract. The remaining facts are supported by the capability model, tool adapters, MCP registry, approval gate, and execution implementations in the pinned baseline. [F7] [F8] [F9] [F10] [F11] [F12] [F29] [F30] [F33] [F34] [F35] [F36] [F37] [S1] [S2] [S3] [S4]

### 2.3 Reference-project lessons used here

The design uses the following pinned evidence, not moving repository layouts or popularity as an argument.

| Reference | Snapshot | Adopted lesson | Deliberate limit |
| --- | --- | --- | --- |
| DeepSeek Harness | `639ed015397290b3745d163aafe02ffee4aa3f84` | Separate a capability contract, its provider, and consumers; make registrations reversible; distinguish final facts from interception. | Do not copy an unrestricted in-process service graph or its browser module loader. |
| Pi | `e7a9bf7e94238f2c0f56f6ff6622750a3ce9caff` | Small author APIs, explicit tool exposure, shared nested-call admission, branch-aware state, and mode-specific UI. | Do not treat same-process extensions as isolated code or replace Flame recovery with session reconstruction. |
| OpenCode v2 | `ffa4c4c730bde7b0b0baed1df1ae5828bf0766fc` | Rebuild derived contribution state; scope registrations; compare activation revisions so a stale failure cannot disable its replacement; use stable, coarse UI slots. | Its revision comparison is not an epoch fence; do not make all business state a transform chain or assume registration rollback undoes external effects. |
| Codex | `d42056091aded7feb1d88ac7e83972108b2aa478` | Bind resources to their environment; carry UI intent through protocol; share execution contracts across local and MCP tools. | Do not copy product-private protocol or infer public Desktop implementation from metadata alone. |

These lessons are supported by the corresponding architecture, plugin API, lifecycle, and resource code. [R1] [R2] [R3] [R4] [R5] [R6] [R7] [R8] [R9] [R10] [R11]

### 2.4 Evidence limits

This document is a design proposal based on source inspection and public specifications. The ownership defects in Section 2.2 were found by reading the baseline's MCP registry, approval gate, schema, and tool authorization code. They have not been reproduced by a test. No plugin migration, repository test run, cross-platform Wails validation, performance measurement, or adversarial sandbox audit has been performed for this artifact. JSON package examples can be checked against the portable schemas; that does not validate the proposed Flame namespace or implementation behavior.

A versioned specification is evidence of a protocol contract, not evidence that the pinned SDK fully implements it. A function named `ClientSession` also does not establish which protocol-era behavior it supports. Implementation readiness must be tested at the selected dependency versions.

## 3. Terminology and single-writer ownership

### 3.1 Terms

| Term | Meaning in this proposal |
| --- | --- |
| Package | A distributable directory and its portable manifest. |
| Release | Exact admitted package bytes and their digest, plus descriptive version metadata. |
| Installation | Runtime-owned identity that binds a source to an approved release and persistent plugin data. |
| Contribution | A declared tool, action, view, renderer, setting, or other accepted capability. |
| Activation | A process-local realization of a selected release's backend contributions and resources. |
| Realization | A particular client's loaded view or other local presentation contribution. |
| Approval | The user's admission of one exact release digest of an installation. Selecting any other digest withdraws it, so changed code always needs renewed review. |
| Grant | Host-issued authorization for a principal, host-brokered operation, and target scope. Deferred with capability requests (Section 21.2); no current contribution consumes one. |
| Tool | A model-facing executable capability admitted through the existing Scope contract. |
| Action | A human-facing intent bound to an existing product operation or tool; not a duplicate business implementation. |
| View | A presentation contribution with placement, scope, rendering requirements, and a resource origin. |
| Environment | The machine or execution environment that owns a resource; not the client currently displaying it. |
| Logical invocation | The operation identity used by an existing command or execution owner, independent of a transport request ID. |

### 3.2 Ownership table

| Fact | Only owner allowed to advance it | Other representations |
| --- | --- | --- |
| Installed release and desired enablement | Runtime installation use case and its domain state | Client settings, CLI output, diagnostic views |
| Installation configuration values, including secret inputs | Runtime installation use case | Masked reads, process environment at launch |
| User-configured MCP server descriptor and enablement | Existing MCP registry use case | `MCPServer` read model, connection supervisor input |
| Installation-provided MCP server descriptor and enablement | Runtime installation use case (admitted release plus configuration) | Registry record with installation origin, `MCPServer` read model |
| Live MCP connection state | The single MCP connection supervisor | Server status, diagnostics |
| Tool identity | The tool's source: built-in definition, MCP server identity plus remote name, or A2A agent | Model-visible name, search results, UI labels |
| Standing approval decisions | Approval policy owner, keyed by tool identity | Approval prompts, settings views |
| Tool exposure decisions (disabled tools) | Tool exposure owner, keyed by tool identity | Run manifests, discovery results |
| Installation approval and revocation | Runtime installation use case, bound to the selected release digest | Installation `state` in reads; client prompts |
| Backend resource lifetime | Its activation/resource owner, constructed by Bootstrap | Health and readiness projections |
| Effective tool definitions | Existing tool admission/registry boundary | Model declarations, generated descriptors, UI catalogs |
| Run's accepted tool authority and exposure | Existing Runtime/Scope execution owners | Search results and client displays |
| Session, Run, Plan, Goal, Interrupt | Existing Runtime domain and application owners | MCP-facing operations, events, views, database encodings |
| Actual external job state | The external system or its designated plugin backend | Runtime observations and result evidence |
| Plugin-private business objects | The plugin's explicit backend domain owner | Tool results and UI projections |
| UI selection, focus, view draft | Current client feature owner | Local persistence and rendering |
| Client contribution registration | Dougong or the corresponding native client host | Installed-view list and diagnostics |
| UI asset identity | Admitted package release or authenticated external resource authority | Cache entries and resource references |

A projection may submit a request to its owner. It may not independently mark a transition committed. For example, a Schedule page may submit a create command, but it must not decide that a schedule exists merely because a transport write succeeded.

### 3.3 Identities that must remain distinct

Use typed values at boundaries for installation, artifact digest, contribution identity, activation generation, view instance, Runtime connection generation, and logical invocation. Avoid interchangeable unstructured strings inside domain logic.

The package `name` is not globally trusted identity. A version label is not an integrity proof. A JSON-RPC ID is not an idempotency key. A path is not filesystem authority. A displayed plugin title is not a permission principal. A revision used for cache invalidation is not proof of compatibility.

A contribution's durable identity is derived from its installation, contribution kind, and package-local name. A model-visible name is a projection subject to provider restrictions; collision handling must preserve the underlying identity rather than granting authority by a familiar short name.

### 3.4 Prerequisite: tool identity and policy ownership

Plugin admission depends on the repair specified in [`tool-identity-and-policy-ownership.md`](tool-identity-and-policy-ownership.md). That document owns the design, the migration, and the acceptance tests. This draft only restates its outcome:

- A typed tool reference identifies every tool by its source. The model-visible name is a projection that keeps today's form. Built-in names are reserved. Colliding projections are excluded symmetrically.
- The approval policy owns every standing decision. It is keyed by tool reference and bound to the source's authority fingerprint, so changing an endpoint, command, or card URL makes earlier decisions stale. MCP auto-approve becomes ordinary approval rules, and `ResolvePromptShortcuts` loses its second input.
- MCP exposure leaves the server record and becomes its own user-owned relation.

### 3.5 Plugin-specific breaking changes

These changes build on Section 3.4 and are needed only once installations exist. Each lands with its persisted-data migration and consumer updates. None keeps the former shape as an alias.

| Contract | Current shape | Required shape | Migration and affected consumers | Slice |
| --- | --- | --- | --- | --- |
| MCP server identity | `mcp_servers.name` primary key; `MCPServerRequest.server` and `MCPListToolsRequest.server` are bare names; the MCP variant of the tool reference holds a bare name | Origin-qualified identity `(origin, name)` in storage, on the wire, and in the tool reference | Table rebuild assigning existing rows the user origin; foreign keys from exposure, approval rules, and OAuth sessions follow; Runtime catalog, generated TypeScript client, Desktop MCP settings, CLI | B |
| `MCPServer` read model | No origin | Reports origin; descriptor not writable for installation origin | Desktop MCP settings, CLI | B |
| MCP mutations | Any server name is writable | Refuse installation-origin records with a stable ownership category | Protocol error catalog, clients | B |
| Authority fingerprint for installation-origin servers | Not applicable | Release digest plus the server's name | Approval rules become stale on every release change | B |
| OAuth session invalidation | Trigger on transport, enablement, URL, authorization, and headers changes | No invalidation step: each credential is bound to the fingerprint of the OAuth target that requested it (origin-qualified identity and credential recipient) and stops matching when either changes; it is removed with its source by cascade. A credential follows its recipient: a user server's endpoint and headers, an installation HTTP server's declared transport, URL and static headers, or an installation stdio server's release digest and name. Static secret inputs follow the same rule | Storage trigger removed, connection layer | B |
| Run admitted tool contract and waiting checkpoint | Bound to BuildID and exact execution state; no tool-source or release binding | Also record each callable tool's reference and installation release | Execution checkpoint encoding; restore refuses a mismatched release | Release switching |

## 4. Architecture and execution boundaries

### 4.1 The product composition

```text
                    Portable package / trusted built-in
                                  |
                  validate + admit source + bind release
                                  |
                       Runtime installation owner
                                  |
                     accepted contribution descriptors
                    /                              \
          Runtime capabilities                 client presentations
                  |                                  |
        existing Scope contracts             Runtime Protocol / SDK
          /                 \                   /       |       \
  trusted local tools     MCP adapters     Desktop/Web  CLI     IDE
          \                 /                   |
       shared invocation policies         existing Dougong host
                  |                        + isolated view broker
       existing product use cases
                  |
        existing persistence and recovery
```

This is a responsibility diagram, not a requirement to create a package for every box. Each new adapter must own actual translation, authority, or resource lifetime. A wrapper whose only purpose is to rename an existing API should not exist.

### 4.2 Execution classes

| Class | Permitted use | Boundary |
| --- | --- | --- |
| Trusted built-in | Compile-time Go modules and host-authored React contributions | Same trusted product binary/application; explicit construction |
| Managed local MCP | Approved external executables owned by Runtime | stdio protocol plus process supervision; OS trust/isolation stated separately |
| Remote MCP | Explicit remote service integration | Authenticated transport, server identity, network and credential policy |
| Declarative presentation | Themes, settings, actions, bounded view descriptors | Strict parsing and host rendering; no arbitrary host code |
| Isolated Web UI | Complex third-party view or MCP App | Host-controlled resource loading and capability broker |

“Managed” describes resource ownership, not a claim that a process is confined. “Built-in” describes trusted composition, not exemption from tool admission or result semantics.

### 4.3 Why not require every tool to use stdio?

Mandatory IPC would force strongly host-bound operations to travel from Runtime to a tool process and back into Runtime's existing use case. It can be implemented, but it introduces another failure window without removing the original semantic owner.

For ordinary external integrations, that boundary is useful: independent deployment, language choice, process lifetime, and protocol reuse. For a Scope tree operation, directory-handle operation, or durable Interrupt, it may require an additional control protocol. The proposal retains local execution where that is the smallest sufficient design.

A future complete-stdio variant is admissible only after equivalent tests demonstrate preserved invocation identity, authorizations, file authority, interruption, delegation, cancellation, and recovery. It must remove superseded tool entry paths rather than keep two production implementations indefinitely. The variant must not be introduced merely to satisfy architectural symmetry. [F1] [F3] [F5]

### 4.4 Explicit construction, not a service locator

Runtime Bootstrap constructs the installation, package parsing, MCP integration, authorization, and resource collaborators once and owns their teardown. Consumers receive narrow, purpose-specific collaborators. There is no `GetService(name)` API, reflection container, ambient current-plugin variable, or optional service bag.

Dynamic lookup of an admitted tool by its public contract is not a general service locator. It is an existing product capability with a bounded vocabulary and authority checks. It must not become a route for looking up arbitrary internal objects.

On the client, existing feature owners continue owning their queries, navigation, and drafts. The external-contribution adapter creates host-controlled wrappers and registers them through Dougong. It does not acquire independent control of the workbench or a second application lifecycle. [F7]

## 5. Portable package format and Flame extensions

### 5.1 Portable contract

Agent Plugins 1.0.0 is a package and discovery contract. Its portable components are Skills and MCP server declarations. `plugin.json` is the root manifest; the fixed component locations are `skills/` and `mcp.json`. The format does not define Flame's domain operations, UI API, permission prompts, or sandbox. Use the published specification and local canonical schemas rather than restating the whole standard here. [A1]

The plugin manifest selects the format by canonical `$schema`. Its `version` is optional and need not be SemVer. Unknown root fields have prescribed report-and-ignore behavior rather than whole-package rejection. This differs from the strict unknown-field handling of Flame's Runtime RPC and belongs only in the package adapter. [A2]

### 5.2 Extension namespace

Use one stable reverse-domain namespace controlled by the project. The examples use `org.example.flame`; it is an illustrative namespace, not a claim of domain ownership or a production identifier. Choosing the real namespace is a release-blocking naming decision, not a reason to change the rest of the architecture.

Client-specific manifest values belong under `extensions`; client-specific packaged files belong under the matching top-level directory. Other clients may ignore that namespace. A package may therefore expose portable tools elsewhere while its Flame pages remain unavailable. [A3]

The proposed namespace contains only Flame-owned declarations: UI requirements, view contributions, references to already-defined tool or product operations, and setting schemas. Capability requests are deferred until a contribution consumes a host-brokered operation (Section 21.2); a package that declares one receives a diagnostic for an unsupported Flame field. It must not duplicate the portable package name, version, Skill paths, or MCP launch configuration.

### 5.3 Loading pipeline and failure boundaries

Parse the portable envelope first, apply its prescribed failure boundaries, then validate the supported Flame namespace. Do not discover executable components before admitting the manifest. Do not execute a package's scripts merely to inspect its metadata.

| Input condition | Required handling |
| --- | --- |
| Invalid required portable manifest fields or unsupported format | Reject the package before component discovery. |
| Unknown root manifest field | Report and ignore it under the standard. |
| Unsupported extension namespace | Ignore its contents under the standard. |
| Invalid supported Flame namespace | Disable its dependent Flame contributions; retain independent portable components where safe. |
| Invalid individual Skill | Exclude that Skill and report it. |
| Invalid MCP envelope | Disable that package's MCP component set, not all independent components. |
| Invalid or unavailable individual MCP server | Isolate the server; do not invent successful readiness. |
| Invalid UI entry resource | Disable the dependent view, not a working independent tool. |
| Security policy refuses execution | Preserve a valid package record with an explicit policy refusal. |

The portable failure behavior comes from loading and MCP guidance. The Flame-namespace behavior above is a proposed product contract. Independent component loading does not authorize starting a workflow missing a dependency it requires. [A4] [A5]

### 5.4 Package paths and persistent data

Maintain separate authority for the immutable package release, the installation's writable data, and the Runtime workspace. Do not redirect a package's standard working directory to whichever Session is selected in Desktop.

For standard stdio declarations, honor `command` as one executable token, the permitted working-directory forms, and the defined non-recursive `PLUGIN_ROOT` / `PLUGIN_DATA` expansion. The data directory persists across updates. These are configuration rules, not confinement of arbitrary subprocess activity. [A5] [A6]

Flame additionally requires staged extraction with checked paths, bounded archive expansion, rejection of device entries and ambiguous duplicate destinations, and filesystem-resolved containment before use. Production releases are immutable after admission. Installation inspection must not follow an untrusted archive path outside its staging area. The same policy applies on platforms with junctions or other path indirections.

Local development directories require an explicit development mode. Their mutability must be visible in diagnostics. They do not silently receive production content-integrity guarantees; accepted invocations retain the exact descriptor generation they used.

### 5.5 Platform-specific executables

A Go executable is built for a target environment, not for the machine displaying its UI. Initial distribution should select an approved, prebuilt artifact matching the Runtime OS and architecture. This selection happens in the distribution/admission layer, outside the portable `mcp.json` schema.

Do not introduce an unstandardized `platforms` field into `mcp.json`. Do not let installation run unrestricted build scripts by default. An interpreted server may use an already-installed approved executable; installing its interpreter or dependencies is a separate, visible operation. A Python or JavaScript backend is not a reason to embed those runtimes into Flame.

### 5.6 Example full-stack package

The following package is illustrative. It provides review operations and a workspace view, not a duplicate Flame execution model.

```text
acme.review-board/
  plugin.json
  mcp.json
  bin/
    review-board
  skills/
    review/
      SKILL.md
  org.example.flame/
    ui/
      board.html
      board.js
      board.css
```

`plugin.json`:

```json
{
  "$schema": "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json",
  "name": "acme.review-board",
  "version": "1.0.0",
  "description": "Review resources with an optional workspace board.",
  "extensions": {
    "org.example.flame": {
      "apiVersion": 1,
      "contributes": {
        "actions": [
          {
            "id": "list",
            "title": "List reviews",
            "binding": {
              "kind": "mcpTool",
              "server": "reviews",
              "tool": "list_reviews"
            }
          },
          {
            "id": "update",
            "title": "Update review",
            "binding": {
              "kind": "mcpTool",
              "server": "reviews",
              "tool": "update_review"
            }
          }
        ],
        "views": [
          {
            "id": "board",
            "title": "Review board",
            "placement": "workspace",
            "scope": "workspace",
            "requires": ["ui.webview.v1"],
            "renderer": {
              "kind": "webview",
              "entry": "./org.example.flame/ui/board.html",
              "bridgeVersion": 1
            },
            "actions": ["list", "update"],
            "fallbackAction": "list"
          }
        ]
      }
    }
  }
}
```

`mcp.json`:

```json
{
  "$schema": "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json",
  "mcpServers": {
    "reviews": {
      "type": "stdio",
      "command": "./bin/review-board",
      "args": ["--data", "${PLUGIN_DATA}/reviews"],
      "cwd": "${PLUGIN_ROOT}"
    }
  }
}
```

The Flame fields are proposed, not standardized. Approving the release admits its exact bytes; it does not pre-approve any tool call. Tool inputs and outputs are defined by the MCP tool descriptor, not copied into the action declaration. A fallback action is a visible user alternative; the host must not automatically execute a mutating fallback when a renderer fails. In this example, opening the page may invoke only the admitted read action for its initial data; update requires a separate authorized user intent.

## 6. Installation, releases, and activation

### 6.1 Domain behavior

Use behavior-rich owners for facts with legal transitions. An installation admits a selected release, changes desired enablement, records accepted configuration revisions, and refuses an unsafe replacement. It does not launch processes, perform network calls, or close file handles. Application use cases coordinate those effects around the domain decisions.

An activation owns runtime resources, pending initialization, and cleanup. It is a process-lifecycle concern, not a durable Session or a second Run. A view realization belongs to a client generation. Plain manifest, wire, storage, and catalog values remain data; adding getters does not turn them into useful domain models. [F2] [F3]

### 6.2 Installation procedure

| Boundary | Admission requirement | Failure consequence |
| --- | --- | --- |
| Source selection | Explicit source and target Runtime, no accidental client-local execution | No install record is advanced. |
| Materialization | Bounded download/extraction into staging; no autorun | Remove owned staging data where safe; preserve diagnostics. |
| Validation | Portable rules, supported namespace, platform compatibility | Reject or isolate components as specified. |
| Trust review | Show source, integrity, executable targets, endpoints, UI network access | Valid but unapproved package remains inactive. Approval names the exact digest. |
| Release admission | Bind exact bytes/digest and validated metadata to the installation | No competing editable copy of package configuration. |
| Activation | Construct required backend resources and publish admitted contributions | Failure is recorded for the exact activation, not as successful enablement. |
| Client realization | Load compatible presentation resources | Local presentation failure does not rewrite installation state. |

Only the installation owner advances the admitted release pointer. A UI progress indicator, downloader completion, or process start does not establish successful activation. Completion should expose separate saved-state and active-state results where they differ.

### 6.3 Desired versus observed state

Avoid a single boolean or a Cartesian-product status enum spanning package, backend, and every connected client. Maintain the few independently meaningful facts and derive diagnostics from them.

An installation can be desired-enabled while one MCP component requires authentication. A release can be staged while the old release remains active. A client can be unable to render a view while backend tools are healthy. These are not inconsistencies requiring a winner-selection rule.

Runtime alone decides whether an installation's declarative presentation contributions are admitted now (active installation and available release) and publishes that decision with every installation read. Clients present exactly the admitted contributions and never rebuild activation from desired enablement, approval, or realization.

Each client owns its own realization state for its own views. A failed load is shown and retried locally, and it never writes back to installation or activation state. A second client's success or failure does not alter it. [R3]

An activation generation prevents stale work from publishing into a replacement. Its diagnostic states may distinguish preparing, ready, draining, stopped, and failed, but exact API values should be finalized with the implementation. A “ready” backend does not imply readiness of every optional component or view; publish component-level conditions where they affect real consumers.

### 6.4 Resource transfer and disposal

Transfer each acquired resource immediately to the established lifetime owner. In the graphical host, use Dougong cleanup rather than accumulating another disposer stack. In Runtime, use the existing Bootstrap and application resource graph. [F7] [F13]

Retirement stops new admission for the retiring activation, withdraws its future contributions, detaches observers, and joins or accounts for owned work. Late cleanup may remove only resources still owned by that exact generation. Never delete by a bare plugin name when a successor may already exist.

Do not await user-supplied cleanup while holding a lock that cleanup needs, or a readiness barrier it may await. This is a lifetime requirement, not a prescription to reproduce OpenCode's implementation. A failure to release a resource remains owned and diagnostic; it does not become a successful stop. [R7]

### 6.5 Crash recovery

After Runtime restarts, rebuild activation from admitted installation state and inspect existing command/execution evidence. Do not restore “running” from a persisted UI flag. Process loss establishes loss of observation, not proof of absent effects.

A plugin's resumable external task needs its own provider-supported identity and reconciliation contract. When the provider cannot establish the outcome, retain the existing unknown-outcome semantics. The installer and supervisor must not independently replay accepted product mutations.

### 6.6 Initial enablement scope

Initial installation and desired enablement are Runtime-owned. Existing Session tool selection and capability admission remain the per-execution control. Do not add separate client-global, client-workspace, Runtime-global, Runtime-workspace, and Session installation stores at once.

Package trust and project hook trust are different facts. Project trust binds a project root and authorizes that project's hooks. Package admission binds an exact release digest. Neither implies the other, and each keeps its own owner.

Project-local packages may be discovered as candidates, but opening a repository does not authorize their execution. Trust admission must happen before executing project code, loading its Web UI, resolving executable credential helpers, or installing dependencies. Per-project enablement can be added when a concrete use case requires it, through the same installation owner rather than a second configuration source.

## 7. Capability catalog and contribution resolution

### 7.1 One definition owner

A tool's executable contract remains owned by its registered Scope implementation or admitted remote descriptor. Model declarations, command forms, search results, and UI summaries are projections. An action binding references that tool instead of copying its description and argument schema into another independently editable catalog.

For plugin-owned business actions that are not tools, the plugin contract can define an operation once and expose it through a deliberately supported backend boundary. The first release should avoid adding a generic custom RPC platform solely for hypothetical operations; MCP tools and a small allowlist of existing Runtime operations cover the initial examples.

A global catalog is a derived view of accepted contributions, not a new service locator. It cannot grant access merely because an entry exists. Authorization, scope, availability, and execution profile checks still apply at invocation.

### 7.2 Contribution kinds and real consumers

| Contribution | Initial consumer | Rule protecting its boundary |
| --- | --- | --- |
| Tool | Scope execution and tool discovery | One validated definition; no authority inferred from name. |
| User action | Command palette, CLI, IDE command, view bridge | Resolves to an admitted tool or specific existing product operation. |
| Workspace or Session view | Graphical client and optional IDE host | Descriptor is serializable; layout remains host-owned. |
| Result renderer | Transcript/tool-result presentation | Cannot alter the stored tool outcome or execute on historical view without authorization. |
| Settings contribution | Plugin configuration UI | Submits validated changes to the configuration owner. |
| Theme resource | Existing appearance owner | Host-rendered bounded token data; not an arbitrary stylesheet or script privilege. Shown only while Runtime reports the installation's presentation as admitted. |
| Skill source | Existing Skill catalog and loader | Exact package provenance; established scope/conflict policy. |

Themes are the admitted declarative presentation contribution. Package locales are deferred (Section 21.2) until a concrete package needs localization; the existing localization owner is not extended for them speculatively.

General service replacement, arbitrary layout takeover, executable third-party request transforms, and plugin-defined Run event kinds are not initial external contribution kinds. Trusted built-ins can continue using existing internal extension points without those points becoming public.

### 7.3 Conflict policy

Names are unique within their defined identity space. Distinct installations with similar human-readable names remain distinct principals. Reject duplicate contribution keys in one admitted declaration batch. Do not silently resolve security identities by load order. Model-visible tool name collisions follow the symmetric-exclusion rule in Section 3.4. An installation's MCP server named `reviews` and a user server named `reviews` are different identities; neither is renamed, and neither shadows the other.

For additive UI positions, deterministic ordering may use explicit order and stable identity. A single-choice presentation, such as a renderer, needs an explicit host selection rule or user choice. Competing registrations do not gain permission to overwrite another plugin's state. Do not expose arbitrary ancestor-slot replacement to ordinary third-party views.

Package-provided Skills must join the existing catalog through a provenance-aware source model. The current catalog resolves one visible Skill per name and enforces bounds; adding package sources requires a deliberate update of those rules, not appending unbounded directories. User overrides must be explicit and must not mutate the admitted package bytes. Skill curation (archive, restore, proposal publication) keeps owning user-managed Skills only. Whether a package-provided Skill is offered is installation configuration. Archiving a package Skill through curation is refused rather than recorded as a second visibility switch. [F28]

### 7.4 Rebuilding derived state

Resolve effective contributions from a validated base plus the currently accepted registrations. Build a new immutable descriptor snapshot and publish it after validation. On failed resolution, do not publish a half-mutated collection. Retain the prior valid observation or expose the explicit failure according to the owning capability's contract.

Prefer keyed registration for simple contributions. Ordered transforms are justified only where a concrete policy needs to transform a whole candidate set. Such transforms must be deterministic, bounded, and unable to retain mutable references into earlier snapshots. A transform system is not a prerequisite for v1.

OpenCode's rebuildable state and scoped registrations provide useful evidence for derived catalogs, but its batch mechanism is not a durable transaction and must not be treated as one. [R7] [R12]

### 7.5 Catalog changes and frozen execution

Separate the current discoverable catalog from a Run's already accepted executable contract. A newly connected server can become available to future admission without silently changing a waiting execution's tool definitions.

Capture exact tool identity, definition version or digest, relevant policy binding, and backend identity at admission where required by existing recovery semantics. A descriptor digest identifies the description accepted by the host; it does not prove a remote service will never change its behavior. A remote server remains an external authority with its own failure model.

A capability revoked during a Run is not restored by the frozen snapshot. Freeze selection and arguments; recheck revocable authority at effectful dispatch. If a required backend is withdrawn, report the condition and apply existing execution-failure semantics. Do not pretend the tool never existed or replace it with a similarly named tool.

### 7.6 Exposure and invocation origin

Model visibility, programmatic callability, enabled state, and UI support are independent. Initial implementations should reuse the exposure controls already present in Scope/Flame; add a new dimension only for an actual consumer.

Plugin tools join the Run's deferred set exactly as MCP tools do today. Activating or retiring a plugin therefore changes future discovery, not the tool declaration block of later Runs, which opens the prefix provider prompt caches depend on. The discovery tool's declaration is static; the Run's frozen deferred catalog reaches the model as a Runtime-owned message that ends each model request, after the conversation, and is projected from the deferred set per request rather than stored in history, checkpoints, or the transcript. Each Run's admitted tool contract already records which definitions it may call. A plugin does not need a parallel log of catalog changes. [F12] [R1] [R4]

A loader/search tool can expose previously admitted definitions at a safe model boundary. It cannot authorize new capabilities. A tool that asks the user or changes the execution tree may require model-only or specially controlled invocation. Do not assume arbitrary nested use is valid merely because a tool is registered.

Record invocation origin as a host fact: model tool, user action, or authorized delegated plugin call. A plugin cannot choose a more trusted origin by putting a string in its arguments. Pi's separation of exposure and nested execution illustrates this distinction, while Flame's existing `search_tools` keeps executable authority separate from staged visibility. [R4] [F12]

## 8. Invocation semantics and delegated authority

### 8.1 Shared semantic path

All executable tools must pass the same applicable policies, independent of their transport. The following ordering is the proposed semantic contract; it need not be implemented as a new generic middleware framework.

```text
resolve authenticated caller and target scope
  -> resolve exact admitted tool/action
  -> apply authorized input preparation
  -> validate the final effective arguments
  -> freeze identity, arguments, target, and policy binding
  -> evaluate non-bypassable restrictions and required approval
  -> dispatch through the selected execution adapter
  -> accept result and retain effect certainty
  -> commit the relevant product observations
  -> project model content and client presentation
  -> notify read-only observers
```

If input is transformed, approval binds the transformed input actually dispatched. Any later change invalidates the approval and requires a new explicit decision. Around-execution wrappers may not substitute the tool, detach caller cancellation, or reclassify the principal.

An already dispatched action can complete after a grant is revoked. Revocation prevents further unauthorized work and requests cancellation where meaningful; it must not suppress evidence that a side effect already occurred.

### 8.2 Arguments versus execution context

Model-generated arguments contain business input. They must not contain authoritative Session IDs, approval decisions, privilege levels, filesystem roots, or a caller-selected Goal incarnation.

The current `executionctx` distinguishes execution workspace from persistent Session workspace, carries immutable scope and model selection, and binds Goal provenance. These facts do not cross stdio automatically. A remote adapter must deliberately project only the context required by its contract. [F19]

For ordinary MCP servers, do not send Flame's entire context. For a managed backend with a real host-operation need, use a host-issued opaque context handle bound to the exact caller and scope. It is not the full Runtime bearer credential.

Illustrative metadata fragment, not a complete MCP message or a shipped field:

```json
{
  "_meta": {
    "org.example.flame/invocation": {
      "contextHandle": "host-issued-opaque-reference",
      "logicalCallId": "call-identity",
      "definitionRevision": "accepted-definition-revision"
    }
  }
}
```

The host validates the context handle against the installation, activation, original invocation, target, expiry, and current grants. Public identity values are not secrets; the authority handle is sensitive and must not enter model content, tool logs, URLs, or persistent user-facing result metadata.

### 8.3 Narrow host operations

A managed tool can request a specific granted operation such as reading an allowed Session resource or submitting a Plan change. It cannot enumerate arbitrary application services or call raw internal methods.

Client-originated product commands continue entering the existing delivery endpoint and shared command admission. A trusted in-process tool may keep using its existing narrow application collaborator under its already admitted execution context. The design must not bounce such an operation back through a new root Run admission or reacquire a conflicting Session lock.

An external host-operation broker needs an equivalent scoped admission contract: it authenticates delegated authority, validates the operation, and calls the one existing use case. It must not be a bypass around product policy, and it must not invoke the same tool recursively through the general tool gateway.

Do not expose this broker in the first release unless a real accepted plugin requires it. Initial read-only graphical views can use a small allowlist of existing query projections through the trusted client bridge. Broader third-party access to Plan, Goal, or execution control is deferred.

### 8.4 Nested calls and recursive privilege

A nested tool call receives no more authority than the initiating principal, the parent invocation, and the callee's grants jointly allow. Selecting a trusted callee does not upgrade an untrusted parent. Keep the parent-child invocation link and existing accounting rules.

Nested calls must traverse relevant argument, permission, quota, cancellation, and observation checks. They need not be misrepresented as separate top-level model transcript entries. Preserve distinct execution evidence and model-visible output. Usage incurred by a nested call is accounted once, to the parent invocation, through the existing usage owner. The nested-call record attached to a parent is bounded, and the bound is visible when exceeded. [R4]

Bound recursion and fan-out using the existing executor's actual limits or an explicit capability limit. Do not create a second workflow scheduler in the plugin layer. Delegated calls must not launch work after their owning invocation has been retired.

### 8.5 Failure certainty

| Observation | Permitted conclusion |
| --- | --- |
| Validation or authorization refuses before dispatch | No dispatch occurred through this boundary. |
| Backend returns a defined business failure | The returned failure is known; partial effects depend on its contract. |
| Backend connection or process is lost after dispatch | Acceptance and effects may be unknown. |
| Client stops observing | Only local observation ended. |
| Cancellation was requested | Intent is known; stopping and rollback are not proven. |
| Completion was durably accepted | The owner may project that exact completed outcome. |

Do not force these into a single generic error string. Preserve current typed errors, causes, result content, and unresolved-effect evidence. Stable categories are needed only where callers branch; internal details stay private. A display formatter may shorten text but cannot change certainty.

The pinned Scope MCP adapter preserves definite Tool failures but does not transport every local classification unchanged. It maps remote `IsError` to a general failed outcome and treats incomplete results separately. Any richer first-party mapping requires a defined adapter contract and tests, not string parsing. [S1] [S4]

### 8.6 Idempotency and replay

Reuse the existing prepared-command journal for client mutations and the existing executor identity for model/tool effects. Do not create a second “plugin command ID” generator for the same admission. Where an external system needs an idempotency key, derive or bind it explicitly from the owning logical operation without replacing that identity.

A wire retry can use a new JSON-RPC ID while continuing the same logical invocation. Equal argument text does not imply the same operation: a user can legitimately perform the same action twice.

After lost acknowledgement, preserve exact parameters, original creation time, owning Runtime namespace, target installation/release binding, and unresolved evidence. Retrying against a different Runtime store, an expired window, or a changed operation contract must be refused rather than turned into fresh work. The client journal remains owned by the shared SDK. [F9] [F13]

Host-level deduplication cannot guarantee exactly-once effects in an unrelated external service. If a backend performed work but failed before recording its result, require provider-side idempotency, reconciliation, or an explicit unknown outcome. Never advertise exactly-once solely because a request key is persisted.

## 9. MCP integration profile

### 9.1 Reuse and version selection

Use the released Scope MCP adapters and the selected Go SDK for protocol details. Do not copy their parsing, content codecs, registration, or client implementation into Flame. Changes necessary for a reusable MCP behavior belong in the relevant library; Flame owns the product translation and grants. [F4] [S1]

Agent Plugins format version, MCP protocol version, Flame plugin API version, and UI bridge version are separate. A package targeting Agent Plugins 1.0.0 does not establish a particular MCP wire version.

The 2026-07-28 MCP specification uses per-request metadata and discovery rather than the earlier initialization/session model. It also changes reverse interaction and subscription behavior. Support exactly the protocol profiles implemented by the selected Scope and SDK release, and test them explicitly. Flame adds no legacy-handshake fallback of its own. A peer outside the supported profiles is refused as an unsupported protocol rather than accommodated. Broader peer support belongs in the reusable MCP library, released before Flame depends on it. [M1] [M2]

### 9.2 Standard transports

Support the declared transport for initial connection. Initial Flame scope should cover stdio and Streamable HTTP through existing capabilities. Legacy HTTP+SSE is not a first-release goal. A refusal must report unsupported transport rather than relabeling it as malformed business content or silently switching an execution target. [A5] [A6]

For modern stdio, stdout is the protocol channel; diagnostic output belongs elsewhere. The 2026-07-28 rules do not permit unsolicited server-to-client JSON-RPC requests on stdout. Server requests for more input use the protocol's multi-round-trip result form. Do not embed private host RPC in that channel and call it standard MCP. [M3]

Protocol permission to issue a fresh request after disconnect does not settle Flame's side-effect safety. The invocation owner still decides whether a particular operation may be retried.

### 9.3 Process lifecycle

One MCP process can serve a cohesive tool family. Do not start a process per tool or per call without a concrete lifecycle requirement. Sharing can be Runtime-wide or scoped to an actual workspace/execution resource, but the chosen owner must be explicit.

A shared process must not use mutable global current-Session/current-workspace variables. Either it is context-independent or each call has a validated scoped contract. Environment variables and process working directory are launch facts, not per-call Session routing.

The supervisor owns startup cancellation, bounded diagnostic capture, process exit observation, graceful stop, forced termination when allowed, descendant cleanup, and joining. Process group or job-object handling must be implemented and tested on supported platforms using established mechanisms. Closing stdin alone is not proof that all descendants stopped.

Restarting a failed server can restore future availability. It must not replay already accepted mutations automatically. Repeated startup failure needs bounded backoff and visible diagnostics; a deterministic invalid configuration must not be retried indefinitely.

### 9.4 Tool descriptors and metadata

Admit descriptors into the existing tool contract. Preserve remote source identity as the tool reference, and derive model-visible names by the projection in Section 3.4. Tool annotations can inform presentation or a policy for an authenticated trusted source; they do not prove safety. A malicious tool named `read` cannot acquire filesystem-reader authority. The existing built-in behavioral catalog is host policy, not a set of privileges that remote descriptors may self-assign. [F16] [F17]

The pinned Scope discovery adapter offers request metadata and concurrency policy hooks. Its default conservative scheduling does not mean MCP cannot run concurrently. Explicitly preserve or reject concurrency contracts when migrating trusted tools; do not infer conflict freedom from a name or unchecked annotation. [S2]

Scope's registration adapter reconstructs invocation information on the server side. Its local synthetic call ID is not automatically the original Flame logical invocation. Propagate exact identity through a supported adapter when the implementation genuinely relies on it. Do not modify every tool to interpret raw protocol metadata. [S3]

### 9.5 Results, resources, and large outputs

Preserve content, structured data, origin, and relevant presentation metadata through the existing codecs. A successful structured output must satisfy the admitted output contract. A protocol parse failure must not be rewritten as a definite failed business operation.

Use Runtime's established offloading and resource-reading behavior for outputs too large for model context. A result can carry a bounded preview and an authorized resource reference. Do not place all output bytes, secrets, or arbitrary HTML in every event stream.

A remote resource URI belongs to the authenticated MCP source, not to the Runtime filesystem merely because it resembles a path. Resource reads require the appropriate authority and size limits. An external URL in a result must not cause the host to fetch arbitrary internal addresses.

### 9.6 Multi-round-trip input

The current MCP pattern can request input through an interim result and continue using the returned state and input responses. A continuation is a new transport exchange, not necessarily a new logical tool operation. The opaque request state belongs to the protocol flow, not to a client-created interpretation. [M4]

To integrate this with Flame, persist the continuation material with the existing pending-effect/Interrupt owner, protect it from logs, bind it to the original source and invocation, and resume only after an authorized answer is committed. Preserve deadlines or expiry supplied by the actual owner. Recheck current authority and exact backend binding before continuing.

The first implementation must not advertise durable MCP interaction until this path is integrated and verified. The current Scope adapter's incomplete-result rejection is a known integration boundary. Rendering the interim content or leaving one RPC blocked while the user is away is not equivalent to durable suspension. [S4]

### 9.7 Long-running Tasks

The Tasks extension can expose a durable remote task handle, support status retrieval, accept input, and return completion after reconnection. Support requires the corresponding extension contract; cancellation is cooperative. It does not supply Flame's Session, Run, tree, or effect guarantees. [M5]

For a genuinely external job, retain its provider-owned task identity and status as external observations. Runtime owns the associated product operation and interpretation, not the external job's transitions. For a host-owned operation, a task representation should project the existing owner rather than maintain another state machine for the same work.

Tasks are deferred until a concrete plugin requires them and the selected SDK/execution integration passes restart, unknown-outcome, input, expiry, and cancellation tests. No generic task scheduler is added to the plugin platform.

### 9.8 Server registry, origin, and connection ownership

There is one MCP server registry and one connection supervisor. Each registry record carries its origin. Origin-qualified identity arrives with Slice B (Section 3.5). The origin names the only owner that may advance the record's descriptor and enablement.

| Origin | Descriptor and enablement owner | Write path | Secrets |
| --- | --- | --- | --- |
| User | Existing MCP registry use case | Existing MCP create, update, and delete operations | Existing write-only authorization, header, and environment changes |
| Installation | Installation use case | Release admission, installation configuration, and enablement only | Declared inputs in installation configuration, with the same write-only masking |

An installation-origin record is a projection of the admitted release's `mcp.json` entry plus the installation's accepted configuration revision. User MCP operations refuse to mutate it and return a stable "owned by installation" category. They do not offer an override that edits a copy, because an edited copy would be a second writer. Disabling one installation-provided server is an installation configuration change, so the record never holds an `Enabled` value that disagrees with its installation.

Both origins enter the same supervisor. A second MCP pool, process supervisor, or OAuth session store for plugins is not permitted. OAuth sessions bind to the origin-qualified server identity and the authorized endpoint, so reinstalling a package or replacing a user server cannot reuse an earlier credential.

Validation belongs to the owner that admits the record. The registry rejects an invalid user write. Installation admission isolates an invalid server and keeps its valid siblings (Section 5.3). Origin-qualified identity makes cross-origin name duplicates impossible by construction. The existing startup check in the connection layer rejects duplicate or invalid stored records and remains a storage-corruption guard. It is not a product failure boundary, because one installation cannot create a record that would prevent Runtime startup.

The read model reports origin, so clients can show where a server comes from and which control changes it. It does not report a writable descriptor for installation-origin records. [F24] [F33] [F37]

## 10. Strongly host-bound tools

### 10.1 Filesystem authority

Flame currently retains a directory authority shared by host inspection and Scope file operations. Reopening the same path in a subprocess can refer to a different directory after a rename or replacement. A string path, an MCP root hint, and an OS directory handle are not interchangeable. [F11]

The default recommendation is to keep the existing file capability owner and its related safety rules intact. A plugin requiring file access requests the smallest appropriate host operation or consumes an approved file snapshot. Ordinary third-party servers with direct filesystem access must be treated according to their actual OS privileges; the broker does not magically constrain that access.

A future dedicated file service must own the complete invariant: target identity, confinement, relevant locks, version checks, actual modifications, and effect evidence. All consumers of that invariant must use its authority. It is not sufficient to move only `edit` while Runtime and the service independently maintain read-before-write evidence.

### 10.2 Patch preparation and actual mutations

The current `FileMutationReporter` supplies prospective paths for approval and guards; it does not certify actual effects. The Scope patch implementation owns the corresponding capability. [F18]

Do not parse a patch independently in Runtime to guess paths while a different parser executes it in a plugin. Retain the shared implementation or introduce a justified prepare/commit contract whose prepared result binds final arguments, workspace identity, expected file versions, and authority. Execution must reject stale preparation.

Actual mutation receipts are observations of what occurred, including partial effects. They are not a substitute for permission before execution. Receipt persistence can itself fail after a filesystem mutation; preserve uncertainty instead of claiming rollback.

### 10.3 Shell and code intelligence

Background shells outlive the launching tool call in the current product. Workspace changes, Session removal, rollback, and shutdown interact with their cleanup. LSP processes also have explicit lifetime and descendant ownership. [F13] [F14] [F32]

A future Shell MCP service can own those processes, but it needs scoped handles, read/stop operations, failure observation, and verified cleanup. Runtime stores references and observations; it must not independently decide the same process is running or stopped. Losing the MCP channel does not prove the shell died.

Code-intelligence configurations, language-specific adapters, diagnostics views, and formatting choices are more suitable early extensions than the entire filesystem lifecycle. Suggested edits must enter existing mutation authority with version checks. Diagnostics based on submitted editor snapshots must remain distinguishable from diagnostics of Runtime files.

### 10.4 Plan, Goal, schedules, and memory

These model-facing tools are adapters to product use cases. Their tool entrance may be externalized without moving the official state. `set_plan`, for example, obtains Session context and invokes the Plan replacement owner. It should not create a plugin-owned replica of Plan. [F20]

Goal outcome reporting must retain admission provenance, not consult only the current mutable Goal status. Memory search must retain project/user scope. Schedule creation must preserve occurrence and Run-admission identity. Tool exposure cannot bypass the original application constraints.

If a whole optional subsystem is extracted later, move its complete coherent domain responsibility and persistence contract together. Keep the core consuming its public capability. Do not maintain a transitional pair of active writers with a “newer one wins” policy.

### 10.5 Questions and delegated agents

The current question tool enters a durable Interrupt and resumes at the corresponding call. The default implementation should stay on the established execution path until an MCP continuation adapter can preserve the same contract. [F21]

Delegation participates in Scope's parent-child execution structure, identity, cancellation, accounting, and restore. A plugin that starts another independent Agent Engine and returns its text is not an equivalent replacement. Any MCP entrance for host delegation must route back into the existing controlled tree through a supported execution seam, not instantiate a second scheduler. [F22]

### 10.6 Tool search

The discovery tool searches a Run-frozen deferred set and stages visibility through an execution-owned callback. Returning a remote server's `tools/list` is not an equivalent operation. Keep visibility admission in Scope/Flame; a plugin can provide search ranking only where the current owner can validate the selected identities. [F12]

## 11. Runtime Protocol and shared client contracts

### 11.1 Product protocol versus plugin transport

Runtime Protocol carries product commands and observations between Runtime and clients. MCP carries tool/resource exchanges between Runtime and capability providers. UI messages carry requests between a view and its host. None of these protocols replaces the others.

Client product operations must retain Go-binding/remote-binding parity through the existing delivery endpoint. Graphical and IDE clients reuse the shared TypeScript client for validation, mutation preparation, and replay. Do not add a Desktop-only Wails shortcut for plugin business operations. [F2] [F9] [F13]

### 11.2 Proposed operation families

These are provisional families, not an alternative hand-maintained API specification. Final method names and shapes must be owned by the Runtime catalog and generated artifacts when implemented.

| Operation family | Semantics |
| --- | --- |
| Plugin inspection | Read installed source, selected release, desired enablement, active component status, and safe diagnostics. |
| Install/enable/disable/update/uninstall | Explicit management commands with current command admission and replay behavior. |
| Contribution discovery | Read accepted action/view/renderer descriptors and current revisions. |
| Action invocation | Resolve a declared action to a specific admitted tool or permitted existing product operation. |
| Resource access | Read authorized package or backend resources without exposing arbitrary filesystem paths. |
| Change observation | Reuse Runtime subscription/invalidation behavior for catalog and relevant resource changes. |

Do not register every third-party operation as a newly generated core method. Conversely, do not create an unrestricted `invoke(anyMethod, anyJson)` tunnel. The stable extension envelope must resolve only identities present in the accepted catalog and validate against their one authoritative contract.

Where a feature already has a Runtime operation, an action can reference it through an allowlisted adapter. This is a declarative binding, not a duplicated schema or alternate implementation. A newly necessary business operation should be added to its proper owner rather than hidden in a generic plugin handler.

### 11.3 Illustrative action envelope

```json
{
  "target": {
    "installationId": "installed-review-board",
    "actionId": "update"
  },
  "expectedDefinitionRevision": "review-actions-r7",
  "scope": {
    "kind": "workspace",
    "workspaceId": "runtime-owned-workspace-reference"
  },
  "input": {
    "reviewId": "review-42",
    "expectedRevision": "review-r3",
    "status": "resolved"
  }
}
```

The envelope's scope is a requested target, not proof of authority. The authenticated caller and bound bridge context must permit it. The host resolves the target action's contract and operation class; a plugin-supplied “read-only” label does not select a cheaper permission path. Mutations use the existing command options and prepared-journal machinery; the example intentionally does not invent another idempotency field.

The action definition revision guards against dispatching different code or input semantics than the caller inspected. The review revision guards the plugin business object's own update. They are different facts, not duplicate counters.

### 11.4 Capability negotiation

Advertise actual capabilities, not client brand names. At minimum, separate backend feature support, plugin API support, presentation support, and the existing interrupt/execution requirements.

Presentation choices are local to a connection or view. Do not compute the intersection of every connected client's UI capability and force that on all others. A connected CLI must not disable Desktop's view; an open Desktop must not prove that a headless Run has a human available.

A missing optional renderer can fall back to generic content. A missing required interruption or authoritative event capability must use the current refusal semantics. Client-side filtering must not manufacture an apparently complete but shortened authoritative Run stream. [F10]

### 11.5 Notifications, snapshots, and bounded reads

Use notifications to signal that a projection may have changed. Re-read the owning state through existing query contracts. If a specific subscription promises a snapshot followed by a consistent tail, reuse its actual admission/cursor mechanism rather than reconstructing it in the plugin UI.

Catalog revisions invalidate caches; they do not grant permission or freeze all future data. Page/cursor scope must include the relevant Runtime, installation, filter, and authority context. Pagination cannot silently cross a replaced Runtime connection.

A v1 plugin observer is not a durable job consumer. The platform must not promise exactly-once delivery of process-local notifications. A future reliable automation hook requires an explicit durable source, cursor, deduplication and retention contract owned by the appropriate existing subsystem.

### 11.6 Errors and privacy

Errors must state the smallest meaningful condition: unsupported plugin API, unavailable component, denied capability, stale definition, wrong Runtime, invalid input, lost acknowledgement, or unknown effect. Reuse existing categories where applicable; add new categories only for new caller decisions.

Do not leak package filesystem paths, credentials, command environment values, raw stack traces, or continuation handles in public errors. Administrative diagnostics can provide redacted source and operation references. Errors preserve their causes internally without making error text a control-flow API.

## 12. Desktop, Web, CLI, and IDE behavior

### 12.1 Consistent capability, different presentation

| Capability | Desktop/Web | CLI | IDE |
| --- | --- | --- | --- |
| Query/action | Form, command, or page control | Explicit command and JSON/text output | Native command, picker, or document |
| Simple configuration | Host-rendered settings form | Typed command/input | Native input where appropriate |
| Rich result | Generic or plugin renderer | Structured result and bounded readable content | Native document or supported Webview |
| Complex workspace page | Isolated Web UI | Underlying actions, export, or explicit unsupported presentation | Optional generic Webview host |
| Human approval | Trusted host prompt | Terminal interaction when supported | Native confirmation |
| Local device operation | Platform adapter with explicit permission | Explicit local operation only | IDE-native adapter only |

Business behavior must not depend on a React component being mounted. Purely visual contributions may be unavailable outside graphical clients. A command or deep link naming an unavailable contribution should receive an explanatory result, not a blank pane or silent disappearance.

### 12.2 View descriptor

A view has a stable contribution identity, user-facing title, placement, scope, renderer kind, resource reference, protocol requirement, permitted action references, and optional fallback action. The package supplies intent; the client supplies actual layout, focus, and lifetime.

Placement and scope are separate. A Session-scoped view can render in a side panel or main area. Moving the panel does not migrate its business state. A badge is data from a read projection, not an arbitrary React callback sent from Go.

Do not expose arbitrary root layout replacement to normal plugins. Begin with the already demonstrated product surfaces: Workspace view, Session side panel, settings section, command entry, and tool-result renderer. Additional surfaces require a real consumer and stable interaction semantics.

### 12.3 Three presentation levels

**Declarative contributions** cover settings, action entries, bounded lists, tables, theme data, and simple status. The host owns rendering, accessibility, and validation.

**Semantic result rendering** covers file excerpts, diffs, search results, diagnostics, and task summaries. An enhanced renderer may improve presentation while the host retains a generic representation of the same data.

**Custom Web UI** covers a complex board, editor, or analysis surface. It runs in an isolated container and communicates through the host broker. It owns its own frontend framework; it does not require sharing the host's React instance.

Do not build a universal JSON UI language or a second package-sharing module loader. MCP Apps can be one supported renderer protocol, while a pure Flame page can use the same host surface without inventing a fake tool solely to acquire navigation.

### 12.4 Adapting into Dougong

The trusted external-contribution adapter reads validated descriptors and creates host-owned React wrappers. It registers these wrappers through existing extension points. Internal React component contracts remain internal. [F7] [F8]

Illustrative local adapter shape, not a compiled SDK example:

```typescript
const component = createIsolatedView(descriptor, bridgeFactory);

ctx.contribute(WORKSPACE_VIEW, {
  id: descriptor.contributionId,
  title: descriptor.title,
  dock: descriptor.scope,
  component,
});
```

The factory owns resource/bridge binding and validates that this descriptor can map to the target surface. A third-party page does not receive `ctx`, `WORKSPACE_VIEW`, the application query cache, or a raw `FlameClient`.

Use Dougong's existing cleanup, contribution update, snapshot publication, and generation behavior. A wrapper exists here because it owns an authority boundary; a second registration ledger or disposer manager would not. [F7]

### 12.5 Opening and restoring views

Opening a page is a presentation action. It must not trigger a mutating tool as an implicit consequence of layout restoration. Initial data loading can use an explicitly permitted query; a model/tool result that already provided initial data should not be executed again just to draw the first frame.

A view instance binds to one Runtime connection generation and an explicit Workspace/Session/Run scope. Client selection changes are messages from the host, not values discovered through ambient globals. A view may refuse an unsupported scope instead of guessing a default.

Persist only appropriate local view state, such as selected tabs or a filter. On restart, resolve current authority and contributions before restoring them. Never auto-install or execute an old renderer merely because a saved layout references it.

### 12.6 Local drafts and concurrent edits

The view owns unsent drafts. The business owner owns committed state and its revision. A save carries the revision the user edited; conflicts remain explicit. The view may offer a refresh or an intentional resubmission, but it cannot overwrite a newer value by changing the expected revision behind the user's back.

Connection replacement retires the previous view bridge and observation tasks. Late responses cannot update the new view or submit a follow-up through the new Runtime. Preserve an unresolved mutation in the shared journal; do not transfer it by silently substituting target identities.

### 12.7 CLI and IDE requirements

CLI should offer generic discovery, invocation, safe output, and clear capability refusals before adding plugin-specific terminal widgets. Reuse Oolong for proven terminal interactions rather than introducing another terminal runtime.

IDE should consume the same shared client and action definitions. A generic Flame Webview host can later display compatible resources inside the existing extension; each Flame plugin should not need a separate VSIX. IDE buffers remain versioned client-local inputs, not automatic authority to overwrite Runtime workspace files. [F9]

A graphical test proves neither native Wails security nor IDE behavior. Supported native carriers need their own bridge, navigation, disposal, focus, and file-access verification.

## 13. UI resource delivery and bridge security

### 13.1 Resource authority

A resource reference identifies the installation/release or authenticated external source that owns it, the relevant environment, and the package-local or provider-local resource identity. It is not a raw client filesystem path.

Runtime resolves and authorizes reads using its existing resource and transport boundaries. The trusted client host receives the permitted bytes and creates an isolated rendering context. The untrusted frame never receives the Runtime bearer token, a general filesystem endpoint, or an arbitrary URL-fetch privilege.

Codex's environment-bound resource locations support this separation. The design should adopt the identity principle without copying private types or treating lexical root checks as complete filesystem protection. [R9]

Package assets are keyed by admitted release digest and resource identity. Reject path escapes, incompatible MIME types, oversized resources, and missing release bindings. A mutable development resource must carry its weaker integrity status. External MCP resources are tied to the originating authenticated service/account and obey separate freshness and authorization rules; their URI is not a package digest.

### 13.2 Static assets and credentials

Do not expose a server directory as a generic public `/plugins/*` filesystem route. Asset delivery must confine access to the exact admitted release and resource set. Existing public application static assets and authenticated plugin data have different policies.

If an iframe cannot attach authorization headers, the host should fetch through its trusted transport and present approved content through a confined resource mechanism. It must not put a long-lived Runtime credential in query parameters or HTML. A platform-specific resource carrier needs an explicit threat model and cross-platform tests; the final carrier is a release gate in Section 21.

Preloading assets must not execute plugin business actions or grant new access. Cache reuse is permitted only for identical authority and content identity. Never reuse privileged data across accounts or Runtime connections solely because the URL matches.

### 13.3 Bridge handshake and lifetime

The host creates a view instance with a bound principal, activation/release, target scope, connection generation, supported bridge version, and granted operations. A frame proves participation in that exact instance through the established trusted handshake, not by sending a self-selected `pluginId`.

Prefer an existing reviewed message-bridge implementation when it satisfies the needed authority and lifecycle semantics. A dedicated message channel can narrow subsequent communication. Initial message handling must validate the source frame and the expected bootstrap exchange. An opaque-origin iframe may report `origin: null`; accepting every message with that origin is not authentication.

Any bootstrap mechanism that requires a wildcard target origin must confine it to a verified frame/channel handoff and must not broadcast credentials or business data. Do not invent a custom cryptographic protocol to compensate for an unclear browser origin design.

Every request is correlated with its view instance. Retiring a view closes the channel, aborts local reads, and prevents new submissions. Already accepted product commands remain owned by Runtime and recover through the shared journal. Retiring the old instance cannot close a successor's channel.

### 13.4 Permitted bridge operations

| Operation | Proposed initial policy |
| --- | --- |
| Read initial view context | Provide only approved scope references, presentation context, and allowed initial data. |
| Invoke an action | Resolve an action explicitly available to this view; enforce all product/tool policies. |
| Query an allowed Runtime projection | Bind to an explicit read capability; do not expose arbitrary protocol methods. |
| Read an approved resource | Constrain to the view's permitted resource authority and size budget. |
| Subscribe to relevant changes | Use bounded subscriptions; reuse host observation and cancellation behavior. |
| Navigate within the plugin | Validate an app-relative destination; the host decides layout and focus. |
| Request external navigation | Use host validation and confirmation policy; reject privileged and executable schemes. |
| Request native functionality | Disabled unless the carrier implements that exact authorized capability. |
| Update model context | Not an implicit rendering effect; requires a separately admitted, bounded product action. |
| Resolve an approval | Not a normal plugin operation; the trusted host owns approval presentation and acceptance. |

The bridge is not a full client SDK proxy. A view that can read a Session is not thereby allowed to export every Session, read all credentials, install packages, or execute arbitrary tools.

### 13.5 CSP and rendering isolation

Third-party Web UI must run outside the main application's origin and privileges, or in a correctly configured opaque-origin sandbox. Do not combine same-origin untrusted content with privileges that let it access the host document. Host DOM, cookies, local storage, application state, and native bindings are not part of the plugin API.

Start with a restrictive content policy and grant only required script/resource origins. Check every relevant channel: scripts, connections, images, styles, fonts, forms, frames, workers, navigation, clipboard, and file downloads. Network permissions in a resource declaration are requests; the effective policy is the host-approved intersection.

The executing carrier owns network enforcement. CSP connection restrictions alone do not cover WebRTC. A constrained entry document may receive brokered bytes and create a local document that inherits its connection policy; creating the document in an unrestricted host context does not establish that guarantee. An unsupported policy makes the carrier unavailable. [Desktop's carrier acceptance](../desktop/README.md#plugin-carrier-acceptance) owns the tested mechanism and carrier findings.

A native carrier may withdraw a channel through public per-view engine configuration when a control proves enforcement and the workbench retains its own capabilities. That configuration and native sender identity must come from the executing owner, not guest claims. The carrier does not share the privileged workbench's script-message controller or persistent website data store.

Bundled assets and brokered reads are the initial preference. Direct remote origins are a separate reviewed capability. A permitted image or navigation endpoint can still carry data; preventing parent DOM access does not prevent exfiltration of data deliberately supplied to a view.

Third-party themes should be bounded token data, not arbitrary CSS capable of concealing trusted UI. The host controls plugin identity chrome, permission dialogs, and failure overlays. A plugin may draw arbitrary content within its surface, but it must not be able to obscure which parts of the application are trusted controls.

### 13.6 MCP Apps reuse

MCP Apps supplies a UI-resource and postMessage-based interaction model distinct from backend MCP transport. It can be adapted into the same view host. Do not reimplement the whole bridge when a maintained implementation meets the product requirements. [M6]

Flame still owns installation, grant checks, resource authority, Run context, and native capability restrictions. Supporting an MCP App does not grant every tool on its server to that frame automatically. A tool-originated view should consume the admitted initial result rather than repeat the business operation for first render.

Static workspace/Session entrypoints and tool-result views can converge on the same internal view descriptor. They remain different triggers. A pure view need not invent a tool just to appear in navigation.

### 13.7 Wails-specific release gate

The browser implementation is not sufficient evidence for the Wails carrier. Before enabling third-party Web UI on a desktop target, verify whether native injection reaches child frames, how custom resources inherit origin, what navigation escapes are possible, how CSP is enforced, and whether channels survive frame teardown unexpectedly.

A platform that cannot enforce the required boundary must report the renderer unavailable. It may still provide tools and safe declarative presentation. Do not silently load third-party HTML in the main privileged document to preserve feature parity.

### 13.8 Deep links and downloads

Deep links identify an already-known installation, view, scope reference, and app-relative route. They must not smuggle a Runtime address, credential, arbitrary executable scheme, or local filesystem authority. Following a link does not authorize plugin installation, upgrade, account switching, or a mutating action.

Downloads are brokered artifacts with validated filenames and an explicit user action. Client-local save locations belong to the native/browser adapter. Remote Runtime paths are never assumed to exist on the client. A view may request a download, but it cannot silently write outside the selected local destination.

## 14. Threat model and grants

### 14.1 Trust boundaries

The threat model includes malicious package authors, compromised updates, hostile remote MCP servers, untrusted workspace content, prompt injection, buggy first-party extensions, conflicting local UI instances, and lost or delayed responses. It does not assume an attacker with unrestricted control of the host OS can be contained by application-level checks.

| Attack or failure | Boundary to protect | Required response |
| --- | --- | --- |
| Package path escape or archive bomb | Installation filesystem | Reject unsafe materialization before activation. |
| Package name impersonates a built-in | Identity/admission | Use installation provenance and grants, not the name. |
| Workspace file asks the agent to install a plugin | User intent/trust | Installation remains a distinct authorized operation. |
| Tool claims `readOnlyHint` but performs writes | Authority | Hints do not grant access; enforce actual execution boundary or disclose trusted-process scope. |
| UI sends a forged plugin or Session ID | Bridge | Derive principal from host binding and validate target scope. |
| MCP endpoint changes after an update | Credential binding | Require appropriate reauthorization; do not reuse secrets with a new authority. |
| Old frame submits after Runtime switch | Generation | Reject old context; do not retarget automatically. |
| Post-result plugin changes a failure to success | Fact ownership | Preserve immutable effect evidence and outcome classification. |
| Policy plugin fails or disappears | Admission | Required checks fail closed; do not treat absence as permission. |
| Backend process ignores cancellation | Resource lifetime | Preserve ownership, enforce supported stop policy, and report uncertain effects. |

### 14.2 Release approval, requests and grants

Installation trust is one closed state bound to the selected release: unapproved, approved, or enabled. The user approves an exact digest, and selecting any other digest returns the installation to unapproved, so changed code is never launched or dispatched without renewed review. Revocation withdraws dispatch authority immediately; it does not erase standing tool approvals of the same code.

Package capability requests and host grants are deferred (Section 21.2). With actions and views withdrawn, no contribution consumes a host-brokered operation, so a request would have no enforcement point and a grant would only pretend to restrict a trusted executable. A manifest that still declares requests receives a diagnostic and nothing is admitted from it.

When a contribution that consumes host-brokered operations returns, a grant should bind at least its principal, operation, target scope, relevant origin or resource identity, and revocation state, and never let a new release inherit broader privileges through the same name.

A model tool call's standing approval is owned by the approval policy owner, keyed by tool reference (Section 3.4) and bound to the release digest and server name of an installation tool. Release approval never pre-populates standing approval. Installation-specific facts belong to the installation. They do not establish a second permission store with its own allow/deny rules, and they are not combined with approval rules through a precedence rule.

Capabilities should describe operations rather than implementation details. A bounded Session read is different from all-history export. Workspace file read is different from arbitrary OS read. Network access to one authenticated service is different from arbitrary outbound HTTP. A plugin-private store is different from the Runtime database.

### 14.3 Secrets

Do not place credentials in portable package fields, UI assets, model context, tool descriptions, resource URLs, or public diagnostics. Portable MCP configuration contains no standard credential-reference or OAuth configuration mechanism; authorization remains host-managed. [A5] [A6]

The secret owner binds credentials to the authenticated service/issuer and installation use. A plugin may request an authenticated operation without receiving the underlying secret. Some local tools genuinely need an environment credential; that release of secret material must be explicit, scoped, and visible as part of the trusted-process model.

Package bytes are public content: static headers and environment values in a package are never treated as secrets, and header or authorization inputs are always secret. A credential follows its recipient. For an HTTP server the recipient is the declared endpoint (transport, URL, and static headers, excluding configured input values); its secret inputs and OAuth credential survive a release that keeps that declaration. For a stdio server the recipient is the executable (release digest and server declaration); its secret inputs are dropped by every release change and must be entered again. Credential inheritance is never based on textual package identity. Redaction must cover stdout/stderr capture, traces, errors, snapshots, and unresolved continuation material.

### 14.4 Native execution policy

Two honest modes are possible: a trusted local executable with the user's effective OS privileges, or an enforceably confined process with documented file/network/process constraints. The product must not describe the first as the second.

Initial local third-party MCP support may use the trusted-executable model with explicit consent and restricted environment inheritance. Broker grants limit use of brokered host APIs; they do not prevent that process from accessing resources directly under its OS account. Untrusted native execution requires a validated OS sandbox/container boundary and is not implied by this proposal.

The existing Scope/Flame process and sandbox capabilities should be reused where they cover the target behavior. Do not create a plugin-specific shell sandbox that duplicates or contradicts the current owner. Unsupported confinement must refuse that capability rather than silently downgrade. [F14]

### 14.5 Installation and supply-chain scope

Initial support should prefer local approved packages and pinned, integrity-checked release artifacts. Publisher identity, transport origin, digest, requested permissions, and executable dependencies are shown before activation. An admitted digest binds bytes, not author quality or lack of malicious behavior.

Dependency scripts are distinct executable actions. Rolling back a package manifest or lockfile does not undo effects from a script already run. Package-manager credential configuration and registry selection must not silently leak to untrusted scripts or fall through from a private source to a public one.

A public marketplace, signature trust hierarchy, key rotation, and remote revocation feed are deferred until distribution requires them. Basic integrity verification and source recording are not deferred. Use established verification primitives; do not design a new signature scheme.

### 14.6 Fail-closed versus graceful presentation degradation

Classify failure by what the component owns. An optional chart can fail independently. A missing renderer can reveal generic content. A required authorization check cannot become “allow” on timeout. A schema mismatch cannot become an empty successful result. A missing durable continuation cannot become a new operation.

A plugin that contributes policy and presentation has different failure consequences for those contributions. Treating the whole package as either “all critical” or “all best effort” is too coarse.

## 15. Updates, uninstallation, and compatibility

### 15.1 Distinct version axes

| Version/identity | What it establishes | What it does not establish |
| --- | --- | --- |
| Agent Plugins `$schema` | Package interpretation contract | Backend MCP protocol or Flame API support |
| MCP negotiated profile | Tool/resource wire behavior | Flame durability or UI features |
| Flame extension API version | Supported product contribution contract | Core Runtime protocol compatibility |
| UI bridge version | Frame/host message contract | Permission or trust |
| Package version label | Author's release metadata | Integrity or even SemVer ordering |
| Artifact digest | Exact admitted bytes | Safety or data-migration compatibility |
| Plugin data schema | Private data interpretation | Permission to change Runtime storage |
| Runtime BuildID | Existing execution/checkpoint compatibility requirement | Compatibility of an arbitrary plugin continuation |

Keep the current Runtime/compiled-client protocol policy unless deliberately changing it. Do not burden third-party plugins with every internal protocol revision; expose a small separately versioned plugin surface. Do not implement speculative compatibility aliases around a wrong model. [F2] [F13]

### 15.2 Safe update sequence

Stage and validate the new release without altering the running one. Compare API requirements, executable targets, endpoints, data compatibility, and resource identity. Selecting it returns the installation to unapproved: the new digest requires explicit admission before it runs.

For the initial implementation, refuse a release switch when active or waiting executions depend on the old implementation unless the affected capability has a proven safe replacement contract. The user can complete or explicitly cancel that work. A catalog or view-only update still needs generation fencing and current permission checks.

"Depends on the old release" is derived from execution's own records. A Run's admitted tool contract records each callable tool reference together with its installation release, and the waiting checkpoint keeps that binding (Section 16.3). The installation owner queries those records at switch time. It does not keep a separate in-use counter, because a counter would be a second owner of a fact execution already holds. Recording the release binding in the admitted contract and checkpoint is new work, and part of the same slice as release switching.

After dependencies are quiescent, retire the old activation, confirm required cleanup, activate the candidate, and publish the selected release according to one installation transaction policy. Exact ordering of durable selection and activation intent must be recoverable: a crash may leave an explicitly pending activation, not two independently active “current” releases.

Initial policy: after a quiescent installation selects the candidate release, candidate activation failure leaves that selected release inactive with explicit diagnostics. The previous release may remain as an available artifact for an explicit rollback, but it is not silently reactivated. Rollback requires valid code, compatible data, renewed approval of that digest, and a new admitted installation operation. Different consumers must not infer different active versions.

The initial recommendation is conservative: stage first, require quiescence, and expose failed activation rather than promising seamless live rollback. Automatic fallback activation and multi-version active backend coexistence are deferred. Reinstalling old code does not undo external initialization effects.

### 15.3 Data changes

`PLUGIN_DATA` persists across updates. A plugin's private schema migration must be explicit, recoverable, and isolated from Runtime's database. A package activation must not execute arbitrary SQL against internal Runtime tables.

For data migrations with irreversible effects, perform a preflight and provide a documented backup/export or rollback policy. Merely reinstalling old code is not a safe data rollback. The migration owner records its own data transition once; the host records the installation operation and its outcome, not a duplicate copy of private business state.

First-party extraction of Memory or Schedules is a separate product migration, not an ordinary package update. It must move the coherent ownership boundary and every in-scope consumer together, including persistence and recovery references. [F5]

### 15.4 Revocation and security updates

Security revocation prevents new privileged admission immediately at the authorization owner. In-flight work is cancelled where supported and its observed effects remain recorded. Waiting work cannot resume under a revoked grant just because it was originally admitted.

A revoked UI resource should stop receiving host data and actions. Historical tool output remains readable through safe generic rendering. Do not delete evidence or silently substitute another plugin to complete outstanding work.

A compromised release may not be retained for continued execution merely to honor a dependency pin. Safety revocation supersedes availability. The resulting refusal or unknown outcome must be explicit.

### 15.5 Uninstallation

Uninstall is separate from disable. Disable stops future activation/admission according to lifetime policy while preserving installation data. Uninstall removes the package association after ensuring resources and dependencies are handled.

The default should preserve plugin-private user data for deliberate later cleanup or export unless the user explicitly chooses deletion. Historical Runtime Items retain generic content and provenance. Referenced active or waiting operations prevent unsafe removal of required artifacts; security revocation can still block their use.

Deleting files must use only paths owned by that installation. An untrusted package path or stale process record cannot authorize deletion of an arbitrary directory. Failed cleanup is reported and remains owned rather than disappearing from the installation list as apparent success.

## 16. Durable state, history, and recovery

### 16.1 State classes

| State | Recommended location/owner | Restore rule |
| --- | --- | --- |
| Installation and admitted release | Runtime installation persistence | Read the official record, then realize resources. |
| Installation approval state | Runtime installation persistence, bound to the selected digest | Read the closed state; a release change has already withdrawn approval. |
| Session/Run/Plan/Goal/Interrupt | Existing Runtime persistence | Use current restore semantics, not plugin reconstruction. |
| Plugin business data | Designated plugin backend/private store | Follow that plugin's versioned data contract. |
| Client view state | Client-local feature storage | Resolve current contribution/authority before restoration. |
| Command with unknown acknowledgement | Shared SDK's prepared record and Runtime admission evidence | Retain exact original parameters and identity. |
| External task continuation | Existing pending operation plus provider handle | Query/continue exact original authority; never assume it survived. |
| Search index or render cache | Derived cache | Rebuild from its owner; never make it a competing writer. |

No universal plugin KV store should be used to hide these distinctions. A plugin-local convenience store is acceptable for plugin-local state; it must not become a replacement Session store or permission ledger.

### 16.2 History and renderer independence

A durable result must remain meaningful without its renderer. Store accepted content, structured data when available, exact source attribution, and a bounded optional presentation reference. The renderer may be unavailable after uninstall, revocation, or version change.

Opening old history does not authorize executing old JavaScript, reinstalling an old package, contacting a removed account, or repeating a tool. When the original UI needs live data, distinguish the historical result from the current query.

If an old result's structure cannot be rendered by the current specialized renderer, use a safe generic projection or explicit unsupported-format notice. Do not reinterpret fields with a new incompatible schema and present the result as the original fact.

### 16.3 Checkpoint dependency binding

A waiting execution that depends on plugin behavior needs enough identity to restore that exact accepted contract or refuse safely. This includes relevant installation/release, source and definition identity, protocol profile, continuation material, and the existing Runtime/Scope checkpoint dependency.

The baseline already binds execution checkpoints to a BuildID and exact execution state. Plugin versioning does not remove that constraint. A new binary or plugin release must not be assumed compatible solely because its tool name and input Schema match. [F23]

Do not expand the checkpoint with a duplicate complete plugin state snapshot unless the executor truly needs it. Prefer explicit dependency references and the existing owner-controlled continuation structure. Protect any sensitive continuation material as secret-bearing state.

### 16.4 Recovery scenarios

| Event | Required recovery behavior |
| --- | --- |
| Browser reloads while a command is in progress | Reattach observations; recover the prepared command if unresolved. |
| UI frame crashes | Retire local bridge; backend work continues under Runtime ownership. |
| MCP process crashes before definite response | Record failure/uncertainty according to dispatch evidence; restart only for future availability or authorized continuation. |
| Runtime restarts with a pending question | Restore the same Interrupt and its exact pending call. |
| Backend task expires | Report expiry without inventing success or automatically starting a replacement. |
| Plugin removed after completed work | Preserve generic historical result and provenance. |
| Plugin denied after security revocation | Refuse new/resumed privileged work; retain evidence. |

### 16.5 Read models are not an event-sourcing mandate

The design does not require converting the whole product into a new plugin event log. Runtime already separates transcript, execution, checkpoints, observations, and product state. Preserve that vocabulary.

A plugin event is allowed only when it represents a real new fact with an owner, versioning rule, privacy policy, and actual consumer. UI invalidation hints should not be promoted to durable product events merely to appear uniform. Conversely, an operation required for reliable background execution cannot depend only on an ephemeral event bus.

## 17. Hooks, observers, and controlled continuation

### 17.1 Extension-point categories

| Category | Allowed behavior | Failure contract |
| --- | --- | --- |
| Contribution registration | Declare supported capabilities and resources | Roll back the rejected registration batch. |
| Input preparation | Propose a bounded pre-admission transformation | Revalidate final input and approval binding. |
| Mandatory restriction | Deny or abstain within a granted scope | Failure cannot broaden authority. |
| Execution observation | Observe start, progress, and accepted outcome | Failure is diagnostic unless a real guarantee requires otherwise. |
| Presentation transform | Produce safe model/client content | Must preserve authoritative outcome and effect evidence. |
| Pre-settlement continuation | Request bounded continuation at an explicit existing execution boundary | Executor admits or rejects; it owns budgets and lifecycle. |
| Settled notification | Observe an already committed final state | Cannot restart or rewrite the settled operation. |

The first implementation should not expose every category to untrusted code. Start with declarative contributions and safe read observations. Reuse existing Hook integration for already-supported behavior. A new executable Hook requires a demonstrated need and a precise failure/ordering contract.

### 17.2 Ordering and immutability

Plugins may not reorder compulsory guards by registering later or returning early from a general middleware chain. If the API permits input transformation, the final value must be frozen and checked before permission approval. An execute wrapper cannot secretly replace it afterward.

A final result observer sees an immutable accepted observation. A content-redaction operation must update all relevant model/client projections of the same content, including structured data if that is exposed. It must not leave a sensitive alternate field visible while claiming redaction.

An observer that requires durable delivery needs a real persistent consumer contract. The initial platform supplies no general exactly-once callback service.

### 17.3 Continuation and loop control

A plugin cannot keep a Run alive by an unconditional `continue` result. Any continuation request must enter the existing execution owner's budget, cancellation, model-selection, and context-admission rules. It is not a background loop owned by the UI.

Distinguish model request completion, tool batch completion, Run settlement, and optional maintenance completion. Pi's actionable versus settled boundaries are useful evidence for this vocabulary, but Flame should use its current Run/Segment language rather than copying Pi event names. [R4]

### 17.4 Installation from agent behavior

An agent may propose installing a plugin, but proposal text is not user authorization. Installation can execute host code, change future tools, and expose credentials. It must be a separately identified management action with a trusted confirmation path.

Do not let a plugin grant its own build-script approval, widen its own grants, or claim the conversation already contains consent. Host decisions bind exact release, requested action, and relevant targets. This requirement applies equally to an assistant-created package and a public package.

## 18. Resource bounds, performance, and observability

### 18.1 Bound real resources

| Resource | Required bound/owner | Failure behavior |
| --- | --- | --- |
| Package download/extraction | Installer byte/file/expansion limits | Refuse before activation; no partial release selection. |
| Manifest and contribution count | Package adapter limits | Typed validation/resource-limit result. |
| MCP frames and tool output | Existing codec/transport/output limits | Preserve protocol failure or bounded offload semantics. |
| Active processes and startup attempts | Runtime supervision limits | Explicit unavailable/quota status; bounded retries. |
| View bridges and queued messages | Client host limits | Reject or backpressure; do not silently drop mutations. |
| UI assets and live data | Resource broker limits | Refuse oversized resource; safe renderer failure. |
| Subscription fan-out | Existing Runtime subscription limits | Typed refusal using advertised limits. |
| Nested calls and execution budget | Scope/Runtime execution owner | Refuse additional work rather than create another scheduler. |
| Diagnostics and stderr | Redacted bounded capture | Retain truncated marker and safe artifact reference if available. |

Use existing owner-provided limits first. The draft intentionally does not invent universal production defaults from unmeasured workloads. For newly introduced resources, hard limits and safe behavior are mandatory before release, with selected values documented in their owning configuration and advertised where clients must reason about them. Absence of a chosen UI queue limit is a release blocker, not permission to ship an unbounded queue.

### 18.2 Backpressure

Preserve the existing distinction between authoritative execution facts and expendable previews. A slow plugin observer cannot require unbounded buffering of every token. Bounded previews may be dropped only under an explicit contract; committed facts and mutation acknowledgements must not silently disappear as if delivered.

A blocked external process must not prevent unrelated client presentation from reading already durable state. Avoid holding global plugin activation locks across external I/O or long tool execution. Scoped queues should have explicit cancellation and retirement behavior.

### 18.3 Measure before optimizing

Compare the existing local path and proposed adapters using representative short reads, patch operations, network tools, large structured results, multiple simultaneous Session invocations, and cold/warm startup. Measure latency percentiles, memory, process count, copied bytes, output offloading, and shutdown behavior.

Do not assert that IPC is negligible because model calls are slow. Do not introduce in-memory bypasses for external tools unless measurements establish a problem and the bypass preserves exactly the same owner and policy contract. A production fast path with different validation is a second maintenance path, not a successful optimization. [F1]

### 18.4 Diagnostics

A plugin diagnostic should identify installation, release/digest, activation generation, component, target Runtime, safe error category, and relevant logical invocation. The UI should distinguish installed, desired-enabled, denied, waiting for authentication, backend failure, and local renderer failure without conflating them.

Reconstruct diagnostic views from their owning records and live resource observations. Do not maintain an independently editable “capability ledger.” Persist only evidence required for recovery, audit, or a real management workflow.

Traces can correlate invocation, backend request, and view action without logging secrets or complete business content. High-cardinality identifiers should be evaluated against the existing telemetry policy. Metrics are not a source of product completion truth.

## 19. Migration boundaries for existing Flame capabilities

### 19.1 Migration means moving ownership, not just files

A feature is extracted only when its public behavior, data owner, callers, lifetime, and verification are understood. A new package name or an MCP wrapper is not evidence of a simpler design.

A complete migration removes the superseded model-facing registration, duplicate configuration, obsolete projections, retired public shape, aliases, and fallbacks in the same owning change. Persisted data is migrated explicitly in that change. No compatibility layer is kept for a design being replaced. Do not preserve two active writers while waiting for a future cleanup. [F2] [F4] [F5]

The table below is a proposed boundary selection, not an exhaustive capability inventory or evidence that any extraction has been implemented.

| Existing capability | Recommended migration unit | Retained semantic owner | Readiness condition |
| --- | --- | --- | --- |
| Specific Skills and resources | Portable content package | Existing Skill parsing, source admission, and loading | Provenance, conflicts, bounds, and immutable package handling |
| Jina/Tavily tools | Remain built-in; optional result renderer only | Shared tool policy, grants, credentials, outcome recording | Extraction only on a demonstrated deployment need, with the same contracts and failure certainty through MCP |
| General HTTP tool | Optional integration capability | Host network/credential policy and effect interpretation | No broader targets or unsafe replay introduced |
| A2A integrations | External-agent connection and tool presentation | Scope A2A contract and Flame external-effect interpretation | No claim that remote jobs are local Scope children |
| Themes | Declarative resource package | Existing graphical appearance owner | Safe tokens, deterministic conflict handling, cleanup; package locales deferred (Section 21.2) |
| Tool previews/file renderers | Optional presentation contribution | Stored result/file authority | Safe generic fallback and revoked-renderer behavior |
| Timeline/Usage/diagnostic views | Read-only view/action package | Runtime trajectory, invocation and accounting records | Queries remain shared, bounded, and source-attributed |
| Enhanced Diff UI and exports | View and format renderer | Runtime mutation, checkpoint, import and restore semantics | Export does not acquire unsafe restore authority |
| Language services and analysis | Config/adapter plus diagnostic view first | Existing workspace/process authority | Scoped process lifecycle, versioned inputs, safe mutation path |
| Memory view and extraction strategy | UI and candidate-generation policy first | Current memory read/publish/curation owners | Candidate versus committed memory stays distinct |
| Skill mining and proposal UX | Suggestion strategy and interface | Proposal review and publication owner | No automatic mutation of installed content |
| Schedules UI/templates/triggers | Product commands and presentation first | Schedule/Occurrence persistence and Run admission | No client-side scheduler or new replay identity |
| Plan/Goal controls | Optional presentation and sanctioned actions | Existing Runtime Plan/Goal use cases | Exact execution provenance and authority |
| Question/delegation/search control tools | Remain existing execution adapters initially | Scope/Runtime execution and Interrupt owners | Any future remote form proves equivalent recovery |
| Provider implementations | Consider separately if deployment requires it | Released Scope provider contracts and Runtime selection | Do not disguise primary model invocation as an MCP tool |

The current assembly, online integration, MCP configuration, Skill management, memory use cases, and scheduling worker support these ownership distinctions. [F14] [F15] [F24] [F25] [F26] [F27] [F28]

### 19.2 First complete backend package: an external Agent Plugins package

The first backend package is an existing, third-party-style Agent Plugins package with Skills and an `mcp.json` declaration. It is not a first-party feature moved behind a process boundary. Such a package exercises admission, release binding, `PLUGIN_ROOT` / `PLUGIN_DATA`, installation-origin MCP records, declared secret inputs, deferred tool exposure, tool-reference-keyed approval, and provenance-aware Skill sources. Every one of those is a boundary the platform must own. It needs no new execution semantics.

Web search and page fetching stay built-in. Extracting them would add the IPC failure window that Section 4.3 rejects and would give users nothing new. If a deployment need later justifies extraction, the current distinction between read-only external queries and generic HTTP mutations must survive. The existing online assembly deliberately does not treat arbitrary HTTP POST/DELETE failures as definite no-effect reads. The direct registration is deleted in the same change. [F15]

Do not create separate packages for each field, icon, and converter.

### 19.3 First graphical package: trajectory analysis

An optional timeline/analysis view can read Runtime's existing trajectory and model-invocation projections. It owns filtering, display layout, and derived presentation metrics. It must not keep another official execution journal or recalculate missing provider evidence as fact.

The same data remains accessible to CLI and IDE through existing queries. A lost renderer affects only rich presentation. A failed query is shown as a failed read, not an empty history.

Start by removing direct imports into the feature's private implementation and exposing only the existing appropriate read/action contracts. Do not externalize a component by exporting all of its private React hooks.

### 19.4 Memory extraction boundary

The current memory read model combines project/user data and optional semantic signals; curation owns a ledger and compare-and-swap publication of a generation. Those are distinct responsibilities with existing authoritative transitions. [F25] [F26]

Initial extensions may generate candidates, rank results through an explicit strategy seam, or render the current state. Commit/review/publish decisions remain with the existing owner. An empty memory set and a failed memory read must remain distinguishable.

Whole-domain extraction requires moving ledger, watermark, generation publication, review, data migration, maintenance lifetime, and context-consumption contracts coherently. A plugin maintaining one “official memory” while Runtime maintains another is not an acceptable intermediate production design.

### 19.5 Scheduling extraction boundary

The existing scheduling worker claims occurrences durably and retries pending dispatch using preserved Session/Run identity. It is not equivalent to a browser timer that resubmits a prompt. [F27]

First extract UI and templates. A later scheduling module must own Schedule and Occurrence coherently and call Runtime's admitted run-start contract. It needs restart, duplicate-trigger, lost-acknowledgement, cancellation, disable/uninstall, and data-migration behavior before becoming optional.

A plugin-visible timer or event notification must not be allowed to pretend it supplies this durable scheduling contract. Keep the default first-party worker wired until a complete replacement is ready.

### 19.6 Implementation placement

Use current owners and repository vocabulary rather than inventing a required plugin-layer matrix.

| Area | Justified change |
| --- | --- |
| Runtime application/integration | Cohesive installation/admission and capability-lifecycle use cases |
| Runtime domain | Only new values/aggregates that protect genuine installation or authorization invariants |
| Runtime adapters/infrastructure | Portable package translation, confined asset access, existing MCP/process integration |
| Runtime protocol/catalog | Proposed product operations, descriptors, errors, and advertised capabilities |
| Runtime shared TypeScript client | Generated validation, typed operations, command preparation and shared observation |
| Graphical host/SDK | Serializable external-descriptor adapter and isolated view broker |
| Existing graphical feature owners | Remove direct coupling and consume their existing public reads/actions |
| CLI and IDE | Shared-capability discovery and their own interaction adapters |
| Scope libraries | Reusable tool/MCP/execution changes only, released before Flame depends on them |

Do not create packages named `manager`, `service`, `impl`, `common`, or `repository` simply to populate this table. Namespace depth and package shape must follow the existing repository rules. A single concrete implementation is sufficient unless a real boundary or variation justifies an interface. [F2]

## 20. End-to-end behavior and acceptance

### 20.1 Full-stack review package

**Intent:** A user installs a review package, opens a board, changes one review, and reads the result in CLI.

```text
User -> Runtime: install selected release into this Runtime
Runtime: stage, validate, request authority, admit release
Runtime -> MCP: activate approved review service
MCP -> Runtime: tool descriptors
Runtime: admit tools and resolve view/action references
Desktop -> Runtime: read accepted contributions
Desktop: create isolated board and scoped bridge
Board -> Host: request list action
Host -> Runtime: authorized action with bound workspace
Runtime -> MCP: invoke existing list tool
Runtime -> Host -> Board: structured review data
User -> Board: resolve one review based on revision r3
Host/SDK: prepare exact mutation once
Runtime -> MCP: admitted update with stable logical identity
MCP: commit review update once or return a defined conflict
CLI -> Runtime: read the same plugin's review data
```

Acceptance requires one backend resource and one business object owner, not identical UI. A stale review revision yields a conflict. Closing the board during the mutation leaves the accepted operation owned by Runtime. Losing acknowledgement leaves an unresolved prepared record rather than generating a new command.

### 20.2 Remote Runtime and local desktop

**Intent:** macOS Desktop connects to a Linux Runtime and loads a plugin page with a Go backend.

The Linux Runtime selects the Linux artifact, owns `PLUGIN_ROOT` / `PLUGIN_DATA`, starts the backend, and resolves Runtime workspaces. The Desktop receives only permitted UI resources and data through its connection. Local save/open requests use the macOS host adapter and explicit user authorization.

A plugin reading `/workspace/project` on Linux must not have its path interpreted on macOS. Switching to another Runtime retires the prior bridges and observations. The old package's cleanup cannot remove the new view, and an old callback cannot use the new connection.

Acceptance requires separate environment identities and no token in HTML, URLs, or frame storage. It does not require speculative multi-server replication.

### 20.3 Durable question and continuation

**Intent:** A tool needs user input, the graphical client closes, and the user later answers from a supported client.

For the current host tool, retain existing Interrupt behavior. For a future MRTR backend, Runtime must bind and persist the exact interim state before safely suspending the execution. The later answer enters the original Interrupt owner, not the old frame. Continuation uses the original backend/release authority and logical invocation with a new transport request ID as appropriate.

Acceptance requires no duplicate question, no repeated pre-question mutation, safe refusal after revocation or incompatible backend replacement, and no reliance on a permanently blocked RPC. A client unable to answer the required interrupt cannot silently resume a weakened Run.

### 20.4 Update while work is outstanding

**Intent:** A user updates a package with an active or waiting operation.

The candidate can be staged and inspected. Initial policy rejects activation if the old release remains required. It does not terminate work silently or switch the resumed invocation to new behavior. A user-authorized cancellation follows existing cancellation semantics and does not assume rollback.

After dependencies are released and cleanup is complete, activation can proceed. A failure reports the exact desired/active state. A previous release may be restored only under the documented activation/data compatibility policy, not merely because its files still exist.

### 20.5 Acceptance matrix

The scenarios below are proposed tests, not executed results. Prioritize actual product boundaries. Multi-client cases here are justified by the explicit multi-client plugin requirement; they do not imply building a multi-Runtime cluster test suite.

| ID | Scenario | Observable acceptance |
| --- | --- | --- |
| P01 | Minimal portable manifest | Accepted without inventing missing component errors. |
| P02 | Unknown root field | Diagnostic and ignored field; otherwise valid package remains loadable. |
| P03 | Invalid required manifest field | No component executes. |
| P04 | Unsupported extension namespace | Ignored without validating its internal value. |
| P05 | Bad supported Flame namespace | Dependent contributions refused; independent valid portable components retain their defined behavior. |
| P06 | One invalid MCP server among valid siblings | Only that server is excluded; catalog and diagnostics are truthful. |
| P07 | Package path/junction escapes root | Read/execute denied before effect. |
| P08 | Oversized archive or resource | Bounded refusal, owned staging cleanup, no selected half-release. |
| P09 | Reserved environment keys and expansion | Standard rejection/expansion behavior, no arbitrary interpolation. |
| P10 | Update with private plugin data | Data retained; no implicit migration execution. |
| P-O01 | User MCP update targets an installation-origin record | Refused with the installation-ownership category; record unchanged. |
| P-O02 | Installation release with one invalid MCP server | Siblings activate; Runtime restart is unaffected. |
| P-O03 | Release changes its digest | Standing approvals for its tools become stale through the fingerprint, and the installation returns to unapproved. |
| P-O04 | Plugin activation mid-Session | Tool declaration block of later Runs byte-identical; tools reachable through deferred discovery and listed in the later Run's trailing catalog message. |
| P-O05 | Release switch while a waiting Run's checkpoint binds the old release | Switch refused from execution records, without a separate counter. |
| P-O06 | User server and installation server named `reviews` project the same tool name | Both are excluded with a diagnostic naming both identities. |
| I01 | Duplicate contribution identity | Registration batch rejected without shadowing. |
| I02 | Familiar built-in name from third party | No inherited privilege or orchestration behavior. |
| I03 | Late activation completion | Retired generation cannot publish contributions. |
| I04 | Cleanup after replacement | Only predecessor-owned resources are removed. |
| I05 | Activation failure after partial registration | No leaked valid-looking partial contribution set. |
| I06 | Cleanup waits on host state | No join under a lock/readiness condition that deadlocks cleanup. |
| T01 | Local and MCP tool use same public behavior | Same relevant schema, policy, result, and diagnostic contract. |
| T02 | Input transform after user inspection | Final input revalidated and approved before dispatch. |
| T03 | Nested tool call | Same applicable grants, policy, cancellation, and parent attribution. |
| T04 | Two Sessions use one MCP process | No context, working-directory, result, or credential crossover. |
| T05 | Mutation succeeds then connection fails | Preserve unknown acknowledgement; no blind new operation. |
| T06 | Cancellation races completion | Record actual completion or uncertainty; do not claim rollback. |
| T07 | Incomplete MCP result | Not exposed as a complete tool result; supported continuation or explicit unsupported behavior. |
| T08 | Invalid structured output | Contract failure with correct certainty, not a successful empty result. |
| T09 | Tool catalog changes during a waiting Run | Original accepted dependency is honored or restore refuses explicitly. |
| T10 | Remote task expires or disappears | Explicit external outcome limitation; no automatic replacement job. |
| A01 | Forged view principal or target | Host binding rejects it regardless of payload labels. |
| A02 | Grant revoked before dispatch | Dispatch denied through the broker/admission boundary. |
| A03 | Grant revoked after effect begins | Stop intent where supported; retain effect evidence. |
| A04 | Updated endpoint/issuer requests old secret | Credential binding prevents unauthorized reuse. |
| A05 | Required policy failure | No fallback to allow. |
| A06 | Agent proposes self-install or grant expansion | Separate trusted authorization required. |
| U01 | Backend ready, Desktop renderer fails | CLI/tool behavior remains available; page reports local failure. |
| U02 | Frame attempts host DOM/storage/native access | Access blocked on each advertised carrier. |
| U03 | Opaque-origin message from wrong frame | Source/channel identity rejects it. |
| U04 | Preload or view restoration | Does not dispatch a mutating action. |
| U05 | Result already supplied initial data | No duplicate tool execution for first render. |
| U06 | View closes while mutation is accepted | Local observation retires; Runtime/journal still owns completion. |
| U07 | Runtime connection replaced | Old bridge cannot invoke or publish into successor. |
| U08 | Unsupported CLI/IDE rendering | Generic operation/result or explicit unsupported presentation. |
| U09 | Uninstall/revoke then open history | Safe generic content; no automatic old-code execution. |
| U10 | Malicious deep link or filename | No automatic install, credential use, path escape, or privileged navigation. |
| F01 | Workspace pathname is rebound | No operation gains authority over a different directory. |
| F02 | Patch contains invalid/multiple targets | Shared parsing, exact approval targets, correct actual effect evidence. |
| F03 | Shell wrapper leaves descendants | Stop/join behavior matches declared process ownership. |
| D01 | Question/Plan/Goal restore | Existing product invariant and provenance retained. |
| D02 | Delegated child waits or cancels | One Scope tree; correct parent identity and accounting. |
| D03 | Schedule start acknowledgement is lost | Preserved occurrence and Run identity; no duplicate firing. |
| D04 | Memory proposal/curation races publication | Only the owning accepted generation becomes official. |
| V01 | Upgrade with active dependency | Candidate staged; unsafe activation refused. |
| V02 | Data migration or activation fails | Honest selected/active/data state; no fictitious rollback. |
| V03 | Uninstall cleanup fails | Resource remains owned and diagnosable. |
| R01 | Runtime restart after accepted mutation | Recover from durable evidence and exact original identity. |
| R02 | Process-local notification was missed | Readable projections refetch correctly; no false exactly-once claim. |
| R03 | Slow/malicious plugin output | Bounded queues and correct backpressure/error contract. |

Not every test belongs in the first change. Each implementation slice must pass the scenarios protecting the behavior it changes. Deferred features stay unavailable until their relevant scenarios pass.

### 20.6 Repository verification commands

When implementation is authorized, use the repository's actual gates from the owning modules. The following commands are documented by the current development workflow; this proposal has not run them. [F4]

```sh
(cd runtime && \
  GOWORK=off go test ./... && \
  GOWORK=off go vet ./... && \
  GOWORK=off go build ./...)

(cd runtime/contract/typescript && npm run check)
(cd desktop/frontend && npm run check)
(cd ide && npm run check)

(cd cli && \
  GOWORK=off go test ./... && \
  GOWORK=off go vet ./... && \
  GOWORK=off go build ./...)

git diff --check
```

When the Runtime protocol catalog changes, regenerate it from its owner:

```sh
(cd runtime && go generate ./...)
```

Run these commands from the repository root after required dependencies are installed. Regenerate changed contracts before dependent validation. These are module gates, not a prescription to run unrelated checks or overwrite a user's environment. Inspect the worktree and scope first. Native Wails targets use `desktop/Taskfile.yml`; a browser build does not prove native isolation. Use targeted race tests only for changed concurrent ownership and real PTY tests only for terminal behavior. Keep the default suite offline and credential-independent.

### 20.7 Release acceptance

The initial feature can ship only after Slice 0 has landed and an actual theme/content package, external backend tool package, and optional graphical view complete their end-to-end paths. Package admission, scope/permission checks, resource limits, explicit failure, cleanup, and supported client behavior must be implemented, not simulated only in mocks.

There must be no second production tool definition or business-state writer left behind for an extracted feature. All affected protocol generation, shared client types, callers, and current documentation must agree. Unsupported advanced features must fail explicitly rather than appear partially functional.

## 21. Bounded open decisions and implementation slices

### 21.1 Decisions that block the first public contract

| Decision | Recommended default | Evidence needed before finalization |
| --- | --- | --- |
| Production extension namespace | One stable project-controlled reverse-domain namespace | Confirm domain/identifier ownership and avoid future renaming. |
| Initial UI carrier | Isolated, host-brokered HTML using an existing suitable bridge | Verify Wails and browser resource origin, CSP, native injection, and disposal. |
| Native third-party trust model | Explicit trusted executable; confinement only when enforced | Platform tests and truthful user-facing access description. |
| Minimal action/read allowlist | Only operations needed by the first packages | Trace concrete consumers and existing delivery/admission behavior. |
| New resource-limit values | Conservative measured values at owning boundaries | Representative offline tests; advertised limits where clients rely on them. |
| Exact public operation/schema names | Generated from Runtime catalog after design review | Binding parity, shared client generation, and conflict-free vocabulary. |
| Credential binding for the first MCP package | Reuse existing secret/OAuth owner | Prove endpoint/issuer changes cannot reuse inappropriate authority. |
| Tool reference wire and storage shape | One closed union over built-in, MCP, and A2A sources | Approval, exposure, and evidence consumers trace to it; the migration of existing rules is defined. |

These are bounded release decisions. They do not reopen the ownership model or require building a general extension framework before a first package can work.

### 21.2 Explicitly deferred capabilities

| Deferred capability | Trigger to reconsider | Constraint that must survive |
| --- | --- | --- |
| Complete stdio conversion | Demonstrated deployment benefit and successful host-control prototypes | No second execution tree, permission model, file authority, or state writer. |
| Extraction of first-party online tools | A deployment that needs them outside the Runtime binary | Same failure certainty; direct registration removed in the same change. |
| Public marketplace and signature trust hierarchy | A real public distribution workflow | Established security primitives and explicit grant renewal. |
| Generic executable third-party Hook pipeline | A concrete extension not expressible by supported capabilities | Non-bypassable restrictions and immutable committed facts. |
| Durable MCP MRTR/Tasks | A required backend with supported SDK behavior | Original invocation, Interrupt ownership, expiry and unknown-effect semantics. |
| Multi-version live backends | Real requirement to update without draining work | Exact dependency pinning and bounded resource retention. |
| Whole Memory or scheduling extraction | Stable plugin data and lifecycle contracts | Move the coherent semantic owner, not only its tool wrapper. |
| Arbitrary plugin dependency graph | More than bundled self-contained releases are required | Deterministic resolution; no ambient shared services or hidden privilege inheritance. |
| Native client-local plugin execution | A capability cannot run at the Runtime environment | Explicit execution location and client-specific authority. |
| WASM/embedded JS runtime | Measured distribution or enforceable-isolation need | Reuse mature runtime; do not add a second agent framework. |
| General UI slot takeover | Proven custom-product requirement | Protect trusted chrome, approvals, and recovery controls. |
| Package locale contributions | A concrete package that needs localization | Host-owned dictionary activation and lifetime; a package never replaces or merges another source's text. |
| Package capability requests and host grants | A contribution that consumes host-brokered operations (Slice C) | Requests are not grants; each grant has an enforcement point at the broker boundary and is never a second tool-approval store. |

A deferred capability is unavailable, not a placeholder implementation returning success. Its reconsideration must start from the real consumer and acceptance evidence.

### 21.3 Recommended implementation slices

The current working implementation stops at Slice A/B. Earlier action/view prototypes
have been withdrawn with their public API, client entrypoints and bridge. Their contracts
must be established after the carrier spike rather than retained as compatibility surfaces.

**Slice 0: tool identity and policy ownership.** Implement [`tool-identity-and-policy-ownership.md`](tool-identity-and-policy-ownership.md) and pass its acceptance tests. It is a breaking repair of the current product, valuable without plugins, and a prerequisite for every later slice.

**Slice A: package admission and declarative resources.** Establish portable loading, provenance, release/data separation, installation inspection, and themes as the one admitted declarative presentation contribution, whose presentation Runtime decides and publishes per installation. Package locales are deferred (Section 21.2). Verify malformed packages, narrow failure boundaries, permission display, and cleanup.

**Slice B: one external Agent Plugins package.** Land the Slice B rows of Section 3.5. Admit a package with Skills and `mcp.json` through installation-origin registry records, declared secret inputs, provenance-aware Skill sources, and deferred exposure (Sections 9.8 and 19.2). No first-party tool is extracted. Verify P-O01 through P-O04 and P-O06.

**Carrier spike, before Slice C's contract.** In the Wails v3 build in use and in the browser build, prototype a sandboxed frame that receives brokered bytes. Determine native-binding injection into child frames, custom-resource origin, CSP enforcement, navigation escape, and teardown. The bridge contract in Section 13 is finalized only from that evidence. A carrier that cannot enforce the boundary reports the renderer unavailable (Section 13.7).

The reproducible browser and native gates now live in [Desktop's carrier acceptance](../desktop/README.md#plugin-carrier-acceptance), which owns their commands and current carrier findings. The network gate includes WebRTC with packet observations outside the frame; a fetch-only CSP check does not establish network isolation. A failed carrier gate blocks that carrier's Slice C admission. Repair the executing carrier's authority boundary before introducing the public view/bridge contract; JavaScript global replacement is not an enforcement owner.

**Slice C: one optional graphical page.** Load an isolated read-oriented view through the existing Dougong host and shared client. Verify native/browser boundaries, connection replacement, initial-result reuse, and local realization failure without backend state changes.

**Slice D: broaden proven surfaces.** Add necessary action forms, result renderers, IDE hosting, or language integrations only after their consumers are concrete. Preserve one definition and one policy path.

**Slice E: durable host integrations, only as justified.** Validate MRTR, Tasks, delegated execution, or whole-subsystem extraction individually. These are not prerequisites for the base heterogeneous plugin model.

Each slice is the smallest complete boundary repair. It includes affected callers, generated contracts, tests, and documentation. It is not an instruction to make commits, install packages, or modify the repository as part of this document-only request.

## 22. Architectural consequences

The proposed system makes independent capability delivery possible without weakening Flame's existing product model. It preserves Go for Runtime and reusable backend implementations, JavaScript for graphical views, and other languages where the external protocol already provides a useful boundary.

Its principal benefit is not a smaller count of adapters. It is a smaller count of competing semantic owners. One tool contract can have local and remote executors. One business object can be manipulated by a tool, a page, and a CLI command. One plugin package can have backend capabilities and optional views without requiring every client to run the same component tree.

The cost is explicit admission, asset authority, resource lifetime, versioning, and failure semantics. Those costs already exist whenever third-party code and multiple clients are involved; making them explicit is preferable to hiding them behind dynamic imports or broad RPC tunnels.

The selected design therefore has four durable boundaries: **Scope owns execution contracts; Runtime owns product facts and admission; clients own interaction and presentation; external providers own only the business facts actually delegated to them.** A plugin may contribute across these boundaries, but it may not dissolve them.

## Appendix A. Review questions

Review the proposed design by tracing one real operation rather than scoring a pattern checklist.

| Question | Satisfactory answer |
| --- | --- |
| Which object can advance each fact? | Exactly one named domain/use-case/provider owner. |
| What is gained by a new interface or process? | A demonstrated translation, authority, lifecycle, or substitution boundary. |
| Which data is merely a projection? | Catalogs, UI state, caches, and wire values have no competing commit path. |
| What happens after partial failure? | The design names actual effects, unknown outcomes, and the owner that retains them. |
| What can a malicious plugin do? | The answer matches enforced OS/UI boundaries and granted APIs, not a manifest promise. |
| How does a different client behave? | It shares business capabilities and expresses supported interaction without fabricating UI parity. |
| Can a waiting execution resume? | It uses exact dependencies and authority or receives an explicit refusal. |
| What obsolete path is removed? | The migration eliminates a duplicate definition/transition rather than adding a permanent wrapper. |
| What is not implemented? | Deferred capabilities remain visibly unavailable and have clear reconsideration triggers. |

## Appendix B. Sources and provenance

Repository links below are pinned to the analyzed revision unless explicitly noted. Specification links were checked on 2026-09-30. Their contracts are incorporated by reference; this draft does not replace upstream documentation. Source excerpts are summarized rather than copied wholesale.

### Flame repository

| Reference | Source | Evidence used |
| --- | --- | --- |
| F1 | [AGENTS.md][F1] | Priorities, single-writer ownership, abstractions, verification, and evolution |
| F2 | [PROJECT_RULES.md][F2] | Exact product ownership, explicit construction, package discipline, and documentation language |
| F3 | [DESIGN_PHILOSOPHY.md][F3] | Behavior-rich owners and one semantic execution path |
| F4 | [DEVELOPMENT.md][F4] | Dependency discipline, verified commands, and documentation scope |
| F5 | [REFACTORING.md][F5] | Complete boundary migration and risk-based verification |
| F6 | [Runtime documentation policy][F6] | Current-system-only architecture baseline |
| F7 | [Graphical architecture][F7] | Existing Dougong host, feature ownership, cleanup, and generation fencing |
| F8 | [Workspace contribution types][F8] | React component-based internal view contracts |
| F9 | [IDE README][F9] | Shared client, prepared commands, scope, and native interaction |
| F10 | [Protocol capabilities][F10] | Execution requirements versus suppressible presentation data |
| F11 | [Filesystem tool authority][F11] | Retained directory handle and path-rebinding checks |
| F12 | [Deferred tool discovery][F12] | Execution-owned tool advertisement |
| F13 | [Runtime README][F13] | Public bindings, replay, lifecycle, protocol, and BuildID constraints |
| F14 | [Tool environment assembly][F14] | Existing capabilities, Scope reuse, process and sandbox composition |
| F15 | [Online tool assembly][F15] | Jina/Tavily/HTTP integration and failure distinctions |
| F16 | [Tool interpretation][F16] | Trusted policy and canonical product projections |
| F17 | [Built-in behavioral descriptors][F17] | Safety, rendering, orchestration, and outcome metadata |
| F18 | [Prospective mutation paths][F18] | Shared mutation reporting versus actual effects |
| F19 | [Execution context][F19] | Session, workspace, model, isolation, and Goal provenance |
| F20 | [Plan tool adapter][F20] | Model input translated into the existing Plan use case |
| F21 | [Question tool][F21] | Durable Interrupt-based interaction |
| F22 | [Delegate binding][F22] | Parent-scoped identity and continuation relationships |
| F23 | [Execution checkpoint][F23] | Exact execution state and dependency binding |
| F24 | [MCP product protocol][F24] | Existing connection, credential, lifecycle, and authorization vocabulary |
| F25 | [Memory read model][F25] | Project/user scope, optional embedding, and derived cache |
| F26 | [Memory curation][F26] | Ledger, watermark, and generation publication |
| F27 | [Schedule worker][F27] | Durable occurrence claims and preserved Run dispatch identity |
| F28 | [Skill use cases][F28] | Discovery limits, name uniqueness, curation and proposal review |
| F29 | [Graphical extension points][F29] | Existing supported internal contribution vocabulary |
| F30 | [Built-in graphical composition][F30] | Current feature/plugin arrangement |
| F31 | [Runtime module dependencies][F31] | Analyzed released Scope dependency baseline |
| F32 | [Runtime architecture][F32] | Lifecycle, persistence, resource and effect boundaries |
| F33 | [MCP server record][F33] | Bare-name identity; descriptor, secrets, enablement, and tool policy in one record; lossy tool-name projection |
| F34 | [Approval call gate][F34] | Remembered rules and MCP auto-approve merged by `ResolvePromptShortcuts` |
| F35 | [SQLite schema][F35] | `approval_rules.tool` keyed by model-visible name; MCP tool policies cascade with their server |
| F36 | [Interaction tool authorization][F36] | Safety class and approval queried by model-visible name; MCP auto-approve by `ToolRef` |
| F37 | [MCP dial][F37] | Startup rejection of duplicate or invalid stored server records |

### Scope MCP at the Flame dependency version

| Reference | Source | Evidence used |
| --- | --- | --- |
| S1 | [MCP package contract][S1] | Adapter scope, content, failure and incomplete-result limitations |
| S2 | [Tool discovery][S2] | Metadata, name projection, concurrency policy, and remote contract admission |
| S3 | [Server registration][S3] | Scope-to-MCP conversion and local invocation reconstruction |
| S4 | [Remote result adapter][S4] | Incomplete results, output validation, and failure mapping |

These links use the released `mcp/v0.40.0` tag referenced by the analyzed Flame module. They are not a claim that this is the newest Scope release. A real implementation must inspect the selected release graph and make any needed upgrade explicitly. [F31]

### Portable and protocol specifications

| Reference | Source | Evidence used |
| --- | --- | --- |
| A1 | [Agent Plugins 1.0.0 specification][A1] | Portable package contract and scope |
| A2 | [Plugin manifest guide][A2] | Root metadata, version selection, and validation exceptions |
| A3 | [Client extensions guide][A3] | Namespaced manifest data and extension directories |
| A4 | [Loading and discovery][A4] | Narrow component failure boundaries |
| A5 | [MCP server configuration][A5] | Transport declarations, paths, variables, and credential limits |
| A6 | [MCP runtime integration][A6] | Native configuration mapping, launch and failure behavior |
| A7 | [Portable manifest JSON Schema][A7] | Portable schema-level validation of the manifest example |
| A8 | [Portable MCP JSON Schema][A8] | Schema-level validation of the MCP example |
| M1 | [MCP 2026-07-28 specification][M1] | Versioned protocol baseline |
| M2 | [MCP 2026-07-28 changes][M2] | Per-request metadata, discovery, and modern protocol differences |
| M3 | [MCP stdio transport][M3] | Framing, message direction, and process lifecycle |
| M4 | [Multi round-trip requests][M4] | Interim input requests and continuation state |
| M5 | [Tasks extension overview][M5] | Provider task handles and cooperative cancellation |
| M6 | [MCP Apps overview][M6] | UI resource and host-message bridge pattern |

Agent Plugins documentation identifies its license as CC BY 4.0. This draft attributes the referenced contract and supplies Flame-specific proposed behavior; it does not reproduce the specification as a replacement standard. [A2]

### Reference-project design evidence

| Reference | Pinned source | Evidence used |
| --- | --- | --- |
| R1 | [DeepSeek Harness architecture][R1] | Capability providers/consumers and composition |
| R2 | [DeepSeek tool runtime][R2] | Guarded tool pipeline and frozen result observation |
| R3 | [DeepSeek client modules][R3] | Backend/browser contribution realization and generation handling |
| R4 | [Pi extensions][R4] | Author API, exposure, nesting, state, lifecycle and modes |
| R5 | [Pi tool wrapper][R5] | Execution-context adaptation only; nested-call admission evidence is in R4 |
| R6 | [OpenCode v2 plugin API][R6] | Domain transforms and runtime hooks |
| R7 | [OpenCode v2 activation][R7] | Revision-compared activation; stale failures cannot disable a replacement; cleanup outside the lock |
| R8 | [OpenCode v2 TUI contract][R8] | Stable view/slot and client-local state boundaries |
| R9 | [Codex resource authority][R9] | Environment-bound plugin resources |
| R10 | [Codex tool runtime][R10] | Shared executor contract with local/MCP differences |
| R11 | [Codex App Server][R11] | UI metadata in tool-call events and history |
| R12 | [OpenCode v2 derived state][R12] | Rebuildable contribution state; batching is not rollback |

### Document maintenance

Keep this external draft as a review artifact. Do not add it to the active Runtime architecture baseline as if implemented. After approving and shipping a slice, update the relevant current architecture and generated contracts, and retire superseded proposal text rather than maintaining parallel normative descriptions. This document's Markdown and PDF are two representations of the same review content, not independent editable specifications.

[F1]: https://github.com/Tangerg/flame/blob/1083cab5794d6bcc252bdd27dac4b23ecb8a5940/AGENTS.md
[F2]: https://github.com/Tangerg/flame/blob/1083cab5794d6bcc252bdd27dac4b23ecb8a5940/PROJECT_RULES.md
[F3]: https://github.com/Tangerg/flame/blob/1083cab5794d6bcc252bdd27dac4b23ecb8a5940/DESIGN_PHILOSOPHY.md
[F4]: https://github.com/Tangerg/flame/blob/1083cab5794d6bcc252bdd27dac4b23ecb8a5940/DEVELOPMENT.md
[F5]: https://github.com/Tangerg/flame/blob/1083cab5794d6bcc252bdd27dac4b23ecb8a5940/REFACTORING.md
[F6]: https://github.com/Tangerg/flame/blob/1083cab5794d6bcc252bdd27dac4b23ecb8a5940/runtime/doc/README.md
[F7]: https://github.com/Tangerg/flame/blob/1083cab5794d6bcc252bdd27dac4b23ecb8a5940/desktop/frontend/ARCHITECTURE.md
[F8]: https://github.com/Tangerg/flame/blob/1083cab5794d6bcc252bdd27dac4b23ecb8a5940/desktop/frontend/src/plugins/sdk/types/workspace.ts
[F9]: https://github.com/Tangerg/flame/blob/1083cab5794d6bcc252bdd27dac4b23ecb8a5940/ide/README.md
[F10]: https://github.com/Tangerg/flame/blob/1083cab5794d6bcc252bdd27dac4b23ecb8a5940/runtime/protocol/capabilities.go
[F11]: https://github.com/Tangerg/flame/blob/1083cab5794d6bcc252bdd27dac4b23ecb8a5940/runtime/internal/adapter/toolset/filesystem.go
[F12]: https://github.com/Tangerg/flame/blob/1083cab5794d6bcc252bdd27dac4b23ecb8a5940/runtime/internal/adapter/toolset/discovery.go
[F13]: https://github.com/Tangerg/flame/blob/1083cab5794d6bcc252bdd27dac4b23ecb8a5940/runtime/README.md
[F14]: https://github.com/Tangerg/flame/blob/1083cab5794d6bcc252bdd27dac4b23ecb8a5940/runtime/internal/adapter/toolset/build.go
[F15]: https://github.com/Tangerg/flame/blob/1083cab5794d6bcc252bdd27dac4b23ecb8a5940/runtime/internal/adapter/toolset/online.go
[F16]: https://github.com/Tangerg/flame/blob/1083cab5794d6bcc252bdd27dac4b23ecb8a5940/runtime/internal/adapter/toolset/interpreter.go
[F17]: https://github.com/Tangerg/flame/blob/1083cab5794d6bcc252bdd27dac4b23ecb8a5940/runtime/internal/adapter/toolset/descriptors.go
[F18]: https://github.com/Tangerg/flame/blob/1083cab5794d6bcc252bdd27dac4b23ecb8a5940/runtime/internal/adapter/toolset/mutation_paths.go
[F19]: https://github.com/Tangerg/flame/blob/1083cab5794d6bcc252bdd27dac4b23ecb8a5940/runtime/internal/adapter/executionctx/scope.go
[F20]: https://github.com/Tangerg/flame/blob/1083cab5794d6bcc252bdd27dac4b23ecb8a5940/runtime/internal/adapter/toolset/builtin/plan_set.go
[F21]: https://github.com/Tangerg/flame/blob/1083cab5794d6bcc252bdd27dac4b23ecb8a5940/runtime/internal/adapter/toolset/builtin/ask_user.go
[F22]: https://github.com/Tangerg/flame/blob/1083cab5794d6bcc252bdd27dac4b23ecb8a5940/runtime/internal/adapter/run/execution/interaction_delegate_binding.go
[F23]: https://github.com/Tangerg/flame/blob/1083cab5794d6bcc252bdd27dac4b23ecb8a5940/runtime/internal/adapter/run/execution/interaction_checkpoint.go
[F24]: https://github.com/Tangerg/flame/blob/1083cab5794d6bcc252bdd27dac4b23ecb8a5940/runtime/protocol/mcp.go
[F25]: https://github.com/Tangerg/flame/blob/1083cab5794d6bcc252bdd27dac4b23ecb8a5940/runtime/internal/application/workspace/agentmemory/search.go
[F26]: https://github.com/Tangerg/flame/blob/1083cab5794d6bcc252bdd27dac4b23ecb8a5940/runtime/internal/application/workspace/agentmemory/curation.go
[F27]: https://github.com/Tangerg/flame/blob/1083cab5794d6bcc252bdd27dac4b23ecb8a5940/runtime/internal/application/automation/schedules/worker.go
[F28]: https://github.com/Tangerg/flame/blob/1083cab5794d6bcc252bdd27dac4b23ecb8a5940/runtime/internal/application/workspace/skills.go
[F29]: https://github.com/Tangerg/flame/blob/1083cab5794d6bcc252bdd27dac4b23ecb8a5940/desktop/frontend/src/plugins/sdk/kernelPoints.ts
[F30]: https://github.com/Tangerg/flame/blob/1083cab5794d6bcc252bdd27dac4b23ecb8a5940/desktop/frontend/src/main/builtinPlugins.ts
[F31]: https://github.com/Tangerg/flame/blob/1083cab5794d6bcc252bdd27dac4b23ecb8a5940/runtime/go.mod
[F32]: https://github.com/Tangerg/flame/blob/1083cab5794d6bcc252bdd27dac4b23ecb8a5940/runtime/doc/ARCHITECTURE.md
[F33]: https://github.com/Tangerg/flame/blob/1083cab5794d6bcc252bdd27dac4b23ecb8a5940/runtime/internal/domain/integration/mcpserver/server.go
[F34]: https://github.com/Tangerg/flame/blob/1083cab5794d6bcc252bdd27dac4b23ecb8a5940/runtime/internal/domain/run/approval/callgate.go
[F35]: https://github.com/Tangerg/flame/blob/1083cab5794d6bcc252bdd27dac4b23ecb8a5940/runtime/internal/infra/sqlite/db.go
[F36]: https://github.com/Tangerg/flame/blob/1083cab5794d6bcc252bdd27dac4b23ecb8a5940/runtime/internal/adapter/run/execution/interaction_tool.go
[F37]: https://github.com/Tangerg/flame/blob/1083cab5794d6bcc252bdd27dac4b23ecb8a5940/runtime/internal/infra/integration/mcp/dial.go
[S1]: https://github.com/Tangerg/scope/blob/mcp/v0.40.0/mcp/doc.go
[S2]: https://github.com/Tangerg/scope/blob/mcp/v0.40.0/mcp/tools.go
[S3]: https://github.com/Tangerg/scope/blob/mcp/v0.40.0/mcp/server.go
[S4]: https://github.com/Tangerg/scope/blob/mcp/v0.40.0/mcp/result.go
[A1]: https://agent-plugins.org/specification
[A2]: https://agent-plugins.org/plugin-authors/manifest
[A3]: https://agent-plugins.org/plugin-authors/client-extensions
[A4]: https://agent-plugins.org/client-implementers/loading-and-discovery
[A5]: https://agent-plugins.org/plugin-authors/mcp-servers
[A6]: https://agent-plugins.org/client-implementers/mcp-runtime
[A7]: https://agent-plugins.org/schemas/1.0.0/plugin.schema.json
[A8]: https://agent-plugins.org/schemas/1.0.0/mcp.schema.json
[M1]: https://modelcontextprotocol.io/specification/2026-07-28
[M2]: https://modelcontextprotocol.io/specification/2026-07-28/changelog
[M3]: https://modelcontextprotocol.io/specification/2026-07-28/basic/transports/stdio
[M4]: https://modelcontextprotocol.io/specification/2026-07-28/basic/patterns/mrtr
[M5]: https://modelcontextprotocol.io/extensions/tasks/overview
[M6]: https://modelcontextprotocol.io/extensions/apps/overview
[R1]: https://github.com/deepseek-ai/deepseek-harness/blob/639ed015397290b3745d163aafe02ffee4aa3f84/docs/architecture.md
[R2]: https://github.com/deepseek-ai/deepseek-harness/blob/639ed015397290b3745d163aafe02ffee4aa3f84/packages/core/tools/src/index.ts
[R3]: https://github.com/deepseek-ai/deepseek-harness/blob/639ed015397290b3745d163aafe02ffee4aa3f84/packages/client/modules/README.md
[R4]: https://github.com/earendil-works/pi/blob/e7a9bf7e94238f2c0f56f6ff6622750a3ce9caff/packages/coding-agent/docs/extensions.md
[R5]: https://github.com/earendil-works/pi/blob/e7a9bf7e94238f2c0f56f6ff6622750a3ce9caff/packages/coding-agent/src/core/extensions/wrapper.ts
[R6]: https://github.com/anomalyco/opencode/blob/ffa4c4c730bde7b0b0baed1df1ae5828bf0766fc/packages/plugin/src/README.md
[R7]: https://github.com/anomalyco/opencode/blob/ffa4c4c730bde7b0b0baed1df1ae5828bf0766fc/packages/core/src/plugin.ts
[R8]: https://github.com/anomalyco/opencode/blob/ffa4c4c730bde7b0b0baed1df1ae5828bf0766fc/packages/plugin/src/tui/context.ts
[R9]: https://github.com/openai/codex/blob/d42056091aded7feb1d88ac7e83972108b2aa478/codex-rs/plugin/src/provider.rs
[R10]: https://github.com/openai/codex/blob/d42056091aded7feb1d88ac7e83972108b2aa478/codex-rs/core/src/tools/registry.rs
[R11]: https://github.com/openai/codex/blob/d42056091aded7feb1d88ac7e83972108b2aa478/codex-rs/app-server/README.md
[R12]: https://github.com/anomalyco/opencode/blob/ffa4c4c730bde7b0b0baed1df1ae5828bf0766fc/packages/core/src/state.ts
