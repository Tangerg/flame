import type { AnyPlugin } from "dougong";
import { definePlugin } from "@/plugins/sdk";
import { COLOR_THEME } from "@/plugins/sdk/kernelPoints";
import type { AppearancePreference } from "../kit/appearance";
import { useAppearanceStore } from "../adapters/appearanceStore";
import { paletteThemeContribution, type PaletteTheme } from "../kit/palette";

export function paletteThemePlugin(
  name: string,
  theme: (preference: AppearancePreference) => PaletteTheme,
): AnyPlugin {
  return definePlugin({
    name,
    setup(ctx) {
      const derive = (preference: AppearancePreference) =>
        paletteThemeContribution(theme(preference), preference.accent, preference.contrast);
      const contribution = ctx.contribute(COLOR_THEME, derive(useAppearanceStore.getState()));
      ctx.cleanup(
        useAppearanceStore.subscribe((next, previous) => {
          if (
            next.customTheme !== previous.customTheme ||
            next.accent !== previous.accent ||
            next.contrast !== previous.contrast
          )
            contribution.update(derive(next));
        }),
      );
    },
  });
}
