export function failureDiagnostics(scope: string, error: unknown): string {
  return [
    `scope: ${scope}`,
    `time: ${new Date().toISOString()}`,
    `agent: ${navigator.userAgent}`,
    errorDetails(error, new Set()),
  ].join("\n");
}

function errorDetails(error: unknown, seen: Set<Error>): string {
  if (!(error instanceof Error)) return String(error);
  if (seen.has(error)) return "[circular error reference]";
  seen.add(error);
  const parts = [`${error.name}: ${error.message}`, error.stack].filter(Boolean);
  if (error.cause !== undefined) parts.push(`caused by: ${errorDetails(error.cause, seen)}`);
  if (error instanceof AggregateError)
    for (const cause of error.errors) parts.push(errorDetails(cause, seen));
  return parts.join("\n");
}

export function failureMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}
