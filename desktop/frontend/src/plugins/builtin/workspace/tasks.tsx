import { contributeLayout, definePlugin } from "@/plugins/sdk";
import { installTaskReadoutPort } from "./adapters/taskReadoutStore";
import { TasksPill } from "./ui/TasksPill";

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
