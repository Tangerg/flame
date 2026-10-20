import { lazy } from "react";
import { definePlugin } from "@/extensions/definePlugin";
import { WORKSPACE_VIEW } from "@/features/workspace/public/views";
export const fileView = definePlugin({
  name: "flame.builtin.view-file",
  setup(ctx) {
    ctx.contribute(WORKSPACE_VIEW, {
      id: "file",
      title: "workspace.view.title.file",
      icon: "folder",
      order: 20,
      dock: "workspace",
      component: lazy(() =>
        import("@/features/workspace/ui/files/FileWorkspace").then((m) => ({
          default: m.FileWorkspace,
        })),
      ),
    });
  },
});
