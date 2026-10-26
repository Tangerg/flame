import { contributeLayout } from "@/extensions/contributeHelpers";
import { definePlugin } from "@/extensions/definePlugin";
import { TurnRail } from "@/features/chat/narrative-rails/TurnRail";
export default definePlugin({
  name: "flame.builtin.narrative-rails",
  setup(ctx) {
    contributeLayout(ctx, "chat.rail.start", {
      id: "turn-rail",
      order: 0,
      component: TurnRail,
    });
  },
});
