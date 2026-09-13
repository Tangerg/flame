import { lazy } from "react";
import { definePlugin } from "@/plugins/sdk";
import { WORKSPACE_VIEW } from "@/plugins/sdk/kernelPoints";

export const subagentsView = definePlugin({
  name: "flame.builtin.subagents-view",
  setup(ctx) {
    ctx.contribute(WORKSPACE_VIEW, {
      id: "subagents",
      title: "subagents.title",
      icon: "bot",
      order: 130,
      dock: "session",
      component: lazy(() =>
        import("./ui/SubagentsPanel").then((m) => ({ default: m.SubagentsPanel })),
      ),
    });
  },
});
