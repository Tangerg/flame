export const WEB_POLICY_FILE = ".vite/flame-web-policy.json";
export const NONCE_PLACEHOLDER = "__FLAME_RESPONSE_NONCE__";
export const SANDBOX_FILE = "genui-sandbox.html";

// The opaque program response deliberately permits authored inline programs.
// Its policy must not be inherited from the application's trusted script realm.
export const SANDBOX_POLICY = [
  "default-src 'none'",
  "script-src 'unsafe-inline'",
  "style-src 'unsafe-inline'",
  "img-src data: blob:",
  "media-src data: blob:",
  "font-src data:",
  "connect-src 'none'",
  "frame-src 'none'",
  "object-src 'none'",
  "base-uri 'none'",
  "form-action 'none'",
].join("; ");

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
