// cappedExponentialBackoffMs doubles baseMs for each prior attempt up to capMs.
// The caller supplies the attempt exponent and its own base/cap, so independent
// retry policies share the formula without coupling their tuning.
export function cappedExponentialBackoffMs(
  exponent: number,
  baseMs: number,
  capMs: number,
): number {
  return Math.min(baseMs * 2 ** exponent, capMs);
}
