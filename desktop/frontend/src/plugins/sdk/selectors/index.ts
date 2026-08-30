// This barrel adds only the selectors with real logic on top of the generic substrate.

// Open extension points — the one read API for plain reads (kernel + plugins).
export {
  lookupExtensionByKey,
  lookupExtensionOwner,
  lookupExtensionPoint,
  useExtensionByKey,
  useExtensionPoint,
} from "./extensions";

// Command execution + slash-command pairing and owner attribution.
export { executeCommand, lookupSlashCommandOwner, useSlashCommands } from "./commands";

// Composer placeholder weighted-random pick.

// StreamEvent handler fan-out (cached sub-index, hit per event).
export { lookupStreamHandlers } from "./events";

// Layout slot (sub-keyed by slot) + workspace views / settings panes.
export {
  useContextDockDestinations,
  useLayoutSlot,
  useSettingsPanes,
  useWorkIndexItems,
  useWorkspaceViews,
} from "./layout";

// Tool owner attribution.
export { lookupToolActionOwner, lookupToolViewOpenerOwner } from "./messages";

// Runtime / data-layer: priority picks + data-provider fetcher.
export { lookupDataProvider, pickAgentSource, resolveAgentRunStartOptions } from "./runtime";

// Theme scheme resolution.
