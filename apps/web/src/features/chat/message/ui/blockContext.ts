import { type ContentBlock } from "@/features/agent/public/contentBlock";
import { type MarkdownReveal } from "@/features/chat/message/ui/markdown/streamReveal";
export interface BlockCtx {
  sessionId: string;
  expandedIds: Set<string>;
  onToggleExpand: (id: string) => void;
  textReveal: MarkdownReveal;
  questionInComposer?: Extract<ContentBlock, { kind: "question" }>;
}
