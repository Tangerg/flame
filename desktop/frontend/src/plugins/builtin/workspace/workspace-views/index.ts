// The eager half of every built-in workspace view. Metadata must stay eager — the tab
// strip, dock destination list and command palette enumerate views before any is opened —
// so it lives here and the body arrives through `lazy`, which `WorkspaceViewBody`'s
// Suspense boundary makes a drop-in `ComponentType`. Badges are the exception; see
// `tabBadges`.

import { lazy } from "react";
import { defineWorkspaceView } from "./defineWorkspaceView";
import { DiffTabBadge, InboxBadge, PlanTabBadge } from "./tabBadges";

export const searchView = defineWorkspaceView({
  id: "search",
  title: "workspace.view.title.search",
  icon: "search",
  order: 10,
  dock: "workspace",
  component: lazy(() => import("./search").then((m) => ({ default: m.SearchTab }))),
});

export const inboxView = defineWorkspaceView({
  id: "inbox",
  title: "workspace.view.title.inbox",
  icon: "bell",
  badge: InboxBadge,
  order: 15,
  dock: "workspace",
  component: lazy(() => import("./inbox").then((m) => ({ default: m.InboxTab }))),
});

export const fileTreeView = defineWorkspaceView({
  id: "explorer",
  title: "workspace.view.title.filetree",
  icon: "folder",
  order: 20,
  dock: "workspace",
  component: lazy(() => import("./filetree").then((m) => ({ default: m.ExplorerView }))),
});

export const fileView = defineWorkspaceView({
  id: "file",
  title: "workspace.view.title.file",
  icon: "filetext",
  order: 25,
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

export const toolsView = defineWorkspaceView({
  id: "tools",
  title: "workspace.view.title.tools",
  icon: "tool",
  order: 70,
  dock: "workspace",
  component: lazy(() => import("./tools").then((m) => ({ default: m.ToolsTab }))),
});

export const skillsView = defineWorkspaceView({
  id: "skills",
  title: "workspace.view.title.skills",
  icon: "sparkle",
  order: 80,
  dock: "workspace",
  component: lazy(() => import("./skills").then((m) => ({ default: m.SkillsTab }))),
});

export const recipesView = defineWorkspaceView({
  id: "recipes",
  title: "workspace.view.title.recipes",
  icon: "command",
  order: 95,
  dock: "workspace",
  component: lazy(() => import("./recipes").then((m) => ({ default: m.RecipesTab }))),
});

export const knowledgeView = defineWorkspaceView({
  id: "knowledge",
  title: "workspace.view.title.knowledge",
  icon: "filetext",
  order: 100,
  dock: "workspace",
  component: lazy(() => import("./knowledge").then((m) => ({ default: m.KnowledgeTab }))),
});

export const agentMemoryView = defineWorkspaceView({
  id: "agent-memory",
  title: "workspace.view.title.agentMemory",
  icon: "book",
  order: 105,
  dock: "workspace",
  component: lazy(() => import("./agentMemory").then((m) => ({ default: m.AgentMemoryTab }))),
});

export const agentDocsView = defineWorkspaceView({
  id: "agent-docs",
  title: "workspace.view.title.agentDocs",
  icon: "book",
  order: 110,
  dock: "workspace",
  component: lazy(() => import("./agent-docs").then((m) => ({ default: m.AgentDocsTab }))),
});

export const planView = defineWorkspaceView({
  id: "plan",
  title: "workspace.view.title.plan",
  icon: "list",
  badge: PlanTabBadge,
  order: 120,
  dock: "session",
  component: lazy(() => import("./plan").then((m) => ({ default: m.PlanTab }))),
});

export const timelineView = defineWorkspaceView({
  id: "timeline",
  title: "workspace.view.title.timeline",
  icon: "history",
  order: 140,
  dock: "session",
  component: lazy(() => import("./timeline").then((m) => ({ default: m.TimelineTab }))),
});

export const notificationsView = defineWorkspaceView({
  id: "notifications",
  title: "workspace.view.title.notifications",
  icon: "bell",
  order: 145,
  dock: "session",
  component: lazy(() => import("./notifications").then((m) => ({ default: m.NotificationsTab }))),
});
