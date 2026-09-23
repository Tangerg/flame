import { colord } from "colord";
import type { Scheme } from "@/lib/appearance";
import { disposeOnHmr } from "@/lib/hmr";
import { definePlugin, type Disposable } from "@/plugins/sdk";
import { COLOR_THEME } from "@/plugins/sdk/kernelPoints";
import { useAppearanceStore } from "../adapters/appearanceStore";
import { colorThemeContribution } from "../kit/colorThemeContribution";
import type { ColorThemePluginSpec } from "../kit/types";
import type { CustomTheme } from "../kit/appearance";
import { WCAG_AA_TEXT, legibleMix, mixOklab } from "../kit/legibility";

const CUSTOM_THEME_ID = "custom";

const mix = (a: string, pct: number, b: string): string =>
  `color-mix(in oklab, ${a} ${pct}%, ${b})`;

function deriveCustomSpec(ct: CustomTheme, accent: string, contrast: number): ColorThemePluginSpec {
  const { bg, fg } = ct;
  const k = Math.min(100, Math.max(0, contrast)) / 100;
  const p = (lo: number, hi: number) => Math.round(lo + (hi - lo) * k);
  const scheme: Scheme = colord(bg).isDark() ? "dark" : "light";
  const extreme = scheme === "dark" ? "#ffffff" : "#000000";
  const chromePct = p(4, 12);
  const chrome = mix(fg, chromePct, bg);
  const plane = mixOklab(fg, bg, chromePct);
  const legible = (floorPct: number) => legibleMix(fg, bg, plane, floorPct, WCAG_AA_TEXT);
  return {
    id: CUSTOM_THEME_ID,
    label: "Custom",
    scheme,
    brand: { accent, textOnAccent: colord(accent).isDark() ? "#ffffff" : "#000000" },
    surfaces: {
      bg,
      surface: chrome,
      elevated: scheme === "dark" ? chrome : mix("#ffffff", p(35, 80), bg),
      sunken: mix("#000000", p(3, 8), bg),
    },
    ink: {
      text: fg,
      textBright: mix(fg, 80, extreme),
      textSoft: mix(fg, legible(p(86, 94)), bg),
      textMuted: mix(fg, legible(p(45, 75)), bg),
      textFaint: mix(fg, legible(p(28, 52)), bg),
    },
    borders: {
      border: mix(fg, p(8, 22), bg),
      borderSoft: mix(fg, p(14, 32), bg),
      divider: mix(fg, p(5, 13), bg),
    },
    semantic: { negative: "#e5484d", warning: "#f5a623", info: "#3b82f6", success: "#30a46c" },
  };
}

export default definePlugin({
  name: "flame.builtin.custom-theme",
  setup(ctx) {
    let contribution: Disposable | undefined;
    const register = () => {
      const { customTheme, accent, contrast } = useAppearanceStore.getState();
      const spec = deriveCustomSpec(customTheme, accent, contrast);
      contribution?.dispose();
      contribution = ctx.contribute(
        COLOR_THEME,
        colorThemeContribution({
          ...spec,
          icon: "spark",
          order: 99,
        }),
      );
    };

    register();
    const unsub = useAppearanceStore.subscribe((s, p) => {
      if (s.customTheme !== p.customTheme || s.accent !== p.accent || s.contrast !== p.contrast)
        register();
    });
    let stopped = false;
    const stop = () => {
      if (stopped) return;
      stopped = true;
      unsub();
      contribution?.dispose();
      contribution = undefined;
    };
    disposeOnHmr(stop);
    ctx.cleanup(stop);
  },
});
