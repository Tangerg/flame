import { type ComponentType } from "react";
import { type SettingsIntent } from "@/foundation/protocolValues.generated";
import { defineExtensionPoint } from "@/extensions/contracts";
import { useExtensionPoint } from "@/extensions/selectors/extensions";
export interface SettingsPaneSpec {
  id: string;
  material?: string;
  intent?: SettingsIntent;
  label: string;
  description?: string;
  icon?: string;
  order?: number;
  group?: string;
  keywords?: readonly string[];
  component: ComponentType;
}

export const SETTINGS_PANE = defineExtensionPoint<SettingsPaneSpec>({
  id: "flame.settingsPane",
  keying: "single",
});

export function useSettingsPanes(): SettingsPaneSpec[] {
  return useExtensionPoint(SETTINGS_PANE);
}
