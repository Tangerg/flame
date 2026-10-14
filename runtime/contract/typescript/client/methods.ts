import type {
  PluginInstallation,
  PluginViewResource,
  ReadPluginViewRequest,
  ReadPluginRendererRequest,
  ReadPluginTrajectoryRequest,
  ReadPluginMemoryRequest,
  ReadPluginSchedulesRequest,
  ReadPluginUsageRequest,
  ReadPluginDiffRequest,
  ReadPluginSkillsRequest,
  ReadPluginSkillRequest,
  RenamePluginSessionRequest,
  InstallPluginRequest,
  StagePluginRequest,
  PluginReleaseRequest,
  ConfigurePluginRequest,
  SetPluginEnablementRequest,
  MCPServerID,
  MCPToolExposure,
  SetMCPToolExposureRequest,
  SetApprovalRuleRequest,
} from "../wire.generated";
import type { RpcCallOptions, RpcClient } from "./client";
import { MutationLookupError, type MutationPromise } from "./mutation";
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
  ExportFormat,
  ExportSessionResponse,
  ExportTrajectoryRequest,
  ExportTrajectoryResponse,
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
  ListInterruptsRequest,
  ListItemsRequest,
  ListItemsResponse,
  ListRunsRequest,
  ListSessionsRequest,
  ListSessionTrajectoryRequest,
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
  SteerRunRequest,
  SubscribeRunRequest,
  SubscribeRunResponse,
  RollbackSessionRequest,
  RollbackSessionResponse,
  RunEvent,
  ReadFileRequest,
  RunRef,
  RunAcceptance,
  GetRunAcceptanceRequest,
  RunScheduleNowResponse,
  Schedule,
  CreateScheduleRequest,
  UpdateScheduleRequest,
  Plan,
  Session,
  SessionArtifact,
  SessionSnapshot,
  TrajectoryEntry,
  SkillDiscovery,
  SkillDetail,
  ManagedSkill,
  SkillProposal,
  SkillProposalRef,
  AgentMemoryItem,
  AgentMemoryReviewDecision,
  AgentMemoryScope,
  AgentMemoryUpdateRequest,
  Goal,
  StartGoalRequest,
  StartRunRequest,
  RuntimeSubscribeRequest,
  RuntimeSubscribeResponse,
  UpdateSessionRequest,
  UpdateMCPServerRequest,
  SessionUsage,
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
import { MutationJournalScopeUnavailableError, createMutationJournal } from "./mutationJournal";
import type { PendingRunCommand } from "./mutationJournal";
import { RpcProtocolError } from "./errors";

export type { MethodsOptions } from "./wireCallPath";

export interface StreamingResult<R, E> {
  result: R;
  events: AsyncIterable<E>;
}

export interface RunOpeningResult<R> {
  result: R;
  observation:
    | { type: "stream"; events: AsyncIterable<RunEvent> }
    | { type: "snapshot"; sessionId: string }
    | { type: "retired"; sessionId: string };
}

export type SteerCommandResult = SteerRunResponse & { type: "accepted" | "retired" };

export interface SteerMutationPromise extends MutationPromise<SteerCommandResult> {
  /** An opaque Runtime nomination, cached before dispatch; it is not acceptance. */
  readonly inputIdentity: Promise<string>;
  retry(options?: { signal?: AbortSignal }): SteerMutationPromise;
}

interface CommandObservation {
  readonly idempotencyKey: string;
  readonly observation:
    { type: "known"; acceptance: RunAcceptance } | { type: "unknown"; cause: unknown };
}

export type RunCommandRecovery = CommandObservation &
  (
    | ({ operation: "start"; params: StartRunRequest } & Pick<
        MutationPromise<RunOpeningResult<StartRunResponse>>,
        "retry"
      >)
    | ({ operation: "resume"; params: ResumeRunRequest } & Pick<
        MutationPromise<RunOpeningResult<ResumeRunResponse>>,
        "retry"
      >)
    | ({ operation: "steer"; params: SteerRunRequest; userItemId: string | null } & Pick<
        SteerMutationPromise,
        "retry"
      >)
  );

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
    list: (query?: PageQuery, signal?: AbortSignal) => AutoPagingPromise<Page<AgentMemoryItem>>;
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
    trajectory: (
      query: ListSessionTrajectoryRequest,
      signal?: AbortSignal,
    ) => AutoPagingPromise<Page<TrajectoryEntry>>;
    create: (params?: CreateSessionRequest, signal?: AbortSignal) => MutationPromise<Session>;
    update: (params: UpdateSessionRequest) => MutationPromise<Session>;
    delete: (sessionId: SessionId) => MutationPromise<void>;
    fork: (params: ForkSessionRequest) => MutationPromise<Session>;
    rollback: (params: RollbackSessionRequest) => MutationPromise<RollbackSessionResponse>;
    export: (
      sessionId: SessionId,
      format?: ExportFormat,
      signal?: AbortSignal,
    ) => Promise<ExportSessionResponse>;
    exportTrajectory: (
      params: ExportTrajectoryRequest,
      signal?: AbortSignal,
    ) => Promise<ExportTrajectoryResponse>;
    import: (artifact: SessionArtifact) => MutationPromise<ImportSessionResponse>;
  };
  modelInvocations: {
    list: (
      query: ListModelInvocationsRequest,
      signal?: AbortSignal,
    ) => AutoPagingPromise<Page<ModelInvocation>>;
  };
  runs: {
    recoverCommands: (signal?: AbortSignal) => Promise<RunCommandRecovery[]>;
    start: (
      params: StartRunRequest,
      signal?: AbortSignal,
    ) => MutationPromise<RunOpeningResult<StartRunResponse>>;
    resume: (
      params: ResumeRunRequest,
      signal?: AbortSignal,
    ) => MutationPromise<RunOpeningResult<ResumeRunResponse>>;
    getAcceptance: (
      params: GetRunAcceptanceRequest,
      signal?: AbortSignal,
    ) => Promise<RunAcceptance>;
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
    ) => SteerMutationPromise;
    get: (runId: RunId, signal?: AbortSignal) => Promise<RunRef>;
    list: (
      query?: Omit<ListRunsRequest, "sessionId"> & { sessionId?: SessionId },
      signal?: AbortSignal,
    ) => AutoPagingPromise<Page<RunRef>>;
  };
  plan: {
    get: (sessionId: SessionId, signal?: AbortSignal) => Promise<Plan>;
  };
  interrupts: {
    list: (
      query?: Omit<ListInterruptsRequest, "sessionId" | "rootRunId"> & {
        sessionId?: SessionId;
        rootRunId?: RunId;
      },
      signal?: AbortSignal,
    ) => AutoPagingPromise<Page<PendingInterruptSet>>;
  };
  items: {
    list: (params: ListItemsRequest, signal?: AbortSignal) => AutoPagingPromise<ListItemsResponse>;
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
    delete: (server: MCPServerID) => MutationPromise<void>;
    test: (params: MCPServerCandidate, signal?: AbortSignal) => Promise<MCPTestResult>;
    listTools: (server?: MCPServerID, signal?: AbortSignal) => Promise<Page<MCPTool>>;
    toolExposure: (server: MCPServerID, signal?: AbortSignal) => Promise<MCPToolExposure>;
    setToolExposure: (params: SetMCPToolExposureRequest) => MutationPromise<void>;
    reconnect: (server: MCPServerID) => MutationPromise<void>;
    authorizationAttempts: {
      create: (
        server: MCPServerID,
        signal?: AbortSignal,
      ) => MutationPromise<MCPAuthorizationAttempt>;
      get: (attemptId: string, signal?: AbortSignal) => Promise<MCPAuthorizationAttempt>;
    };
  };
  plugins: {
    renameSession: (params: RenamePluginSessionRequest) => MutationPromise<Session>;
    readView: (params: ReadPluginViewRequest, signal?: AbortSignal) => Promise<PluginViewResource>;
    readRenderer: (
      params: ReadPluginRendererRequest,
      signal?: AbortSignal,
    ) => Promise<PluginViewResource>;
    readTrajectory: (
      params: ReadPluginTrajectoryRequest,
      signal?: AbortSignal,
    ) => Promise<Page<TrajectoryEntry>>;
    readDiff: (params: ReadPluginDiffRequest, signal?: AbortSignal) => Promise<Diff>;
    readUsage: (params: ReadPluginUsageRequest, signal?: AbortSignal) => Promise<UsageSummary>;
    readSkills: (params: ReadPluginSkillsRequest, signal?: AbortSignal) => Promise<SkillDiscovery>;
    readSkill: (params: ReadPluginSkillRequest, signal?: AbortSignal) => Promise<SkillDetail>;
    readSkillLibrary: (
      params: ReadPluginViewRequest,
      signal?: AbortSignal,
    ) => Promise<Page<ManagedSkill>>;
    readSkillProposals: (
      params: ReadPluginSkillsRequest,
      signal?: AbortSignal,
    ) => Promise<Page<SkillProposal>>;
    readSchedules: (
      params: ReadPluginSchedulesRequest,
      signal?: AbortSignal,
    ) => Promise<Page<Schedule>>;
    readMemory: (
      params: ReadPluginMemoryRequest,
      signal?: AbortSignal,
    ) => Promise<Page<AgentMemoryItem>>;
    list: (signal?: AbortSignal) => Promise<Page<PluginInstallation>>;
    install: (params: InstallPluginRequest) => MutationPromise<PluginInstallation>;
    stage: (params: StagePluginRequest) => MutationPromise<PluginInstallation>;
    select: (params: PluginReleaseRequest) => MutationPromise<PluginInstallation>;
    approve: (params: PluginReleaseRequest) => MutationPromise<PluginInstallation>;
    configure: (params: ConfigurePluginRequest) => MutationPromise<PluginInstallation>;
    setEnablement: (params: SetPluginEnablementRequest) => MutationPromise<PluginInstallation>;
    revoke: (installationId: string) => MutationPromise<PluginInstallation>;
    uninstall: (installationId: string) => MutationPromise<void>;
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
    session: (sessionId: SessionId, signal?: AbortSignal) => Promise<SessionUsage>;
    summary: (params?: UsageSummaryRequest, signal?: AbortSignal) => Promise<UsageSummary>;
  };
  agentMemory: {
    list: (
      target: AgentMemoryTarget & PageQuery,
      signal?: AbortSignal,
    ) => AutoPagingPromise<Page<AgentMemoryItem>>;
    review: (id: string, decision: AgentMemoryReviewDecision) => MutationPromise<void>;
    update: (params: AgentMemoryUpdateRequest) => MutationPromise<AgentMemoryItem>;
    delete: (id: string) => MutationPromise<void>;
    add: (params: AgentMemoryTarget & { content: string }) => MutationPromise<AgentMemoryItem>;
  };
  goals: {
    get: (sessionId: SessionId, signal?: AbortSignal) => Promise<Goal | null>;
    start: (
      params: Omit<StartGoalRequest, "sessionId"> & { sessionId: SessionId },
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
    listRules: (sessionId?: SessionId, signal?: AbortSignal) => Promise<ListApprovalRulesResult>;
    setRule: (params: SetApprovalRuleRequest) => MutationPromise<void>;
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
      list: (query, signal) =>
        call("agentMemory.list", { ...query, scope: "project", workspace }, { signal }),
      add: (content) => call("agentMemory.add", { scope: "project", workspace, content }),
    },
  };
}

export function createMethods(client: RpcClient, options: MethodsOptions = {}): Methods {
  const runEventStreamOptions = (signal?: AbortSignal) => ({
    signal,
    replayLimits: options.capabilities?.()?.limits.runReplay,
  });

  const mutationJournal =
    options.mutationJournal ??
    createMutationJournal({
      scope: () => options.capabilities?.()?.limits.idempotency,
    });
  const { call, perform, openMutation } = createWireCallPath(client, {
    ...options,
    mutationJournal,
  });

  const attachRunStream = async <R extends { segmentId: string }>(
    signal: AbortSignal | undefined,
    request: (options: Pick<RpcCallOptions, "signal" | "onRequestRpcId">) => Promise<R>,
  ): Promise<StreamingResult<R, RunEvent>> => {
    const stream = streamRunEvents(client, runEventStreamOptions(signal));
    const result = await callOrDispose(stream, () =>
      request({ signal: stream.requestSignal, onRequestRpcId: stream.bindRequest }),
    );
    stream.bind(result.segmentId);
    return { result, events: stream.events };
  };

  const queryAcceptance = (
    operation: "start" | "resume" | "steer",
    namespace: string,
    key: string,
    signal?: AbortSignal,
  ) =>
    perform("runs.getAcceptance", { namespace, key, operation }, { signal })
      .then((receipt) => {
        if (
          receipt.type !== "notRecorded" &&
          operation !== "resume" &&
          (!receipt.userItemId || (operation === "steer" && !receipt.input?.length))
        )
          throw new RpcProtocolError("runs.getAcceptance result", [
            {
              path: "$",
              detail: `accepted ${operation} omitted its original input identity or content`,
            },
          ]);
        return receipt;
      })
      .catch((cause: unknown) => {
        throw new MutationLookupError(cause);
      });

  function openRun(
    method: "runs.start",
    params: StartRunRequest,
    signal?: AbortSignal,
    original?: PendingRunCommand,
  ): MutationPromise<RunOpeningResult<StartRunResponse>>;
  function openRun(
    method: "runs.resume",
    params: ResumeRunRequest,
    signal?: AbortSignal,
    original?: PendingRunCommand,
  ): MutationPromise<RunOpeningResult<ResumeRunResponse>>;
  function openRun(
    method: "runs.start" | "runs.resume",
    params: StartRunRequest | ResumeRunRequest,
    signal?: AbortSignal,
    original?: PendingRunCommand,
  ): MutationPromise<RunOpeningResult<ResumeRunResponse>> {
    return openMutation(
      method,
      params,
      async (idempotencyKey, attempt, preparedParams) => {
        const namespace = attempt.lookupNamespace();
        if (!namespace)
          throw new MutationJournalScopeUnavailableError(
            "run opening requires the Runtime store identity",
          );
        const acceptance = await queryAcceptance(
          method === "runs.start" ? "start" : "resume",
          namespace,
          idempotencyKey,
          attempt.signal,
        );
        if (acceptance.type !== "notRecorded") {
          return {
            result: {
              runId: acceptance.runId,
              segmentId: acceptance.segmentId,
              ...(acceptance.userItemId ? { userItemId: acceptance.userItemId } : {}),
            },
            observation: {
              type: acceptance.type === "retired" ? "retired" : "snapshot",
              sessionId: acceptance.sessionId,
            },
          };
        }
        const idempotencyNamespace = attempt.authorizeDispatch();
        const opened = await attachRunStream(attempt.signal, (options) =>
          perform(method, preparedParams, {
            ...options,
            idempotencyKey,
            idempotencyNamespace,
            requestMeta: attempt.requestMeta,
          }),
        );
        return { result: opened.result, observation: { type: "stream", events: opened.events } };
      },
      signal,
      undefined,
      original?.idempotencyKey,
      original?.reserve,
    );
  }

  const openSteer = (
    params: SteerRunRequest,
    signal?: AbortSignal,
    original?: PendingRunCommand,
  ): SteerMutationPromise => {
    let knownIdentity: string | undefined;
    const observers = new Set<ReturnType<typeof Promise.withResolvers<string>>>();
    const publishIdentity = async (userItemId: string) => {
      if (knownIdentity !== undefined && knownIdentity !== userItemId)
        throw new RpcProtocolError("runs.steer identity", [
          { path: "$.userItemId", detail: "Runtime changed the original input identity" },
        ]);
      knownIdentity = userItemId;
      const published = [...observers];
      for (const observer of observers) observer.resolve(userItemId);
      observers.clear();
      await Promise.all(published.map((observer) => observer.promise));
    };
    const expose = (mutation: MutationPromise<SteerCommandResult>): SteerMutationPromise => {
      const identity = Promise.withResolvers<string>();
      if (knownIdentity !== undefined) identity.resolve(knownIdentity);
      else observers.add(identity);
      // Observing nomination is optional; the command still reports every failure.
      identity.promise.catch(() => undefined);
      const exposed = mutation.then(
        (result) => result,
        (error: unknown) => {
          observers.delete(identity);
          identity.reject(error);
          throw error;
        },
      );
      return Object.defineProperties(exposed, {
        idempotencyKey: { enumerable: true, value: mutation.idempotencyKey },
        inputIdentity: { enumerable: true, value: identity.promise },
        retry: {
          enumerable: true,
          value: (options?: { signal?: AbortSignal }) => expose(mutation.retry(options)),
        },
      }) as SteerMutationPromise;
    };
    const mutation = openMutation(
      "runs.steer",
      params,
      async (key, attempt, prepared): Promise<SteerCommandResult> => {
        if (attempt.steerItemId !== null) await publishIdentity(attempt.steerItemId);
        const namespace = attempt.lookupNamespace();
        if (!namespace)
          throw new MutationJournalScopeUnavailableError(
            "steer requires the Runtime store identity",
          );
        const acceptance = await queryAcceptance("steer", namespace, key, attempt.signal);
        const userItemId =
          acceptance.type === "notRecorded"
            ? acceptance.candidateUserItemId
            : acceptance.userItemId;
        if (!userItemId)
          throw new RpcProtocolError("runs.getAcceptance result", [
            { path: "$", detail: "Steer lookup omitted its input identity" },
          ]);
        attempt.bindSteerItem(userItemId);
        await publishIdentity(userItemId);
        if (acceptance.type !== "notRecorded") return { type: acceptance.type, userItemId };
        const acknowledged = await perform("runs.steer", prepared, {
          signal: attempt.signal,
          idempotencyKey: key,
          idempotencyNamespace: attempt.authorizeDispatch(),
          requestMeta: attempt.requestMeta,
        });
        if (acknowledged.userItemId !== userItemId)
          throw new RpcProtocolError("runs.steer result", [
            { path: "$.userItemId", detail: "ACK differs from the nominated input identity" },
          ]);
        return { type: "accepted", ...acknowledged };
      },
      signal,
      undefined,
      original?.idempotencyKey,
      original?.reserve,
    );
    return expose(mutation);
  };

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
          {
            sessionId,
            ...(includeDescendants ? { includeDescendants: true } : {}),
          },
          { signal },
        ),
      trajectory: (query, signal) => call("sessions.trajectory", query, { signal }),
      create: (params, signal) => call("sessions.create", params ?? {}, { signal }),
      update: (params) => call("sessions.update", params),
      delete: (sessionId) => call("sessions.delete", { sessionId }),
      fork: (params) => call("sessions.fork", params),
      rollback: (params) => call("sessions.rollback", params),
      export: (sessionId, format, signal) =>
        call("sessions.export", { sessionId, format }, { signal }),
      exportTrajectory: (params, signal) => call("sessions.exportTrajectory", params, { signal }),
      import: (artifact) =>
        call("sessions.import", {
          artifact,
        }),
    },
    modelInvocations: {
      list: (query, signal) => call("modelInvocations.list", query, { signal }),
    },
    runs: {
      recoverCommands: async (signal) =>
        Promise.all(
          mutationJournal.pendingCommands().map(async (command): Promise<RunCommandRecovery> => {
            let observation: RunCommandRecovery["observation"];
            try {
              const acceptance = await queryAcceptance(
                command.operation,
                command.namespace,
                command.idempotencyKey,
                signal,
              );
              if (acceptance.type !== "notRecorded") command.settle();
              observation = { type: "known", acceptance };
            } catch (cause) {
              observation = { type: "unknown", cause };
            }
            const observed = {
              idempotencyKey: command.idempotencyKey,
              observation,
            };
            switch (command.operation) {
              case "start":
                return {
                  ...observed,
                  operation: "start",
                  params: structuredClone(command.params),
                  retry: (options) =>
                    openRun("runs.start", command.params, options?.signal, command),
                };
              case "resume":
                return {
                  ...observed,
                  operation: "resume",
                  params: structuredClone(command.params),
                  retry: (options) =>
                    openRun("runs.resume", command.params, options?.signal, command),
                };
              case "steer":
                return {
                  ...observed,
                  operation: "steer",
                  userItemId: command.steerItemId,
                  params: structuredClone(command.params),
                  retry: (options) => openSteer(command.params, options?.signal, command),
                };
            }
          }),
        ),
      getAcceptance: (params, signal) => call("runs.getAcceptance", params, { signal }),
      start: (params, signal) => openRun("runs.start", params, signal),
      resume: (params, signal) => openRun("runs.resume", params, signal),
      subscribe: (params, signal, options) =>
        attachRunStream(signal, (callOptions) =>
          call("runs.subscribe", params, { ...callOptions, lastEventId: options?.lastEventId }),
        ),
      cancel: (runId, reason) => call("runs.cancel", { runId, reason }),
      steer: (runId, expectedSegmentId, input) => openSteer({ runId, expectedSegmentId, input }),
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
      toolExposure: (server, signal) => call("mcp.tools.exposure", { server }, { signal }),
      setToolExposure: (params) => call("mcp.tools.setExposure", params),
      reconnect: (server) => call("mcp.servers.reconnect", { server }),
      authorizationAttempts: {
        create: (server, signal) =>
          call("mcp.authorizationAttempts.create", { server }, { signal }),
        get: (attemptId, signal) =>
          call("mcp.authorizationAttempts.get", { attemptId }, { signal }),
      },
    },
    plugins: {
      renameSession: (params) => call("plugins.renameSession", params),
      readView: (params, signal) => call("plugins.readView", params, { signal }),
      readRenderer: (params, signal) => call("plugins.readRenderer", params, { signal }),
      readTrajectory: (params, signal) => call("plugins.readTrajectory", params, { signal }),
      readDiff: (params, signal) => call("plugins.readDiff", params, { signal }),
      readUsage: (params, signal) => call("plugins.readUsage", params, { signal }),
      readSkills: (params, signal) => call("plugins.readSkills", params, { signal }),
      readSkill: (params, signal) => call("plugins.readSkill", params, { signal }),
      readSkillLibrary: (params, signal) => call("plugins.readSkillLibrary", params, { signal }),
      readSkillProposals: (params, signal) =>
        call("plugins.readSkillProposals", params, { signal }),
      readSchedules: (params, signal) => call("plugins.readSchedules", params, { signal }),
      readMemory: (params, signal) => call("plugins.readMemory", params, { signal }),
      list: (signal) => call("plugins.list", {}, { signal }),
      install: (params) => call("plugins.install", params),
      stage: (params) => call("plugins.stage", params),
      select: (params) => call("plugins.select", params),
      approve: (params) => call("plugins.approve", params),
      configure: (params) => call("plugins.configure", params),
      setEnablement: (params) => call("plugins.setEnablement", params),
      revoke: (installationId) => call("plugins.revoke", { installationId }),
      uninstall: (installationId) => call("plugins.uninstall", { installationId }),
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
      setRule: (params) => call("approval.setRule", params),
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
