import { type Contributor } from "@/extensions/definePlugin";
import { type SettingsPaneSpec } from "@/features/settings/public/panes";
import { SETTINGS_PANE } from "@/features/settings/public/panes";
export function registerSettingsPane(ctx: Contributor, pane: SettingsPaneSpec) {
  return ctx.contribute(SETTINGS_PANE, pane);
}
