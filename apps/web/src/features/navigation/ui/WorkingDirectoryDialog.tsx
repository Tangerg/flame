import { workingDirectoryPicker } from "@/features/navigation/application/ports/workingDirectoryPicker";
import { useT } from "@/lib/i18n/index";
import { InputDialog } from "@/ui/index";
import { useCreateSession } from "@/features/agent/public/session";
import { focusComposer } from "@/features/composer/public/focus";
import { runtimeCommandsAvailable } from "@/features/connection/public/serviceStatus";
import {
  updateWorkingDirectorySelection,
  useWorkingDirectorySelection,
} from "@/features/navigation/adapters/workingDirectorySelection";
export function WorkingDirectoryDialog() {
  const t = useT();
  const create = useCreateSession();
  const selection = useWorkingDirectorySelection((state) => state.selection);
  if (!selection) return null;
  const browse = async () => {
    const current = useWorkingDirectorySelection.getState().selection;
    if (current?.owner !== selection.owner || current.busy) return;
    updateWorkingDirectorySelection(selection, { busy: true, error: null });
    try {
      await workingDirectoryPicker().browse(selection.owner);
    } catch (error) {
      const title = t("session.error.chooseWorkingDirectory");
      const detail = error instanceof Error ? error.message : "";
      updateWorkingDirectorySelection(selection, {
        error: detail ? `${title}\n${detail}` : title,
      });
    } finally {
      updateWorkingDirectorySelection(selection, { busy: false });
    }
  };
  const submit = async () => {
    const current = useWorkingDirectorySelection.getState().selection;
    if (!runtimeCommandsAvailable() || current?.owner !== selection.owner || current.busy) return;
    updateWorkingDirectorySelection(selection, { busy: true, error: null });
    const session = await create({ cwd: selection.path.trim() });
    if (session) {
      if (updateWorkingDirectorySelection(selection, null)) focusComposer();
    } else
      updateWorkingDirectorySelection(selection, { busy: false, error: t("session.error.create") });
  };
  return (
    <InputDialog
      title={t("session.directory.title")}
      description={t("session.directory.description", { endpoint: selection.endpoint })}
      label={t("session.directory.path")}
      value={selection.path}
      busy={selection.busy}
      error={selection.error}
      onChange={(path) => updateWorkingDirectorySelection(selection, { path, error: null })}
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
