export function missingBrowserFeatures(): string[] {
  const missing: string[] = [];
  if (typeof Promise.withResolvers !== "function") missing.push("Promise.withResolvers");
  if (typeof AbortSignal.any !== "function") missing.push("AbortSignal.any");
  if (typeof Map.groupBy !== "function") missing.push("Map.groupBy");
  if (typeof Array.prototype.toSorted !== "function") missing.push("Array.prototype.toSorted");
  if (typeof Array.prototype.toReversed !== "function") missing.push("Array.prototype.toReversed");
  if (typeof HTMLScriptElement.supports !== "function" || !HTMLScriptElement.supports("importmap"))
    missing.push('HTMLScriptElement.supports("importmap")');
  return missing;
}

export async function startBrowserApplication(
  load: () => Promise<unknown>,
): Promise<{ status: "started" } | { status: "unsupported"; missing: string[] }> {
  const missing = missingBrowserFeatures();
  if (missing.length) return { status: "unsupported", missing };
  await load();
  return { status: "started" };
}
