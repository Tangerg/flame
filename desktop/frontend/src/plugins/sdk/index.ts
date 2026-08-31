// The ONE door into the plugin system: the kernel's own built-ins import from here too,
// with no privileged back door. The named selectors are only the few reads that add real
// logic on top of the generic substrate.

// App-wide config store.
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
export type {
  AmbientShell,
  CommandsService,
  ConfigService,
  I18nService,
  WindowService,
  WorkspaceService,
} from "./services";

// Built-in kernel points (COLOR_THEME / COMMAND / LAYOUT_SLOT / …).
export * from "./kernelPoints";

// Plugin error aggregation.
export {
  type PluginError,
  type PluginErrorSource,
  reportPluginError,
  usePluginErrorStore,
} from "./errors";

// Persistent notification feed + the app-side notify pair that writes to it.
export { notifyError, notifyInfo, useNotificationStore } from "./notifications";
export type { NotifyOptions, NotifySource } from "./notifications";
// Cached read hooks over a contributed data provider.
export { createDataQuery, createParameterizedDataQuery } from "./dataQuery";
export type { ParameterizedQueryOptions } from "./dataQuery";
// `normalizeCombo` and the toast-event contract stay internal: points normalize combos on
// contribute, and toasts go through `host.notify`.

// Read side. Plain reads use the generic substrate (use/lookupExtensionPoint,
// use/lookupExtensionByKey); the rest are selectors with real logic.
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
  useContextDockDestinations,
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
  ContextDockDestinationSpec,
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
  WorkIndexItemScope,
  WorkIndexItemSpec,
  WorkIndexItemVariant,
  WorkspaceViewSpec,
} from "./types";
export type { NotificationEntry, NotificationLevel } from "./types";

// The context lives in the SDK so plugin authors import from one place; kernel UI takes the
// Provider from `./messageContext` directly.
export { useCurrentMessage, useCurrentMessageSessionId } from "./messageContext";
