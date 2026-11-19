import { contributeLayout } from "@/extensions/contributeHelpers";
import { definePlugin } from "@/extensions/definePlugin";
import { SelectionQuote } from "@/features/composer/quote-selection/SelectionQuote";
import type { ComposerAccess } from "@/features/composer/public/composer";
import { COMMAND } from "@/features/command/public/extensionPoints";
import { captureComposerSelectionQuote } from "@/features/composer/public/selectionQuote";
export default (composer: ComposerAccess) =>
  definePlugin({
    name: "flame.builtin.quote-selection",
    setup(ctx) {
      ctx.contributions.add(COMMAND, {
        id: "composer.quoteSelection",
        label: "quote.selection",
        // macOS reserves Shift-Command-Q for system logout.
        combo: "Ctrl+Shift+Q",
        run: () => {
          captureComposerSelectionQuote(composer)?.();
        },
      });
      contributeLayout(ctx.contributions, "app.overlay", {
        id: "selection-quote",
        order: 60,
        component: SelectionQuote,
      });
    },
  });
