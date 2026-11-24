import { type AgentSessionSummary } from "@/features/agent/public/session";
import { type WorkspaceFolderSummary } from "@/features/workspace/public/queries";
import { type ProjectSummary } from "@/features/projects/public/projects";
import {
  type WorkGroup,
  type WorkIndexContent,
  type WorkFolder,
  type WorkProjectGroup,
  type WorkSession,
} from "@/features/navigation/domain/workIndex";
interface BuildWorkIndexInput {
  folders: readonly WorkspaceFolderSummary[] | undefined;
  projects: readonly ProjectSummary[] | undefined;
  sessions: readonly AgentSessionSummary[];
}

function compareTimeDesc(a: { time: string }, b: { time: string }): number {
  if (a.time === b.time) return 0;
  return a.time < b.time ? 1 : -1;
}

function comparePinnedFirst(a: AgentSessionSummary, b: AgentSessionSummary): number {
  if (Boolean(a.favorite) !== Boolean(b.favorite)) return a.favorite ? -1 : 1;
  return compareTimeDesc(a, b);
}

function toWorkSessionAttention(session: AgentSessionSummary): WorkSession["attention"] {
  if (session.status === "running") return "running";
  if (session.status === "waiting") return "waiting";
  return "none";
}

function toWorkSession(session: AgentSessionSummary): WorkSession {
  return {
    id: session.id,
    revision: session.revision,
    title: session.title,
    attention: toWorkSessionAttention(session),
    favorite: session.favorite,
    ...(session.projectId ? { projectId: session.projectId } : {}),
    workspacePath: session.workspace.path,
    time: session.time,
  };
}

function toWorkFolder(folder: WorkspaceFolderSummary): WorkFolder {
  return {
    id: folder.id,
    name: folder.name,
    cwdMissing: folder.cwdMissing,
  };
}

export function buildWorkIndex({
  folders,
  projects,
  sessions,
}: BuildWorkIndexInput): WorkIndexContent | undefined {
  if (!folders && sessions.length === 0) return undefined;

  const byProject = new Map<string, AgentSessionSummary[]>();
  const byDirectory = new Map<string, AgentSessionSummary[]>();
  for (const session of sessions) {
    if (session.projectId) {
      const members = byProject.get(session.projectId);
      if (members) members.push(session);
      else byProject.set(session.projectId, [session]);
      continue;
    }
    const key = session.workspace.path;
    const directory = byDirectory.get(key);
    if (directory) directory.push(session);
    else byDirectory.set(key, [session]);
  }

  // A folder whose Sessions all joined Projects has nothing left to show here.
  const groups: WorkGroup[] | undefined = folders?.flatMap((folder) => {
    const owned = byDirectory.get(folder.id) ?? [];
    byDirectory.delete(folder.id);
    if (owned.length === 0 && folder.sessionCount > 0) return [];
    return [
      {
        folder: toWorkFolder(folder),
        sessions: owned.sort(comparePinnedFirst).map(toWorkSession),
      },
    ];
  });

  const projectGroups: WorkProjectGroup[] | undefined = projects?.map((project) => ({
    project: { id: project.id, name: project.name, archived: project.archived },
    sessions: (byProject.get(project.id) ?? []).sort(comparePinnedFirst).map(toWorkSession),
  }));

  const recents = [...byDirectory.values()].flat().sort(comparePinnedFirst).map(toWorkSession);

  return { groups, projects: projectGroups, recents };
}
