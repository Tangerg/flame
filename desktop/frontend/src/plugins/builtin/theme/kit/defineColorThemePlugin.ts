import type { AnyPlugin } from "dougong";
import { definePlugin } from "@/plugins/sdk";
import { COLOR_THEME } from "@/plugins/sdk/kernelPoints";
import { colorThemeContribution } from "./colorThemeContribution";
import type { ColorThemePluginSpec } from "./types";

export type { ColorThemePluginSpec } from "./types";

export function defineColorThemePlugin(spec: ColorThemePluginSpec): AnyPlugin {
  const theme = colorThemeContribution(spec);
  return definePlugin({
    name: `flame.builtin.color-theme-${spec.id}`,
    setup(ctx) {
      ctx.contribute(COLOR_THEME, theme);
    },
  });
}
