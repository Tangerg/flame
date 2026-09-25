import type { AnyPlugin } from "dougong";
import appearance from "./settings/appearance";
import approvalsPane from "./settings/approvals";
import personalization from "./settings/personalization";
import chatSearch from "./chat/chat-search";
import quoteSelection from "./chat/quote-selection";
import {
  composerBootstrap,
  composerKeymap,
  composerRunOptions,
  composerSend,
  composerToolbar,
} from "./chat/composer";
import connectionSettings from "./settings/connection-settings";
import agentBootstrap from "./agent/bootstrap";
import observability from "./observability";
import runtime from "./runtime";
import conversationExport from "./workspace/conversationExport";
import {
  defaultAccents,
  defaultCommands,
  defaultDataProviders,
  defaultRoles,
  defaultTitle,
} from "./defaults";
import diagnostics from "./workspace/diagnostics";
import workspaceBootstrap from "./workspace/bootstrap";
import { workspaceService } from "./workspace/adapters/workspaceService";
import workspaceEvents from "./workspace/events";
import { workspaceSessionNavigation } from "./workspace/sessionNavigation";
import { workspaceKeymap } from "./workspace/keymap";
import sessionSearch from "./command/session-search";
import commandMenu from "./command/command-menu";
import hooksPane from "./settings/hooks";
import schedulesPane from "./settings/schedules";
import iconGallery from "./settings/icon-gallery";
import mcpServersPane from "./settings/mcp-servers";
import rpcAgent from "./agent/rpcAgent";
import { kernelChat, kernelSettings, kernelSidebar } from "./shell/kernel";
import nativeShell from "./shell/native-shell";
import providerSetup from "./shell/provider-setup";
import { localePlugins } from "./i18n";
import mainRoute from "./shell/main-route";
import navigationBootstrap from "./navigation/bootstrap";
import {
  messageCopy,
  messageEdit,
  messageFeedback,
  messageRegenerate,
} from "./chat/message-actions";
import goal from "./chat/goal";
import narrativeRails from "./chat/narrative-rails";
import planProgress from "./chat/plan-progress";
import pluginsPane from "./settings/plugins-pane";
import providersPane from "./settings/providers";
import contextUsage from "./chat/context-usage";
import shortcuts from "./command/shortcuts";
import usagePane from "./settings/usage";
import { sidebarActions, sidebarFooter, sidebarProjects, sidebarRecents } from "./sidebar";
import slashHints from "./chat/slash-hints";
import { completionNotify, statusNotifications, windowTitle } from "./shell/status";
import { tasksPill } from "./workspace/tasks";
import { appearancePlugins } from "./theme";
import toaster from "./shell/toaster";
import { toolActions, toolIcons } from "./chat/tools/toolMeta";
import { subagentsView } from "./chat/message/subagents";
import { markdownFile } from "./chat/message/markdownFile";
import { taskPreview } from "./chat/message/taskPreview";
import toolViewOpener from "./workspace/tool-view-opener";
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
} from "./chat/tools/previews";
import {
  diffView,
  fileView,
  agentMemoryView,
  skillsView,
  timelineView,
} from "./workspace/workspace-views";

const infrastructure: AnyPlugin[] = [
  nativeShell,
  observability,
  navigationBootstrap,
  agentBootstrap,
  runtime,
  workspaceBootstrap,
  workspaceService,
  defaultDataProviders,
  workspaceEvents,
  workspaceSessionNavigation,
  workspaceKeymap,
  rpcAgent,
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
  messageFeedback,
];

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
  pluginsPane,
  providersPane,
  usagePane,
  mcpServersPane,
  hooksPane,
  schedulesPane,
  diffView,
  fileView,
  subagentsView,
  markdownFile,
  timelineView,
  skillsView,
  agentMemoryView,
  diagnostics,
];

const kernel: AnyPlugin[] = [kernelSidebar, kernelChat, kernelSettings];

const sidebar: AnyPlugin[] = [sidebarActions, sidebarProjects, sidebarRecents, sidebarFooter];

const overlays: AnyPlugin[] = [
  toaster,
  chatSearch,
  quoteSelection,
  defaultCommands,
  tasksPill,
  statusNotifications,
  completionNotify,
  windowTitle,
  shortcuts,
  sessionSearch,
  commandMenu,
  iconGallery,
  narrativeRails,
  goal,
  planProgress,
  providerSetup,
  contextUsage,
  conversationExport,
];

export const builtinPlugins: AnyPlugin[] = [
  ...infrastructure,
  ...messageRendering,
  ...toolRenderingPlugins,
  ...composer,
  ...panes,
  ...kernel,
  ...sidebar,
  ...overlays,
];
