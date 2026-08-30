// A pure CONSUMER of the in-memory stores: the OTel providers are installed always-on by
// the bootstrap plugin, never lazily here, because trace-context propagation must work
// whether or not anyone opened Diagnostics.

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
