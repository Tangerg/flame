import type { AgentMemoryEntry } from "../workspaceQueries";

export type AgentMemoryDecision = "approve" | "reject";

export interface AgentMemoryAddInput {
  scope: "project" | "user";
  cwd?: string;
  content: string;
}

export interface AgentMemoryGateway {
  review(id: string, decision: AgentMemoryDecision): Promise<void>;
  updateContent(id: string, content: string): Promise<AgentMemoryEntry>;
  setPinned(id: string, pinned: boolean): Promise<AgentMemoryEntry>;
  delete(id: string): Promise<void>;
  add(input: AgentMemoryAddInput): Promise<AgentMemoryEntry>;
}
