import { visualFilePage } from "./filePage";
import { GOAL_WORKFLOW } from "@/features/plugins/public/workflows";
import { mutationResult } from "@/test/runtimeClient";
import { type VisualClient } from "./client";
import { type FlameClient } from "@flame/runtime-client";
import {
  VISUAL_FILE_MENTION_DRAFT,
  VISUAL_WORKSPACE_PATH,
  type VisualFileListState,
} from "./agentFixtureFacts";
import { type CommandResult, type ServerCapabilities } from "@flame/runtime-client/wire";

import { subagentsView } from "@/features/chat/message/subagents";
import { registerApprovalPolicyDataProviders } from "@/features/agent/adapters/runtimeDataProviders";
import { reduceAgentEvent } from "@/features/agent/application/fold/reducer";
import {
  type AgentRuntimeGateway,
  type AgentSteerSubmission,
} from "@/features/agent/application/runtimeGateway";
import { projectAgentSessionSnapshot } from "@/features/agent/application/session/sessionSnapshot";
import { type AgentSessionView } from "@/features/agent/public/sessionView";
import { AGENT_SESSIONS_KEY, type AgentSessionSummary } from "@/features/agent/public/session";

import { AGENT_SESSIONS } from "@/features/agent/public/services";
import { APPROVAL_MODE_KEY } from "@/features/agent/public/approvalPolicy";
import { type ApprovalModeResult } from "@/features/agent/public/approvalPolicy";
import { createComposerBootstrap } from "@/features/composer/index";
import { composerKeymap } from "@/features/composer/index";
import { composerSend } from "@/features/composer/index";
import { composerToolbar } from "@/features/composer/index";
import contextUsage from "@/features/chat/context-usage/index";
import narrativeRails from "@/features/chat/narrative-rails/index";
import chatSearch from "@/features/chat/search/index";
import { messageCopy } from "@/features/chat/message-actions/index";
import { messageEdit } from "@/features/chat/message-actions/index";
import { messageRegenerate } from "@/features/chat/message-actions/index";
import { builtinVisualStyles } from "@/features/theme/visualStyles/index";
import customTheme from "@/features/theme/themes/custom-theme";
import flameDark from "@/features/theme/themes/flame-dark";
import flameLight from "@/features/theme/themes/flame-light";
import { defaultAccents } from "@/features/theme/defaultAccents";
import { createGoalPlugin } from "@/features/goal/index";
import { type GoalState } from "@/features/goal/application/goalReadModel";
import planProgress from "@/features/chat/plan-progress/index";
import { workbenchChat, workbenchSettings } from "@/features/shell/workbench/index";
import shortcutsSettings from "@/features/command/shortcuts/index";
import { workspaceKeymap } from "@/features/workspace/keymap";
import { MODELS_KEY } from "@/features/providers/public/queries";
import { SelectableModel } from "@/features/providers/public/queries";
import { queryClient } from "@/lib/queryClient";
import { loadPluginsForTest } from "@/test/kernel";
import { createToolRenderingPlugins } from "@/app/builtinPlugins";
import { commitVisualInterruptResponses } from "./interruptResponses";
import { DATA_PROVIDER } from "@/extensions/dataProvider";
import { definePlugin } from "@/extensions/definePlugin";
import { visualNavigationBootstrap, visualRuntimeStream } from "./visualRuntimePorts";
import { visualToolResults } from "./visualToolResults";
import {
  WORKSPACE_FOLDERS_KEY,
  type WorkspaceFolderSummary,
} from "@/features/workspace/public/queries";
import {
  WORKSPACE_LIST_FILES_KEY,
  type WorkspaceFileEntry,
  type WorkspaceListFilesQuery,
} from "@/features/workspace/application/workspaceQueries";
import {
  projectRuntimeAgentSnapshot,
  type RuntimeAgentSessionSnapshot,
  AGENT_SESSION_TAIL_EVENTS,
  VISUAL_GOALS,
  VISUAL_ROOT_RUN_ID,
  VISUAL_SESSION_ID,
  type VisualAgentState,
} from "./agentSessionSnapshots";
import { installVisualRuntimeServiceStatusPort } from "./installVisualRuntimeServiceStatusPort";
import { VISUAL_MODELS } from "./runtimeSnapshots";
function visualSession(
  state: VisualAgentState,
  snapshot: RuntimeAgentSessionSnapshot,
): AgentSessionSummary {
  const root = snapshot.runs.find((run) => run.id === VISUAL_ROOT_RUN_ID);
  return {
    id: VISUAL_SESSION_ID,
    revision: 7,
    title: `Agent · ${state}`,
    status: root && root.status !== "finished" ? root.status : "idle",
    provider: VISUAL_MODELS[0]!.provider,
    model: VISUAL_MODELS[0]!.id,
    reasoningEffort: VISUAL_MODELS[0]!.capabilities?.reasoningDefaultLevel,
    workspace: {
      path: VISUAL_WORKSPACE_PATH,
      availability: state === "cwd-missing" ? "missing" : "available",
    },
    time: "2026-07-31T08:00:00.000Z",
  };
}

function visualAgentRuntimeGateway(
  snapshot: RuntimeAgentSessionSnapshot,
  initial: AgentSessionSummary,
  sessions: Map<string, AgentSessionSummary>,
  runtimeClient: () => FlameClient,
): AgentRuntimeGateway {
  return {
    createSession: async ({ cwd, projectId }) => {
      sessions.set(initial.id, {
        ...initial,
        projectId,
        workspace: { ...initial.workspace, path: cwd },
      });
      return initial.id;
    },
    deleteSession: async (id) => {
      sessions.delete(id);
    },
    updateSession: async ({ sessionId, expectedRevision, title, favorite, cwd, project }) => {
      const session = sessions.get(sessionId);
      if (!session || session.revision !== expectedRevision)
        throw new Error("Visual Session changed");
      sessions.set(sessionId, {
        ...session,
        revision: session.revision + 1,
        ...(title !== undefined ? { title } : {}),
        ...(favorite !== undefined ? { favorite } : {}),
        ...(cwd !== undefined ? { workspace: { ...session.workspace, path: cwd } } : {}),
        ...(project ? { projectId: project.type === "set" ? project.projectId : undefined } : {}),
      });
    },
    forkSession: async ({ sessionId }) => {
      const session = sessions.get(sessionId);
      if (!session) throw new Error("Visual Session is unavailable");
      const id = `${session.id}_fork`;
      sessions.set(id, { ...session, id, revision: 1 });
      return id;
    },
    loadSessionSnapshot: async (sessionId) => ({
      snapshot: projectRuntimeAgentSnapshot(snapshot),
      rollbackBasis: {
        expectedLastRootRunId: snapshot.runs.findLast((run) => !run.parentRunId)?.id ?? "",
        expectedWorkspacePath: sessions.get(sessionId)?.workspace.path ?? initial.workspace.path,
      },
      projectAssociatedSharedMaterial: (shared) => shared,
    }),
    loadSessionUsage: async () => ({}),
    rollbackSession: async () => ({ droppedRuns: [] }),
    steerRun: (runId, segmentId, input) => {
      const mutation = mutationResult(async () => {
        document.documentElement.dataset.visualSteeredRun = runId;
        document.documentElement.dataset.visualSteeredSegment = segmentId;
        document.documentElement.dataset.visualSentInput = JSON.stringify(input);
        return { type: "accepted" as const, userItemId: "item_visual_steer" };
      });
      const expose = (current: typeof mutation): AgentSteerSubmission =>
        Object.defineProperties(
          current.then((value) => value),
          {
            idempotencyKey: { value: current.idempotencyKey },
            inputIdentity: { value: Promise.resolve("item_visual_steer") },
            retry: { value: () => expose(current.retry()) },
          },
        ) as AgentSteerSubmission;
      return expose(mutation);
    },
    isInputReceiptUnknown: () => false,
    isRunGone: () => false,
    isReplayLost: () => false,
    setApprovalMode: (mode) => runtimeClient().approval.setMode(mode),
    forgetApprovalRule: async () => undefined,
  };
}

const VISUAL_WORKSPACE_FILES: readonly WorkspaceFileEntry[] = [
  { path: "runtime/session/store.go", name: "store.go", type: "file", sizeBytes: 12_288 },
  {
    path: "runtime/session/store_test.go",
    name: "store_test.go",
    type: "file",
    sizeBytes: 8_192,
  },
  {
    path: "runtime/contract/openrpc.json",
    name: "openrpc.json",
    type: "file",
    sizeBytes: 40_960,
  },
  {
    path: "apps/web/src/main.tsx",
    name: "main.tsx",
    type: "file",
    sizeBytes: 1_024,
  },
  { path: "README.md", name: "README.md", type: "file", sizeBytes: 2_048 },
];

// Literal path-fragment selection over captured presentation data; Runtime owns real search ranking.
const fileListProvider = (state: VisualFileListState | undefined) => {
  const failure =
    state === "error" ? new Error("workspace file listing is temporarily unavailable.") : undefined;
  return definePlugin({
    name: "flame.visual.file-list",
    setup(ctx) {
      ctx.contributions.add(DATA_PROVIDER, {
        key: WORKSPACE_LIST_FILES_KEY,
        fetcher: async (params) => {
          if (state !== undefined) {
            const root = document.documentElement;
            root.dataset.visualFileListAttempts = String(
              Number(root.dataset.visualFileListAttempts ?? 0) + 1,
            );
          }
          // Hold the captured read; production query state owns the loading presentation.
          if (state === "loading") return new Promise<never>(() => {});
          if (failure) throw failure;
          const { nameQuery, limit } = (params ?? {}) as WorkspaceListFilesQuery;
          const fragment = nameQuery?.toLowerCase();
          const selected =
            fragment === undefined
              ? VISUAL_WORKSPACE_FILES
              : VISUAL_WORKSPACE_FILES.filter((file) => file.path.toLowerCase().includes(fragment));
          return visualFilePage(selected.slice(0, limit));
        },
      });
    },
  });
};

const visualAgentSessions = (
  visual: VisualClient,
  runtimeClient: () => FlameClient,
  sessions: ReadonlyMap<string, AgentSessionSummary>,
) =>
  definePlugin({
    name: "flame.visual.agent-session-ports",
    provides: { sessions: AGENT_SESSIONS },
    setup(ctx) {
      registerApprovalPolicyDataProviders(ctx.contributions, runtimeClient);
      ctx.contributions.add(DATA_PROVIDER, {
        key: AGENT_SESSIONS_KEY,
        fetcher: async () => [...sessions.values()],
      });
      return {
        sessions: {
          getActiveSessionId: () => visual.agent.access.getActiveSessionId(),
          getLifecycleSnapshot: () => visual.agent.access.getAgentSessionLifecycleSnapshot(),
          subscribeActiveSessionId: (listener) =>
            visual.agent.access.subscribeActiveSessionId(listener),
          subscribeLifecycle: (listener) =>
            visual.agent.access.subscribeAgentSessionLifecycle(listener),
          subscribeDeleted: () => () => undefined,
        },
      };
    },
  });

function seedCommandOutput(
  snapshot: RuntimeAgentSessionSnapshot,
  commandOutput: string | undefined,
) {
  if (commandOutput !== undefined) {
    const command = snapshot.items.find(
      (item) => item.type === "toolCall" && item.tool.name === "shell",
    );
    if (!command || command.type !== "toolCall")
      throw new Error("The fixture needs a shell call to show command output");
    snapshot.items = snapshot.items.map((item) =>
      item === command
        ? {
            ...command,
            tool: {
              ...command.tool,
              result: { ...(command.tool.result as CommandResult), output: commandOutput },
            },
          }
        : item,
    );
  }
}

type AgentProjectionStore = ReturnType<VisualClient["agent"]["projection"]["store"]["getState"]>;

// A recorded view replaces the Session's projection the way a fresh read would.
function publishVisualView(store: AgentProjectionStore, view: AgentSessionView, failure: string) {
  const refresh = store.beginViewRefresh(VISUAL_SESSION_ID, "invalidate");
  if (!refresh || !store.commitViewRefresh(VISUAL_SESSION_ID, refresh, view))
    throw new Error(failure);
}

// Idle observes durable drafts; explicit scene seeds must retire pending restoration.
function seedComposer(
  composer: ReturnType<VisualClient["composer"]["store"]["getState"]>,
  state: VisualAgentState,
  fileListState: VisualFileListState | undefined,
) {
  const model = VISUAL_MODELS[0]!;
  const reasoningEffort = model.capabilities?.reasoningDefaultLevel;
  composer.setModel({
    kind: "explicit",
    provider: model.provider,
    model: model.id,
    ...(reasoningEffort ? { reasoningEffort } : {}),
  });
  if (state === "idle" && !fileListState) return;
  composer.replaceDraft({ text: seededDraft(state, fileListState) });
}

function seededDraft(state: VisualAgentState, fileListState: VisualFileListState | undefined) {
  if (fileListState) return VISUAL_FILE_MENTION_DRAFT;
  return state === "steer" ? "Tighten the error copy and continue." : "";
}

export async function installVisualAgentFixture(
  visual: VisualClient,
  runtimeClient: () => FlameClient,
  state: VisualAgentState,
  commandOutput: string | undefined,
  snapshot: RuntimeAgentSessionSnapshot,
  capabilities: ServerCapabilities,
  fileListState?: VisualFileListState,
): Promise<AgentSessionView> {
  seedCommandOutput(snapshot, commandOutput);
  const projectless = state === "empty" || state === "runtime-down";
  const initial = visualSession(state, snapshot);
  const sessions = new Map(projectless ? [] : [[initial.id, initial]]);
  const goalAuthor = {
    installationId: "ed7c6993-768d-454d-adf8-34fbdb263c87",
    release: "f".repeat(64),
    contribution: "goal-workflow",
  };
  const goal = VISUAL_GOALS[state];
  visual.installedPlugins.access.workflows.publish(
    state === "runtime-down" ? [] : [{ source: goalAuthor, roles: [GOAL_WORKFLOW] }],
    [
      {
        role: GOAL_WORKFLOW,
        installationId: goalAuthor.installationId,
        contribution: goalAuthor.contribution,
      },
    ],
  );
  queryClient.clear();
  visual.projection.store.setState({ capabilities });
  installVisualRuntimeServiceStatusPort(visual, state === "runtime-down" ? "unavailable" : "ready");
  visual.agent.dependencies = {
    ...visual.agent.dependencies,
    runtime: visualAgentRuntimeGateway(snapshot, initial, sessions, runtimeClient),
  };

  visual.agent.sessions.store.setState({
    openSessionIds: projectless ? [] : [VISUAL_SESSION_ID],
    lastSessionId: projectless ? "" : VISUAL_SESSION_ID,
  });
  visual.navigation.go({ session: projectless ? "" : VISUAL_SESSION_ID });
  queryClient.setQueryDefaults([AGENT_SESSIONS_KEY], { staleTime: Infinity });
  queryClient.setQueryDefaults([WORKSPACE_FOLDERS_KEY], { staleTime: Infinity });
  queryClient.setQueryDefaults([MODELS_KEY], { staleTime: Infinity });
  queryClient.setQueryDefaults([APPROVAL_MODE_KEY], { staleTime: Infinity });
  queryClient.setQueryData<WorkspaceFolderSummary[]>([WORKSPACE_FOLDERS_KEY], []);
  queryClient.setQueryData(
    [MODELS_KEY],
    VISUAL_MODELS.map(
      (model) =>
        new SelectableModel({
          ...model,
          ...model.capabilities,
          label: model.displayName ?? model.id,
        }),
    ),
  );
  queryClient.setQueryData<ApprovalModeResult>(
    [APPROVAL_MODE_KEY],
    await runtimeClient().approval.getMode(),
  );
  await loadPluginsForTest(
    flameLight(visual.theme.access.preference),
    defaultAccents,
    flameDark(visual.theme.access.preference),
    customTheme(visual.theme.access.preference),
    ...builtinVisualStyles,
    visualAgentSessions(visual, runtimeClient, sessions),
    visualRuntimeStream(visual),
    visualNavigationBootstrap(visual),
    createComposerBootstrap(visual.composer),
    composerKeymap(visual.agent.access, visual.composer.access, visual.runtime),
    composerToolbar,
    contextUsage,
    composerSend,
    narrativeRails,
    chatSearch,
    workbenchChat,
    workbenchSettings(visual.workspace.access),
    shortcutsSettings(visual.workspace.access),
    workspaceKeymap(visual.workspace.access),
    messageCopy,
    messageEdit,
    messageRegenerate,
    createGoalPlugin(
      visual.agent.access,
      visual.composer.access,
      visual.runtime,
      runtimeClient,
      visual.goals,
      visual.installedPlugins.access.workflows,
      visual.messages,
    ),
    planProgress,
    subagentsView,
    ...createToolRenderingPlugins(visual.workspace.access, visual.messages),
    await visualToolResults(runtimeClient, state === "genui-report"),
    fileListProvider(fileListState),
  );
  queryClient.setQueryData([AGENT_SESSIONS_KEY], [...sessions.values()]);

  seedComposer(visual.composer.store.getState(), state, fileListState);

  let view = projectAgentSessionSnapshot(projectRuntimeAgentSnapshot(snapshot));
  for (const event of AGENT_SESSION_TAIL_EVENTS[state]) {
    view = reduceAgentEvent(view, event);
  }
  view = {
    ...view,
    shared: {
      ...view.shared,
      goal: {
        available: true,
        goal: goal ? { ...goal, author: goalAuthor } : null,
      } satisfies GoalState,
    },
  };

  const store = visual.agent.projection.store.getState();
  store.ensureSession(VISUAL_SESSION_ID);
  publishVisualView(store, view, `Failed to install visual agent state "${state}"`);
  store.setSend(VISUAL_SESSION_ID, (input) => {
    document.documentElement.dataset.visualSentInput = JSON.stringify(input);
    return Promise.resolve({
      status: "accepted",
      sessionId: VISUAL_SESSION_ID,
      runId: VISUAL_ROOT_RUN_ID,
      userItemId: "item_visual_submitted",
    });
  });
  const rootStopPresentation = new URLSearchParams(location.search).get("root-stop");
  let rootStopInvocations = 0;
  store.setStop(VISUAL_SESSION_ID, () => {
    document.documentElement.dataset.visualStoppedRoot = VISUAL_ROOT_RUN_ID;
    document.documentElement.dataset.visualRootStopCalls = String(++rootStopInvocations);
    if (rootStopPresentation === "unavailable") {
      installVisualRuntimeServiceStatusPort(visual, "unavailable");
      return false;
    }
    if (rootStopPresentation === "settled") {
      const root = snapshot.runs[0];
      if (
        snapshot.runs.length !== 1 ||
        root?.id !== VISUAL_ROOT_RUN_ID ||
        root.status === "finished"
      )
        throw new Error("The stop fixture requires one unfinished captured root Run");
      const finished = {
        ...root,
        status: "finished" as const,
        finishedAt: "2026-07-31T08:02:00.000Z",
        outcome: { type: "canceled" as const, detail: "The isolated fixture stopped this Run." },
      };
      delete finished.activeSegmentId;
      snapshot.runs = [finished];
      snapshot.pendingInterruptSets = [];
      const stoppedView = AGENT_SESSION_TAIL_EVENTS[state].reduce(
        reduceAgentEvent,
        projectAgentSessionSnapshot(projectRuntimeAgentSnapshot(snapshot)),
      );
      publishVisualView(store, stoppedView, "Failed to publish the captured root outcome");
    }
    return true;
  });
  store.setResume(VISUAL_SESSION_ID, (runId, responses, onSettled) => {
    commitVisualInterruptResponses(snapshot, responses);
    document.documentElement.dataset.visualResumedRun = runId;
    document.documentElement.dataset.visualResumedItem = responses[0]?.itemId ?? "";
    document.documentElement.dataset.visualResumedResponse = JSON.stringify(
      responses[0]?.response ?? null,
    );
    onSettled?.();
    publishVisualView(
      store,
      projectAgentSessionSnapshot(projectRuntimeAgentSnapshot(snapshot)),
      "Failed to publish the visual Runtime response",
    );
    return true;
  });
  const cancelPresentation = new URLSearchParams(location.search).get("subagent-cancel");
  let cancelInvocations = 0;
  store.setCancelRun(VISUAL_SESSION_ID, (runId) => {
    document.documentElement.dataset.visualCanceledRun = runId;
    document.documentElement.dataset.visualCancelCalls = String(++cancelInvocations);
    if (cancelPresentation === "unavailable") {
      installVisualRuntimeServiceStatusPort(visual, "unavailable");
      return;
    }
    if (cancelPresentation !== "settled") return;
    const captured = snapshot.runs.find((run) => run.id === runId);
    if (!captured || captured.status === "finished")
      throw new Error("The subagent fixture requires an unfinished captured run");
    snapshot.runs = snapshot.runs.map((run) => {
      if (run.id !== runId) return run;
      const finished = {
        ...run,
        status: "finished" as const,
        finishedAt: "2026-07-31T08:02:00.000Z",
        outcome: { type: "canceled" as const, detail: "The isolated fixture canceled this run." },
      };
      delete finished.activeSegmentId;
      return finished;
    });
    snapshot.pendingInterruptSets = snapshot.pendingInterruptSets
      .map((pending) => ({
        ...pending,
        interrupts: pending.interrupts.filter((interrupt) => interrupt.runId !== runId),
      }))
      .filter((pending) => pending.interrupts.length > 0);
    publishVisualView(
      store,
      projectAgentSessionSnapshot(projectRuntimeAgentSnapshot(snapshot)),
      "Failed to publish the captured subagent outcome",
    );
  });

  return view;
}
