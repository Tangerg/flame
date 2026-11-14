import { type WorkspaceAccess } from "@/features/workspace/public/workspace";
import { type NotificationFeed } from "@/features/notifications/public/notifications";

import { t } from "@/lib/i18n/index";
import { failureText } from "@/lib/rpcErrors";

type LocalFileActionResult = { ok: true } | { ok: false; error: string };

async function runLocalFileAction(
  notifications: NotificationFeed,
  action: () => Promise<boolean>,
  failedTitle: string,
  path: string,
): Promise<LocalFileActionResult> {
  let description: string;
  try {
    if (await action()) return { ok: true };
    description = t("file.localUnavailable");
  } catch (error) {
    description = failureText(error, "");
  }
  const title = t(failedTitle, { path });
  const message = description ? `${title}\n${description}` : title;
  notifications.record({ plugin: "file", level: "error", message });
  return { ok: false, error: message };
}

export function openLocalFile(
  notifications: NotificationFeed,
  workspaceAccess: WorkspaceAccess,
  cwd: string,
  path: string,
): Promise<LocalFileActionResult> {
  return runLocalFileAction(
    notifications,
    () => workspaceAccess.local.open(cwd, path),
    "file.openFailed",
    path,
  );
}

export function revealLocalFile(
  notifications: NotificationFeed,
  workspaceAccess: WorkspaceAccess,
  cwd: string,
  path: string,
): Promise<LocalFileActionResult> {
  return runLocalFileAction(
    notifications,
    () => workspaceAccess.local.reveal(cwd, path),
    "file.revealFailed",
    path,
  );
}
