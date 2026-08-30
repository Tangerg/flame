/** Encode an ordered string tuple as one collision-free map key. */
export function tupleKey(...parts: readonly string[]): string {
  return JSON.stringify(parts);
}
