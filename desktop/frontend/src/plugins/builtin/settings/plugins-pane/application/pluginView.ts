import type {
  AgentMemoryListRequest,
  SkillDiscovery,
  SkillDetail,
  Page,
  ManagedSkill,
  SkillProposal,
  WorkspaceRef,
  ReadPluginViewRequest,
  ReadPluginRendererRequest,
} from "@flame/runtime-contract/wire";

export type PluginViewStage =
  | "carrier"
  | "resource"
  | "initial"
  | "handshake"
  | "read"
  | "publication"
  | "protocol"
  | "guest"
  | "retirement";
export type PluginViewStatus =
  | { type: "loading" }
  | { type: "ready" }
  | { type: "failure"; stage: PluginViewStage; reason: string };

export interface PluginViewReads<T> {
  readonly binding: Readonly<ReadPluginViewRequest | ReadPluginRendererRequest>;
  resource(signal: AbortSignal): Promise<string>;
  initial(signal: AbortSignal): Promise<T>;
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
