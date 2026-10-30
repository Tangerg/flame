import { mutationResult } from "@/test/runtimeClient";
import { type VisualClient } from "./client";
import { type FlameClient } from "@flame/runtime-client";
import { VISUAL_WORKSPACE_PATH } from "./agentFixtureFacts";
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
import { messageCopy } from "@/features/chat/message-actions/index";
import { messageEdit } from "@/features/chat/message-actions/index";
import { createMessageFeedbackPlugin } from "@/features/chat/message-actions/index";
import { messageRegenerate } from "@/features/chat/message-actions/index";
import { builtinVisualStyles } from "@/features/theme/visualStyles/index";
import customTheme from "@/features/theme/themes/custom-theme";
import flameDark from "@/features/theme/themes/flame-dark";
import flameLight from "@/features/theme/themes/flame-light";
import { defaultAccents } from "@/features/theme/defaultAccents";
import { createGoalPlugin } from "@/features/goal/index";
import { type GoalState } from "@/features/goal/application/goalReadModel";
import planProgress from "@/features/chat/plan-progress/index";
import { workbenchChat } from "@/features/shell/workbench/index";
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
  WORKSPACE_PROJECTS_KEY,
  type WorkspaceProjectSummary,
} from "@/features/workspace/public/queries";
import {
  WORKSPACE_LIST_FILES_KEY,
  type WorkspaceFileEntry,
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
  session: AgentSessionSummary,
  runtimeClient: () => FlameClient,
): AgentRuntimeGateway {
  return {
    createSession: async ({ cwd }) => ({
      ...session,
      workspace: { ...session.workspace, path: cwd },
    }),
    deleteSession: async () => undefined,
    updateSession: async ({ expectedRevision, title, favorite, cwd }) => ({
      ...session,
      revision: expectedRevision + 1,
      ...(title !== undefined ? { title } : {}),
      ...(favorite !== undefined ? { favorite } : {}),
      ...(cwd !== undefined ? { workspace: { ...session.workspace, path: cwd } } : {}),
    }),
    forkSession: async () => ({ id: `${VISUAL_SESSION_ID}_fork` }),
    loadSessionSnapshot: async () => ({
      snapshot: projectRuntimeAgentSnapshot(snapshot),
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
    allowMCPTool: async () => undefined,
    forgetApprovalRule: async () => undefined,
  };
}

const fileListProvider = definePlugin({
  name: "flame.visual.file-list",
  setup(ctx) {
    ctx.contribute(DATA_PROVIDER, {
      key: WORKSPACE_LIST_FILES_KEY,
      fetcher: async () =>
        [
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
        ] satisfies WorkspaceFileEntry[],
    });
  },
});

const visualAgentSessions = (visual: VisualClient, runtimeClient: () => FlameClient) =>
  definePlugin({
    name: "flame.visual.agent-session-ports",
    provides: { sessions: AGENT_SESSIONS },
    setup(ctx) {
      registerApprovalPolicyDataProviders(ctx, runtimeClient);
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

export async function installVisualAgentFixture(
  visual: VisualClient,
  runtimeClient: () => FlameClient,
  state: VisualAgentState,
  commandOutput: string | undefined,
  snapshot: RuntimeAgentSessionSnapshot,
  capabilities: ServerCapabilities,
): Promise<AgentSessionView> {
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
  const projectless = state === "empty" || state === "runtime-down";
  queryClient.clear();
  visual.projection.store.setState({ capabilities });
  installVisualRuntimeServiceStatusPort(visual, state === "runtime-down" ? "unavailable" : "ready");
  visual.agent.dependencies = {
    ...visual.agent.dependencies,
    runtime: visualAgentRuntimeGateway(snapshot, visualSession(state, snapshot), runtimeClient),
  };

  visual.agent.sessions.store.setState({
    openSessionIds: projectless ? [] : [VISUAL_SESSION_ID],
    lastSessionId: projectless ? "" : VISUAL_SESSION_ID,
  });
  visual.navigation.go({ session: projectless ? "" : VISUAL_SESSION_ID });
  queryClient.setQueryDefaults([AGENT_SESSIONS_KEY], { staleTime: Infinity });
  queryClient.setQueryDefaults([WORKSPACE_PROJECTS_KEY], { staleTime: Infinity });
  queryClient.setQueryDefaults([MODELS_KEY], { staleTime: Infinity });
  queryClient.setQueryDefaults([APPROVAL_MODE_KEY], { staleTime: Infinity });
  queryClient.setQueryData(
    [AGENT_SESSIONS_KEY],
    projectless ? [] : [visualSession(state, snapshot)],
  );
  queryClient.setQueryData<WorkspaceProjectSummary[]>([WORKSPACE_PROJECTS_KEY], []);
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
    visualAgentSessions(visual, runtimeClient),
    visualRuntimeStream(visual),
    visualNavigationBootstrap(visual),
    createComposerBootstrap(visual.composer),
    composerKeymap(visual.agent.access, visual.composer.access, visual.runtime),
    composerToolbar,
    contextUsage,
    composerSend,
    narrativeRails,
    workbenchChat,
    messageCopy,
    messageEdit,
    messageRegenerate,
    createMessageFeedbackPlugin(runtimeClient),
    createGoalPlugin(
      visual.agent.access,
      visual.composer.access,
      visual.runtime,
      runtimeClient,
      visual.goals,
    ),
    planProgress,
    subagentsView,
    ...createToolRenderingPlugins(visual.workspace.access),
    await visualToolResults(runtimeClient, state === "genui-report"),
    fileListProvider,
  );

  visual.composer.store.setState((current) => ({
    composer: current.composer.edit((draft) =>
      draft.withValue(state === "steer" ? "Tighten the error copy and continue." : ""),
    ),
    modelPreference: {
      kind: "explicit",
      provider: VISUAL_MODELS[0]!.provider,
      model: VISUAL_MODELS[0]!.id,
      ...(VISUAL_MODELS[0]!.capabilities?.reasoningDefaultLevel
        ? { reasoningEffort: VISUAL_MODELS[0]!.capabilities?.reasoningDefaultLevel }
        : {}),
    },
  }));

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
        goal: VISUAL_GOALS[state] ?? null,
      } satisfies GoalState,
    },
  };

  const store = visual.agent.projection.store.getState();
  store.ensureSession(VISUAL_SESSION_ID);
  const refresh = store.beginViewRefresh(VISUAL_SESSION_ID, true);
  if (!refresh || !store.commitViewRefresh(VISUAL_SESSION_ID, refresh, view)) {
    throw new Error(`Failed to install visual agent state "${state}"`);
  }
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
      const refresh = store.beginViewRefresh(VISUAL_SESSION_ID, true);
      if (!refresh || !store.commitViewRefresh(VISUAL_SESSION_ID, refresh, stoppedView)) {
        throw new Error("Failed to publish the captured root outcome");
      }
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
    const refresh = store.beginViewRefresh(VISUAL_SESSION_ID, true);
    if (
      !refresh ||
      !store.commitViewRefresh(
        VISUAL_SESSION_ID,
        refresh,
        projectAgentSessionSnapshot(projectRuntimeAgentSnapshot(snapshot)),
      )
    ) {
      throw new Error("Failed to publish the visual Runtime response");
    }
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
    const refresh = store.beginViewRefresh(VISUAL_SESSION_ID, true);
    if (
      !refresh ||
      !store.commitViewRefresh(
        VISUAL_SESSION_ID,
        refresh,
        projectAgentSessionSnapshot(projectRuntimeAgentSnapshot(snapshot)),
      )
    ) {
      throw new Error("Failed to publish the captured subagent outcome");
    }
  });

  return view;
}
