import { type VisualStyleSpec } from "@/features/theme/public/types";
import { type AnyPlugin } from "dougong";
import { definePlugin } from "@/extensions/definePlugin";
import { VISUAL_STYLE } from "@/features/theme/public/extensionPoints";
export function defineVisualStylePlugin(spec: VisualStyleSpec): AnyPlugin {
  return definePlugin({
    name: `flame.builtin.visual-style-${spec.id}`,
    setup(ctx) {
      ctx.contribute(VISUAL_STYLE, spec);
    },
  });
}
