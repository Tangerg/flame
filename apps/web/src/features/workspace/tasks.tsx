import { contributeLayout } from "@/extensions/contributeHelpers";
import { definePlugin } from "@/extensions/definePlugin";
import { installTaskReadoutPort } from "@/features/workspace/adapters/taskReadoutStore";
import { TasksPill } from "@/features/workspace/ui/TasksPill";
export const tasksPill = definePlugin({
  name: "flame.builtin.tasks",
  setup(ctx) {
    ctx.cleanup(installTaskReadoutPort());
    contributeLayout(ctx, "sidebar.footer.status", {
      id: "tasks",
      order: 0,
      component: TasksPill,
    });
  },
});
