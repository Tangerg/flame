import { contributeLayout, definePlugin } from "@/plugins/sdk";
import { TurnRail } from "./ui/TurnRail";

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
