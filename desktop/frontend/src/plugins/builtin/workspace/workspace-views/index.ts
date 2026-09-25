import { lazy } from "react";
import { defineWorkspaceView } from "./defineWorkspaceView";
import { DiffTabBadge } from "./tabBadges";

export const fileView = defineWorkspaceView({
  id: "file",
  title: "workspace.view.title.file",
  icon: "folder",
  order: 20,
  dock: "workspace",
  component: lazy(() => import("./file").then((m) => ({ default: m.FileViewTab }))),
});

export const diffView = defineWorkspaceView({
  id: "diff",
  title: "workspace.view.title.diff",
  icon: "diff",
  badge: DiffTabBadge,
  order: 40,
  dock: "workspace",
  component: lazy(() => import("./diff").then((m) => ({ default: m.DiffWorkspaceSurface }))),
});

export const skillsView = defineWorkspaceView({
  id: "skills",
  title: "workspace.view.title.skills",
  icon: "sparkle",
  order: 80,
  dock: "workspace",
  component: lazy(() => import("./skills").then((m) => ({ default: m.SkillsTab }))),
});

export const agentMemoryView = defineWorkspaceView({
  id: "agent-memory",
  title: "workspace.view.title.agentMemory",
  icon: "brain",
  order: 105,
  dock: "workspace",
  component: lazy(() => import("./agentMemory").then((m) => ({ default: m.AgentMemoryTab }))),
});

export const timelineView = defineWorkspaceView({
  id: "timeline",
  title: "workspace.view.title.timeline",
  icon: "history",
  order: 140,
  dock: "session",
  component: lazy(() => import("./timeline").then((m) => ({ default: m.TimelineTab }))),
});
