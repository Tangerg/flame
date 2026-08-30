// Adding a kernel point is one `defineExtensionPoint` block here plus one selector.

import type {
  AgentRunOptionsProviderSpec,
  AgentSourceSpec,
  CommandSpec,
  ComposerKeyBindingSpec,
  ComposerSubmitModeSpec,
  ContextDockDestinationSpec,
  StreamEventHandler,
  DataProviderSpec,
  LayoutSlotSpec,
  LocaleSpec,
  LogSubscriber,
  MessageRoleSpec,
  PluginErrorFallbackSpec,
  ReadyHandler,
  RouteSpec,
  SettingsPaneSpec,
  ShortcutSpec,
  SlashCommandSpec,
  ColorThemeSpec,
  AccentSpec,
  VisualStyleSpec,
  ToolActionSpec,
  ToolPreviewComponent,
  ToolViewOpenerSpec,
  WorkIndexItemSpec,
  WorkspaceViewSpec,
} from "./types";
import { defineExtensionPoint } from "./contracts";
import { normalizeCombo } from "./combo";

export const COLOR_THEME = defineExtensionPoint<ColorThemeSpec>({
  id: "flame.colorTheme",
  keying: "single",
});
export const ACCENT = defineExtensionPoint<AccentSpec>({
  id: "flame.accent",
  keying: "single",
});
export const VISUAL_STYLE = defineExtensionPoint<VisualStyleSpec>({
  id: "flame.visualStyle",
  keying: "single",
});
export const LOCALE = defineExtensionPoint<LocaleSpec>({
  id: "flame.locale",
  keying: "single",
});

export const ROUTE = defineExtensionPoint<RouteSpec>({
  id: "flame.route",
  keying: "single",
});
export const AGENT_SOURCE = defineExtensionPoint<AgentSourceSpec>({
  id: "flame.agent.source",
  keying: "single",
});
export const AGENT_RUN_OPTIONS = defineExtensionPoint<AgentRunOptionsProviderSpec>({
  id: "flame.agent.runOptions",
  keying: "single",
  keyOf: (s) => s.id,
});
export const DATA_PROVIDER = defineExtensionPoint<DataProviderSpec>({
  id: "flame.data.provider",
  keying: "single",
  keyOf: (s) => s.key,
});
export const ERROR_FALLBACK = defineExtensionPoint<PluginErrorFallbackSpec>({
  id: "flame.plugin.errorFallback",
  keying: "single",
});

// Slash trigger lives in the map key, not on the spec — contributors pass it
// via `opts.key`. `normalizeKey` folds the leading "/" so callers can register
// "ping" or "/ping" and look it up either way.
export const SLASH_COMMAND = defineExtensionPoint<SlashCommandSpec>({
  id: "flame.composer.slashCommand",
  keying: "single",
  normalizeKey: (k) => (k.startsWith("/") ? k : `/${k}`),
});
// Key combos fold "Cmd+K" / "mod+k" to one canonical form on both contribute
// and lookup, so registrations and keydown lookups always agree.
export const COMPOSER_KEY_BINDING = defineExtensionPoint<ComposerKeyBindingSpec>({
  id: "flame.composer.keyBinding",
  keying: "single",
  keyOf: (s) => s.key,
  normalizeKey: normalizeCombo,
});
export const COMPOSER_SUBMIT_MODE = defineExtensionPoint<ComposerSubmitModeSpec>({
  id: "flame.composer.submitMode",
  keying: "single",
  keyOf: (mode) => mode.id,
});

export const SHORTCUT = defineExtensionPoint<ShortcutSpec>({
  id: "flame.shortcut",
  keying: "single",
  keyOf: (s) => s.key,
  normalizeKey: normalizeCombo,
});

export const COMMAND = defineExtensionPoint<CommandSpec>({
  id: "flame.command",
  keying: "single",
});
export const SETTINGS_PANE = defineExtensionPoint<SettingsPaneSpec>({
  id: "flame.settingsPane",
  keying: "single",
});
export const WORKSPACE_VIEW = defineExtensionPoint<WorkspaceViewSpec>({
  id: "flame.workspaceView",
  keying: "single",
});
export const CONTEXT_DOCK_DESTINATION = defineExtensionPoint<ContextDockDestinationSpec>({
  id: "flame.contextDock.destination",
  keying: "single",
  keyOf: (destination) => destination.viewId,
});

// ---- multi-handler surfaces (every contribution coexists, runs in order) --
export const LOG_SUBSCRIBER = defineExtensionPoint<LogSubscriber>({
  id: "flame.log.subscriber",
  keying: "multi",
});

export const READY_HANDLER = defineExtensionPoint<ReadyHandler>({
  id: "flame.lifecycle.ready",
  keying: "multi",
});

// The item wraps its sub-key (name / eventType / slot) alongside the payload;
// the events + layout selectors build a cached secondary index over it (see
// `createPointSubIndex`). The reducer hits these per StreamEvent.
export const STREAM_EVENT_HANDLER = defineExtensionPoint<{
  eventType: string;
  handler: StreamEventHandler;
}>({ id: "flame.events.stream", keying: "multi" });
export const LAYOUT_SLOT = defineExtensionPoint<{ slot: string; spec: LayoutSlotSpec }>({
  id: "flame.layoutSlot",
  keying: "multi",
});

export const WORK_INDEX_ITEM = defineExtensionPoint<WorkIndexItemSpec>({
  id: "flame.workIndex.item",
  keying: "single",
});

export const MESSAGE_ROLE = defineExtensionPoint<MessageRoleSpec>({
  id: "flame.message.role",
  keying: "single",
});
export const TOOL_ACTION = defineExtensionPoint<ToolActionSpec>({
  id: "flame.tool.action",
  keying: "single",
});
export const TOOL_VIEW_OPENER = defineExtensionPoint<ToolViewOpenerSpec>({
  id: "flame.tool.viewOpener",
  keying: "single",
});
// Keyed by an explicit `opts.key` rather than a field on the item, because the item IS the
// component.
export const TOOL_PREVIEW = defineExtensionPoint<ToolPreviewComponent>({
  id: "flame.tool.preview",
  keying: "single",
});
export const TOOL_ICON = defineExtensionPoint<string>({
  id: "flame.tool.icon",
  keying: "single",
});
// A tool whose whole outcome already sits on a surface that STAYS on screen, so the
// narrative need not repeat it. The value names that surface rather than being a bare flag,
// so the claim stays answerable. Claim only what the surface shows in FULL: a tool that
// asks the person something is not presented by it however much it echoes — hiding
// `exit_plan_mode` would hide the question.
export const TOOL_STANDING_SURFACE = defineExtensionPoint<string>({
  id: "flame.tool.standingSurface",
  keying: "single",
});
