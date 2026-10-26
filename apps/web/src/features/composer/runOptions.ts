import { type ComposerAccess } from "@/features/composer/public/composer";
import { AGENT_RUN_OPTIONS } from "@/features/agent/public/runOptions";
import { definePlugin } from "@/extensions/definePlugin";
import { resolveComposerRunOptions } from "@/features/composer/application/modelSelection";
import { selectedComposerModelPreference } from "@/features/composer/public/modelPreference";
export const composerRunOptions = (composerAccess: ComposerAccess) =>
  definePlugin({
    name: "flame.builtin.composer-run-options",
    setup(ctx) {
      ctx.contribute(AGENT_RUN_OPTIONS, {
        id: "composer.model",
        priority: 0,
        resolve: () => resolveComposerRunOptions(selectedComposerModelPreference(composerAccess)),
      });
    },
  });
