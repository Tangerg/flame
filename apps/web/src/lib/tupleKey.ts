export function tupleKey(...parts: readonly string[]): string {
  return JSON.stringify(parts);
}
