export type ReviewDecision = "approve" | "reject";
import type { AgentMemoryEntry, MemoryScope } from "../workspaceQueries";

export interface AgentMemoryAddInput {
  scope: MemoryScope;
  cwd?: string;
  content: string;
}

export interface AgentMemoryGateway {
  review(id: string, decision: ReviewDecision): Promise<void>;
  updateContent(id: string, content: string): Promise<AgentMemoryEntry>;
  setPinned(id: string, pinned: boolean): Promise<AgentMemoryEntry>;
  delete(id: string): Promise<void>;
  add(input: AgentMemoryAddInput): Promise<AgentMemoryEntry>;
}
