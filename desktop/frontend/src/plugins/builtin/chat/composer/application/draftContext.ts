// DERIVED, never stored: the draft text is the single source of what is attached, so a chip
// row computed from it cannot disagree with the message that gets sent.

/** The SAME rule `activeMention` applies, so a chip appears for exactly what the picker
 *  would have completed and `user@host` is not a file. */
const MENTION = /(^|\s)@(\S+)/g;

export interface DraftMention {
  path: string;
  /** Removing a chip removes THIS occurrence: the same file mentioned twice is two chips,
   *  and closing one must not close the other. */
  start: number;
  end: number;
}

export function draftMentions(value: string): DraftMention[] {
  const out: DraftMention[] = [];
  MENTION.lastIndex = 0;
  for (let match = MENTION.exec(value); match !== null; match = MENTION.exec(value)) {
    const lead = match[1]?.length ?? 0;
    const path = match[2] ?? "";
    if (path === "") continue;
    const start = match.index + lead;
    out.push({ path, start, end: start + 1 + path.length });
  }
  return out;
}

// Collapses the seam to one space: removing the token alone leaves a double space that the
// next thing typed then sits after.
export function removeMention(value: string, mention: DraftMention): string {
  const before = value.slice(0, mention.start);
  const after = value.slice(mention.end);
  if (before === "" || after === "") return (before + after).trim();
  return `${before.replace(/\s+$/, "")} ${after.replace(/^\s+/, "")}`;
}
