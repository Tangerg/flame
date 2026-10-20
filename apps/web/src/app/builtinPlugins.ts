import { shellServices } from "./shellServices";
import { type ClientHost } from "@/platform/host";
import { type createRuntimeConnection } from "@/features/connection/adapters/runtimeConnection";
import { createMessageImagesPlugin } from "@/features/chat/message/images";
import { type AnyPlugin } from "dougong";
import appearance from "@/features/settings/appearance";
import { createBrandPlugin } from "@/features/settings/brand/index";
import approvalsPane from "@/features/approvals/index";
import personalization from "@/features/settings/personalization/index";
import chatSearch from "@/features/chat/search/index";
import quoteSelection from "@/features/composer/quote-selection/index";
import { composerBootstrap } from "@/features/composer/index";
import { composerKeymap } from "@/features/composer/index";
import { composerRunOptions } from "@/features/composer/index";
import { composerSend } from "@/features/composer/index";
import { composerToolbar } from "@/features/composer/index";
import connectionSettings from "@/features/connection/settings/index";
import { createAgentBootstrapPlugin } from "@/features/agent/bootstrap/index";
import { createObservabilityPlugin } from "@/features/observability/index";
import { PRODUCT_NAME, PRODUCT_SLUG, CLIENT_VERSION } from "@/app/product";
import { createRuntimePlugin } from "@/features/connection/index";
import conversationExport from "@/features/workspace/conversationExport";
import { defaultAccents } from "@/features/theme/defaultAccents";
import { defaultCommands } from "@/features/command/defaultCommands";
import { defaultRoles } from "@/features/chat/defaultRoles";
import { defaultTitle } from "@/app/defaultTitle";
import { createWorkspaceBootstrapPlugin } from "@/features/workspace/bootstrap";
import { workspaceService } from "@/features/workspace/adapters/workspaceService";
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
import { appearancePlugins } from "@/features/theme/index";
import toaster from "@/features/notifications/toaster/index";
import { toolActions, toolIcons } from "@/features/chat/tools/toolMeta";
import { subagentsView } from "@/features/chat/message/subagents";
import toolViewOpener from "@/features/workspace/tool-view-opener/index";
import { fileView } from "@/features/workspace/views";
export const toolRenderingPlugins: AnyPlugin[] = [toolActions, toolViewOpener, toolIcons];

export function createBuiltinPlugins(
  connection: ReturnType<typeof createRuntimeConnection>,
  host: ClientHost,
): AnyPlugin[] {
  const runtimeClient = connection.client;
  const infrastructure: AnyPlugin[] = [
    nativeShell,
    createObservabilityPlugin({ name: `${PRODUCT_SLUG}-frontend`, version: CLIENT_VERSION }),
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
    createBrandPlugin((source) => host.saveImage(source)),
    approvalsPane,
    personalization,
    connectionSettings,
    createPluginsPlugin(runtimeClient, connection.endpoint),
    createProvidersPlugin(runtimeClient),
    createMCPServersPlugin(runtimeClient),
    createHooksPlugin(runtimeClient),
    fileView,
    subagentsView,
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
    createCompletionNotifyPlugin(host, PRODUCT_NAME),
    windowTitle,
    shortcuts,
    sessionSearch,
    commandMenu,
    narrativeRails,
    createGoalPlugin(runtimeClient),
    planProgress,
    providerSetup,
    contextUsage,
    conversationExport,
  ];

  return [
    shellServices,
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
