import type { ContentBlock } from "@/plugins/sdk/types/contentBlock";
import type { MarkdownReveal } from "./markdown/streamReveal";

export interface BlockCtx {
  expandedIds: Set<string>;
  onToggleExpand: (id: string) => void;
  textReveal: MarkdownReveal;
  /** Only this exact block is already presented by the enclosing composer. */
  questionInComposer?: Extract<ContentBlock, { kind: "question" }>;
}
