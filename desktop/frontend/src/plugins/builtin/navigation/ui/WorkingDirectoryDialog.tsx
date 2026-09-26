import { useT } from "@/lib/i18n";
import { InputDialog } from "@/ui";
import { notifyError } from "@/plugins/sdk";
import { useCreateSession } from "@/plugins/builtin/agent/public/session";
import { focusComposer } from "@/plugins/builtin/chat/composer/public/focus";
import { runtimeCommandsAvailable } from "@/plugins/builtin/runtime/public/serviceStatus";
import {
  browseWorkingDirectory,
  updateWorkingDirectorySelection,
  useWorkingDirectorySelection,
} from "../adapters/workingDirectorySelection";

export function WorkingDirectoryDialog() {
  const t = useT();
  const create = useCreateSession();
  const selection = useWorkingDirectorySelection((state) => state.selection);
  if (!selection) return null;
  const browse = async () => {
    const current = useWorkingDirectorySelection.getState().selection;
    if (current?.owner !== selection.owner || current.busy) return;
    updateWorkingDirectorySelection(selection, { busy: true });
    try {
      await browseWorkingDirectory(selection);
    } catch (error) {
      if (useWorkingDirectorySelection.getState().selection?.owner !== selection.owner) return;
      notifyError(t("session.error.chooseWorkingDirectory"), {
        description: error instanceof Error ? error.message : undefined,
        source: "session",
      });
    } finally {
      updateWorkingDirectorySelection(selection, { busy: false });
    }
  };
  const submit = async () => {
    const current = useWorkingDirectorySelection.getState().selection;
    if (!runtimeCommandsAvailable() || current?.owner !== selection.owner || current.busy) return;
    updateWorkingDirectorySelection(selection, { busy: true });
    const session = await create({ cwd: selection.path.trim() });
    if (session) {
      if (updateWorkingDirectorySelection(selection, null)) focusComposer();
    } else updateWorkingDirectorySelection(selection, { busy: false });
  };
  return (
    <InputDialog
      title={t("session.directory.title")}
      description={t("session.directory.description", { endpoint: selection.endpoint })}
      label={t("session.directory.path")}
      value={selection.path}
      busy={selection.busy}
      onChange={(path) => updateWorkingDirectorySelection(selection, { path })}
      onConfirm={() => void submit()}
      onClose={() => updateWorkingDirectorySelection(selection, null)}
      confirmLabel={t("session.directory.create")}
      cancelLabel={t("common.cancel")}
      browse={
        selection.canBrowse
          ? { label: t("session.directory.browse"), run: () => void browse() }
          : undefined
      }
    />
  );
}
