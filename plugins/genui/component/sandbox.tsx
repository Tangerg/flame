const policy = [
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

function attribute(value: string): string {
  return value
    .replaceAll("&", "&amp;")
    .replaceAll('"', "&quot;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;");
}

function page(body: string): string {
  return (
    '<!doctype html><html><head><meta charset="utf-8">' +
    '<meta http-equiv="Content-Security-Policy" content="' +
    attribute(policy) +
    '"><meta name="viewport" content="width=device-width,initial-scale=1">' +
    "<style>:root{color-scheme:light dark}html,body{margin:0;height:100%}body{font:14px/1.5 system-ui,sans-serif}iframe{display:block;width:100%;height:100%;border:0}</style>" +
    "</head><body>" +
    body +
    "</body></html>"
  );
}

export function sandboxDocument(title: string, html: string): string {
  // The trusted outer document also constrains navigation of the untrusted
  // frame. A CSP inside the authored document alone cannot block self-navigation.
  return page(
    '<iframe sandbox="allow-scripts" referrerpolicy="no-referrer" title="' +
      attribute(title) +
      '" srcdoc="' +
      attribute(page(html)) +
      '"></iframe>',
  );
}

export function SandboxView({
  title,
  html,
  height = 360,
}: {
  title: string;
  html: string;
  height?: number;
}) {
  return (
    <iframe
      title={title}
      srcDoc={sandboxDocument(title, html)}
      sandbox="allow-scripts"
      referrerPolicy="no-referrer"
      allow="camera 'none'; microphone 'none'; geolocation 'none'; clipboard-read 'none'; clipboard-write 'none'"
      loading="lazy"
      style={{ width: "100%", height, border: 0 }}
    />
  );
}
