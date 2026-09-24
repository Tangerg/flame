export function failureDiagnostics(scope: string, error: unknown): string {
  const detail =
    error instanceof Error ? (error.stack ?? `${error.name}: ${error.message}`) : String(error);
  return [
    `scope: ${scope}`,
    `time: ${new Date().toISOString()}`,
    `agent: ${navigator.userAgent}`,
    detail,
  ].join("\n");
}

export function failureMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}
