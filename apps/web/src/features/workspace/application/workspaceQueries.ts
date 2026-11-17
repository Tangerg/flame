import { type FileEntryType } from "@/foundation/protocolValues.generated";
import {
  createDataQuery,
  createPagedDataQuery,
  createParameterizedDataQuery,
} from "@/extensions/dataQuery";
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

export interface WorkspaceFileChangesQuery {
  cwd?: string;
}

export interface WorkspaceListFilesQuery {
  cwd?: string;
  path?: string;
  recursive?: boolean;
  nameQuery?: string;
  limit?: number;
  cursor?: string;
}

export interface WorkspaceFileEntry {
  path: string;
  name: string;
  type: FileEntryType;
  sizeBytes?: number;
}

export interface WorkspaceFilePage {
  data: WorkspaceFileEntry[];
  nextCursor?: string;
  complete: boolean;
  coverage: { scannedEntries: number; excludedEntries: number; budgetExhausted: boolean };
}

export interface WorkspaceReadFileQuery {
  path: string;
  cwd?: string;
  startLine?: number;
  endLine?: number;
  fromEnd?: boolean;
}

export interface WorkspaceFileContent {
  content: string;
  startLine: number;
  endLine: number;
  totalLines: number;
  truncated?: boolean;
}

export const WORKSPACE_PROJECTS_KEY = "projects";

export const WORKSPACE_FILES_CHANGED_KEY = "files-changed";

export const WORKSPACE_LIST_FILES_KEY = "list-files";

export const WORKSPACE_READ_FILE_KEY = "read-file";

export const useWorkspaceProjects =
  createDataQuery<WorkspaceProjectSummary[]>(WORKSPACE_PROJECTS_KEY);

export const useWorkspaceFileChanges = createParameterizedDataQuery<
  WorkspaceFileChangesQuery,
  WorkspaceFileChange[]
>(WORKSPACE_FILES_CHANGED_KEY);

export const useWorkspaceListFiles = createParameterizedDataQuery<
  WorkspaceListFilesQuery,
  WorkspaceFilePage
>(WORKSPACE_LIST_FILES_KEY);

export const useWorkspaceReadFile = createParameterizedDataQuery<
  WorkspaceReadFileQuery,
  WorkspaceFileContent
>(WORKSPACE_READ_FILE_KEY, {
  keepWhenContinuous: (previous, next) => previous.cwd === next.cwd && previous.path === next.path,
});

export const useWorkspaceDirectory = createPagedDataQuery<
  WorkspaceListFilesQuery,
  WorkspaceFilePage
>(WORKSPACE_LIST_FILES_KEY);
