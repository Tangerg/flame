// Typed wrappers for runtime/doc/API.md §7. Streaming methods return `{ result, events }`;
// a run stream carries the whole run tree and ends on the ROOT segment's `segment.finished`.

import type { RpcClient } from "./client";
import type { MutationPromise } from "./mutation";
import { createWireCallPath, type MethodsOptions, type WireCall } from "./wireCallPath";
import type { RunId, SegmentId, SessionId } from "./ids";
import type {
  AgentDoc,
  ApprovalMode,
  ApprovalModeResult,
  CancelRunResponse,
  ContentBlock,
  MCPServerCandidate,
  UpdateProviderRequest,
  UpdateGoalRequest,
  CreateSessionRequest,
  Diff,
  ExportSessionResponse,
  FeedbackRequest,
  FileContent,
  FileEntry,
  FileHead,
  ForkSessionRequest,
  GetDiffRequest,
  GetFileHeadRequest,
  GrepRequest,
  GrepResult,
  HooksListResult,
  ImportSessionResponse,
  DiscoverResponse,
  EmbeddingRole,
  InvokeToolRequest,
  ListApprovalRulesResult,
  ListFilesRequest,
  ListItemsResponse,
  ListSessionsRequest,
  MCPAuthorizationAttempt,
  MCPServer,
  MCPTestResult,
  MCPTool,
  KnowledgeEntry,
  KnowledgeScope,
  Model,
  PendingInterruptSet,
  Page,
  PageQuery,
  Provider,
  ProviderTestResult,
  ResumeRunRequest,
  ResumeRunResponse,
  StartRunResponse,
  SubscribeRunRequest,
  SubscribeRunResponse,
  RollbackSessionRequest,
  RollbackSessionResponse,
  RunEvent,
  ReadFileRequest,
  Recipe,
  ItemListScope,
  ItemOrder,
  RunRef,
  RunStatus,
  RunScheduleNowResponse,
  Schedule,
  CreateScheduleRequest,
  UpdateScheduleRequest,
  Plan,
  Session,
  SessionArtifact,
  SessionSnapshot,
  Skill,
  ManagedSkill,
  SkillProposal,
  SkillProposalRef,
  AgentMemoryItem,
  AgentMemoryList,
  AgentMemoryScope,
  Goal,
  GoalBudget,
  StartRunRequest,
  RuntimeSubscribeRequest,
  RuntimeSubscribeResponse,
  ToolSpec,
  UpdateSessionRequest,
  UpdateMCPServerRequest,
  Usage,
  UsageSummary,
  UsageSummaryRequest,
  UtilityRole,
  RuntimeEvent,
  WorkspaceFileChange,
  WorkspaceInfo,
  WorkspaceRef,
  WorkspaceSummary,
} from "@flame/runtime-contract/wire";
import { streamRunEvents, streamRuntimeEvents } from "./stream";
import type { AutoPagingPromise } from "./pagination";
import { RUNTIME_SUBSCRIBE_METHOD } from "./transport";

export type { MethodsOptions } from "./wireCallPath";

export interface StreamingResult<R, E> {
  result: R;
  events: AsyncIterable<E>;
}

// Streaming methods subscribe BEFORE the call (head-drop race), so a REJECTED call must
// dispose explicitly: nobody iterates `events`, so its self-cleaning iterator never runs.
async function callOrDispose<R>(
  stream: { dispose: () => void },
  call: () => Promise<R>,
): Promise<R> {
  try {
    return await call();
  } catch (err) {
    stream.dispose();
    throw err;
  }
}

export interface WorkspaceMethods {
  readonly ref: Readonly<WorkspaceRef>;
  changes: {
    list: (signal?: AbortSignal) => Promise<Page<WorkspaceFileChange>>;
  };
  diff: {
    get: (params?: Omit<GetDiffRequest, "workspace">) => Promise<Diff>;
  };
  files: {
    head: (params: Omit<GetFileHeadRequest, "workspace">) => Promise<FileHead>;
    search: (params: Omit<GrepRequest, "workspace">) => Promise<GrepResult>;
    list: (
      params?: Omit<ListFilesRequest, "workspace">,
      signal?: AbortSignal,
    ) => AutoPagingPromise<Page<FileEntry>>;
    read: (params: Omit<ReadFileRequest, "workspace">) => Promise<FileContent>;
  };
  recipes: {
    list: () => Promise<Page<Recipe>>;
  };
  hooks: {
    list: () => Promise<HooksListResult>;
  };
  // Taken from the BINDING, not the caller: otherwise a decision can name one workspace
  // while its source list named another.
  skills: {
    listDiscovered: () => Promise<Page<Skill>>;
    listProposals: () => Promise<Page<SkillProposal>>;
    approveProposal: (ref: Omit<SkillProposalRef, "workspace">) => MutationPromise<void>;
    rejectProposal: (ref: Omit<SkillProposalRef, "workspace">) => MutationPromise<void>;
  };
  agentDocs: {
    list: () => Promise<Page<AgentDoc>>;
  };
  knowledge: {
    list: () => Promise<Page<KnowledgeEntry>>;
    get: (scope: KnowledgeScope) => Promise<KnowledgeEntry>;
    update: (params: {
      scope: KnowledgeScope;
      content: string;
      expectedRevision: string;
    }) => MutationPromise<KnowledgeEntry>;
  };
  agentMemory: {
    list: () => Promise<AgentMemoryList>;
    add: (content: string) => MutationPromise<AgentMemoryItem>;
  };
}

export type AgentMemoryTarget =
  | { scope: Extract<AgentMemoryScope, "user"> }
  | { scope: Extract<AgentMemoryScope, "project">; workspace: WorkspaceRef };

export interface Methods {
  runtime: {
    discover: (signal?: AbortSignal) => Promise<DiscoverResponse>;
  };
  sessions: {
    list: (query?: ListSessionsRequest, signal?: AbortSignal) => AutoPagingPromise<Page<Session>>;
    get: (sessionId: SessionId, signal?: AbortSignal) => Promise<Session>;
    // ONE transactionally coherent read: a recovery fold must not combine facts from
    // different database snapshots.
    snapshot: (
      sessionId: SessionId,
      includeDescendants?: boolean,
      signal?: AbortSignal,
    ) => Promise<SessionSnapshot>;
    create: (params?: CreateSessionRequest, signal?: AbortSignal) => MutationPromise<Session>;
    update: (params: UpdateSessionRequest) => MutationPromise<Session>;
    delete: (sessionId: SessionId) => MutationPromise<void>;
    fork: (params: ForkSessionRequest) => MutationPromise<Session>;
    // `session_busy` while a run is in flight. `restoreType` files|both also restores the
    // working tree, gated on `features.checkpoints`.
    rollback: (params: RollbackSessionRequest) => MutationPromise<RollbackSessionResponse>;
    export: (sessionId: SessionId, format?: "md" | "json") => Promise<ExportSessionResponse>;
    // Rebuilds under the artifact's ORIGINAL id, so it is idempotent.
    import: (artifact: SessionArtifact) => MutationPromise<ImportSessionResponse>;
  };
  runs: {
    start: (
      params: StartRunRequest,
      signal?: AbortSignal,
    ) => MutationPromise<StreamingResult<StartRunResponse, RunEvent>>;
    resume: (
      params: ResumeRunRequest,
      signal?: AbortSignal,
    ) => MutationPromise<StreamingResult<ResumeRunResponse, RunEvent>>;
    // BOTH ids required: naming only the run attaches to whatever segment is executing, so
    // a client folding an earlier one continues into a different execution. Mismatch is
    // `stale_segment`.
    subscribe: (
      params: SubscribeRunRequest,
      signal?: AbortSignal,
      // The last event the caller FOLDED. Omitted means tail-only.
      options?: { lastEventId?: string },
    ) => Promise<StreamingResult<SubscribeRunResponse, RunEvent>>;
    cancel: (runId: RunId, reason?: string) => MutationPromise<CancelRunResponse>;
    // Naming the segment makes a run that parked and resumed between typing and sending
    // REFUSE with `stale_segment` rather than answer work the person never saw.
    steer: (
      runId: RunId,
      expectedSegmentId: SegmentId,
      input: ContentBlock[],
    ) => MutationPromise<void>;
    get: (runId: RunId, signal?: AbortSignal) => Promise<RunRef>;
    // Omitting `statuses` returns every position; descendants require negotiated
    // `features.subagents`.
    list: (
      query?: PageQuery & {
        sessionId?: SessionId;
        statuses?: RunStatus[];
        includeDescendants?: boolean;
      },
      signal?: AbortSignal,
    ) => AutoPagingPromise<Page<RunRef>>;
  };
  plan: {
    // An UNWRITTEN session omits `state`; an explicit clear returns a committed state with a
    // positive revision and no steps.
    get: (sessionId: SessionId, signal?: AbortSignal) => Promise<Plan>;
  };
  interrupts: {
    // A page never SPLITS a set: a set is what one `runs.resume` answers.
    list: (
      query?: PageQuery & { sessionId?: SessionId; rootRunId?: RunId },
      signal?: AbortSignal,
    ) => AutoPagingPromise<Page<PendingInterruptSet>>;
  };
  items: {
    // `order` defaults to "asc" — the order a fold can replay.
    list: (
      params: {
        scope: ItemListScope;
        order?: ItemOrder;
        cursor?: string;
        limit?: number;
      },
      signal?: AbortSignal,
    ) => AutoPagingPromise<ListItemsResponse>;
  };
  workspaces: {
    resolve: (ref?: WorkspaceRef, signal?: AbortSignal) => Promise<WorkspaceInfo>;
    list: (signal?: AbortSignal) => Promise<Page<WorkspaceSummary>>;
    /** Resolves the runtime default when `ref` is omitted. */
    open: (ref?: WorkspaceRef) => Promise<WorkspaceMethods>;
  };
  /** Bind ONCE; every resource operation inherits the identity. */
  workspace: (ref: WorkspaceRef) => WorkspaceMethods;
  // Lossy "this moved, read it again" signals with NO replay; resubscribing IS the resync.
  runtimeEvents: {
    subscribe: (
      params: RuntimeSubscribeRequest,
      signal?: AbortSignal,
    ) => Promise<StreamingResult<RuntimeSubscribeResponse, RuntimeEvent>>;
  };
  // Targets the CANONICAL root from `workspace(ref).hooks.list()`.
  hooks: {
    setTrust: (projectRoot: string, trusted: boolean) => MutationPromise<void>;
  };
  // Workspace-INDEPENDENT: a managed skill is addressed by name alone, and archive and
  // restore never delete.
  skills: {
    listLibrary: () => Promise<Page<ManagedSkill>>;
    archive: (name: string) => MutationPromise<void>;
    restore: (name: string) => MutationPromise<void>;
  };
  mcp: {
    // One resource carries durable configuration AND live state. `update` is
    // omission=preserve, which is why it is distinct from `create`.
    list: () => Promise<Page<MCPServer>>;
    create: (params: MCPServerCandidate) => MutationPromise<MCPServer>;
    update: (params: UpdateMCPServerRequest) => MutationPromise<MCPServer>;
    delete: (server: string) => MutationPromise<void>;
    // NOT persisted; a failed probe is `{ ok:false, error }`, not an RPC error.
    test: (params: MCPServerCandidate) => Promise<MCPTestResult>;
    listTools: (server?: string) => Promise<Page<MCPTool>>;
    reconnect: (server: string) => MutationPromise<void>;
    authorizationAttempts: {
      // An asynchronous RESOURCE, not a command ack: `get` observes the outcome across
      // reconnects.
      create: (server: string, signal?: AbortSignal) => MutationPromise<MCPAuthorizationAttempt>;
      get: (attemptId: string, signal?: AbortSignal) => Promise<MCPAuthorizationAttempt>;
    };
  };
  providers: {
    list: () => Promise<Page<Provider>>;
    update: (params: UpdateProviderRequest) => MutationPromise<Provider>;
    test: (provider: string) => Promise<ProviderTestResult>;
  };
  models: {
    list: (provider?: string) => Promise<Page<Model>>;
    // An EMPTY model means unset, so maintenance work runs on the main turn model.
    getUtilityRole: () => Promise<UtilityRole>;
    setUtilityRole: (params: UtilityRole) => MutationPromise<UtilityRole>;
    // An EMPTY model leaves Agent Memory on keyword ranking.
    getEmbeddingRole: () => Promise<EmbeddingRole>;
    setEmbeddingRole: (params: EmbeddingRole) => MutationPromise<EmbeddingRole>;
  };
  tools: {
    list: () => Promise<Page<ToolSpec>>;
    invoke: (params: InvokeToolRequest) => MutationPromise<unknown>;
  };
  usage: {
    session: (sessionId: SessionId, signal?: AbortSignal) => Promise<Usage>;
    summary: (params?: UsageSummaryRequest, signal?: AbortSignal) => Promise<UsageSummary>;
  };
  // The HITL review surface over the agent's SELF-maintained memory. Distinct from the
  // FLAME.md cascade; `capability_not_negotiated` when the store is not wired.
  agentMemory: {
    list: (target: AgentMemoryTarget) => Promise<AgentMemoryList>;
    review: (id: string, decision: "approve" | "reject") => MutationPromise<void>;
    update: (params: {
      id: string;
      content?: string;
      pinned?: boolean;
    }) => MutationPromise<AgentMemoryItem>;
    delete: (id: string) => MutationPromise<void>;
    add: (params: AgentMemoryTarget & { content: string }) => MutationPromise<AgentMemoryItem>;
  };
  // goals.* (§7.14, capability-gated): Goal mode — the autonomous execution
  // loop. get returns the session's goal or null (no goal); start opens one
  // (session_busy if one is already actively driving); stop pauses the loop;
  // resume re-activates a paused/blocked goal. Omitting provider/model runs the
  // loop on the runtime default.
  goals: {
    get: (sessionId: SessionId, signal?: AbortSignal) => Promise<Goal | null>;
    start: (
      params: {
        sessionId: SessionId;
        objective: string;
        provider?: string;
        model?: string;
        budget?: GoalBudget;
      },
      signal?: AbortSignal,
    ) => MutationPromise<Goal>;
    update: (params: UpdateGoalRequest, signal?: AbortSignal) => MutationPromise<Goal>;
    clear: (sessionId: SessionId, signal?: AbortSignal) => MutationPromise<void>;
    stop: (sessionId: SessionId, signal?: AbortSignal) => MutationPromise<Goal>;
    resume: (sessionId: SessionId, signal?: AbortSignal) => MutationPromise<Goal>;
  };
  feedback: {
    create: (params: FeedbackRequest) => MutationPromise<void>;
  };
  // Approval runtime control (B9) — global stance + remember management. Not gated.
  approval: {
    getMode: () => Promise<ApprovalModeResult>;
    setMode: (mode: ApprovalMode) => MutationPromise<ApprovalModeResult>;
    // Rules visible from the session: its session rules + its project's rules
    // + all global rules (the runtime resolves the session cwd).
    listRules: (sessionId: SessionId) => Promise<ListApprovalRulesResult>;
    // Remove one rule by id; clear-all = loop the visible ids.
    forgetRule: (id: string) => MutationPromise<void>;
  };
  // Scheduled runs (§7.9): cron-triggered headless runs of a saved prompt,
  // fired by the runtime's scheduler worker while serving.
  schedules: {
    list: (query?: PageQuery) => AutoPagingPromise<Page<Schedule>>;
    create: (params: CreateScheduleRequest) => MutationPromise<Schedule>;
    update: (params: UpdateScheduleRequest) => MutationPromise<Schedule>;
    delete: (id: string) => MutationPromise<void>;
    runNow: (id: string) => MutationPromise<RunScheduleNowResponse>;
  };
}

function bindWorkspace(call: WireCall, ref: WorkspaceRef): WorkspaceMethods {
  // Copy and freeze the identity so a caller cannot silently retarget an
  // already-created resource client by mutating the original object.
  const workspace = Object.freeze({ path: ref.path });

  return {
    ref: workspace,
    changes: {
      list: (signal) =>
        call("workspace.changes.list", { workspace }, signal ? { signal } : undefined),
    },
    diff: {
      get: (params) => call("workspace.diff.get", { ...params, workspace }),
    },
    files: {
      head: (params) => call("workspace.files.head", { ...params, workspace }),
      search: (params) => call("workspace.files.search", { ...params, workspace }),
      list: (params, signal) =>
        call("workspace.files.list", { ...params, workspace }, signal ? { signal } : undefined),
      read: (params) => call("workspace.files.read", { ...params, workspace }),
    },
    recipes: {
      list: () => call("recipes.list", { workspace }),
    },
    hooks: {
      list: () => call("hooks.list", { workspace }),
    },
    skills: {
      listDiscovered: () => call("skills.discovered.list", { workspace }),
      listProposals: () => call("skills.proposals.list", { workspace }),
      approveProposal: (ref) => call("skills.proposals.approve", { ...ref, workspace }),
      rejectProposal: (ref) => call("skills.proposals.reject", { ...ref, workspace }),
    },
    agentDocs: {
      list: () => call("agentDocs.list", { workspace }),
    },
    knowledge: {
      list: () => call("knowledge.list", { workspace }),
      get: (scope) => call("knowledge.get", { scope, workspace }),
      update: (params) => call("knowledge.update", { ...params, workspace }),
    },
    agentMemory: {
      list: () => call("agentMemory.list", { scope: "project", workspace }),
      add: (content) => call("agentMemory.add", { scope: "project", workspace, content }),
    },
  };
}

export function createMethods(client: RpcClient, options: MethodsOptions = {}): Methods {
  const runEventStreamOptions = (signal?: AbortSignal) => ({
    signal,
    replayLimits: options.capabilities?.()?.limits.runReplay,
  });

  const { call, perform, openMutation } = createWireCallPath(client, options);

  let defaultWorkspaceRef: Promise<WorkspaceRef> | undefined;
  const openWorkspace = async (ref?: WorkspaceRef): Promise<WorkspaceMethods> => {
    if (ref) return bindWorkspace(call, ref);
    if (!defaultWorkspaceRef) {
      const pending = call("workspaces.resolve", {}).then((resolved) => resolved.ref);
      defaultWorkspaceRef = pending;
      void pending.catch(() => {
        if (defaultWorkspaceRef === pending) defaultWorkspaceRef = undefined;
      });
    }
    return bindWorkspace(call, await defaultWorkspaceRef);
  };

  return {
    runtime: {
      discover: (signal) => call("runtime.discover", {}, { signal }),
    },
    sessions: {
      list: (query, signal) => call("sessions.list", query ?? {}, { signal }),
      get: (sessionId, signal) => call("sessions.get", { sessionId }, { signal }),
      snapshot: (sessionId, includeDescendants, signal) =>
        call(
          "sessions.snapshot",
          { sessionId, ...(includeDescendants ? { includeDescendants: true } : {}) },
          { signal },
        ),
      create: (params, signal) => call("sessions.create", params ?? {}, { signal }),
      update: (params) => call("sessions.update", params),
      delete: (sessionId) => call("sessions.delete", { sessionId }),
      fork: (params) => call("sessions.fork", params),
      rollback: (params) => call("sessions.rollback", params),
      export: (sessionId, format) => call("sessions.export", { sessionId, format }),
      import: (artifact) =>
        call("sessions.import", {
          artifact,
        }),
    },
    runs: {
      start: (params, signal) =>
        openMutation(
          "runs.start",
          params,
          async (idempotencyKey, attempt) => {
            // Subscribe BEFORE the POST, then bind the response to the runtime-assigned
            // root segmentId. Under streamable HTTP the response + its event frames
            // arrive on the same ordered stream, so the first events follow the
            // response immediately; binding only after `call` resolves could drop
            // the head (see streamRunEvents).
            const stream = streamRunEvents(client, runEventStreamOptions(attempt.signal));
            const result = await callOrDispose(stream, () =>
              perform("runs.start", params, {
                signal: stream.requestSignal,
                idempotencyKey,
                idempotencyNamespace: attempt.idempotencyNamespace,
                onRequestRpcId: stream.bindRequest,
              }),
            );
            stream.bind(result.segmentId);
            return { result, events: stream.events };
          },
          signal,
        ),
      resume: (params, signal) =>
        openMutation(
          "runs.resume",
          params,
          async (idempotencyKey, attempt) => {
            // A resume opens a NEW segment of the SAME run — bind the response to it.
            const stream = streamRunEvents(client, runEventStreamOptions(attempt.signal));
            const result = await callOrDispose(stream, () =>
              perform("runs.resume", params, {
                signal: stream.requestSignal,
                idempotencyKey,
                idempotencyNamespace: attempt.idempotencyNamespace,
                onRequestRpcId: stream.bindRequest,
              }),
            );
            stream.bind(result.segmentId);
            return { result, events: stream.events };
          },
          signal,
        ),
      subscribe: async (params, signal, options) => {
        // Reattach to the segment the caller named; the ack echoes it, and the
        // response binds to it (same deferred-bind head-drop guard).
        const stream = streamRunEvents(client, runEventStreamOptions(signal));
        const result = await callOrDispose(stream, () =>
          call("runs.subscribe", params, {
            signal: stream.requestSignal,
            lastEventId: options?.lastEventId,
            onRequestRpcId: stream.bindRequest,
          }),
        );
        stream.bind(result.segmentId);
        return { result, events: stream.events };
      },
      cancel: (runId, reason) => call("runs.cancel", { runId, reason }),
      steer: (runId, expectedSegmentId, input) =>
        call("runs.steer", { runId, expectedSegmentId, input }),
      get: (runId, signal) => call("runs.get", { runId }, { signal }),
      list: (query, signal) => call("runs.list", query ?? {}, signal ? { signal } : undefined),
    },
    plan: {
      get: (sessionId, signal) => call("plan.get", { sessionId }, signal ? { signal } : undefined),
    },
    runtimeEvents: {
      subscribe: async (params, signal) => {
        const stream = streamRuntimeEvents(client, signal);
        const result = await callOrDispose(stream, () =>
          call(RUNTIME_SUBSCRIBE_METHOD, params, {
            signal: stream.requestSignal,
            onRequestRpcId: stream.bindRequest,
          }),
        );
        return { result, events: stream.events };
      },
    },
    interrupts: {
      list: (query, signal) =>
        call("interrupts.list", query ?? {}, signal ? { signal } : undefined),
    },
    items: {
      list: (params, signal) => call("items.list", params, signal ? { signal } : undefined),
    },
    workspaces: {
      resolve: (ref, signal) => call("workspaces.resolve", ref ? { ref } : {}, { signal }),
      list: (signal) => call("workspaces.list", {}, { signal }),
      open: openWorkspace,
    },
    workspace: (ref) => bindWorkspace(call, ref),
    hooks: {
      setTrust: (projectRoot, trusted) =>
        call("hooks.setTrust", {
          projectRoot,
          trusted,
        }),
    },
    skills: {
      listLibrary: () => call("skills.library.list", {}),
      archive: (name) => call("skills.library.archive", { name }),
      restore: (name) => call("skills.library.restore", { name }),
    },
    mcp: {
      list: () => call("mcp.servers.list", {}),
      create: (params) => call("mcp.servers.create", params),
      update: (params) => call("mcp.servers.update", params),
      delete: (server) => call("mcp.servers.delete", { server }),
      test: (params) => call("mcp.servers.test", params),
      listTools: (server) => call("mcp.tools.list", server ? { server } : {}),
      reconnect: (server) => call("mcp.servers.reconnect", { server }),
      authorizationAttempts: {
        create: (server, signal) =>
          call("mcp.authorizationAttempts.create", { server }, { signal }),
        get: (attemptId, signal) =>
          call("mcp.authorizationAttempts.get", { attemptId }, { signal }),
      },
    },
    providers: {
      list: () => call("providers.list", {}),
      update: (params) => call("providers.update", params),
      test: (provider) => call("providers.test", { provider }),
    },
    models: {
      list: (provider) => call("models.list", provider ? { provider } : {}),
      getUtilityRole: () => call("models.getUtilityRole", {}),
      setUtilityRole: (params) => call("models.setUtilityRole", params),
      getEmbeddingRole: () => call("models.getEmbeddingRole", {}),
      setEmbeddingRole: (params) => call("models.setEmbeddingRole", params),
    },
    tools: {
      list: () => call("tools.list", {}),
      invoke: (params) => call("tools.invoke", params),
    },
    usage: {
      session: (sessionId, signal) => call("usage.session", { sessionId }, { signal }),
      summary: (params, signal) => call("usage.summary", params ?? {}, { signal }),
    },
    agentMemory: {
      list: (params) => call("agentMemory.list", params ?? {}),
      review: (id, decision) =>
        call("agentMemory.review", {
          id,
          decision,
        }),
      update: (params) => call("agentMemory.update", params),
      delete: (id) => call("agentMemory.delete", { id }),
      add: (params) => call("agentMemory.add", params),
    },
    goals: {
      get: (sessionId, signal) => call("goals.get", { sessionId }, { signal }),
      start: (params, signal) => call("goals.start", params, { signal }),
      update: (params, signal) => call("goals.update", params, { signal }),
      clear: (sessionId, signal) => call("goals.clear", { sessionId }, { signal }),
      stop: (sessionId, signal) => call("goals.stop", { sessionId }, { signal }),
      resume: (sessionId, signal) => call("goals.resume", { sessionId }, { signal }),
    },
    feedback: {
      create: (params) => call("feedback.create", params),
    },
    approval: {
      getMode: () => call("approval.getMode", {}),
      setMode: (mode) => call("approval.setMode", { mode }),
      listRules: (sessionId) => call("approval.listRules", { sessionId }),
      forgetRule: (id) => call("approval.forgetRule", { id }),
    },
    schedules: {
      list: (query) => call("schedules.list", query ?? {}),
      create: (params) => call("schedules.create", params),
      update: (params) => call("schedules.update", params),
      delete: (id) => call("schedules.delete", { id }),
      runNow: (id) => call("schedules.runNow", { id }),
    },
  };
}
