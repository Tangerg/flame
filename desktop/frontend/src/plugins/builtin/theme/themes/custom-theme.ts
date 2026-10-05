import { colord } from "colord";
import { definePlugin } from "@/plugins/sdk";
import { COLOR_THEME } from "@/plugins/sdk/kernelPoints";
import { useAppearanceStore } from "../adapters/appearanceStore";
import { paletteThemeContribution } from "../kit/palette";

export default definePlugin({
  name: "flame.builtin.custom-theme",
  setup(ctx) {
    const currentTheme = () => {
      const { customTheme, accent, contrast } = useAppearanceStore.getState();
      return paletteThemeContribution(
        {
          id: "custom",
          label: "Custom",
          scheme: colord(customTheme.bg).isDark() ? "dark" : "light",
          icon: "spark",
          order: 99,
          palette: { background: customTheme.bg, foreground: customTheme.fg },
        },
        accent,
        contrast,
      );
    };

    const contribution = ctx.contribute(COLOR_THEME, currentTheme());
    ctx.cleanup(
      useAppearanceStore.subscribe((s, p) => {
        if (s.customTheme !== p.customTheme || s.accent !== p.accent || s.contrast !== p.contrast)
          contribution.update(currentTheme());
      }),
    );
  },
});
