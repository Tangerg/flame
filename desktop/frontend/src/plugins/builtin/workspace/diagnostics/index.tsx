import { definePlugin } from "@/plugins/sdk";
import { WORKSPACE_VIEW } from "@/plugins/sdk/kernelPoints";
import { DiagnosticsView } from "./DiagnosticsView";

export default definePlugin({
  name: "flame.builtin.diagnostics",
  setup(ctx) {
    ctx.contribute(WORKSPACE_VIEW, {
      id: "diagnostics",
      title: "workspace.view.title.diagnostics",
      icon: "spark",
      order: 115,
      splittable: true,
      component: DiagnosticsView,
    });
  },
});
