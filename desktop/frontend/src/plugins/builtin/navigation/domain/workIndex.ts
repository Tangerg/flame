export type WorkSessionAttention = "running" | "waiting" | "none";

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

/**
 * Every session is split exactly once: it belongs to a project when its directory is one the
 * workspace knows, and is otherwise recent work with no home yet. Two lists rather than one
 * tree, because inventing a project from an arbitrary path gives scratch work a false home.
 */
export interface WorkIndexContent {
  groups: WorkGroup[];
  recents: WorkSession[];
}

export interface WorkIndex {
  /** Both absent until the first answer arrives — distinct from "known empty". */
  groups: WorkGroup[] | undefined;
  recents: WorkSession[] | undefined;
  activeSessionId: string;
  activeCwd: string | undefined;
  isLoading: boolean;
  isError: boolean;
}
