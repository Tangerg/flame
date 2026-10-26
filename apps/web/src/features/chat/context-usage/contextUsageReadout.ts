// Past this share of the window, the gauge speaks in how much is used rather than
// how much is left.
const HALFWAY_PERCENT = 50;

export interface ContextUsageReadout {
  ratio: number;
  percent: number;
  remainingPercent: number;
  pastHalfway: boolean;
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
  const percent = Math.round(ratio * 100);
  return {
    ratio,
    percent,
    remainingPercent: 100 - percent,
    pastHalfway: percent >= HALFWAY_PERCENT,
    usedTokens: used,
    windowTokens,
  };
}
