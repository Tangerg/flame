export function failureDiagnostics(scope: string, error: unknown): string {
  const detail =
    error instanceof Error
      ? [`${error.name}: ${error.message}`, error.stack].filter(Boolean).join("\n")
      : String(error);
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
