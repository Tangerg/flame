import { cloneJSON } from "@qimu/sdk/json";
import {
  callPluginCommand,
  pendingPluginCommand,
  type PluginCommandRecovery,
} from "./pluginCommand";
import { callPluginStream, queryPluginStream } from "./pluginStream";
import type {
  OpenPluginBrowserRealmRequest,
  PluginBrowserSession,
  AcquirePluginComponentRequest,
  ClosePluginBrowserRealmRequest,
  PluginBrowserUseGrant,
  OpenPluginResourcesRequest,
  PluginResourceLease,
  ReadPluginResourceRequest,
  PluginResource,
  CallPluginRequest,
  PluginCommandRequest,
  CallPluginResponse,
  ResolvePluginPointRequest,
  PluginPointBinding,
  ResolvePluginRequirementsRequest,
  PluginCollaborator,
  PluginProviderSelection,
  SelectPluginProviderRequest,
  PluginComposition,
  SetPluginCompositionRequest,
  ResolvePluginCompositionRequest,
  PluginOperationBinding,
  PluginInstallation,
  InstallPluginRequest,
  StagePluginRequest,
  PluginReleaseRequest,
  ConfigurePluginRequest,
  SetPluginEnablementRequest,
  MCPServerID,
  MCPToolExposure,
  SetMCPToolExposureRequest,
  SetApprovalRuleRequest,
} from "./generated/wire.generated";
import type { RpcCallOptions, RpcClient } from "./client";
import { MutationLookupError, type MutationPromise } from "./mutation";
import { createWireCallPath, type MethodsOptions, type WireCall } from "./wireCallPath";
import type { RunId, SegmentId, SessionId } from "./ids";
import type {
  ModelInvocation,
  ListModelInvocationsRequest,
  ListModelEndpointsRequest,
  ModelEndpointInvocation,
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
} from "@flame/runtime-client/wire";
import { streamRunEvents, streamRuntimeEvents } from "./stream";
import { openResourceLease, type ResourceSubscription } from "./resources";
import type { AutoPagingPromise } from "./pagination";
import { RUNTIME_SUBSCRIBE_METHOD } from "./transport";
import {
  MutationJournalScopeUnavailableError,
  createMutationJournal,
  type PluginCommandInput,
  type PluginCommandBinding,
} from "./mutationJournal";
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
}

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
    endpoints: (
      query: ListModelEndpointsRequest,
      signal?: AbortSignal,
    ) => AutoPagingPromise<Page<ModelEndpointInvocation>>;
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
    openBrowserRealm: (
      request: OpenPluginBrowserRealmRequest,
      signal?: AbortSignal,
    ) => MutationPromise<PluginBrowserSession>;
    acquireComponent: (
      request: AcquirePluginComponentRequest,
      signal?: AbortSignal,
    ) => MutationPromise<PluginBrowserUseGrant>;
    closeBrowserRealm: (
      request: ClosePluginBrowserRealmRequest,
      signal?: AbortSignal,
    ) => MutationPromise<void>;
    getCommand: (request: PluginCommandRequest, signal?: AbortSignal) => Promise<CallPluginRequest>;
    retireCommand: (request: PluginCommandRequest, signal?: AbortSignal) => MutationPromise<void>;
    pendingCommands: () => PluginCommandRecovery[];
    queryStream: <T>(
      request: CallPluginRequest,
      consume: (chunk: unknown, signal: AbortSignal) => void | Promise<void>,
      decode: (output: unknown) => T,
      signal?: AbortSignal,
    ) => Promise<T>;
    stream: <T>(
      intent: string,
      input: PluginCommandInput,
      prepare: () => Promise<PluginCommandBinding>,
      consume: (chunk: unknown, signal: AbortSignal) => void | Promise<void>,
      decode: (output: unknown) => T,
      signal?: AbortSignal,
    ) => Promise<T>;
    command: <T>(
      intent: string,
      input: PluginCommandInput,
      prepare: () => Promise<PluginCommandBinding>,
      decode: (output: unknown) => T,
      signal?: AbortSignal,
    ) => Promise<T>;
    listProviders: (signal?: AbortSignal) => Promise<Page<PluginProviderSelection>>;
    listCompositions: (signal?: AbortSignal) => Promise<Page<PluginComposition>>;
    setComposition: (
      request: SetPluginCompositionRequest,
      signal?: AbortSignal,
    ) => MutationPromise<PluginComposition>;
    resolveComposition: (
      request: ResolvePluginCompositionRequest,
      signal?: AbortSignal,
    ) => Promise<Page<PluginOperationBinding>>;
    selectProvider: (
      request: SelectPluginProviderRequest,
      signal?: AbortSignal,
    ) => MutationPromise<PluginProviderSelection>;
    openResources: (
      request: OpenPluginResourcesRequest,
      signal?: AbortSignal,
    ) => Promise<ResourceSubscription<PluginResourceLease>>;
    readResource: (
      request: ReadPluginResourceRequest,
      signal?: AbortSignal,
    ) => Promise<PluginResource>;
    resolvePoint: (
      request: ResolvePluginPointRequest,
      signal?: AbortSignal,
    ) => Promise<Page<PluginPointBinding>>;
    resolveRequirements: (
      request: ResolvePluginRequirementsRequest,
      signal?: AbortSignal,
    ) => Promise<Page<PluginCollaborator>>;
    call: (
      request: CallPluginRequest,
      options?: RpcCallOptions,
    ) => Promise<CallPluginResponse> | MutationPromise<CallPluginResponse>;
    list: (signal?: AbortSignal) => Promise<Page<PluginInstallation>>;
    install: (params: InstallPluginRequest) => MutationPromise<PluginInstallation>;
    stage: (params: StagePluginRequest) => MutationPromise<PluginInstallation>;
    select: (params: PluginReleaseRequest) => MutationPromise<PluginInstallation>;
    configure: (params: ConfigurePluginRequest) => MutationPromise<PluginInstallation>;
    setEnablement: (params: SetPluginEnablementRequest) => MutationPromise<PluginInstallation>;
    uninstall: (installationId: string) => MutationPromise<void>;
  };
  providers: {
    list: (signal?: AbortSignal) => Promise<Page<Provider>>;
    update: (params: UpdateProviderRequest) => MutationPromise<Provider>;
    test: (provider: string, signal?: AbortSignal) => Promise<ProviderTestResult>;
  };
  models: {
    list: (provider: string, signal?: AbortSignal) => Promise<Page<Model>>;
    getUtilityRole: (signal?: AbortSignal) => Promise<UtilityRole>;
    setUtilityRole: (params: UtilityRole) => MutationPromise<UtilityRole>;
    getEmbeddingRole: (signal?: AbortSignal) => Promise<EmbeddingRole>;
    setEmbeddingRole: (params: EmbeddingRole) => MutationPromise<EmbeddingRole>;
  };
  usage: {
    session: (sessionId: SessionId, signal?: AbortSignal) => Promise<SessionUsage>;
    summary: (params?: UsageSummaryRequest, signal?: AbortSignal) => Promise<UsageSummary>;
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
  const { call, perform, openMutation, capturePluginCommand } = createWireCallPath(client, {
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
            beforeDispatch: attempt.markDispatched,
          }),
        );
        return { result: opened.result, observation: { type: "stream", events: opened.events } };
      },
      {
        signal,
        journalKey: original?.idempotencyKey,
        reserveOriginal: original?.reserve,
      },
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
          beforeDispatch: attempt.markDispatched,
        });
        if (acknowledged.userItemId !== userItemId)
          throw new RpcProtocolError("runs.steer result", [
            { path: "$.userItemId", detail: "ACK differs from the nominated input identity" },
          ]);
        return { type: "accepted", ...acknowledged };
      },
      {
        signal,
        journalKey: original?.idempotencyKey,
        reserveOriginal: original?.reserve,
      },
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
      endpoints: (query, signal) => call("modelInvocations.endpoints", query, { signal }),
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
                  params: cloneJSON(command.params),
                  retry: (options) =>
                    openRun("runs.start", command.params, options?.signal, command),
                };
              case "resume":
                return {
                  ...observed,
                  operation: "resume",
                  params: cloneJSON(command.params),
                  retry: (options) =>
                    openRun("runs.resume", command.params, options?.signal, command),
                };
              case "steer":
                return {
                  ...observed,
                  operation: "steer",
                  userItemId: command.steerItemId,
                  params: cloneJSON(command.params),
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
      openBrowserRealm: (request, signal) => call("plugins.openBrowserRealm", request, { signal }),
      acquireComponent: (request, signal) => call("plugins.acquireComponent", request, { signal }),
      closeBrowserRealm: (request, signal) =>
        call("plugins.closeBrowserRealm", request, { signal }),
      getCommand: (request, signal) => call("plugins.getCommand", request, { signal }),
      retireCommand: (request, signal) => call("plugins.retireCommand", request, { signal }),
      pendingCommands: () =>
        mutationJournal
          .pendingPluginCommands()
          .map((command) => pendingPluginCommand(command, client, { perform })),
      queryStream: (request, consume, decode, signal) =>
        queryPluginStream(client, { perform }, request, consume, decode, signal),
      stream: (intent, input, prepare, consume, decode, signal) =>
        callPluginStream(
          client,
          { perform, openMutation, capturePluginCommand },
          intent,
          input,
          prepare,
          consume,
          decode,
          signal,
        ),
      command: (intent, input, prepare, decode, signal) =>
        callPluginCommand(
          { perform, openMutation, capturePluginCommand },
          intent,
          input,
          prepare,
          decode,
          signal,
        ),
      listProviders: (signal) => call("plugins.listProviders", {}, { signal }),
      listCompositions: (signal) => call("plugins.listCompositions", {}, { signal }),
      setComposition: (request, signal) => call("plugins.setComposition", request, { signal }),
      resolveComposition: (request, signal) =>
        call("plugins.resolveComposition", request, { signal }),
      selectProvider: (request, signal) => call("plugins.selectProvider", request, { signal }),
      openResources: (request, signal) => openResourceLease(client, call, request, signal),
      readResource: (request, signal) => call("plugins.readResource", request, { signal }),
      resolvePoint: (request, signal) => call("plugins.resolvePoint", request, { signal }),
      resolveRequirements: (request, signal) =>
        call("plugins.resolveRequirements", request, { signal }),
      call: (request, options) => call("plugins.call", request, options),
      list: (signal) => call("plugins.list", {}, { signal }),
      install: (params) => call("plugins.install", params),
      stage: (params) => call("plugins.stage", params),
      select: (params) => call("plugins.select", params),
      configure: (params) => call("plugins.configure", params),
      setEnablement: (params) => call("plugins.setEnablement", params),
      uninstall: (installationId) => call("plugins.uninstall", { installationId }),
    },
    providers: {
      list: (signal) => call("providers.list", {}, { signal }),
      update: (params) => call("providers.update", params),
      test: (provider, signal) => call("providers.test", { provider }, { signal }),
    },
    models: {
      list: (provider, signal) => call("models.list", { provider }, { signal }),
      getUtilityRole: (signal) => call("models.getUtilityRole", {}, { signal }),
      setUtilityRole: (params) => call("models.setUtilityRole", params),
      getEmbeddingRole: (signal) => call("models.getEmbeddingRole", {}, { signal }),
      setEmbeddingRole: (params) => call("models.setEmbeddingRole", params),
    },
    usage: {
      session: (sessionId, signal) => call("usage.session", { sessionId }, { signal }),
      summary: (params, signal) => call("usage.summary", params ?? {}, { signal }),
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
  };
}
