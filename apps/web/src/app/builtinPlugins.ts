import { type Goals } from "@/features/goal/application/goals";
import { type MCPServerMutationOwner } from "@/features/mcp/application/mcpServerMutationOwner";
import { type Providers } from "@/features/providers/adapters/providers";
import { type Theme } from "@/features/theme/adapters/theme";
import { type NotificationOwner } from "@/features/notifications/adapters/notifications";
import { type InstalledPluginsOwner } from "@/features/plugins/adapters/installedPlugins";
import { type AgentOwner } from "@/features/agent/adapters/agent";
import { type ComposerOwner } from "@/features/composer/adapters/composerStatePorts";
import { type WorkspaceOwner } from "@/features/workspace/adapters/workspace";
import { type createWorkingDirectoryPicker } from "@/features/navigation/adapters/workingDirectorySelection";
import { type Navigator } from "@/lib/navigation";
import { shellServices } from "./shellServices";
import { type ImageExport } from "@/foundation/imageExport";
import { type createRuntimeConnection } from "@/features/connection/adapters/runtimeConnection";
import { type AnyPlugin } from "dougong";
import appearance from "@/features/settings/appearance";
import { createBrandPlugin } from "@/features/settings/brand/index";
import approvalsPane from "@/features/approvals/index";
import personalization from "@/features/settings/personalization/index";
import chatSearch from "@/features/chat/search/index";
import quoteSelection from "@/features/composer/quote-selection/index";
import { createComposerBootstrap } from "@/features/composer/index";
import { composerKeymap } from "@/features/composer/index";
import { composerRunOptions } from "@/features/composer/index";
import { composerSend } from "@/features/composer/index";
import { composerToolbar } from "@/features/composer/index";
import connectionSettings from "@/features/connection/settings/index";
import { createAgentBootstrapPlugin } from "@/features/agent/bootstrap/index";
import { createObservabilityPlugin } from "@/features/observability/index";
import { PRODUCT_SLUG, CLIENT_VERSION } from "@/app/product";
import { createRuntimePlugin } from "@/features/connection/index";
import conversationExport from "@/features/workspace/conversationExport";
import { defaultAccents } from "@/features/theme/defaultAccents";
import { defaultCommands } from "@/features/command/defaultCommands";
import { defaultRoles } from "@/features/chat/defaultRoles";
import { defaultTitle } from "@/app/defaultTitle";
import { createWorkspaceBootstrapPlugin } from "@/features/workspace/bootstrap";
import { createWorkspaceService } from "@/features/workspace/adapters/workspaceService";
import { createWorkspaceEventsPlugin } from "@/features/workspace/events";
import { workspaceSessionNavigation } from "@/features/workspace/sessionNavigation";
import { workspaceKeymap } from "@/features/workspace/keymap";
import sessionSearch from "@/features/navigation/session-search/index";
import commandMenu from "@/features/command/menu/index";
import { createHooksPlugin } from "@/features/hooks/index";
import { createMCPServersPlugin } from "@/features/mcp/index";
import { createRpcAgentPlugin } from "@/features/agent/rpcAgent";
import {
  workbenchChat,
  workbenchSettings,
  workbenchSidebar,
} from "@/features/shell/workbench/index";
import nativeShell from "@/features/shell/native/index";
import providerSetup from "@/features/providers/setup/index";
import { localePlugins } from "@/features/i18n/index";
import mainRoute from "@/app/routes";
import { createNavigationBootstrapPlugin } from "@/features/navigation/bootstrap";
import { messageCopy } from "@/features/chat/message-actions/index";
import { messageEdit } from "@/features/chat/message-actions/index";
import { createMessageFeedbackPlugin } from "@/features/chat/message-actions/index";
import { messageRegenerate } from "@/features/chat/message-actions/index";
import { createGoalPlugin } from "@/features/goal/index";
import narrativeRails from "@/features/chat/narrative-rails/index";
import planProgress from "@/features/chat/plan-progress/index";
import { createPluginsPlugin } from "@/features/plugins/index";
import { createProvidersPlugin } from "@/features/providers/index";
import contextUsage from "@/features/chat/context-usage/index";
import shortcuts from "@/features/command/shortcuts/index";
import { sidebarActions } from "@/features/sidebar/index";
import { sidebarFooter } from "@/features/sidebar/index";
import { sidebarProjects } from "@/features/sidebar/index";
import { sidebarRecents } from "@/features/sidebar/index";
import slashHints from "@/features/composer/slash-hints/index";
import { createCompletionNotifyPlugin } from "@/features/notifications/index";
import { statusNotifications } from "@/features/notifications/index";
import { windowTitle } from "@/features/shell/windowTitle";
import { tasksPill } from "@/features/workspace/tasks";
import { createAppearancePlugins } from "@/features/theme/index";
import toaster from "@/features/notifications/toaster/index";
import { toolActions, toolIcons } from "@/features/chat/tools/toolMeta";
import { subagentsView } from "@/features/chat/message/subagents";
import toolViewOpener from "@/features/workspace/toolViewOpener";
import { fileView } from "@/features/workspace/views";
export function createToolRenderingPlugins(workspace: WorkspaceOwner["access"]): AnyPlugin[] {
  return [toolActions, toolViewOpener(workspace), toolIcons];
}

export function createBuiltinPlugins(
  connection: ReturnType<typeof createRuntimeConnection>,
  saveImage: ImageExport,
  navigation: Navigator,
  agent: AgentOwner,
  composerOwner: ComposerOwner,
  workspace: WorkspaceOwner,
  directory: ReturnType<typeof createWorkingDirectoryPicker>,
  notifications: NotificationOwner,
  installedPlugins: InstalledPluginsOwner,
  theme: Theme,
  providers: Providers,
  mcpServers: MCPServerMutationOwner,
  goals: Goals,
): AnyPlugin[] {
  const agentAccess = agent.access;
  const composerAccess = composerOwner.access;
  const workspaceAccess = workspace.access;
  const runtimeAccess = connection.access;
  const directoryAccess = directory.access;
  const runtimeClient = connection.client;
  const infrastructure: AnyPlugin[] = [
    nativeShell,
    createObservabilityPlugin({ name: `${PRODUCT_SLUG}-frontend`, version: CLIENT_VERSION }),
    createNavigationBootstrapPlugin(directory),
    createAgentBootstrapPlugin(agent, runtimeClient),
    createRuntimePlugin(connection),
    createWorkspaceBootstrapPlugin(workspace, runtimeClient),
    createWorkspaceService(navigation, workspace),
    createWorkspaceEventsPlugin(agentAccess, runtimeAccess, runtimeClient),
    workspaceSessionNavigation,
    workspaceKeymap(workspaceAccess),
    createRpcAgentPlugin(agentAccess, runtimeClient),
    defaultTitle,
    defaultAccents,
    ...createAppearancePlugins(theme),
    ...localePlugins,
    mainRoute,
  ];

  const messageRendering: AnyPlugin[] = [
    defaultRoles,
    messageCopy,
    messageEdit,
    messageRegenerate,
    createMessageFeedbackPlugin(runtimeClient),
  ];

  const composer: AnyPlugin[] = [
    createComposerBootstrap(composerOwner),
    slashHints,
    composerToolbar,
    composerRunOptions(composerAccess),
    composerKeymap(agentAccess, composerAccess, runtimeAccess),
    composerSend,
  ];

  const panes: AnyPlugin[] = [
    appearance,
    createBrandPlugin(saveImage),
    approvalsPane,
    personalization,
    connectionSettings,
    createPluginsPlugin(installedPlugins),
    createProvidersPlugin(runtimeClient, providers.mutations),
    createMCPServersPlugin(runtimeClient, mcpServers),
    createHooksPlugin(runtimeClient),
    fileView,
    subagentsView,
  ];

  const workbench: AnyPlugin[] = [
    workbenchSidebar,
    workbenchChat,
    workbenchSettings(workspaceAccess),
  ];

  const sidebar: AnyPlugin[] = [sidebarActions, sidebarProjects, sidebarRecents, sidebarFooter];

  const overlays: AnyPlugin[] = [
    toaster,
    chatSearch,
    quoteSelection,
    defaultCommands(
      agentAccess,
      composerAccess,
      workspaceAccess,
      runtimeAccess,
      directoryAccess,
      navigation,
      theme.access,
    ),
    tasksPill,
    statusNotifications,
    createCompletionNotifyPlugin(notifications),
    windowTitle(agentAccess),
    shortcuts(workspaceAccess),
    sessionSearch,
    commandMenu,
    narrativeRails,
    createGoalPlugin(agentAccess, composerAccess, runtimeAccess, runtimeClient, goals),
    planProgress,
    providerSetup,
    contextUsage,
    conversationExport(workspaceAccess),
  ];

  return [
    shellServices,
    ...infrastructure,
    ...messageRendering,
    ...createToolRenderingPlugins(workspaceAccess),
    ...composer,
    ...panes,
    ...workbench,
    ...sidebar,
    ...overlays,
  ];
}
