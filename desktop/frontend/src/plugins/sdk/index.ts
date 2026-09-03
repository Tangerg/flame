// The one door into the plugin system; built-ins import from here too, with no back door.

export { getConfig, hasConfig, setConfig, useConfigStore } from "./config";

export type { ConfigValue } from "./config";

export { definePlugin } from "./definePlugin";
export { startKernel, stopKernel } from "./bootstrap";
export type { Contributor, PluginContext, PluginSpec } from "./definePlugin";
export { contributeLayout } from "./contributeHelpers";
export type { Contribution } from "./contracts";
export {
  contributionsTo,
  subscribeContributions,
  useContributions,
  useInstalledPlugins,
} from "./kernel";
export { COMMANDS, CONFIG, I18N, WINDOW, WORKSPACE } from "./services";
export {
  toolResultShape,
  type ToolResultShape,
  commandToolResult,
  patchToolResult,
  searchToolResult,
  webSearchToolResult,
} from "./toolResult";
export type {
  AmbientShell,
  CommandsService,
  ConfigService,
  I18nService,
  WindowService,
  WorkspaceService,
} from "./services";

export * from "./kernelPoints";

export {
  type PluginError,
  type PluginErrorSource,
  reportPluginError,
  usePluginErrorStore,
} from "./errors";

export { notifyError, notifyInfo, useNotificationStore } from "./notifications";
export type { NotifyOptions, NotifySource } from "./notifications";
export { useCommandAction } from "./commandAction";
export type { CommandAction, CommandActionConfig } from "./commandAction";
export { createDataQuery, createParameterizedDataQuery } from "./dataQuery";
export type { ParameterizedQueryOptions } from "./dataQuery";

export {
  executeCommand,
  lookupStreamHandlers,
  lookupDataProvider,
  lookupExtensionByKey,
  lookupExtensionOwner,
  lookupExtensionPoint,
  lookupSlashCommandOwner,
  lookupToolActionOwner,
  lookupToolViewOpenerOwner,
  pickAgentSource,
  resolveAgentRunStartOptions,
  useExtensionByKey,
  useExtensionPoint,
  useLayoutSlot,
  useWorkIndexItems,
  useSettingsPanes,
  useSlashCommands,
  useWorkspaceViews,
} from "./selectors";
export { appendTimelineEntry } from "./types/agentTimeline";

export type { KeyValueStore } from "./storage";
export type { AgentMessagePhase, AgentPlan, PlanStep } from "./types/agentSessionView";

export type {
  AgentDriver,
  AgentCancelResult,
  AgentEventEnvelope,
  AgentInterrupt,
  AgentItem,
  AgentItemDelta,
  AgentItemStatus,
  AgentMessagePart,
  AgentPendingInterruptSet,
  AgentQuestion,
  AgentQuestionField,
  AgentQuestionOption,
  AgentRunFact,
  AgentSegmentOutcome,
  AgentStreamEvent,
  AgentToolInvocation,
  AgentRunOptionsProviderSpec,
  AgentRunStartOptions,
  AgentSourceSpec,
  CommandSpec,
  ComposerKeyBindingSpec,
  ComposerKeyContext,
  ComposerSubmitModeContext,
  ComposerSubmitModeDraft,
  ComposerSubmitModeSpec,
  ContextDockDestinationScope,
  ContentBlock,
  StreamEventHandler,
  DataProviderSpec,
  Disposable,
  ExtensionContributionOptions,
  ExtensionKeying,
  ExtensionPoint,
  LayoutSlotSpec,
  LogLevel,
  MessageRoleSpec,
  ReadyHandler,
  RouteSpec,
  SettingsPaneSpec,
  ShortcutHandler,
  ShortcutSpec,
  SlashCommandRunCtx,
  SlashCommandSpec,
  ColorThemeSpec,
  NeutralStep,
  ThemeNeutralSteps,
  AccentSpec,
  VisualStyleSpec,
  ToolActionSpec,
  ToolPreviewComponent,
  ToolPreviewProps,
  ToolViewOpenerSpec,
  WorkIndexItemSpec,
  WorkspaceViewSpec,
} from "./types";
export type { NotificationEntry, NotificationLevel } from "./types";

export { useCurrentMessage, useCurrentMessageSessionId } from "./messageContext";
