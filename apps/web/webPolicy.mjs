export const WEB_POLICY_FILE = ".vite/flame-web-policy.json";
export const NONCE_PLACEHOLDER = "__FLAME_RESPONSE_NONCE__";
const DOCUMENT_DIRECTIVES = [
  "default-src 'self'",
  `script-src 'nonce-${NONCE_PLACEHOLDER}' 'strict-dynamic'`,
  "style-src 'self' 'unsafe-inline'",
  "img-src 'self' data: blob: http: https:",
  "media-src 'self' data: blob: http: https:",
  "font-src 'self' data:",
  // Runtime endpoints are explicitly selected by the user, independently of
  // the application's delivery origin and the native IPC transport.
  "connect-src 'self' http: https: ws: wss:",
  "worker-src 'self' blob:",
  "frame-src 'self' https://www.youtube-nocookie.com",
  "object-src 'none'",
  "base-uri 'none'",
  "form-action 'none'",
];
export const DOCUMENT_POLICY = [...DOCUMENT_DIRECTIVES, "frame-ancestors 'none'"].join("; ");
export const EMBEDDED_DOCUMENT_POLICY = [...DOCUMENT_DIRECTIVES, "frame-ancestors 'self'"].join(
  "; ",
);

export function documentResponse(policy, html, nonce, kind) {
  if (kind !== "main" && kind !== "same-origin") throw new Error("unknown web document kind");
  if (!html.includes(policy.noncePlaceholder))
    throw new Error("web document has no response nonce placeholder");
  return {
    body: html.replaceAll(policy.noncePlaceholder, nonce),
    headers: {
      "Content-Type": "text/html; charset=utf-8",
      "Cache-Control": "no-store",
      "Content-Security-Policy": (kind === "main"
        ? policy.documentPolicy
        : policy.embeddedPolicy
      ).replaceAll(policy.noncePlaceholder, nonce),
      "X-Frame-Options": kind === "main" ? "DENY" : "SAMEORIGIN",
      "X-Content-Type-Options": "nosniff",
    },
  };
}
