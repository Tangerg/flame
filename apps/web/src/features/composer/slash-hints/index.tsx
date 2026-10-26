import { definePlugin } from "@/extensions/definePlugin";
import { SLASH_COMMAND } from "@/features/composer/public/extensionPoints";
import { slashHintContributions } from "@/features/composer/slash-hints/slashHints";
export default definePlugin({
  name: "flame.builtin.slash-hints",
  setup(ctx) {
    for (const { cmd, spec } of slashHintContributions()) {
      ctx.contribute(SLASH_COMMAND, spec, { key: cmd });
    }
  },
});
