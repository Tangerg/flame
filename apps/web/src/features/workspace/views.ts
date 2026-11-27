import { lazy } from "react";
import { definePlugin } from "@/extensions/definePlugin";
import { WORKSPACE_VIEW } from "@/features/workspace/public/views";
export const fileView = definePlugin({
  name: "flame.builtin.view-file",
  setup(ctx) {
    ctx.contributions.add(WORKSPACE_VIEW, {
      id: "file",
      title: "workspace.view.title.file",
      order: 20,
      dock: "workspace",
      component: lazy(() =>
        import(/* @flame-lazy:workspace */ "@/features/workspace/ui/files/FileWorkspace").then(
          (m) => ({
            default: m.FileWorkspace,
          }),
        ),
      ),
    });
  },
});

export const taskWorktreeView = definePlugin({
  name: "flame.builtin.view-task-worktrees",
  setup(ctx) {
    ctx.contributions.add(WORKSPACE_VIEW, {
      id: "task-worktrees",
      title: "workspace.tasks.title",
      order: 25,
      dock: "workspace",
      component: lazy(() =>
        import(/* @flame-lazy:workspace */ "@/features/workspace/ui/TaskWorktrees").then((m) => ({
          default: m.TaskWorktrees,
        })),
      ),
    });
  },
});
