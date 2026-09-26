export type WorkspaceInvalidationTarget =
  | "all"
  | "agentMemory"
  | "agentSessionProjection"
  | "approvalMode"
  | "approvalRules"
  | "diff"
  | "fileList"
  | "fileRead"
  | "filesChanged"
  | "hooks"
  | "models"
  | "mcpServers"
  | "mcpTools"
  | "providers"
  | "schedules"
  | "sessionUsage"
  | "sessions"
  | "usageSummary"
  | "utilityRole"
  | "embeddingRole"
  | "skills"
  | "managedSkills"
  | "skillProposals";

type WorkspaceEventType =
  | "files.changed"
  | "skills.changed"
  | "mcp.changed"
  | "schedules.changed"
  | "sessions.changed"
  | "runs.changed"
  | "plan.changed"
  | "goals.changed"
  | "interrupts.changed"
  | "hooks.changed"
  | "models.changed"
  | "approvals.changed"
  | "agentMemory.changed"
  | "resync";

type WorkspaceTopic = Exclude<WorkspaceEventType, "resync">;

export interface WorkspaceEventLike {
  type: WorkspaceEventType;
  sequence: number;
  sessionIds?: string[];
  topics?: WorkspaceTopic[];
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

const FILE_TARGETS = new Set<WorkspaceInvalidationTarget>([
  "filesChanged",
  "diff",
  "fileList",
  "fileRead",
  "hooks",
  "skills",
]);

export function workspaceQueryAffected(
  target: WorkspaceInvalidationTarget,
  params: WorkspaceQueryScope | undefined,
  event: WorkspaceEventLike,
): boolean {
  if (!FILE_TARGETS.has(target)) return true;
  if (event.type !== "files.changed" && event.type !== "resync") return true;
  // Resync's watch IDs narrow only files.changed. Authored resources can be
  // global, including a user Skill used in several workspaces.
  if (event.type === "resync") {
    if (target === "skills" && event.topics?.includes("skills.changed")) return true;
    if (target === "hooks" && event.topics?.includes("hooks.changed")) return true;
  }
  const roots = affectedWorkspaces(event);
  if (roots && params?.cwd === undefined) {
    const defaultWorkspace = event.watchScopes?.find((scope) => scope.cwd === undefined);
    if (defaultWorkspace && !roots.includes(defaultWorkspace.workspace.path)) return false;
  }
  if (roots && params?.cwd !== undefined) {
    const matches = roots.some(
      (root) =>
        params.cwd === root ||
        event.watchScopes?.some(
          (scope) => scope.workspace.path === root && scope.cwd === params.cwd,
        ),
    );
    if (!matches) return false;
  }
  if (target !== "fileRead" && target !== "fileList" && target !== "diff") return true;
  if (!event.paths?.length) return true;
  const queryPath = relativePath(params?.path ?? ".", event.workspace?.path);
  const paths = event.paths.map((path) => relativePath(path, event.workspace?.path));
  if (queryPath === undefined || paths.includes(undefined)) return true;
  return paths.some(
    (path) =>
      path !== undefined &&
      (atOrBelow(queryPath, path) || (target !== "fileRead" && atOrBelow(path, queryPath))),
  );
}

function affectedWorkspaces(event: WorkspaceEventLike): string[] | undefined {
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

export function workspaceInvalidations(ev: WorkspaceEventLike): WorkspaceInvalidationTarget[] {
  switch (ev.type) {
    case "files.changed":
      return ["filesChanged", "diff", "fileList", "fileRead", "hooks", "skills"];
    case "skills.changed":
      return ["skills", "managedSkills", "skillProposals"];
    case "mcp.changed":
      return ["mcpServers", "mcpTools"];
    case "schedules.changed":
      return ["schedules"];
    case "sessions.changed":
      return ["sessions"];
    case "runs.changed":
      return ["sessionUsage", "usageSummary", "agentSessionProjection"];
    case "interrupts.changed":
      return ["agentSessionProjection"];
    case "goals.changed":
      return ["agentSessionProjection"];
    case "plan.changed":
      return ["agentSessionProjection"];
    case "hooks.changed":
      return ["hooks"];
    case "models.changed":
      return ["providers", "models", "utilityRole", "embeddingRole"];
    case "approvals.changed":
      return ["approvalMode", "approvalRules"];
    case "agentMemory.changed":
      return ["agentMemory"];
    case "resync": {
      if (!ev.topics?.length) return ["all"];
      const targets = new Set<WorkspaceInvalidationTarget>();
      for (const topic of ev.topics) {
        for (const target of workspaceInvalidations({ type: topic, sequence: ev.sequence })) {
          targets.add(target);
        }
      }
      return [...targets];
    }
    default: {
      const unhandled: never = ev.type;
      return unhandled;
    }
  }
}
