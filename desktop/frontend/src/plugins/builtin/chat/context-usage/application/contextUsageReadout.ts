export interface ContextUsageReadout {
  /** 0…1. */
  ratio: number;
  percent: number;
  usedTokens: number;
  windowTokens: number;
}

/**
 * The latest request's prompt footprint against the served model's context window, from
 * `RunProgress.contextTokens` — Session and Run usage totals cannot answer this because
 * they sum multiple model rounds. Null whenever the answer would be a guess: a gauge
 * reading zero claims "empty", which here would be false.
 */
export function contextUsageReadout(
  usedTokens: number | undefined,
  windowTokens: number | undefined,
): ContextUsageReadout | null {
  if (!windowTokens || windowTokens <= 0) return null;
  if (!usedTokens || usedTokens <= 0) return null;
  const used = Math.min(usedTokens, windowTokens);
  const ratio = used / windowTokens;
  return {
    ratio,
    percent: Math.round(ratio * 100),
    usedTokens: used,
    windowTokens,
  };
}
