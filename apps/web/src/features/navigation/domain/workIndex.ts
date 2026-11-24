type WorkSessionAttention = "running" | "waiting" | "none";

export interface WorkSession {
  id: string;
  revision: number;
  title: string;
  attention: WorkSessionAttention;
  favorite?: boolean;
  projectId?: string;
  workspacePath: string;
  time: string;
}

export interface WorkFolder {
  id: string;
  name: string;
  cwdMissing?: boolean;
}

export interface WorkGroup {
  folder: WorkFolder;
  sessions: WorkSession[];
}

export interface WorkProject {
  id: string;
  name: string;
  archived: boolean;
}

export interface WorkProjectGroup {
  project: WorkProject;
  sessions: WorkSession[];
}

export function sessionIsRunning(session: WorkSession): boolean {
  return session.attention === "running";
}

export function sessionIsWaiting(session: WorkSession): boolean {
  return session.attention === "waiting";
}

export function sessionNeedsAttention(session: WorkSession): boolean {
  return session.attention !== "none";
}

export function workGroupWaitingCount(group: { sessions: readonly WorkSession[] }): number {
  return group.sessions.filter(sessionIsWaiting).length;
}

export function workGroupHasRunning(group: { sessions: readonly WorkSession[] }): boolean {
  return group.sessions.some(sessionIsRunning);
}

// A Session in a Project appears only under that Project, never again under its
// folder or in recents.
export interface WorkIndexContent {
  groups: WorkGroup[] | undefined;
  projects: WorkProjectGroup[] | undefined;
  recents: WorkSession[];
}

export interface WorkIndex {
  groups: WorkGroup[] | undefined;
  projects: WorkProjectGroup[] | undefined;
  recents: WorkSession[] | undefined;
  activeSessionId: string;
  activeCwd: string | undefined;
  isLoading: boolean;
  error: unknown;
  retry: () => void;
}
