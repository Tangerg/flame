import { toolFamilyNames } from "@/lib/toolFamilies";
import { contributeLayout, definePlugin } from "@/plugins/sdk";
import { TOOL_STANDING_SURFACE } from "@/plugins/sdk/kernelPoints";
import { ActivePlan } from "./ui/ActivePlan";

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
