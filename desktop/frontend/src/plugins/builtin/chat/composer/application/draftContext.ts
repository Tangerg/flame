const REFERENCE = /(^|\s)@(?:"((?:[^"\\]|\\.)+)"|(\S+))/g;

const QUOTE = '"';
const BACKSLASH = String.fromCharCode(92);

export interface DraftMention {
  path: string;
  start: number;
  end: number;
}

export function formatFileReference(path: string): string {
  if (!/\s/.test(path) && !path.startsWith('"')) return `@${path}`;
  const escaped = path.replace(/[\u0022\\]/g, (char) => BACKSLASH + char);
  return ["@", QUOTE, escaped, QUOTE].join("");
}

export function draftMentions(value: string, knownPaths: ReadonlySet<string>): DraftMention[] {
  const out: DraftMention[] = [];
  REFERENCE.lastIndex = 0;
  for (let match = REFERENCE.exec(value); match !== null; match = REFERENCE.exec(value)) {
    const lead = match[1]?.length ?? 0;
    const path = match[2] !== undefined ? match[2].replace(/\\(.)/g, "$1") : (match[3] ?? "");
    if (!knownPaths.has(path)) continue;
    const start = match.index + lead;
    out.push({ path, start, end: match.index + match[0].length });
  }
  return out;
}

export function removeMention(value: string, mention: DraftMention): string {
  const before = value.slice(0, mention.start);
  const after = value.slice(mention.end);
  if (before === "" || after === "") return (before + after).trim();
  return `${before.replace(/\s+$/, "")} ${after.replace(/^\s+/, "")}`;
}
