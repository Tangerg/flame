const QUOTE = '"';
const BACKSLASH = String.fromCharCode(92);

export function formatFileReference(path: string): string {
  if (!/\s/.test(path) && !path.startsWith('"')) return `@${path}`;
  const escaped = path.replace(/[\u0022\\]/g, (char) => BACKSLASH + char);
  return ["@", QUOTE, escaped, QUOTE].join("");
}
