export interface ContextUsageReadout {
  ratio: number;
  percent: number;
  usedTokens: number;
  windowTokens: number;
}

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
