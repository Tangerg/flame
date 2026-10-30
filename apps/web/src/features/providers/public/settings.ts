import { PLUGINS_PANE } from "@/features/settings/public/paneIDs";
import { useSettingsPanes } from "@/features/settings/public/panes";

export function useProviderSettingsPane(): string {
  const panes = useSettingsPanes().filter((pane) => pane.intent === "modelProviders");
  return panes.length === 1 ? panes[0]!.id : PLUGINS_PANE;
}
