type WorkSessionAttention = "running" | "waiting" | "none";

export interface WorkSession {
  id: string;
  revision: number;
  title: string;
  attention: WorkSessionAttention;
  favorite?: boolean;
  time: string;
}

export interface WorkProject {
  id: string;
  name: string;
  cwdMissing?: boolean;
}

export interface WorkGroup {
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

export function workGroupWaitingCount(group: WorkGroup): number {
  return group.sessions.filter(sessionIsWaiting).length;
}

export function workGroupHasRunning(group: WorkGroup): boolean {
  return group.sessions.some(sessionIsRunning);
}

export interface WorkIndexContent {
  groups: WorkGroup[];
  recents: WorkSession[];
}

export interface WorkIndex {
  groups: WorkGroup[] | undefined;
  recents: WorkSession[] | undefined;
  activeSessionId: string;
  activeCwd: string | undefined;
  isLoading: boolean;
  error: unknown;
  retry: () => void;
}
