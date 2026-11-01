import { PLUGINS_PANE } from "./paneIDs";
import { useSettingsPanes } from "./panes";

export function useSettingsPaneForIntent(intent: string): string {
  const panes = useSettingsPanes().filter((pane) => pane.intent === intent);
  return panes.length === 1 ? panes[0]!.id : PLUGINS_PANE;
}
