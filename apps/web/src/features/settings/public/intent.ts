import { type SettingsIntent } from "@/foundation/protocolValues.generated";
import { PLUGINS_PANE } from "./paneIDs";
import { useSettingsPanes } from "./panes";

export function useSettingsPaneForIntent(intent: SettingsIntent): string {
  const panes = useSettingsPanes().filter((pane) => pane.intent === intent);
  return panes.length === 1 ? panes[0]!.id : PLUGINS_PANE;
}
