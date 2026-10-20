import { type AnyPlugin } from "dougong";
import { definePlugin } from "@/extensions/definePlugin";
import { COLOR_THEME } from "@/features/theme/public/extensionPoints";
import { type AppearancePreference } from "@/features/theme/kit/appearance";
import { useAppearanceStore } from "@/features/theme/adapters/appearanceStore";
import { paletteThemeContribution, type PaletteTheme } from "@/features/theme/kit/palette";
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
