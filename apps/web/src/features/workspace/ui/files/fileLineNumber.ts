export function parseFileLineNumber(value: string): number | null {
  const text = value.trim();
  if (!/^[0-9\uFF10-\uFF19]+$/.test(text)) return null;
  const line = Number(text.normalize("NFKC"));
  return Number.isSafeInteger(line) && line > 0 ? line : null;
}
