import { useNotificationStore } from "@/features/notifications/public/notifications";
import { localWorkspace } from "@/features/workspace/application/ports/localWorkspace";
import { t } from "@/lib/i18n/index";
import { failureText } from "@/lib/rpcErrors";

type LocalFileActionResult = { ok: true } | { ok: false; error: string };

async function runLocalFileAction(
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
  useNotificationStore.getState().push({ plugin: "file", level: "error", message });
  return { ok: false, error: message };
}

export function openLocalFile(cwd: string, path: string): Promise<LocalFileActionResult> {
  return runLocalFileAction(() => localWorkspace().open(cwd, path), "file.openFailed", path);
}

export function revealLocalFile(cwd: string, path: string): Promise<LocalFileActionResult> {
  return runLocalFileAction(() => localWorkspace().reveal(cwd, path), "file.revealFailed", path);
}
