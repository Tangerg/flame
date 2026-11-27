import type { RuntimeEventType, RuntimeTopic } from "@flame/runtime-client/wire";

export type WorkspaceInvalidationTarget =
  | "all"
  | "agentSessionProjection"
  | "approvalMode"
  | "approvalRules"
  | "fileList"
  | "fileRead"
  | "filesChanged"
  | "models"
  | "projects"
  | "providers"
  | "sessionUsage"
  | "sessions";

export interface WorkspaceInvalidationEvent {
  type: RuntimeEventType;
  sessionIds?: string[];
  topics?: RuntimeTopic[];
  workspace?: { path: string };
  paths?: string[];
  watchId?: string;
  watchIds?: string[];
  watchScopes?: readonly WorkspaceWatchScope[];
}

export interface WorkspaceWatchScope {
  watchId: string;
  workspace: { path: string };
  cwd?: string;
}

interface WorkspaceQueryScope {
  cwd?: string;
  path?: string;
}

const FILE_TARGETS = new Set<WorkspaceInvalidationTarget>(["filesChanged", "fileList", "fileRead"]);

export function workspaceQueryAffected(
  target: WorkspaceInvalidationTarget,
  params: WorkspaceQueryScope | undefined,
  event: WorkspaceInvalidationEvent,
): boolean {
  if (!FILE_TARGETS.has(target)) return true;
  if (event.type !== "files.changed" && event.type !== "resync") return true;
  const roots = affectedWorkspaces(event);
  if (roots && !workspaceInScope(roots, params?.cwd, event)) return false;
  if (target !== "fileRead" && target !== "fileList") return true;
  return pathInScope(target, params?.path, event);
}

// A query without its own folder reads the default watched workspace.
function workspaceInScope(
  roots: readonly string[],
  cwd: string | undefined,
  event: WorkspaceInvalidationEvent,
): boolean {
  if (cwd === undefined) {
    const defaultWorkspace = event.watchScopes?.find((scope) => scope.cwd === undefined);
    return !defaultWorkspace || roots.includes(defaultWorkspace.workspace.path);
  }
  return roots.some(
    (root) =>
      cwd === root ||
      event.watchScopes?.some((scope) => scope.workspace.path === root && scope.cwd === cwd),
  );
}

// A read is affected by changes at or below its path; a listing also by its ancestors.
function pathInScope(
  target: "fileRead" | "fileList",
  queryPath: string | undefined,
  event: WorkspaceInvalidationEvent,
): boolean {
  if (!event.paths?.length) return true;
  const query = relativePath(queryPath ?? ".", event.workspace?.path);
  const paths = event.paths.map((path) => relativePath(path, event.workspace?.path));
  if (query === undefined || paths.includes(undefined)) return true;
  return paths.some(
    (path) =>
      path !== undefined &&
      (atOrBelow(query, path) || (target === "fileList" && atOrBelow(path, query))),
  );
}

function affectedWorkspaces(event: WorkspaceInvalidationEvent): string[] | undefined {
  if (event.workspace) return [event.workspace.path];
  const ids = event.watchId ? [event.watchId] : event.watchIds;
  if (!ids?.length) return undefined;
  const roots: string[] = [];
  for (const id of ids) {
    const scope = event.watchScopes?.find((candidate) => candidate.watchId === id);
    if (!scope) return undefined;
    roots.push(scope.workspace.path);
  }
  return roots;
}

function relativePath(path: string, root?: string): string | undefined {
  if (root && path.startsWith(`${root}/`)) path = path.slice(root.length + 1);
  if (path.startsWith("/") || path.split("/").includes("..")) return undefined;
  return (
    path
      .split("/")
      .filter((part) => part && part !== ".")
      .join("/") || "."
  );
}

function atOrBelow(path: string, parent: string): boolean {
  return parent === "." || path === parent || path.startsWith(`${parent}/`);
}

const TOPIC_INVALIDATIONS = {
  "files.changed": ["filesChanged", "fileList", "fileRead"],
  "skills.changed": [],
  "plugins.changed": ["approvalRules"],
  "mcp.changed": ["approvalRules"],
  "sessions.changed": ["sessions"],
  "runs.changed": ["sessionUsage", "agentSessionProjection"],
  "interrupts.changed": ["agentSessionProjection"],
  "goals.changed": ["agentSessionProjection"],
  "plan.changed": ["agentSessionProjection"],
  "hooks.changed": [],
  "models.changed": ["providers", "models"],
  "approvals.changed": ["approvalMode", "approvalRules"],
  "plugins.dataChanged": [],
  "projects.changed": ["projects"],
} as const satisfies Record<
  RuntimeTopic,
  readonly WorkspaceInvalidationTarget[]
>;

export function workspaceInvalidations(ev: WorkspaceInvalidationEvent): WorkspaceInvalidationTarget[] {
  if (ev.type !== "resync") return [...TOPIC_INVALIDATIONS[ev.type]];
  if (!ev.topics?.length) return ["all"];
  const targets = new Set<WorkspaceInvalidationTarget>();
  for (const topic of ev.topics) {
    for (const target of TOPIC_INVALIDATIONS[topic]) {
      targets.add(target);
    }
  }
  return [...targets];
}
