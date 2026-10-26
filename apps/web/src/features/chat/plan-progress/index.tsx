import { toolFamilyNames } from "@/lib/toolFamilies";
import { contributeLayout } from "@/extensions/contributeHelpers";
import { definePlugin } from "@/extensions/definePlugin";
import { TOOL_STANDING_SURFACE } from "@/features/chat/public/tools/types";
import { ActivePlan } from "@/features/chat/plan-progress/ActivePlan";
const PLAN_SURFACE = "composer.overlay.top:plan";

export default definePlugin({
  name: "flame.builtin.plan-progress",
  setup(ctx) {
    contributeLayout(ctx, "composer.overlay.top", {
      id: "plan-progress",
      order: 0,
      component: ActivePlan,
    });
    for (const key of toolFamilyNames("plan")) {
      ctx.contribute(TOOL_STANDING_SURFACE, PLAN_SURFACE, { key });
    }
  },
});
