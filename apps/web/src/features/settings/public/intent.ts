import { PLUGINS_PANE } from "./paneIDs";
import { useSettingsPanes } from "./panes";

export function useSettingsPaneForIntent(intent: string): string {
  const panes = useSettingsPanes().filter((pane) => pane.intent === intent);
  return panes.length === 1 ? panes[0]!.id : PLUGINS_PANE;
}

// An installed pane that declares a host-known intent sits with the settings it
// configures; any other installed pane is an extension.
const INTENT_GROUP: Readonly<Record<string, string>> = {
  notifications: "general",
  "model-providers": "models",
  "mcp-servers": "integrations",
};

export function settingsGroupForIntent(intent: string | undefined): string {
  return (intent !== undefined && INTENT_GROUP[intent]) || "extensions";
}
