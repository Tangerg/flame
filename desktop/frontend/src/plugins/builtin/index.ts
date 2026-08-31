// Started as ONE Host transaction, with service contracts resolved from each plugin's
// `requires`/`provides` — this array's order is only a tie-breaker between independent
// plugins, not dependency semantics.
//
// Last-write-wins slots ARE array-order driven, so keep destructive overrides later.

import type { AnyPlugin } from "dougong";
import appearance from "./settings/appearance";
import approvalsPane from "./settings/approvals";
import personalization from "./settings/personalization";
import chatSearch from "./chat/chat-search";
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
import contextDockDestinations from "./workspace/context-dock";
import agentFold from "./agent/bootstrap/foldPlugin";
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
import globalKeymap from "./command/global-keymap";
import sessionSearch from "./command/session-search";
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
import recipesSlash from "./chat/recipes";
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
import toolViewOpener from "./workspace/tool-view-opener";
import {
  askUserPreview,
  shellPreview,
  applyPatchPreview,
  file,
  globPreview,
  goalPreviews,
  grep,
  httpPreviews,
  lspPreviews,
  planPreviews,
  recallPreviews,
  schedulePreview,
  skillPreview,
  taskPreview,
  toolSearchPreviewPlugin,
  webSearchPreview,
} from "./chat/tools/previews";
import {
  agentDocsView,
  diffView,
  fileView,
  filesView,
  fileTreeView,
  knowledgeView,
  agentMemoryView,
  inboxView,
  notificationsView,
  planView,
  recipesView,
  runSummaryView,
  searchView,
  skillsView,
  skillLibraryView,
  skillProposalsView,
  terminalView,
  timelineView,
  toolStatsView,
  toolsView,
} from "./workspace/workspace-views";

// Agent fold — fold v2 RunEvents (run.* / item.* / state.*) into view state.
// All semantics (messages, reasoning, tools, plan, questions, HITL) are
// first-class Items now, so the built-in agent fold owns the whole fold.
const protocol: AnyPlugin[] = [agentFold];

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
  rpcAgent,
  defaultTitle,
  defaultAccents,
  ...appearancePlugins,
  ...localePlugins,
  mainRoute,
];

// Protocol content blocks render directly in the message module; there is deliberately no
// second renderer registry.
const messageRendering: AnyPlugin[] = [
  defaultRoles,
  messageCopy,
  messageEdit,
  messageRegenerate,
  messageFeedback,
];

// Tool rendering — previews, header actions, icon glyph map.
//
// Exported so the visual fixture installs the same complete rendering registry as
// production; a hand-picked preview list would drift and render valid tools as JSON.
export const toolPreviewPlugins: AnyPlugin[] = [
  shellPreview,
  applyPatchPreview,
  file,
  grep,
  globPreview,
  lspPreviews,
  planPreviews,
  goalPreviews,
  skillPreview,
  taskPreview,
  askUserPreview,
  webSearchPreview,
  recallPreviews,
  toolSearchPreviewPlugin,
  schedulePreview,
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
  // After slashHints so a user recipe named like a built-in hint wins the
  // shared slash key (it carries a real run handler; the hint is display-only).
  recipesSlash,
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
  contextDockDestinations,
  diffView,
  fileView,
  terminalView,
  filesView,
  fileTreeView,
  planView,
  timelineView,
  toolStatsView,
  runSummaryView,
  toolsView,
  skillsView,
  skillLibraryView,
  skillProposalsView,
  recipesView,
  searchView,
  agentDocsView,
  knowledgeView,
  agentMemoryView,
  inboxView,
  notificationsView,
  diagnostics,
];

const kernel: AnyPlugin[] = [kernelSidebar, kernelChat, kernelSettings];

const sidebar: AnyPlugin[] = [sidebarActions, sidebarProjects, sidebarRecents, sidebarFooter];

const overlays: AnyPlugin[] = [
  toaster,
  chatSearch,
  defaultCommands,
  tasksPill,
  statusNotifications,
  completionNotify,
  windowTitle,
  shortcuts,
  globalKeymap,
  sessionSearch,
  iconGallery,
  narrativeRails,
  goal,
  planProgress,
  providerSetup,
  contextUsage,
  conversationExport,
];

export const builtinPlugins: AnyPlugin[] = [
  ...protocol,
  ...infrastructure,
  ...messageRendering,
  ...toolRenderingPlugins,
  ...composer,
  ...panes,
  ...kernel,
  ...sidebar,
  ...overlays,
];
