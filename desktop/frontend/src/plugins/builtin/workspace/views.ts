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
      icon: "sparkle",
      order: 80,
      dock: "workspace",
      component: lazy(() => import("./ui/skills/Skills").then((m) => ({ default: m.Skills }))),
    });
  },
});

export const agentMemoryView = definePlugin({
  name: "flame.builtin.view-agent-memory",
  setup(ctx) {
    ctx.contribute(WORKSPACE_VIEW, {
      id: "agent-memory",
      title: "workspace.view.title.agentMemory",
      icon: "brain",
      order: 105,
      dock: "workspace",
      component: lazy(() =>
        import("./ui/memory/AgentMemory").then((m) => ({ default: m.AgentMemory })),
      ),
    });
  },
});

export const timelineView = definePlugin({
  name: "flame.builtin.view-timeline",
  setup(ctx) {
    ctx.contribute(WORKSPACE_VIEW, {
      id: "timeline",
      title: "workspace.view.title.timeline",
      icon: "history",
      order: 140,
      dock: "session",
      component: lazy(() =>
        import("./ui/timeline/Timeline").then((m) => ({ default: m.Timeline })),
      ),
    });
  },
});
