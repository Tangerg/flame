import { useMemo } from "react";
import { labelText, useLocale, type Label, type MessageKey } from "@/lib/i18n/index";
import { presentationTitle } from "@/lib/i18n/presentationTitle";
import { type ComponentType } from "react";
import { defineExtensionPoint } from "@/extensions/contracts";
import { useExtensionPoint } from "@/extensions/selectors/extensions";
export interface SettingsPaneSpec {
  id: string;
  material?: string;
  intent?: string;
  label: Label;
  localizedTitles?: Readonly<Record<string, string>>;
  description?: MessageKey;
  order?: number;
  group?: string;
  keywords?: readonly Label[];
  component: ComponentType;
}

export const SETTINGS_PANE = defineExtensionPoint<SettingsPaneSpec>({
  id: "flame.settingsPane",
  keying: "single",
});

export function useSettingsPanes(): SettingsPaneSpec[] {
  const entries = useExtensionPoint(SETTINGS_PANE);
  const locale = useLocale();
  return useMemo(
    () =>
      entries.map((entry) =>
        entry.localizedTitles
          ? {
              ...entry,
              label: {
                text: presentationTitle(labelText(entry.label), entry.localizedTitles, locale),
              },
            }
          : entry,
      ),
    [entries, locale],
  );
}
