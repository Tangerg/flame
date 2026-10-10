import type { ClientHost } from "@/platform/host";
import type { createRuntimeConnection } from "./runtimeConnection";
import { createMessageImagesPlugin } from "@/plugins/builtin/chat/message/images";
import type { AnyPlugin } from "dougong";
import appearance from "@/plugins/builtin/settings/appearance";
import approvalsPane from "@/plugins/builtin/settings/approvals";
import personalization from "@/plugins/builtin/settings/personalization";
import chatSearch from "@/plugins/builtin/chat/chat-search";
import quoteSelection from "@/plugins/builtin/chat/quote-selection";
import {
  composerBootstrap,
  composerKeymap,
  composerRunOptions,
  composerSend,
  composerToolbar,
} from "@/plugins/builtin/chat/composer";
import connectionSettings from "@/plugins/builtin/settings/connection-settings";
import { createAgentBootstrapPlugin } from "@/plugins/builtin/agent/bootstrap";
import observability from "@/plugins/builtin/observability";
import { createRuntimePlugin } from "@/plugins/builtin/runtime";
import conversationExport from "@/plugins/builtin/workspace/conversationExport";
import {
  defaultAccents,
  defaultCommands,
  defaultRoles,
  defaultTitle,
} from "@/plugins/builtin/defaults";
import diagnostics from "@/plugins/builtin/workspace/diagnostics";
import { createWorkspaceBootstrapPlugin } from "@/plugins/builtin/workspace/bootstrap";
import { workspaceService } from "@/plugins/builtin/workspace/adapters/workspaceService";
import { createWorkspaceEventsPlugin } from "@/plugins/builtin/workspace/events";
import { workspaceSessionNavigation } from "@/plugins/builtin/workspace/sessionNavigation";
import { workspaceKeymap } from "@/plugins/builtin/workspace/keymap";
import sessionSearch from "@/plugins/builtin/command/session-search";
import commandMenu from "@/plugins/builtin/command/command-menu";
import { createHooksPlugin } from "@/plugins/builtin/settings/hooks";
import { createSchedulesPlugin } from "@/plugins/builtin/settings/schedules";
import iconGallery from "@/plugins/builtin/settings/icon-gallery";
import { createMCPServersPlugin } from "@/plugins/builtin/settings/mcp-servers";
import { createRpcAgentPlugin } from "@/plugins/builtin/agent/rpcAgent";
import {
  workbenchChat,
  workbenchSettings,
  workbenchSidebar,
} from "@/plugins/builtin/shell/workbench";
import nativeShell from "@/plugins/builtin/shell/native-shell";
import providerSetup from "@/plugins/builtin/shell/provider-setup";
import { localePlugins } from "@/plugins/builtin/i18n";
import mainRoute from "@/plugins/builtin/shell/main-route";
import { createNavigationBootstrapPlugin } from "@/plugins/builtin/navigation/bootstrap";
import {
  messageCopy,
  messageEdit,
  createMessageFeedbackPlugin,
  messageRegenerate,
} from "@/plugins/builtin/chat/message-actions";
import { createGoalPlugin } from "@/plugins/builtin/chat/goal";
import narrativeRails from "@/plugins/builtin/chat/narrative-rails";
import planProgress from "@/plugins/builtin/chat/plan-progress";
import { createPluginsPane } from "@/plugins/builtin/settings/plugins-pane";
import { createProvidersPlugin } from "@/plugins/builtin/providers";
import contextUsage from "@/plugins/builtin/chat/context-usage";
import shortcuts from "@/plugins/builtin/command/shortcuts";
import { createUsagePlugin } from "@/plugins/builtin/settings/usage";
import {
  sidebarActions,
  sidebarFooter,
  sidebarProjects,
  sidebarRecents,
} from "@/plugins/builtin/sidebar";
import slashHints from "@/plugins/builtin/chat/slash-hints";
import {
  createCompletionNotifyPlugin,
  statusNotifications,
  windowTitle,
} from "@/plugins/builtin/shell/status";
import { tasksPill } from "@/plugins/builtin/workspace/tasks";
import { appearancePlugins } from "@/plugins/builtin/theme";
import toaster from "@/plugins/builtin/shell/toaster";
import { toolActions, toolIcons } from "@/plugins/builtin/chat/tools/toolMeta";
import { subagentsView } from "@/plugins/builtin/chat/message/subagents";
import { markdownFile } from "@/plugins/builtin/chat/message/markdownFile";
import { taskPreview } from "@/plugins/builtin/chat/message/taskPreview";
import toolViewOpener from "@/plugins/builtin/workspace/tool-view-opener";
import {
  askUserPreview,
  shellPreview,
  applyPatchPreview,
  file,
  globPreview,
  grep,
  httpPreviews,
  lspPreviews,
  recallPreviews,
  skillPreview,
  toolSearchPreviewPlugin,
  webSearchPreview,
} from "@/plugins/builtin/chat/tools/previews";
import { diffView, fileView, agentMemoryView, skillsView } from "@/plugins/builtin/workspace/views";

export const toolPreviewPlugins: AnyPlugin[] = [
  shellPreview,
  applyPatchPreview,
  file,
  grep,
  globPreview,
  lspPreviews,
  skillPreview,
  taskPreview,
  askUserPreview,
  webSearchPreview,
  recallPreviews,
  toolSearchPreviewPlugin,
  httpPreviews,
];

export const toolRenderingPlugins: AnyPlugin[] = [
  ...toolPreviewPlugins,
  toolActions,
  toolViewOpener,
  toolIcons,
];

export function createBuiltinPlugins(
  connection: ReturnType<typeof createRuntimeConnection>,
  host: ClientHost,
): AnyPlugin[] {
  const runtimeClient = connection.client;
  const infrastructure: AnyPlugin[] = [
    nativeShell,
    observability,
    createNavigationBootstrapPlugin(
      () => host.chooseWorkingDirectory(),
      connection.localWorkspaceAvailable,
    ),
    createAgentBootstrapPlugin(runtimeClient),
    createRuntimePlugin(runtimeClient, connection.sidecar, () => connection.bootstrap().runtime),
    createWorkspaceBootstrapPlugin(runtimeClient, host, connection.localWorkspaceAvailable),
    workspaceService,
    createWorkspaceEventsPlugin(runtimeClient),
    workspaceSessionNavigation,
    workspaceKeymap,
    createRpcAgentPlugin(runtimeClient),
    defaultTitle,
    defaultAccents,
    ...appearancePlugins,
    ...localePlugins,
    mainRoute,
  ];

  const messageRendering: AnyPlugin[] = [
    defaultRoles,
    messageCopy,
    messageEdit,
    messageRegenerate,
    createMessageFeedbackPlugin(runtimeClient),
    createMessageImagesPlugin((source) => host.saveImage(source)),
  ];

  const composer: AnyPlugin[] = [
    composerBootstrap,
    slashHints,
    composerToolbar,
    composerRunOptions,
    composerKeymap,
    composerSend,
  ];

  const panes: AnyPlugin[] = [
    appearance,
    approvalsPane,
    personalization,
    connectionSettings,
    createPluginsPane(runtimeClient, host.pluginCarrier),
    createProvidersPlugin(runtimeClient),
    createUsagePlugin(runtimeClient),
    createMCPServersPlugin(runtimeClient),
    createHooksPlugin(runtimeClient),
    createSchedulesPlugin(runtimeClient),
    diffView,
    fileView,
    subagentsView,
    markdownFile,
    skillsView,
    agentMemoryView,
    diagnostics,
  ];

  const workbench: AnyPlugin[] = [workbenchSidebar, workbenchChat, workbenchSettings];

  const sidebar: AnyPlugin[] = [sidebarActions, sidebarProjects, sidebarRecents, sidebarFooter];

  const overlays: AnyPlugin[] = [
    toaster,
    chatSearch,
    quoteSelection,
    defaultCommands,
    tasksPill,
    statusNotifications,
    createCompletionNotifyPlugin(host),
    windowTitle,
    shortcuts,
    sessionSearch,
    commandMenu,
    iconGallery,
    narrativeRails,
    createGoalPlugin(runtimeClient),
    planProgress,
    providerSetup,
    contextUsage,
    conversationExport,
  ];

  return [
    ...infrastructure,
    ...messageRendering,
    ...toolRenderingPlugins,
    ...composer,
    ...panes,
    ...workbench,
    ...sidebar,
    ...overlays,
  ];
}
