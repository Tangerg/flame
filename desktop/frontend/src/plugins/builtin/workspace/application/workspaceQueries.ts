import type { WorkspaceDiffMode } from "./diffVocabulary";
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

export interface WorkspaceSkill {
  name: string;
  description: string;
  scope: "project" | "user";
}

export interface WorkspaceSkillDiscovery {
  skills: WorkspaceSkill[];
  diagnostics: { name: string; detail: string }[];
}

export interface WorkspaceSkillDetail extends WorkspaceSkill {
  path: string;
  revision: string;
  instructions: string;
}

export interface WorkspaceSkillDetailQuery {
  cwd?: string;
  name: string;
}

export interface WorkspaceCatalogQuery {
  cwd?: string;
}

export interface ManagedSkill {
  name: string;
  description: string;
  lifecycle: "active" | "archived";
}

export interface SkillProposal {
  workspace: string;
  name: string;
  revision: string;
  scope: "project" | "user";
  description: string;
  instructions: string;
  origin: "requested" | "mined";
  revises: boolean;
  sourceSession: string;
}

export interface AgentMemoryQuery {
  scope: "project" | "user";
  cwd?: string;
}

export interface AgentMemoryEntry {
  id: string;
  scope: "project" | "user";
  content: string;
  origin: "auto" | "user";
  status: "active" | "pending";
  pinned: boolean;
  sessionId: string;
  day: string;
  createdAt: string;
  updatedAt: string;
}

export interface WorkspaceDiffQuery {
  cwd?: string;
  path?: string;
  mode?: WorkspaceDiffMode;
  limit?: number;
}

export interface WorkspaceFileChangesQuery {
  cwd?: string;
}

export type WorkspaceDiffRow =
  | { type: "hunk"; text: string }
  | { type: "context"; leftLine: number; rightLine: number; code: string }
  | { type: "added"; rightLine: number; code: string }
  | { type: "deleted"; leftLine: number; code: string };

export interface WorkspaceFileDiff {
  path: string;
  status: "added" | "modified" | "deleted" | "renamed" | "untracked";
  previousPath?: string;
  added?: number;
  removed?: number;
  binary?: boolean;
  rows: WorkspaceDiffRow[];
}

type WorkspaceDiffBaseline = { type: "head" | "mergeBase"; commit: string } | { type: "emptyTree" };

export interface WorkspaceDiff {
  baseline: WorkspaceDiffBaseline;
  files: WorkspaceFileDiff[];
  truncated?: boolean;
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
  type: "file" | "dir" | "symlink";
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
export const WORKSPACE_DIFF_KEY = "diff";
export const WORKSPACE_SKILLS_KEY = "skills";
export const WORKSPACE_SKILL_DETAIL_KEY = "skill-detail";
export const WORKSPACE_MANAGED_SKILLS_KEY = "managed-skills";
export const WORKSPACE_SKILL_PROPOSALS_KEY = "skill-proposals";
export const WORKSPACE_AGENT_MEMORY_KEY = "agent-memory";
export const WORKSPACE_LIST_FILES_KEY = "list-files";
export const WORKSPACE_READ_FILE_KEY = "read-file";

export const useWorkspaceProjects =
  createDataQuery<WorkspaceProjectSummary[]>(WORKSPACE_PROJECTS_KEY);
export const useWorkspaceFileChanges = createParameterizedDataQuery<
  WorkspaceFileChangesQuery,
  WorkspaceFileChange[]
>(WORKSPACE_FILES_CHANGED_KEY);
export const useWorkspaceDiff = createParameterizedDataQuery<WorkspaceDiffQuery, WorkspaceDiff>(
  WORKSPACE_DIFF_KEY,
);
export const useWorkspaceSkills = createParameterizedDataQuery<
  WorkspaceCatalogQuery,
  WorkspaceSkillDiscovery
>(WORKSPACE_SKILLS_KEY);
export const useWorkspaceSkillDetail = createParameterizedDataQuery<
  WorkspaceSkillDetailQuery,
  WorkspaceSkillDetail
>(WORKSPACE_SKILL_DETAIL_KEY);
export const useManagedSkills = createDataQuery<ManagedSkill[]>(WORKSPACE_MANAGED_SKILLS_KEY);
export const useSkillProposals = createParameterizedDataQuery<
  WorkspaceCatalogQuery,
  SkillProposal[]
>(WORKSPACE_SKILL_PROPOSALS_KEY);
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
