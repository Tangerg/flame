export function failureText(
  error: unknown,
  fallback = "The operation could not be completed.",
): string {
  return error instanceof Error ? error.message : fallback;
}
