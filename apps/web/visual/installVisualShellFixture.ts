import { type VisualClient } from "./client";
import { queryClient } from "@/lib/queryClient";
import { type FlameClient } from "@flame/runtime-client";
import { createElement } from "react";
import { RenameSession } from "../../../plugins/trajectory/component/component";
import { InstalledSessionActionComponent } from "@/features/plugins/ui/InstalledComponent";
import { SIDEBAR_DEFAULT_WIDTH_PX } from "@/lib/shellGeometry";
import { AGENT_SESSIONS_KEY, type AgentSessionSummary } from "@/features/agent/public/session";
import sessionSearch from "@/features/navigation/session-search/index";
import commandMenu from "@/features/command/menu/index";
import { useSessionSearchStore } from "@/features/navigation/session-search/sessionSearchState";
import { useCommandMenuStore } from "@/features/command/menu/application/commandMenuState";
import { defaultCommands } from "@/features/command/defaultCommands";
import { sidebarActions } from "@/features/sidebar/index";
import { sidebarFooter } from "@/features/sidebar/index";
import { sidebarProjects } from "@/features/sidebar/index";
import { sidebarRecents } from "@/features/sidebar/index";
import { builtinVisualStyles } from "@/features/theme/visualStyles/index";
import flameDark from "@/features/theme/themes/flame-dark";
import flameLight from "@/features/theme/themes/flame-light";
import { defaultAccents } from "@/features/theme/defaultAccents";
import {
  WORKSPACE_PROJECTS_KEY,
  type WorkspaceProjectSummary,
} from "@/features/workspace/public/queries";
import { DATA_PROVIDER } from "@/extensions/dataProvider";
import { SESSION_ACTION } from "@/features/navigation/public/sessionActions";
import { definePlugin } from "@/extensions/definePlugin";
import { type AnyPlugin } from "dougong";
import {
  type VisualShellOverlay,
  type VisualShellTheme,
  type VisualWorkIndexState,
} from "./shellFixtureStates";
import { loadPluginsForTest } from "@/test/kernel";
import { installVisualRuntimeServiceStatusPort } from "./installVisualRuntimeServiceStatusPort";
import { visualComponentProps } from "./installedComponentProps";
import { visualNavigationBootstrap, visualRuntimeStream } from "./visualRuntimePorts";
const ACTIVE_SESSION_ID = "visual-shell-active";

const PROJECTS: WorkspaceProjectSummary[] = [
  {
    id: "/Users/visual/scope",
    name: "scope",
    sessionCount: 6,
  },
  {
    id: "/Users/visual/runtime",
    name: "runtime-lab",
    sessionCount: 2,
  },
];

const SESSIONS: AgentSessionSummary[] = [
  session(
    ACTIVE_SESSION_ID,
    "Refine Runtime protocol",
    "idle",
    "/Users/visual/scope",
    "14:29",
    true,
  ),
  session(
    "visual-shell-running",
    "Align desktop shell and Work Index",
    "running",
    "/Users/visual/scope",
    "14:18",
  ),
  session(
    "visual-shell-waiting",
    "Review cancellation ownership",
    "waiting",
    "/Users/visual/scope",
    "11:30",
  ),
  session(
    "visual-shell-long",
    "Validate a deliberately long mixed CJK / English session title without breaking the row",
    "idle",
    "/Users/visual/scope",
    "2026-07-30T14:30:00Z",
  ),
  session(
    "visual-shell-checks",
    "Close conformance gates",
    "idle",
    "/Users/visual/scope",
    "2026-07-29T14:30:00Z",
  ),
  session(
    "visual-shell-more",
    "Remove compatibility residue",
    "idle",
    "/Users/visual/scope",
    "2026-07-28T14:30:00Z",
  ),
  session(
    "visual-shell-runtime",
    "Trace replay recovery",
    "idle",
    "/Users/visual/runtime",
    "2026-07-27T14:30:00Z",
  ),
  session(
    "visual-shell-scratch",
    "Inspect an unregistered checkout",
    "idle",
    "/Users/visual/scratch",
    "2026-07-25T14:30:00Z",
  ),
  session("visual-shell-adrift", "Draft a release note", "idle", "", "2026-07-24T14:30:00Z"),
];

function session(
  id: string,
  title: string,
  status: AgentSessionSummary["status"],
  cwd: string,
  time: string,
  favorite = false,
): AgentSessionSummary {
  return {
    id,
    revision: 7,
    title,
    status,
    provider: "openai",
    model: "gpt-5.6",
    workspace: { path: cwd, availability: "available" },
    favorite,
    time: time.includes("T") ? time : `2026-07-31T${time}:00Z`,
  };
}

function pending<T>(): Promise<T> {
  return new Promise<T>(() => {});
}

function dataProviderPlugin(state: VisualWorkIndexState): AnyPlugin {
  return definePlugin({
    name: "flame.visual.work-index-data",
    setup(ctx) {
      ctx.contribute(DATA_PROVIDER, {
        key: WORKSPACE_PROJECTS_KEY,
        fetcher: async () => {
          if (state === "loading") return pending<WorkspaceProjectSummary[]>();
          if (state === "error") {
            throw new Error("Visual fixture Runtime connection unavailable");
          }
          return state === "empty" ? [] : PROJECTS;
        },
      });
      ctx.contribute(DATA_PROVIDER, {
        key: AGENT_SESSIONS_KEY,
        fetcher: async () => (state === "populated" ? SESSIONS : []),
      });
    },
  });
}

function sessionActionsPlugin(runtimeClient: () => FlameClient): AnyPlugin {
  return definePlugin({
    name: "flame.visual.session-actions",
    setup(ctx) {
      const lifetime = ctx.lifetime("visual-rename-session");
      const sources = visualComponentProps(runtimeClient(), lifetime, "rename-session");
      lifetime.contribute(SESSION_ACTION, {
        id: "package:visual:rename-session",
        title: "flame.trajectory · Name this investigation",
        component: ({ session }) =>
          createElement(InstalledSessionActionComponent, {
            ...sources,
            entry: RenameSession,
            session,
          }),
      });
    },
  });
}

export async function installVisualShellFixture(
  visual: VisualClient,
  runtimeClient: () => FlameClient,
  state: VisualWorkIndexState,
  theme: VisualShellTheme,
  sidebarOpen: boolean,
  overlay: VisualShellOverlay | null = null,
): Promise<void> {
  queryClient.clear();
  queryClient.setQueryDefaults([WORKSPACE_PROJECTS_KEY], { retry: false });
  queryClient.setQueryDefaults([AGENT_SESSIONS_KEY], { retry: false });

  installVisualRuntimeServiceStatusPort(visual);
  visual.agent.sessions.store.setState({
    openSessionIds: state === "populated" ? [ACTIVE_SESSION_ID] : [],
    lastSessionId: "",
  });
  visual.navigation.go({ session: state === "populated" ? ACTIVE_SESSION_ID : "" });
  visual.theme.store.setState({ theme, visualStyle: "flame" });
  visual.workspace.layout.setState({
    sidebarCollapsed: !sidebarOpen,
    sidebarWidth: SIDEBAR_DEFAULT_WIDTH_PX,
  });

  await loadPluginsForTest(
    visualRuntimeStream(visual),
    visualNavigationBootstrap(visual),
    sessionActionsPlugin(runtimeClient),
    dataProviderPlugin(state),
    flameLight(visual.theme.access.preference),
    defaultAccents,
    flameDark(visual.theme.access.preference),
    ...builtinVisualStyles,
    sessionSearch,
    defaultCommands(
      visual.agent.access,
      visual.composer.access,
      visual.workspace.access,
      visual.runtime,
      visual.directory.access,
      visual.navigation,
      visual.theme.access,
    ),
    commandMenu,
    sidebarActions,
    sidebarProjects,
    sidebarRecents,
    sidebarFooter,
  );

  useSessionSearchStore.setState({ open: overlay === "finder" });
  useCommandMenuStore.setState({ open: overlay === "commands" });
}
