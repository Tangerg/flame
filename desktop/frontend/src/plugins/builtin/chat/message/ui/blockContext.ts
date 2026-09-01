import type { MarkdownReveal } from "./markdown/streamReveal";

export interface BlockCtx {
  onSelectTool: (id: string) => void;
  expandedIds: Set<string>;
  onToggleExpand: (id: string) => void;
  textReveal: MarkdownReveal;
}
