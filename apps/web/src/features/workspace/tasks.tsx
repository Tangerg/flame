import { contributeLayout } from "@/extensions/contributeHelpers";
import { definePlugin } from "@/extensions/definePlugin";
import { TasksPill } from "@/features/workspace/ui/TasksPill";
export const tasksPill = definePlugin({
  name: "flame.builtin.tasks",
  setup(ctx) {
    contributeLayout(ctx, "sidebar.footer.status", {
      id: "tasks",
      order: 0,
      component: TasksPill,
    });
  },
});
