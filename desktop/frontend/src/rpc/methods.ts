import type { RpcClient } from "./client";
import type { MutationPromise } from "./mutation";
import { createWireCallPath, type MethodsOptions, type WireCall } from "./wireCallPath";
import type { RunId, SegmentId, SessionId } from "./ids";
import type {
  ModelInvocation,
  ListModelInvocationsRequest,
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
  ForkSessionRequest,
  GetDiffRequest,
  HooksListResult,
  ImportSessionResponse,
  DiscoverResponse,
  EmbeddingRole,
  ListApprovalRulesResult,
  ListFilesRequest,
  ListItemsResponse,
  ListSessionsRequest,
  MCPAuthorizationAttempt,
  MCPServer,
  MCPTestResult,
  MCPTool,
  Model,
  PendingInterruptSet,
  Page,
  PageQuery,
  Provider,
  ProviderTestResult,
  ResumeRunRequest,
  ResumeRunResponse,
  StartRunResponse,
  SteerRunResponse,
  SubscribeRunRequest,
  SubscribeRunResponse,
  RollbackSessionRequest,
  RollbackSessionResponse,
  RunEvent,
  ReadFileRequest,
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
  SkillDiscovery,
  SkillDetail,
  ManagedSkill,
  SkillProposal,
  SkillProposalRef,
  AgentMemoryItem,
  AgentMemoryList,
  AgentMemoryScope,
  Goal,
  StartRunRequest,
  RuntimeSubscribeRequest,
  RuntimeSubscribeResponse,
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

interface WorkspaceMethods {
  readonly ref: Readonly<WorkspaceRef>;
  changes: {
    list: (signal?: AbortSignal) => Promise<Page<WorkspaceFileChange>>;
  };
  diff: {
    get: (params?: Omit<GetDiffRequest, "workspace">, signal?: AbortSignal) => Promise<Diff>;
  };
  files: {
    list: (
      params?: Omit<ListFilesRequest, "workspace">,
      signal?: AbortSignal,
    ) => AutoPagingPromise<Page<FileEntry>>;
    read: (
      params: Omit<ReadFileRequest, "workspace">,
      signal?: AbortSignal,
    ) => Promise<FileContent>;
  };
  hooks: {
    list: (signal?: AbortSignal) => Promise<HooksListResult>;
  };
  skills: {
    listDiscovered: (signal?: AbortSignal) => Promise<SkillDiscovery>;
    getDiscovered: (name: string, signal?: AbortSignal) => Promise<SkillDetail>;
    listProposals: (signal?: AbortSignal) => Promise<Page<SkillProposal>>;
    approveProposal: (ref: Omit<SkillProposalRef, "workspace">) => MutationPromise<void>;
    rejectProposal: (ref: Omit<SkillProposalRef, "workspace">) => MutationPromise<void>;
  };
  agentMemory: {
    list: (signal?: AbortSignal) => Promise<AgentMemoryList>;
    add: (content: string) => MutationPromise<AgentMemoryItem>;
  };
}

type AgentMemoryTarget =
  | { scope: Extract<AgentMemoryScope, "user"> }
  | { scope: Extract<AgentMemoryScope, "project">; workspace: WorkspaceRef };

export interface Methods {
  runtime: {
    discover: (signal?: AbortSignal) => Promise<DiscoverResponse>;
  };
  sessions: {
    list: (query?: ListSessionsRequest, signal?: AbortSignal) => AutoPagingPromise<Page<Session>>;
    get: (sessionId: SessionId, signal?: AbortSignal) => Promise<Session>;
    snapshot: (
      sessionId: SessionId,
      includeDescendants?: boolean,
      signal?: AbortSignal,
    ) => Promise<SessionSnapshot>;
    create: (params?: CreateSessionRequest, signal?: AbortSignal) => MutationPromise<Session>;
    update: (params: UpdateSessionRequest) => MutationPromise<Session>;
    delete: (sessionId: SessionId) => MutationPromise<void>;
    fork: (params: ForkSessionRequest) => MutationPromise<Session>;
    rollback: (params: RollbackSessionRequest) => MutationPromise<RollbackSessionResponse>;
    export: (
      sessionId: SessionId,
      format?: "md" | "json",
      signal?: AbortSignal,
    ) => Promise<ExportSessionResponse>;
    import: (artifact: SessionArtifact) => MutationPromise<ImportSessionResponse>;
  };
  modelInvocations: {
    list: (
      query: ListModelInvocationsRequest,
      signal?: AbortSignal,
    ) => AutoPagingPromise<Page<ModelInvocation>>;
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
    subscribe: (
      params: SubscribeRunRequest,
      signal?: AbortSignal,
      options?: { lastEventId?: string },
    ) => Promise<StreamingResult<SubscribeRunResponse, RunEvent>>;
    cancel: (runId: RunId, reason?: string) => MutationPromise<CancelRunResponse>;
    steer: (
      runId: RunId,
      expectedSegmentId: SegmentId,
      input: ContentBlock[],
    ) => MutationPromise<SteerRunResponse>;
    get: (runId: RunId, signal?: AbortSignal) => Promise<RunRef>;
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
    get: (sessionId: SessionId, signal?: AbortSignal) => Promise<Plan>;
  };
  interrupts: {
    list: (
      query?: PageQuery & { sessionId?: SessionId; rootRunId?: RunId },
      signal?: AbortSignal,
    ) => AutoPagingPromise<Page<PendingInterruptSet>>;
  };
  items: {
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
    open: (ref?: WorkspaceRef, signal?: AbortSignal) => Promise<WorkspaceMethods>;
  };
  workspace: (ref: WorkspaceRef) => WorkspaceMethods;
  runtimeEvents: {
    subscribe: (
      params: RuntimeSubscribeRequest,
      signal?: AbortSignal,
    ) => Promise<StreamingResult<RuntimeSubscribeResponse, RuntimeEvent>>;
  };
  hooks: {
    setTrust: (projectRoot: string, trusted: boolean) => MutationPromise<void>;
  };
  skills: {
    listLibrary: (signal?: AbortSignal) => Promise<Page<ManagedSkill>>;
    archive: (name: string) => MutationPromise<void>;
    restore: (name: string) => MutationPromise<void>;
  };
  mcp: {
    list: (signal?: AbortSignal) => Promise<Page<MCPServer>>;
    create: (params: MCPServerCandidate) => MutationPromise<MCPServer>;
    update: (params: UpdateMCPServerRequest) => MutationPromise<MCPServer>;
    delete: (server: string) => MutationPromise<void>;
    test: (params: MCPServerCandidate, signal?: AbortSignal) => Promise<MCPTestResult>;
    listTools: (server?: string, signal?: AbortSignal) => Promise<Page<MCPTool>>;
    reconnect: (server: string) => MutationPromise<void>;
    authorizationAttempts: {
      create: (server: string, signal?: AbortSignal) => MutationPromise<MCPAuthorizationAttempt>;
      get: (attemptId: string, signal?: AbortSignal) => Promise<MCPAuthorizationAttempt>;
    };
  };
  providers: {
    list: (signal?: AbortSignal) => Promise<Page<Provider>>;
    update: (params: UpdateProviderRequest) => MutationPromise<Provider>;
    test: (provider: string, signal?: AbortSignal) => Promise<ProviderTestResult>;
  };
  models: {
    list: (provider?: string, signal?: AbortSignal) => Promise<Page<Model>>;
    getUtilityRole: (signal?: AbortSignal) => Promise<UtilityRole>;
    setUtilityRole: (params: UtilityRole) => MutationPromise<UtilityRole>;
    getEmbeddingRole: (signal?: AbortSignal) => Promise<EmbeddingRole>;
    setEmbeddingRole: (params: EmbeddingRole) => MutationPromise<EmbeddingRole>;
  };
  usage: {
    session: (sessionId: SessionId, signal?: AbortSignal) => Promise<Usage>;
    summary: (params?: UsageSummaryRequest, signal?: AbortSignal) => Promise<UsageSummary>;
  };
  agentMemory: {
    list: (target: AgentMemoryTarget, signal?: AbortSignal) => Promise<AgentMemoryList>;
    review: (id: string, decision: "approve" | "reject") => MutationPromise<void>;
    update: (params: {
      id: string;
      content?: string;
      pinned?: boolean;
    }) => MutationPromise<AgentMemoryItem>;
    delete: (id: string) => MutationPromise<void>;
    add: (params: AgentMemoryTarget & { content: string }) => MutationPromise<AgentMemoryItem>;
  };
  goals: {
    get: (sessionId: SessionId, signal?: AbortSignal) => Promise<Goal | null>;
    start: (
      params: {
        sessionId: SessionId;
        objective: string;
        provider?: string;
        model?: string;
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
  approval: {
    getMode: (signal?: AbortSignal) => Promise<ApprovalModeResult>;
    setMode: (mode: ApprovalMode) => MutationPromise<ApprovalModeResult>;
    listRules: (sessionId: SessionId, signal?: AbortSignal) => Promise<ListApprovalRulesResult>;
    forgetRule: (id: string) => MutationPromise<void>;
  };
  schedules: {
    list: (query?: PageQuery, signal?: AbortSignal) => AutoPagingPromise<Page<Schedule>>;
    create: (params: CreateScheduleRequest) => MutationPromise<Schedule>;
    update: (params: UpdateScheduleRequest) => MutationPromise<Schedule>;
    delete: (id: string) => MutationPromise<void>;
    runNow: (id: string) => MutationPromise<RunScheduleNowResponse>;
  };
}

function bindWorkspace(call: WireCall, ref: WorkspaceRef): WorkspaceMethods {
  const workspace = Object.freeze({ path: ref.path });

  return {
    ref: workspace,
    changes: {
      list: (signal) =>
        call("workspace.changes.list", { workspace }, signal ? { signal } : undefined),
    },
    diff: {
      get: (params, signal) => call("workspace.diff.get", { ...params, workspace }, { signal }),
    },
    files: {
      list: (params, signal) =>
        call("workspace.files.list", { ...params, workspace }, signal ? { signal } : undefined),
      read: (params, signal) => call("workspace.files.read", { ...params, workspace }, { signal }),
    },
    hooks: {
      list: (signal) => call("hooks.list", { workspace }, { signal }),
    },
    skills: {
      listDiscovered: (signal) => call("skills.discovered.list", { workspace }, { signal }),
      getDiscovered: (name, signal) =>
        call("skills.discovered.get", { workspace, name }, { signal }),
      listProposals: (signal) => call("skills.proposals.list", { workspace }, { signal }),
      approveProposal: (ref) => call("skills.proposals.approve", { ...ref, workspace }),
      rejectProposal: (ref) => call("skills.proposals.reject", { ...ref, workspace }),
    },
    agentMemory: {
      list: (signal) => call("agentMemory.list", { scope: "project", workspace }, { signal }),
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

  const openWorkspace = async (
    ref?: WorkspaceRef,
    signal?: AbortSignal,
  ): Promise<WorkspaceMethods> => {
    signal?.throwIfAborted();
    const resolved = ref ?? (await call("workspaces.resolve", {}, { signal })).ref;
    signal?.throwIfAborted();
    return bindWorkspace(call, resolved);
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
      export: (sessionId, format, signal) =>
        call("sessions.export", { sessionId, format }, { signal }),
      import: (artifact) =>
        call("sessions.import", {
          artifact,
        }),
    },
    modelInvocations: {
      list: (query, signal) => call("modelInvocations.list", query, { signal }),
    },
    runs: {
      start: (params, signal) =>
        openMutation(
          "runs.start",
          params,
          async (idempotencyKey, attempt) => {
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
      listLibrary: (signal) => call("skills.library.list", {}, { signal }),
      archive: (name) => call("skills.library.archive", { name }),
      restore: (name) => call("skills.library.restore", { name }),
    },
    mcp: {
      list: (signal) => call("mcp.servers.list", {}, { signal }),
      create: (params) => call("mcp.servers.create", params),
      update: (params) => call("mcp.servers.update", params),
      delete: (server) => call("mcp.servers.delete", { server }),
      test: (params, signal) => call("mcp.servers.test", params, { signal }),
      listTools: (server, signal) => call("mcp.tools.list", server ? { server } : {}, { signal }),
      reconnect: (server) => call("mcp.servers.reconnect", { server }),
      authorizationAttempts: {
        create: (server, signal) =>
          call("mcp.authorizationAttempts.create", { server }, { signal }),
        get: (attemptId, signal) =>
          call("mcp.authorizationAttempts.get", { attemptId }, { signal }),
      },
    },
    providers: {
      list: (signal) => call("providers.list", {}, { signal }),
      update: (params) => call("providers.update", params),
      test: (provider, signal) => call("providers.test", { provider }, { signal }),
    },
    models: {
      list: (provider, signal) => call("models.list", provider ? { provider } : {}, { signal }),
      getUtilityRole: (signal) => call("models.getUtilityRole", {}, { signal }),
      setUtilityRole: (params) => call("models.setUtilityRole", params),
      getEmbeddingRole: (signal) => call("models.getEmbeddingRole", {}, { signal }),
      setEmbeddingRole: (params) => call("models.setEmbeddingRole", params),
    },
    usage: {
      session: (sessionId, signal) => call("usage.session", { sessionId }, { signal }),
      summary: (params, signal) => call("usage.summary", params ?? {}, { signal }),
    },
    agentMemory: {
      list: (params, signal) => call("agentMemory.list", params ?? {}, { signal }),
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
      getMode: (signal) => call("approval.getMode", {}, { signal }),
      setMode: (mode) => call("approval.setMode", { mode }),
      listRules: (sessionId, signal) => call("approval.listRules", { sessionId }, { signal }),
      forgetRule: (id) => call("approval.forgetRule", { id }),
    },
    schedules: {
      list: (query, signal) => call("schedules.list", query ?? {}, { signal }),
      create: (params) => call("schedules.create", params),
      update: (params) => call("schedules.update", params),
      delete: (id) => call("schedules.delete", { id }),
      runNow: (id) => call("schedules.runNow", { id }),
    },
  };
}
