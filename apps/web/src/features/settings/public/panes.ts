import { type ComponentType } from "react";
import { defineExtensionPoint } from "@/extensions/contracts";
import { useExtensionPoint } from "@/extensions/selectors/extensions";
export interface SettingsPaneSpec {
  id: string;
  material?: string;
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
