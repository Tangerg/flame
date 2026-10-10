import type {
  AgentMemoryListRequest,
  SkillDiscovery,
  SkillDetail,
  Page,
  ManagedSkill,
  SkillProposal,
  WorkspaceRef,
  GetDiffRequest,
  Diff,
} from "@flame/runtime-contract/wire";

export type PluginViewStatus =
  { type: "loading" } | { type: "ready" } | { type: "failure"; reason: string };

// Settlement joins all reads accepted by the operation, including on failure or cancellation.
export interface PluginViewReads<T> {
  load(signal: AbortSignal): Promise<{ html: string; initial: T }>;
  read(cursor: string | undefined, signal: AbortSignal): Promise<T>;
}

export type MemoryViewTarget = Pick<AgentMemoryListRequest, "scope" | "workspace">;

export type SkillViewTarget =
  | { type: "available" | "review"; workspace: WorkspaceRef }
  | { type: "library" }
  | { type: "detail"; workspace: WorkspaceRef; name: string };

export type SkillViewSnapshot =
  | { type: "available"; data: SkillDiscovery }
  | { type: "detail"; data: SkillDetail }
  | { type: "library"; data: Page<ManagedSkill> }
  | { type: "review"; data: Page<SkillProposal> };

export type DiffViewTarget = Pick<GetDiffRequest, "workspace" | "mode"> & { focusPath: string };
export interface DiffViewSnapshot {
  diff: Diff;
  focusPath: string;
}
