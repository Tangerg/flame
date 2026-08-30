// Beside the scroller rather than inside it, so the map holds still while the transcript
// moves — which is the only reason a map is useful. A contribution rather than shell
// furniture, because a navigation aid over the narrative is replaceable by definition.

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
