# Flame Runtime architecture

Flame Runtime is the product backend and sole authority for durable agent semantics. Its center is the user-visible `Run`, not a provider call, framework process, transport connection, or database row.

## Product model

Runtime owns Session, Run, Segment, Item, Goal, Plan, Interrupt, execution, persistence, recovery, provider/model selection, and compaction. Each fact has one Domain or Application owner. Protocol objects, storage records, streams, and client caches encode or project those facts without advancing them independently.

The main aggregate relationships are:

- A Session owns the durable workspace and selection context for Runs.
- A Run owns one logical execution lifecycle.
- A Segment is one active interval of a Run; interruption and resume create another Segment without creating another logical Run.
- Items are durable observable history. Streaming deltas are replaceable previews.
- Goals and Plans are distinct product concepts with their own invariants and Application workflows.

An aggregate validates its complete initial state, keeps mutable representation private, and exposes intention-revealing queries and legal transitions. Application code coordinates aggregates instead of reproducing their rules with switches over exported fields. Wire payloads, storage records, configuration values, provider messages, and read projections remain strict data because they translate facts rather than own them.

A Goal outcome report requires the immutable Goal incarnation that admitted its Run. Application validates that origin before loading or changing the current Goal; a superseded Run cannot complete its replacement.

## Dependency direction

| Ring | Responsibility |
| --- | --- |
| Domain | Aggregates, values, invariants, legal transitions, and pure policy |
| Application | Use-case ordering, transactions, workflow lifetime, and consumer-owned ports |
| Adapter | Translation to Scope, providers, MCP, persistence, and other external systems |
| Infra | Filesystem, SQLite, Git, process, LSP, telemetry, and other technical mechanisms |
| Delivery | Binding-neutral endpoint, dispatch, HTTP/SSE transport, and protocol projection |
| Bootstrap | Concrete construction, ownership transfer, startup, recovery, and shutdown |

Domain does not depend on Application or outer rings. Application depends on Domain and declares narrow interfaces for effects it consumes. Adapters and Infra satisfy those interfaces. Delivery invokes Application through one endpoint. Bootstrap is the only concrete composition root.

External SDK types do not cross their adapter. Protocol types do not enter Domain or Application. Persistence records are decoded into valid Domain values before use.

## Execution boundary

Scope's Agent Framework is the only process, strategy, child-tree, tool-loop, and checkpoint execution engine. Runtime does not copy its scheduler or interpret private framework state.

Scope owns canonical model-response byte budgets and checks that a complete response can fit before provider dispatch. A rejected pre-call budget is a definite host failure with no model attempt or unknown external effect; exceeding the budget after dispatch retains the unknown outcome. Deferred Tool advertisements belong to the exact Tool invocation and close when it returns, including through Runtime's Tool adapters.

`adapter/agentexec` is the anti-corruption boundary. It maps Runtime commands and values to public Scope contracts, observes framework outcomes, and maps them back to Runtime facts. Application owns product admission, transaction ordering, cancellation intent, durable waiting state, and durable terminal publication. Scope owns execution cancellation and immutable termination; Runtime does not maintain a second dispatch cancellation tree or override a settled termination with later intent.

A model error remains an unknown external outcome in Scope. Its Dispatcher alone distinguishes definite settlements from errors requiring an unknown settlement; Runtime does not maintain a second external-dispatch state machine. Runtime retains diagnostics and gates dispatch on the active product Segment. After committing the failed-call fact, Runtime explicitly cancels that member and projects its provider failure only when Scope acknowledges that model-failure stop. Interrupted Effects retained in terminal snapshots are evidence, not resumable work. A failed authoritative projection after external execution still follows the unknown-effect recovery path. If Scope stops consuming a stream, a failed observation commit is retained as diagnostic evidence without yielding again.

Framework observations are wake-ups, not durable commits. Runtime reconciles authoritative framework state into an Application write set before publishing durable product facts. A completed durable Item or snapshot wins over a missing or duplicated preview event.

The live registry fences opening commits together with owner registration. A consumer that has read a durable Running Run waits for that handoff before resolving its stream or cancellation owner. Waiting-state cancellation does not join the opening fence: a competing resume must still be rejected through Session admission without waiting for its commit. Failed openings leave any existing owner untouched.

Cold stream recovery uses `runs.subscribe` with `snapshot: true`. The tree owner fences its durable commits and event publication while the Session use case reads material and the successor tail attaches. The reply contains both the snapshot and its tail head; a replay cursor is mutually exclusive with this mode. Ordinary replay retains the consumed cursor. Waiting for the fence revalidates the addressed Segment so a replacement cannot be paired with an old tail.

Usage accumulates across the execution tree for accounting. Each member retains its own terminal outcome.

A structurally valid Scope model response completes its invocation and commits reported usage even when it contains no portable message. It creates no assistant Item or conversation message. Scope Interaction owns the resulting `interaction.model.invalid_response` rejection; Runtime projects a failed Run with `provider_rejected`, retaining the completed call and its accounting instead of inventing an unknown outcome.

A human-input barrier first proves an externally addressed wait, then uses Scope's `CaptureTree` to drain external work and obtain an acknowledged safe cut. Runtime submits each running member's pause once; pause acceptance is an intent receipt, not an updated snapshot. Scope owns capture quiescence and commit ordering; Runtime owns the persistent product pause. A parked Interaction holds new Effect dispatch until the next product Segment activates. Canceling a waiting child can wake its parent in Scope before that activation; the execution adapter keeps that Effect behind the same continuation boundary. Activation releases it before reconciling the parent Tool results and reducing model context. Scope cancellation unblocks the waiter through the dispatch context; Runtime release also opens the local wait boundary.

Delegates adapt only Runtime's task input to Scope Interaction. Their executions, checkpoints, and outputs remain Scope-owned. The parent receives the complete `interaction.Output`, preserving reasoning, refusal, media, metadata, and direct Tool results. Runtime commits each model response and its accounting before returning it to Scope; that single boundary owns assistant transcript content. Process termination closes the Segment without a second message projection or confirmation handshake. There is no Runtime reply envelope or in-memory reply cache.

A Delegate retains its admitted child across a human-input barrier. Each continuation opens fresh Segments, so the executor observation reopens the parent Tool attempt before forwarding child results. Application reuses the durable Tool Item identity; continuation does not admit another child or repeat its completed work.

The Delegate's Scope Descriptor is the sole input admission contract during registration, start, and restoration. Its generated schema measures summary length in characters and preserves instruction formatting. Runtime does not impose a second byte limit or trim rule after Scope accepts that input.

Scope's chat contract owns provider ToolCall and ToolResult validation. Runtime preserves their opaque correlation IDs exactly in model context, delegation lineage, and recovery. These values are distinct from Runtime's bounded executor Effect IDs; no product character grammar or length limit is imposed on provider call IDs.

Scope schedules the outer Tool contract. An argument-rewriting hook, authorizer, or approval path makes that contract exclusive; only immutable paths preserve an inner concurrency declaration.

Tool decorators expose their inner Tool through Scope's `Unwrap` contract. Scope binds the complete input validator before execution, including typed decoder constraints beyond JSON Schema. Search concurrency and discovery wrappers preserve this admission boundary: an undecodable argument produces a known rejection and model feedback, rather than entering the executable and leaving an unknown Effect. Runtime consumes arguments from Scope's admitted `Invocation`; the attributed `ToolCall` retains the original model proposal. The two need not have identical text because Scope normalizes blank arguments to an empty object before admission.

Scope's Binding freezes each Tool definition before Runtime applies safety policy. The observed Tool and deployment identity use that same frozen definition. Scope's ToolSet admits the visible and deferred execution catalog, including name collisions; direct diagnostic listing and invocation use Scope's Registry. Runtime owns safety classification and resource lifetime without a second Tool manifest validator.

MCP catalog validation, disabled-tool policy, discovery grouping, and automatic approval resolve the original source/tool pair through Scope's `tool.Capability`. The MCP boundary owns translation to the product identity; model-facing names never substitute for it. Ordinary decorators preserve identity, and Scope's outermost capability takes precedence. Missing or malformed MCP identities and invalid wrapping chains fail catalog resolution instead of silently removing tools or treating them as built-ins. A failed mutation-capability lookup or an unavailable workspace for declared mutation paths remains unknown, never evidence that the Tool mutates nothing.

MCP management reads, connected Tool counts, and execution publication derive from the same admitted Scope Tool snapshot. Listing never performs another remote discovery; reconnect replaces the snapshot for all three consumers. Scope Binding compiles each schema before a connection or probe succeeds, and Scope Registry admits the prospective catalog across connected servers. Runtime retains remote identity, description, and catalog-capacity policy. Diagnostic and MCP metadata carry Scope ToolDefinition directly; Delivery alone projects schemas to protocol objects while preserving exact numeric literals. Runtime has no parallel schema value type or parser.

Resolved Tool manifests own Scope's pinned filesystem directory authority. A successful resolution transfers that resource to the execution's deployment set; manifest copies share its lifetime. Failed assembly and recovery probes release every acquired manifest. Live execution retains them across waiting and continuation, and closes them only after Scope's Engine drains. Direct diagnostic calls and catalog reads own and close their short-lived manifests. Read-tool construction borrows a required executor and never opens an unowned fallback executor.

Scope derives its filesystem executor from the manifest's open `os.Root`. Read fingerprints, protected-directory inspection, native edits, patches, and formatting share that directory authority even after its pathname is replaced. Runtime uses its existing keyed locks to coordinate file policies across concurrent Runs; Scope retains execution-tree scheduling. Separate filesystem calls are not an atomic transaction against external writers. Git-backed search and LSP still require a named workspace: search rejects a detached or rebound pathname, and mutation diagnostics report unavailability instead of attributing another directory's results. External formatter configuration also requires the original named workspace.

Formatting uses Scope's bounded normalized read and exact-text edit, with host version checks around source observation and before applying the result. Scope owns replacement, parent handles, permissions, BOM, and line endings. Runtime owns formatter selection, limits, and best-effort diagnostics. Mutation attribution consumes native executor acknowledgements: a successful edit reports its target, and a patch reports every acknowledged effect, including partial creation during a failed move. Prospective patch paths remain approval and locking inputs; they are never evidence of a completed mutation.

Scope owns Git patch parsing, supported operations, prospective endpoints, and execution acknowledgements. Runtime discovers `ApplyPatchTool.MutationPaths` through Scope capability traversal and uses those endpoints for approval, locking, and read-before-write policy. The query shares the executor’s parser and performs no I/O; Runtime resolves its paths under the shared directory authority. Only execution acknowledgements record actual mutations, including partial effects on failure.

Scope owns every capability Runtime consumes rather than restates: the chat and Tool contracts, the Agent Framework's execution and recovery helpers, the filesystem Tools and their directory authority, MCP discovery and admission, the Skill format and its overlay precedence, the model catalog's limits and pricing arithmetic, conversation history, and the GenAI telemetry conventions. Runtime keeps the product decisions around them — which providers it offers, which credentials it stores, what a Tool's safety class means, when to compact, what a Run is.

Nine Scope capabilities are deliberately unused, and the reason belongs here so it is not re-litigated. `history.WindowStore` bounds a read by message count while preserving whole turns; Runtime's request capacity is a token budget measured against the selected model's real limit, and substituting a count would decide compaction by a number that no provider agrees with. `core/tokenizer` cannot replace the provider's own count, which is already authoritative at the compaction threshold; the local estimate it would abstract is a preflight filter, and a BPE vocabulary is wrong for a provider that does not share it. `core/vectorstore` takes the query text and owns embedding, while Agent Memory caches vectors per item and must still rank when the embedder is absent. `chatclient.JSONSchema` structured output is an optional provider capability, so utility calls that must work on every configured model ask for Markdown and parse it. `fs.GrepTool` and `fs.GlobTool` require the complete `Grepper`/`Globber` contract — multiline, file types, context lines, output modes — over a backend that shells out to ripgrep, while Runtime's search answers from its own finite ignore-aware catalog with an exact total and no external binary. `tool.Guard` authorizes with a boolean, while a Runtime approval also rewrites the arguments the call will execute, asks a human, and remembers a scope — none of which a yes/no can carry; a refusal still produces Scope's `FailureKindRejected`, so the two agree on what a rejected call is. `interaction.DirectResultTool` ends a turn with a Tool's own result, and no Runtime Tool does: every result goes back to the model, including the ones that interrupt first. `chatclient.Template`, `StreamClient`, and its tool middleware describe a client that owns prompting, streaming, and the tool loop, all of which belong to the Agent Framework here. `mcp` prompt conversion, elicitation, and progress reporting are surfaces of an MCP server and of MCP features Runtime does not expose; it consumes remote Tools only. `fs.WriteTool` offers a whole-file write, which Runtime does not put in front of a model — `edit` and `apply_patch` state what they replace, and that is what makes an approval reviewable.

Runtime decodes and encodes with `encoding/json/v2`. Duplicate members, trailing values, and unknown members are refused by the decoder rather than by a Runtime pass, and two options carry rules the previous encoder applied implicitly: deterministic member order wherever bytes are hashed, compared, or persisted, and `omitzero` wherever a present-but-empty value is a fact rather than an absence. The tag rule that follows from those semantics is one line: a collection uses `omitempty`, which drops an empty list or map, and everything else uses `omitzero`, because v2's `omitempty` never drops a `false` or a zero number and would leave a tag claiming an omission it does not perform. Three boundaries keep `encoding/json` because only it can decode a JSON number into an `any` exactly; an architecture test holds that list.

Scope's `chat.Usage` is the provider's own per-call report, kept whole through the execution facts, the durable model-invocation journal, and the protocol: a breakdown the provider does not support stays absent rather than becoming a reported zero. Runtime folds it once into `accounting.Tokens`, its cumulative counter, which carries cost and calls and has no absent state. Runtime keeps no second usage value and no second spelling of these counts.

Durable conversation history implements Scope's `history.Store`, keyed by `history.ConversationID`, with Runtime's retention capabilities beside it. `adapter/persistence` owns the single translation: a Session names exactly one conversation, and a batch whose commit the store could not observe reaches the use cases as an uncertain write rather than a definite rejection. The use-case port stays Session-keyed, so Application holds no external storage contract.

Telemetry for model calls, Tool calls, and execution trees is Scope's. The provider instance is wrapped closest to the provider, so a span records the provider's own timing and failure before Runtime translates either; Tools are instrumented as the Run's manifest freezes, where the capability chain stays reachable through `Unwrap`; the execution tree's observer is an Engine event listener that also wraps the TreeCommitter, and it closes only after a successful Engine drain. A client-driven diagnostic Tool call is still a Tool call and is instrumented by the same middleware. Runtime configures the OpenTelemetry providers and adds no parallel span vocabulary: its own spans carry what Scope cannot see — which product operation asked, and what it spent — and never a second copy of a request, usage, or finish-reason fact.

Runtime validates native Tool output before transforming it for storage or presentation. Text offloading applies only when the blob reader can recover the complete text body; parallel structured details, citations, metadata, and media remain intact in Scope's output and the durable model result. Invalid external output retains Scope's unknown-effect semantics instead of becoming a successful text placeholder.

Tool continuation uses the executor's stable call identity. Edited approval arguments change the execution input while preserving that identity; a new call with the same name or arguments receives its own Item. One remaining-call index owns whether a suspended Item still needs to resume or settle.

A Question owns its completed prompt Item and answer schema. Its unfinished Tool suspends and resumes through the ordinary Tool continuation path; the prompt retains the handler's semantic input without copying its execution identity.

Human-input prompts, resolutions, and continuation state use Scope Payload's canonical encoding and strict typed decoding. Scope computes their digests. Runtime owns the approval and question meanings, validates their product constraints, and binds each resolution to the exact waiting prompt.

Frozen model instructions are compared as complete Scope Payload values. Message and part metadata, exact numeric literals, citations, and ordered content all participate; insignificant JSON object ordering and whitespace do not. Runtime does not maintain a partial message-field comparison alongside Scope's message contract.

A child's terminal projection precedes the waiting barrier even when an earlier sibling still needs input. Only parent Tool results wait for the model's declared call order. Completed children leave the product continuation set; the restored Scope tree retains their pending parent Tool results until that order can advance. Application supplies each waiting member's drained Tool identities so restoration does not reopen a parent result that was already committed.

Canceling the final waiting child also opens a continuation Segment. Its observer reopens only surviving parent Tool attempts. The executor supplies the exact model-visible cancellation result during preparation; Application commits that value with the canceled Items and resulting checkpoint before the tree can advance.

Continuation state retains only unfinished Tool identities. A canceled child's settled parent Item and model-context result remain in their durable owners; the executor retires that call when cancellation applies, so later continuations do not carry a separate result acknowledgment.

Accepting an approval settles its verdict; its Tool Item remains open until execution settles or the Run ends. Reported and synthesized terminal outcomes share the same Tool cleanup. Definite Runtime preparation failures and rejected argument edits use ordinary Tool settlement to commit their exact model-visible results before the executor advances. Input waits, cancellation, and uncertain effects retain their framework control semantics; restart and later Runs retain committed failures.

Runtime policy and pre-Tool hooks own their public refusal reasons. The execution adapter returns each refusal as Scope's explicit rejected Tool outcome; product failure metadata projects that same output. Refusal does not invoke the operation or its post-execution hooks. Policy callback errors are host failures, including errors containing another invocation's result; they cannot become public Tool feedback. Scope owns the generic outcome contract and does not interpret Plan mode, approval answers, or hook policy.

Filesystem path and read-before-write guards use that same rejected outcome. Protected paths, unread files, and stale reads produce error feedback and a denied Tool Item without invoking post-execution hooks or reporting unknown effects. The model may correct the call and continue.

Direct diagnostic calls enter Scope's frozen input contract before Runtime normalizes workspace paths. Scope admits the normalized invocation again before execution. Invalid field names, nulls, and numeric bounds cannot disappear during typed re-encoding; input rejection projects as `invalid_params` through both Runtime bindings.

Result publication callbacks first ask the Segment owner for a durable receipt, ordered with commits on its event stream. The receipt binds the Scope Effect and digest to the Session, Run, and active Segment. Only an unpublished batch needs pending product metadata and reducer mutation. Direct Tool completion uses these same committed results without synthesizing an assistant answer.

Unknown external effects fail closed. Runtime stops live unresolved execution through Scope, then projects the immutable outcomes; it does not independently terminalize every Run as Lost. A canceled or timed-out Run can retain unresolved effects. Ordinary Tool processes contribute their evidence to the calling Run, while Delegates own theirs. Process and Effect identities, original termination cause, and bounded diagnostics commit atomically with the terminal Run and appear in both cold reads and terminal events. Already published Tool results retain their durable Items; unstarted effects are not reported as unknown. Scope's typed settled-results view preserves known results from interrupted rounds. The execution-tree head, incarnation-local sequence, commit identity ledger, and newly settled product facts commit in one transaction before Scope receives acknowledgement. Checkpoint identities bind writer and sequence; Effect boundary identities remain unique across writer activation. Reconciliation requires the exact current commit, not merely repeated snapshot content. Runtime failures remain distinct from logical process termination; Runtime drains external work before projecting retained uncertainty.

Root Await establishes the root outcome; Join establishes local subtree drainage. Reconciliation stays available during Join, followed by a final postorder sweep before Engine closure. Terminal projection waits for a durable receipt, including when background scanning is stopped. A held root outcome survives missing-child cleanup. Consumer rejection of a model stream does not request cancellation; the Dispatcher supplies the actual error and unresolved evidence.

Each parent Run admits at most four active delegated children, including pending reservations. The product admission pump owns this limit independently of Scope's mixed Tool/Delegate tree capacity. Aborted initialization releases a reservation; a started child releases capacity only after its drained terminal commits. Delegates inherit generation options without root output framing or stop strings. Checkpoints retain those options, and deployment identities bind their effective policy across restoration.

Runtime uses Scope's unlimited cumulative quotas for Interaction model calls, process Steps/Effects/Signals, ordinary Tools, Delegates, and lifetime child/process counts. Depth, active-child concurrency, pending mailbox capacity, and operation-specific waits bound resource use. Model-call identities and pending result attribution retain Scope's uint64 sequences through checkpoint restoration.

## Persistence and recovery

SQLite stores current Application and Domain state, not live framework objects, goroutines, contexts, SDK clients, or transport connections. Aggregate decoding is strict: unknown fields, invalid states, truncated values, and trailing content are rejected.

Checkpoint and waiting facts commit in the Application order required to recover the same logical Run. Terminalization and checkpoint cleanup preserve one durable winner. Recovery reconstructs from durable Runtime state and public framework checkpoints; it does not infer state from event delivery or client caches.

Runtime decodes its checkpoint envelope with the standard strict JSON decoder, rejecting duplicate and unknown members, including field-name aliases. Scope alone decodes and validates the enclosed execution tree.

Bootstrap creates one file-lease set from the persistence bundle's data directory and supplies it to Session admission, Goal driving, and ordered Run-then-Goal recovery. Each use case requires its ownership backend at construction, and recovery requires both reconcilers.

Lease acquisition distinguishes contention from operational failure. Only a contended lease proves another owner is live; filesystem and lock errors abort admission or reconciliation with their cause. The persistence and authored-file adapters report the first failure in each outage through Runtime logging, including when the embedding host has not configured tracing, and retry without inventing a change notification. A successful resample resumes observation and permits a later outage to be reported again. Filesystem reconciliation reads the backend's current registrations so deleted and recreated directories regain their watches. Git observation retains its last successful HEAD/index snapshot across read failures. Failed samples retry with bounded exponential backoff without requiring another filesystem event; persistent failure or a terminated backend ends the subscription explicitly. Watcher event loss schedules a fresh semantic read. Workspace path observations reuse the bounded filesystem mechanism for exact files and directory entries, preserving the originating workspace in invalidations.

Background recovery reports the first consecutive sweep failure through the same logging channel and continues retrying. Schedule scanning and dispatch failures remain visible while durable pending occurrences retain their retry path. Schedule deletion removes future recurrence, while captured occurrences retain their execution ownership. Accepting an occurrence updates the Schedule display fact only while that Schedule still exists; a manual opening still requires its source Schedule. Each scanner pass admits at most 32 attempts, reserves at least half for newly due work, and advances a process-local seek cursor through pending occurrences; restart begins that scan again without changing durable execution identities. Due selection excludes Schedules with pending occurrences before applying its limit. SQL queries and domain restoration own the returned page guarantees; Application does not revalidate their identity, ordering, uniqueness, or closed values. Goal driver failures and rejected Run starts are logged with their Session and Goal incarnation identities; durable Goal pause reasons keep stable cause codes rather than diagnostic errors. These diagnostics do not require tracing configuration.

The active development contract has one current storage shape. SQLite installs that shape directly and does not maintain a schema-version or migration graph. A breaking schema change replaces the old shape completely; incompatible development state is reset explicitly unless the user authorizes a real migration requirement.

Executor restore compatibility belongs to the exact BuildID and framework Deployment references. Checkpoint payloads, policy, context sources, and Tool-input continuations encode the current shape without independent hand-maintained schema counters. Decoding still validates complete identities, capabilities, prompt digests, and structural relationships before restoring execution.

## Provider and integration boundaries

MCP configuration requires its durable registry, live connection ports, tool catalog, and shared tool policy at construction. An empty registry represents no configured servers; it does not remove any of these use cases or turn missing implementations into empty query results. Startup and background connection failures produce diagnostics with their server identity even when tracing is unconfigured. A failed startup connection or tool catalog leaves that server unavailable while independently configured servers remain usable.

Provider identity is the exact provider/model pair plus model-owned options. Credential precedence, endpoints, SDK construction, request lowering, capability mapping, and provider-specific failures remain inside provider adapters. Product and delivery code do not infer a provider from a model name.

Auxiliary text generation has one live-role completion boundary. It validates the request envelope before resolving the provider and accepts only a natural stop with non-empty text. Truncated, filtered, refused, or otherwise incomplete output remains a failure; compaction and background curation never persist it as a completed artifact.

The ordinary chat contract owns complete and streaming calls. Complete-request token counting and other provider-specific capabilities remain separate narrow contracts discovered at the provider boundary. Runtime advertises only exact implemented behavior; it does not guess from a provider name, approximate unavailable behavior, or add optional methods to every client.

A provider declares whether its model identities come from the bundled catalog or its endpoint. Endpoint discovery is authoritative even when empty, and failures remain errors; missing required endpoint or credential configuration is a parameter error. Bundled metadata may enrich discovered identities without supplying replacement results. Catalog slice results transfer ownership to the caller.

MCP, LSP, Git, filesystem, execution, and other integrations are grouped by the external system they translate. A wrapper remains only when it owns policy, translation, confinement, authority, or resource lifecycle.

## Protocol and bindings

The Contract Registry is the method and policy source used by delivery and contract generation. Generated artifacts in `contract` are the machine truth for methods, schemas, capabilities, errors, unions, and transport endpoints. Discovery identity and capability-catalog constraints are declared there and enforced by the generated validators, so consumers do not maintain another schema.

Plan, Goal, Schedule, agent-memory, and file-observation use cases are present in every complete Runtime. Discovery advertises them directly; Session snapshots include the current Plan and any current Goal, and portable import restores Plan as part of the atomic Session write. Git availability remains a host fact, and repository observation failures remain errors at registration. Capability negotiation still governs optional client behavior and host-dependent integrations.

The module-root Go binding and HTTP/JSON-RPC binding enter the same delivery endpoint before:

1. request validation and capability admission;
2. idempotency and replay handling;
3. invocation lifecycle control;
4. Application execution;
5. response, error, and event projection.

That endpoint is the single authority for the wire contract. It validates every request's parameters and every response and event against the generated validators, so a consumer of either binding never has to recheck a shape Runtime already published. An invalid response or event becomes an internal error instead of reaching the caller.

A stream carries its own failure. An operation's event source reports a delivery-side defect as the stream's error rather than closing cleanly, so a consumer can distinguish a Run that stopped producing events from a Runtime that could not describe one. The JSON-RPC transport has no error frame, so it ends the stream at the first event it cannot publish and lets the client resume from its last event id; it never skips a frame and continues.

Per-call metadata rides HTTP headers rather than the JSON-RPC body. `Idempotency-Key` and `Idempotency-Namespace` carry the replay identity, `Last-Event-Id` carries the resume cursor a reconnecting client returns, `Authorization` carries the local-token gate, and W3C `traceparent`, `tracestate`, and `baggage` extend the caller's trace into the backend. Every response names its request and server through `Request-Id` and `X-Server`; a streaming response adds `X-Method`. The transport owns that request set in one place, because a header its handlers read but its CORS allowlist omits fails a browser client's preflight before any handler runs. The generated contract describes methods and shapes, so this envelope is the transport's own to state.

A failure the transport answers itself is not an operation's failure, and says so. It arrives as `application/problem+json` with an HTTP status and a `type` under `urn:flame:transport:`, naming one of `unsupported_media_type`, `request_too_large`, `invalid_request`, `unauthorized`, `response_encoding_failed`, or `internal_error`. Matching that namespace tells a client the request never reached an operation, or that its response never left, which is why the prefix exists even where a suffix shares a word with a Problem type: the two vocabularies are independent and neither renames the other.

The Go binding does not serialize through HTTP, but it does not bypass product semantics. Protocol changes publish one current shape without aliases, fallback decoding, dual methods, or dual events.

Public Go operation methods share one binding implementation between an owned `Runtime` and an attached `Client`. The local invocation enters Endpoint directly; the remote invocation translates the HTTP envelope, metadata, strict response decoding, and SSE lifetime. Neither path reconstructs admission, recovery, or execution policy. Remote close detaches only that client. Accepted execution remains owned by the serving Runtime.

`contract/typescript/client` is the TypeScript protocol consumer shared by Web, Desktop, and IDE. It owns transport, exact wire values, command preparation, and replay evidence; renderer stores and native host APIs remain with their clients. Each client owns its command journal and retires it explicitly. Creating another client does not revoke an unrelated client's authority.

The HTTP host may serve a built Web distribution on its origin. Bootstrap supplies the confined `infra/filesystem/webassets` handler; delivery owns routing and never opens filesystem paths. Static assets are independent of the protocol catalog and expose no product operations or credentials. The generated API routes retain their authentication and dispatch. Browser, CLI, and IDE workspace references are opaque server paths; client-local files and versioned editor buffers are separate input material.

## Composition and lifecycle

Bootstrap constructs one endpoint and one resource graph. One Instance lifecycle owns startup rollback and ordered shutdown: stop delivery, join accepted operations and workers, stop Application producers, drain maintenance, join execution, and release resources. A caller timeout never cancels cleanup; a settled component failure allows a later Close attempt. Construction has no separate builder lifecycle. Public `runtime.Runtime` owns that Bootstrap instance and rejects new work after closing begins.

A required collaborator is required once, where the object is built. A constructor that refuses to return a partial object makes its methods total, so they do not re-check the receiver and invent an answer for a state composition cannot produce — an empty catalog, an unknown-server error, or a successful no-op for work that never happened. A feature the user has not configured is different: it is stored state with its own query, not a missing implementation.

Optional means the user disabled it, not that composition forgot it. A durable write-set names every store it writes to, because whether a delete, fork, rollback or commit is complete cannot depend on how it was assembled; a use case that reports a committed change names its observation and invalidation channels for the same reason. Tests that do not exercise a capability supply an inert collaborator, not an absent one.

A constructed value does not re-derive its own construction. Domain identities, selections and canonical representations keep their material private and admit it through one parser, so a later `Validate` asks only whether the value was constructed at all — which is what a write-set or read boundary can actually receive.

An aggregate whose constructors and transitions all end in its own check cannot exist while invalid, so that check is unexported and no reader calls it. `transcript.Item` and `run.Run` are the worked examples. Every `Item` constructor, `RestoreItem`, and every settle, fork and approval transition close on it. `Run` reaches the same place two ways: `Admit`, `Restore`, `finish` and `WithMessageMark` close on it, while `Resume`, `Suspend` and `AdvanceProgress` guard exactly the fields they touch and provably preserve the rest — a transition either re-proves the whole aggregate or narrows to what it changed, never neither. What such a type still cannot prevent is its zero value, which is writable wherever the type is visible; the one question a consumer may still ask is therefore whether an aggregate was attached at all, not whether it is legal.

A transition does not open by validating its own receiver. `session.Session` and `schedule.Schedule` closed on their check correctly and then re-derived it on entry to `Apply`, `Fork`, `Edit`, `NextRevision` and the restore replacement — a value object asking whether the value it was called on is legal. What entry still owns is anything the caller just wrote: `ScheduledAfter` validates the product fields because `Edit` assigns a patched cron immediately before calling it, and that is a check of new input, not of the receiver.

An injected generator is required at construction, not re-checked at every call. Composition owns the identity source and the use case owns its namespace, so a composed generator cannot return an empty identity, and a generator that somehow did would be rejected by the aggregate that receives it with a better error than the call site could write.

A `Replacement` is a decided compare-and-set command, and persistence executes it rather than re-deciding it. Every one is built by a constructor that proves identity agreement and exact revision succession, so the store reads its expected version and state and writes; the schema's own key, foreign key and range constraints are what stop a malformed row, not a second pass over the same rule at the storage boundary.

A value is validated where it enters the process, and read everywhere else. The boundaries that own that answer are the ones facing something untrusted: a use case turning caller input into a domain value, an adapter parsing a remote server's reply, and a store decoding its own persisted rows. An Application that re-checks identity, uniqueness or field agreement on the way back out of its own port is not defending the product — it is a second implementation of a rule the producer already owns, and it drifts. Uniqueness stated by a primary key, a lookup keyed by the identity it was asked for, and a projection whose contradictory states the producer clears at the transition are all facts to consume, not to reprove. A test that reaches this code through a fake port returning something the real one cannot produce is freezing the defense, not the contract.

Production construction consumes the complete storage bundle opened by persistence. Plan, Goal, permission, Schedule, mutation recovery, role persistence, and agent-memory review and curation are always wired; absent user configuration is stored state, not a missing implementation. Agent-memory search may use keyword ranking without an embedding provider. Narrow use-case tests supply their own collaborators without adding partial production configurations. Hook execution and management share one resolver built from the user home and durable trust store. A project with no hooks or revoked trust still has complete inspection and trust-management use cases.

Skill discovery, library curation, and proposal review require complete implementations. Bootstrap requires an absolute user Skill directory and constructs its store, usage recorder, and maintenance workers even when the library is empty. A Skill store requires an absolute library root and a valid scope, and a maintenance component requires its sweeper at construction.

Every goroutine has one owner, stop condition, and join path. Request cancellation governs the request; accepted Run execution uses a Runtime-owned lifetime. Transport disconnect does not implicitly cancel durable execution.

Authoritative execution publication has no fixed wall-clock deadline. Admission follows its execution context; observed model and Tool outcomes survive execution cancellation until the product owner releases the executor. Release cancels outstanding publication waits when the Run pump stops consuming. Tree reconciliation and final effect inspection follow their owner lifetime. Best-effort lifecycle notifications, refetchable hints, and cleanup callers retain bounded waits. Auxiliary model resolution and generation follow their caller's context, including required compaction, without an adapter-imposed timeout.

The execution registry owns each Interaction session from assembly until release
succeeds. Publication adds a callable index; it does not acquire the resources.
Waiting-tree probes and failed restores use the same release path as live roots,
including killing and joining an unpublished restored Process before closing its
Engine. Cleanup failures retain the original error and remain owned for shutdown.
Closing admission also rejects publication; shutdown joins admitted assembly
before taking its resource snapshot. Delivery similarly retains each invocation
until its handler returns or transfers ownership to a stream, including panic exits.

A Goal drive's completion includes releasing its execution lease. Lifecycle commands retain the same join handle while release is in progress, including after a caller stops waiting, so a successor cannot mistake its predecessor's unfinished cleanup for foreign ownership.

Detached shells remain Runtime-owned after the Tool call and Run that launched them. Changing a Session's workspace or isolation policy stops that Session's shells and retires its derived context and isolated copy before exposing the replacement. Session deletion and rollback stop the same owned processes; a destructive working-tree restore additionally stops shells below the shared workspace across every Session before touching files. History or file rollback discards the old isolated copy so removed effects cannot reappear in a later Run. Runtime shutdown stops shells before destroying the isolated directories they may still use.

Foreground Run and Session commands wait, with caller cancellation, while an in-process recovery probe holds their Session. Recovery is an internal consistency check, not a user-visible live execution conflict. Genuine live Run and destructive-mutation conflicts still reject admission. Ownership releases the kernel lease before publishing local availability so a woken command cannot race an unreleased lease.

Process-local authority follows the facts that justify it. Working-context construction requires the user home and the agent-memory, Goal, Plan, and hook readers; absent user content is an owner-provided empty result. Session cleanup requires the working-context, tool, and shell owners at construction; compaction requires its context invalidator. Durable compaction invalidates context only after its history rewrite succeeds. Replacing, rolling back, or compacting effective model context clears that Session's read-before-write evidence. Restoring a working tree clears such evidence for every Session that observed paths below the shared workspace.

Context compaction is decided only at an imminent main-model call from that call's complete request footprint: instructions, durable and transient messages, Tools, model options, provider limits, and provider-native counting when available. Protocol message count and Run completion are not pressure signals. The same path performs any durable rewrite and emits the observable boundary; post-Run maintenance only consumes the resulting fact.

The bounded summary transcript retains Tool call identities, names, arguments, and result error status alongside output. Summarization may reduce content but must not erase the operation that produced it or turn a failed operation into apparent success. Request budgeting uses the complete model request; auxiliary transcript rendering does not own a competing size estimate.

That boundary first commits completed Delegate results already present in the imminent request. Durable context comparison must observe those results even when background reconciliation has not run yet, including after canceling a waiting sibling and restoring the parent.

Required compaction resolves its current lifecycle Hook policy before calling the summary model or rewriting history. A configuration or trust-read failure stops compaction and preserves its cause. Hook command, observe-only lifecycle Hook, refetchable Tool projection, Skill usage recording, and post-Run maintenance failures produce diagnostics without requiring an active tracing span. Their best-effort policy does not revise the committed Tool result or published lifecycle boundary.

Working-tree checkpoints are scoped by both Session and canonical workspace identity. A Session relocation may retain independent history for each workspace, but a Run checkpoint can only restore the exact workspace that produced it; the storage adapter verifies the complete persisted identity before any Git mutation.

Process-local notifications carry no product truth. They wake consumers, which reread durable projections.

## Internal value ownership

Synchronous calls borrow mutable inputs until return. A store or adapter that returns a newly decoded slice transfers it to the caller; retaining or asynchronously using mutable data requires an independent snapshot. Immutable Domain values share their private representation across containers and copy mutable inputs or projections only at their boundary. Validation protects external input and persistent decoding, rather than repeatedly reconstructing an already-valid value.

## Package shape

Large rings may use `ring/context/package` where a context contains several peer packages. Namespace directories contain no Go facade. Direct ring packages are reserved for ring-wide mechanisms or aggregates that also name their context.

Keep related behavior in responsibility-named files inside one package. Split a package only for a distinct aggregate vocabulary, workflow lifecycle, external translation, or reusable mechanism. Merge forwarding, identity-only, helper-only, and one-concept packages into the owner that gives them meaning.

## Verification

Tests protect observable protocol and binding behavior, Domain invariants, Application transaction ordering, strict persistence, recovery, execution lifecycle, and dependency direction. The primary lifecycle matrix covers Goal, Plan, steer, HITL, interruption and resume, compaction, long context, long execution, provider failure, restart, and recovery through one Runtime. Architecture tests prevent outer dependencies from leaking inward and keep public SDKs at their adapters; they do not freeze private filenames, fields, function inventories, or exact package counts. Multi-client, multi-server, and race scenarios need evidence that Runtime owns that concurrency.
