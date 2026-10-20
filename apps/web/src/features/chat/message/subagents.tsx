import { lazy } from "react";
import { definePlugin } from "@/extensions/definePlugin";
import { WORKSPACE_VIEW } from "@/features/workspace/public/views";
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
        import("@/features/chat/message/ui/SubagentsPanel").then((m) => ({
          default: m.SubagentsPanel,
        })),
      ),
    });
  },
});
