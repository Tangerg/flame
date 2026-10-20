import { AGENT_RUN_OPTIONS } from "@/features/agent/public/runOptions";
import { definePlugin } from "@/extensions/definePlugin";
import { resolveComposerRunOptions } from "@/features/composer/application/modelSelection";
import { selectedModelSelection } from "@/features/composer/public/selectedModel";
export const composerRunOptions = definePlugin({
  name: "flame.builtin.composer-run-options",
  setup(ctx) {
    ctx.contribute(AGENT_RUN_OPTIONS, {
      id: "composer.model",
      priority: 0,
      resolve: () => resolveComposerRunOptions(selectedModelSelection()),
    });
  },
});
