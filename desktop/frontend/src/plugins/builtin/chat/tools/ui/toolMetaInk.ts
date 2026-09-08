import type { ToolMetaTone } from "@/plugins/builtin/agent/public/messagePresentation";
import { vocab } from "@/ui";

/**
 * What a tool row's trailing meta is written in.
 *
 * `ToolMetaTone` is a closed pair for a good reason, stated where it is declared — but the two
 * surfaces that render it had each mapped it themselves, and disagreed: a card wrote `muted` as
 * `fgMuted` and a group member wrote the same tone as `fgFaint`. Neither was wrong. The meta
 * sits at the QUIET step of the row it trails, and the two rows sit at different steps — a
 * card's row is full ink with a muted trailing slot, a group member's whole row is already
 * muted — so "quiet" resolves to different values.
 *
 * Written out per surface because this ink ladder is discrete: there is no relative operator
 * for "one step below whatever this row is". What the table buys is that the disagreement is
 * now a statement instead of an accident.
 */
export const toolMetaInk = {
  card: { muted: vocab.muted, negative: vocab.negative },
  member: { muted: vocab.faint, negative: vocab.negative },
} as const satisfies Record<"card" | "member", Record<ToolMetaTone, unknown>>;
