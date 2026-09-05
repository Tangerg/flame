import { contributeLayout, definePlugin } from "@/plugins/sdk";
import { TOOL_STANDING_SURFACE } from "@/plugins/sdk/kernelPoints";
import { ActivePlan } from "./ui/ActivePlan";

const PLAN_SURFACE = "composer.overlay.top:plan";

/** The tools this surface answers for, so their transcript rows are dropped. Exported because
 *  the rendering-composition test asks which tools the transcript draws, and the answer is
 *  each surface owner's to give. */
export const PLAN_STANDING_TOOLS = ["enter_plan_mode", "set_plan", "exit_plan_mode"] as const;

export default definePlugin({
  name: "flame.builtin.plan-progress",
  setup(ctx) {
    contributeLayout(ctx, "composer.overlay.top", {
      id: "plan-progress",
      order: 0,
      component: ActivePlan,
    });
    // Every Plan tool, not just the one that writes it. Plan mode has a standing bar and a
    // mode indicator; a transcript row for entering, updating or leaving it is a second telling
    // of a state the composer already holds.
    for (const key of PLAN_STANDING_TOOLS) {
      ctx.contribute(TOOL_STANDING_SURFACE, PLAN_SURFACE, { key });
    }
  },
});
