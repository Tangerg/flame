import { lazy } from "react";
import { definePlugin } from "@/plugins/sdk";
import { WORKSPACE_VIEW } from "@/plugins/sdk/kernelPoints";
import { DiffTabBadge } from "./ui/review/DiffTabBadge";

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
        import("./ui/files/FileWorkspace").then((m) => ({ default: m.FileWorkspace })),
      ),
    });
  },
});

export const diffView = definePlugin({
  name: "flame.builtin.view-diff",
  setup(ctx) {
    ctx.contribute(WORKSPACE_VIEW, {
      id: "diff",
      title: "workspace.view.title.diff",
      icon: "diff",
      badge: DiffTabBadge,
      order: 40,
      dock: "workspace",
      component: lazy(() =>
        import("./ui/review/DiffWorkspace").then((m) => ({ default: m.DiffWorkspace })),
      ),
    });
  },
});

export const skillsView = definePlugin({
  name: "flame.builtin.view-skills",
  setup(ctx) {
    ctx.contribute(WORKSPACE_VIEW, {
      id: "skills",
      title: "workspace.view.title.skills",
      icon: "library",
      order: 80,
      dock: "workspace",
      component: lazy(() => import("./ui/skills/Skills").then((m) => ({ default: m.Skills }))),
    });
  },
});
