export function normalizeRuntimeEndpoint(input: string): string | null {
  let url: URL;
  try {
    url = new URL(input.trim());
  } catch {
    return null;
  }
  if (
    (url.protocol !== "http:" && url.protocol !== "https:") ||
    url.username ||
    url.password ||
    url.href.includes("?") ||
    url.href.includes("#")
  )
    return null;
  return url.href.replace(/\/+$/, "");
}

export function requireRuntimeEndpoint(input: string): string {
  const endpoint = normalizeRuntimeEndpoint(input);
  if (endpoint === null)
    throw new TypeError("Runtime address must be HTTP(S) without credentials, query, or fragment");
  return endpoint;
}
