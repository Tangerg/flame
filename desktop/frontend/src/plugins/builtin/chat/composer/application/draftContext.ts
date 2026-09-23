const MENTION = /(^|\s)@(\S+)/g;

export interface DraftMention {
  path: string;
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

export function removeMention(value: string, mention: DraftMention): string {
  const before = value.slice(0, mention.start);
  const after = value.slice(mention.end);
  if (before === "" || after === "") return (before + after).trim();
  return `${before.replace(/\s+$/, "")} ${after.replace(/^\s+/, "")}`;
}
