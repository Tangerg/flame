// The eager half of every built-in workspace view: id, title, icon, order, badge.
//
// Bodies load on demand. A user sees at most one or two of these at a time, but a static
// barrel put all twenty on the startup path — the largest of them render diffs, terminals
// and skill catalogues, none of which the first paint needs. The metadata has to stay eager
// (the tab strip, the dock destination list and the command palette all enumerate views
// before any is opened), so it lives here and the component arrives through `lazy`.
//
// `WorkspaceViewBody` already mounts each view inside Suspense, which is what makes a
// LazyExoticComponent a drop-in `ComponentType` here. Badges are the exception — see
// `tabBadges`.

import { lazy } from "react";
import { defineWorkspaceView } from "./defineWorkspaceView";
import { DiffTabBadge, InboxBadge, PlanTabBadge } from "./tabBadges";

export const searchView = defineWorkspaceView({
  id: "search",
  title: "workspace.view.title.search",
  icon: "search",
  order: 10,
  splittable: true,
  component: lazy(() => import("./search").then((m) => ({ default: m.SearchTab }))),
});

export const inboxView = defineWorkspaceView({
  id: "inbox",
  title: "workspace.view.title.inbox",
  icon: "bell",
  badge: InboxBadge,
  order: 15,
  splittable: true,
  component: lazy(() => import("./inbox").then((m) => ({ default: m.InboxTab }))),
});

export const fileTreeView = defineWorkspaceView({
  id: "explorer",
  title: "workspace.view.title.filetree",
  icon: "folder",
  order: 20,
  splittable: true,
  component: lazy(() => import("./filetree").then((m) => ({ default: m.ExplorerView }))),
});

export const fileView = defineWorkspaceView({
  id: "file",
  title: "workspace.view.title.file",
  icon: "filetext",
  order: 25,
  splittable: true,
  component: lazy(() => import("./file").then((m) => ({ default: m.FileViewTab }))),
});

export const filesView = defineWorkspaceView({
  id: "files",
  title: "workspace.view.title.files",
  icon: "filetext",
  order: 30,
  splittable: true,
  component: lazy(() => import("./files").then((m) => ({ default: m.FilesView }))),
});

export const diffView = defineWorkspaceView({
  id: "diff",
  title: "workspace.view.title.diff",
  icon: "diff",
  badge: DiffTabBadge,
  order: 40,
  splittable: true,
  component: lazy(() => import("./diff").then((m) => ({ default: m.DiffWorkspaceSurface }))),
});

export const terminalView = defineWorkspaceView({
  id: "terminal",
  title: "workspace.view.title.terminal",
  icon: "terminal",
  order: 60,
  splittable: true,
  component: lazy(() =>
    import("./terminal").then((m) => ({ default: m.TerminalWorkspaceSurface })),
  ),
});

export const toolsView = defineWorkspaceView({
  id: "tools",
  title: "workspace.view.title.tools",
  icon: "tool",
  order: 70,
  splittable: true,
  component: lazy(() => import("./tools").then((m) => ({ default: m.ToolsTab }))),
});

export const skillsView = defineWorkspaceView({
  id: "skills",
  title: "workspace.view.title.skills",
  icon: "sparkle",
  order: 80,
  splittable: true,
  component: lazy(() => import("./skills").then((m) => ({ default: m.SkillsTab }))),
});

export const skillProposalsView = defineWorkspaceView({
  id: "skill-proposals",
  title: "workspace.view.title.skillProposals",
  icon: "sparkle",
  order: 85,
  splittable: true,
  component: lazy(() => import("./skillProposals").then((m) => ({ default: m.SkillProposalsTab }))),
});

export const skillLibraryView = defineWorkspaceView({
  id: "skill-library",
  title: "workspace.view.title.skillLibrary",
  icon: "sparkle",
  order: 90,
  splittable: true,
  component: lazy(() => import("./skillLibrary").then((m) => ({ default: m.SkillLibraryTab }))),
});

export const recipesView = defineWorkspaceView({
  id: "recipes",
  title: "workspace.view.title.recipes",
  icon: "command",
  order: 95,
  splittable: true,
  component: lazy(() => import("./recipes").then((m) => ({ default: m.RecipesTab }))),
});

export const knowledgeView = defineWorkspaceView({
  id: "knowledge",
  title: "workspace.view.title.knowledge",
  icon: "filetext",
  order: 100,
  splittable: true,
  component: lazy(() => import("./knowledge").then((m) => ({ default: m.KnowledgeTab }))),
});

export const agentMemoryView = defineWorkspaceView({
  id: "agent-memory",
  title: "workspace.view.title.agentMemory",
  icon: "book",
  order: 105,
  splittable: true,
  component: lazy(() => import("./agentMemory").then((m) => ({ default: m.AgentMemoryTab }))),
});

export const agentDocsView = defineWorkspaceView({
  id: "agent-docs",
  title: "workspace.view.title.agentDocs",
  icon: "book",
  order: 110,
  splittable: true,
  component: lazy(() => import("./agent-docs").then((m) => ({ default: m.AgentDocsTab }))),
});

export const planView = defineWorkspaceView({
  id: "plan",
  title: "workspace.view.title.plan",
  icon: "list",
  badge: PlanTabBadge,
  order: 120,
  splittable: true,
  component: lazy(() => import("./plan").then((m) => ({ default: m.PlanTab }))),
});

export const runSummaryView = defineWorkspaceView({
  id: "run-summary",
  title: "workspace.view.title.runSummary",
  icon: "check",
  order: 130,
  splittable: true,
  component: lazy(() => import("./run-summary").then((m) => ({ default: m.RunSummaryTab }))),
});

export const timelineView = defineWorkspaceView({
  id: "timeline",
  title: "workspace.view.title.timeline",
  icon: "history",
  order: 140,
  splittable: true,
  component: lazy(() => import("./timeline").then((m) => ({ default: m.TimelineTab }))),
});

export const notificationsView = defineWorkspaceView({
  id: "notifications",
  title: "workspace.view.title.notifications",
  icon: "bell",
  order: 145,
  splittable: true,
  component: lazy(() => import("./notifications").then((m) => ({ default: m.NotificationsTab }))),
});

export const toolStatsView = defineWorkspaceView({
  id: "tool-stats",
  title: "workspace.view.title.toolStats",
  icon: "chart",
  order: 150,
  splittable: true,
  component: lazy(() => import("./toolStats").then((m) => ({ default: m.ToolStatsTab }))),
});
