export function cappedExponentialBackoffMs(
  exponent: number,
  baseMs: number,
  capMs: number,
): number {
  return Math.min(baseMs * 2 ** exponent, capMs);
}
