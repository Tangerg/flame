export function normalizeRuntimeEndpoint(input: string): string | null {
  const raw = input.trim();
  // WHATWG URLs repair missing authorities and discard empty credentials before
  // exposing their components. Check the supplied authority before normalization.
  const authority = /^https?:\/\/([^/?#\\]+)/i.exec(raw)?.[1];
  if (!authority || authority.includes("@") || raw.includes("\\") || /[\t\r\n]/.test(raw))
    return null;
  let url: URL;
  try {
    url = new URL(raw);
  } catch {
    return null;
  }
  if (
    (url.protocol !== "http:" && url.protocol !== "https:") ||
    url.port === "0" ||
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

export function endpointAcceptsBearer(endpoint: string): boolean {
  const url = new URL(endpoint);
  return (
    url.protocol === "https:" ||
    url.hostname === "localhost" ||
    url.hostname === "[::1]" ||
    /^127(?:\.\d+){3}$/.test(url.hostname)
  );
}
