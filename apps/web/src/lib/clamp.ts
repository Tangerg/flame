// clamp constrains a number to an inclusive range. Non-finite input propagates:
// callers that must tolerate garbage guard it at their own boundary, because the
// right substitute (empty, full, previous) is a domain decision, not a math one.
export function clamp(value: number, min: number, max: number): number {
  return Math.max(min, Math.min(max, value));
}

export function clamp01(value: number): number {
  return clamp(value, 0, 1);
}
