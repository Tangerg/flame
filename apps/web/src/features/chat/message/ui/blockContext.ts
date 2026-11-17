import { type ContentBlock } from "@/features/agent/public/contentBlock";
export interface BlockCtx {
  sessionId: string;
  expandedIds: Set<string>;
  onExpandedChange: (id: string, expanded: boolean) => void;
  questionInComposer?: Extract<ContentBlock, { kind: "question" }>;
}
