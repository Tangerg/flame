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
    // Only `set_plan` is claimed: `enter_plan_mode` is the FACT of switching into planning,
    // which no surface shows, and `exit_plan_mode` interrupts to ask for approval — a
    // question belongs where the person is reading.
    ctx.contribute(TOOL_STANDING_SURFACE, PLAN_SURFACE, { key: "set_plan" });
  },
});
