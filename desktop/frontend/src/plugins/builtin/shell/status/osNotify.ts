// The OS notification, distinct from lib/notify's in-app toast. The CALLER owns the focus
// gate — fire only when the window is blurred or hidden, never while the user is watching
// the run. Permission is primed at load, while the window is focused so the prompt is
// allowed at all; unsupported or denied is a no-op.

// Call
// it early (plugin setup, app focused) so the browser allows the prompt — a
// request issued from an unfocused window is often silently dropped.
export function ensureOsNotifyPermission(): void {
  if (typeof Notification === "undefined") return;
  if (Notification.permission === "default") void Notification.requestPermission();
}

interface OsNotifyOptions {
  body?: string;
  // tag coalesces repeats: a second notification with the same tag replaces the
  // first rather than stacking (e.g. per-session, so a busy session pings once).
  tag?: string;
}

export function osNotify(title: string, opts?: OsNotifyOptions): void {
  if (typeof Notification === "undefined" || Notification.permission !== "granted") return;
  try {
    const n = new Notification(title, { body: opts?.body, tag: opts?.tag });
    n.onclick = () => {
      window.focus();
      n.close();
    };
  } catch {
    // Some webviews reject Notification construction without a service worker —
    // there's nothing actionable, so skip silently.
  }
}
