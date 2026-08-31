// Three base colours; the full palette is DERIVED with CSS `color-mix()` so the browser
// resolves the ladders against bg/fg at paint time rather than hand-computing hexes.
//
// Registers under id "custom" like any other theme, and RE-registers on every change —
// that mutation is what gives live preview while dragging the pickers.

import { colord } from "colord";
import type { Scheme } from "@/lib/appearance";
import { disposeOnHmr } from "@/lib/hmr";
import { definePlugin, type Disposable } from "@/plugins/sdk";
import { COLOR_THEME } from "@/plugins/sdk/kernelPoints";
import { useAppearanceStore } from "../adapters/appearanceStore";
import { colorThemeContribution } from "../kit/colorThemeContribution";
import type { ColorThemePluginSpec } from "../kit/types";
import type { CustomTheme } from "../kit/appearance";

const CUSTOM_THEME_ID = "custom";

// mix(a, pct, b) → CSS color-mix string: pct% of `a`, rest `b`. Resolved by
// the browser, so the derived ladder tracks the base colors exactly.
const mix = (a: string, pct: number, b: string): string =>
  `color-mix(in oklab, ${a} ${pct}%, ${b})`;

/** Derive a full theme spec from the custom bg/fg + the shared global accent.
 *  `contrast` (0–100) scales how far each derived ladder spreads from the
 *  base colors — low = flat/subtle, high = punchy. */
function deriveCustomSpec(ct: CustomTheme, accent: string, contrast: number): ColorThemePluginSpec {
  const { bg, fg } = ct;
  const k = Math.min(100, Math.max(0, contrast)) / 100; // 0..1 — global contrast
  // lerp a fg-toward-bg mix percentage by contrast, then round to an int.
  const p = (lo: number, hi: number) => Math.round(lo + (hi - lo) * k);
  const scheme: Scheme = colord(bg).isDark() ? "dark" : "light";
  const extreme = scheme === "dark" ? "#ffffff" : "#000000";
  const chrome = mix(fg, p(4, 12), bg);
  return {
    id: CUSTOM_THEME_ID,
    label: "Custom",
    scheme,
    // (--depth-step is set globally from contrast in uiStore.applyTheme)
    brand: { accent, textOnAccent: colord(accent).isDark() ? "#ffffff" : "#000000" },
    surfaces: {
      bg,
      surface: chrome,
      // A card lifts away from the ink. On a dark base that is the same step the
      // chrome takes; on a light one it has to go the other way, toward white.
      elevated: scheme === "dark" ? chrome : mix("#ffffff", p(35, 80), bg),
      // A well recedes under the plane in both schemes, so one formula serves.
      sunken: mix("#000000", p(3, 8), bg),
    },
    ink: {
      text: fg,
      textBright: mix(fg, 80, extreme), // nudge toward pure white/black
      textSoft: mix(fg, p(86, 94), bg),
      textMuted: mix(fg, p(45, 75), bg),
      textFaint: mix(fg, p(28, 52), bg),
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
      // COLOR_THEME is single-keyed. A live custom palette therefore replaces
      // its one contribution; publishing the same `custom` key twice aborts the
      // entire Zustand listener chain before React, persistence and the document
      // painter can observe the preference change.
      contribution?.dispose();
      contribution = ctx.contribute(
        COLOR_THEME,
        colorThemeContribution({
          ...spec,
          icon: "spark",
          order: 99, // after the built-in packs, before plugin themes
        }),
      );
    };

    register();
    // Re-derive when the base colors, shared accent, or global contrast
    // change. applyTheme then re-applies the tokens.
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
