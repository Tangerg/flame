import type { FileEntryType } from "@flame/runtime-contract/wire";

export type MemoryScope = "project" | "user";
export type MemoryOrigin = "auto" | "user";
export type MemoryStatus = "active" | "pending";
import { createDataQuery, createParameterizedDataQuery } from "@/plugins/sdk";

export interface WorkspaceProjectSummary {
  id: string;
  name: string;
  sessionCount: number;
  cwdMissing?: boolean;
}

export interface WorkspaceFileChange {
  path: string;
  change: "add" | "mod" | "del" | "renamed";
  previousPath?: string;
  added?: number;
  removed?: number;
  binary?: boolean;
}

export interface AgentMemoryQuery {
  scope: MemoryScope;
  cwd?: string;
}

export interface AgentMemoryEntry {
  id: string;
  scope: MemoryScope;
  content: string;
  origin: MemoryOrigin;
  status: MemoryStatus;
  pinned: boolean;
  sessionId?: string;
  createdAt: string;
  updatedAt: string;
}

export interface WorkspaceFileChangesQuery {
  cwd?: string;
}

export interface WorkspaceListFilesQuery {
  cwd?: string;
  path?: string;
  recursive?: boolean;
  limit?: number;
}

export interface WorkspaceFileEntry {
  path: string;
  name: string;
  type: FileEntryType;
  sizeBytes?: number;
}

export interface WorkspaceReadFileQuery {
  path: string;
  cwd?: string;
  startLine?: number;
  endLine?: number;
}

export interface WorkspaceFileContent {
  content: string;
  startLine: number;
  totalLines: number;
  truncated?: boolean;
}

export const WORKSPACE_PROJECTS_KEY = "projects";
export const WORKSPACE_FILES_CHANGED_KEY = "files-changed";
export const WORKSPACE_AGENT_MEMORY_KEY = "agent-memory";
export const WORKSPACE_LIST_FILES_KEY = "list-files";
export const WORKSPACE_READ_FILE_KEY = "read-file";

export const useWorkspaceProjects =
  createDataQuery<WorkspaceProjectSummary[]>(WORKSPACE_PROJECTS_KEY);
export const useWorkspaceFileChanges = createParameterizedDataQuery<
  WorkspaceFileChangesQuery,
  WorkspaceFileChange[]
>(WORKSPACE_FILES_CHANGED_KEY);
export const useAgentMemory = createParameterizedDataQuery<AgentMemoryQuery, AgentMemoryEntry[]>(
  WORKSPACE_AGENT_MEMORY_KEY,
);
export const useWorkspaceListFiles = createParameterizedDataQuery<
  WorkspaceListFilesQuery,
  WorkspaceFileEntry[]
>(WORKSPACE_LIST_FILES_KEY);
export const useWorkspaceReadFile = createParameterizedDataQuery<
  WorkspaceReadFileQuery,
  WorkspaceFileContent
>(WORKSPACE_READ_FILE_KEY);
