import type { MarkdownReveal } from "./markdown/MarkdownMessage";

/**
 * The transcript's shared controls and presentation preferences, holding NO session data.
 * One instance is handed to every turn, so any field here is compared against every turn's
 * memo — one that changes mid-run re-renders the whole transcript per token. Session facts
 * travel per turn as `TurnFacts`, which is why the two stay separate parameters.
 */
export interface BlockCtx {
  onSelectTool: (id: string) => void;
  expandedIds: Set<string>;
  onToggleExpand: (id: string) => void;
  /** Mutually exclusive text presentation policy for every block in this turn. */
  textReveal: MarkdownReveal;
}
