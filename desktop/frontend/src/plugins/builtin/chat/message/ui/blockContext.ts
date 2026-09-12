import type { MarkdownReveal } from "./markdown/streamReveal";

export interface BlockCtx {
  expandedIds: Set<string>;
  onToggleExpand: (id: string) => void;
  textReveal: MarkdownReveal;
}
