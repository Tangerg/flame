export function ensureOsNotifyPermission(): void {
  if (typeof Notification === "undefined") return;
  if (Notification.permission === "default") void Notification.requestPermission();
}

interface OsNotifyOptions {
  body?: string;
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
  } catch {}
}
