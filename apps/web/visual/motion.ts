// Reject unknown modes so a review cannot silently exercise a different motion setting.
export function fixtureMotionScale(value: string | null): 0 | 1 {
  if (value === null || value === "off") return 0;
  if (value === "full") return 1;
  throw new Error('visual fixture motion must be "full" or "off", or omitted for off');
}
