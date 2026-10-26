import { definePlugin } from "@/extensions/definePlugin";
import { type Theme } from "@/features/theme/adapters/theme";
export function appearancePainter(theme: Theme) {
  return definePlugin({
    name: "flame.builtin.appearance-painter",
    setup(ctx) {
      ctx.cleanup(theme.start());
    },
  });
}
