import { useComposer } from "@/features/composer/public/composer";
import { useRuntime } from "@/features/connection/public/runtime";
import { useWorkingDirectory } from "@/features/navigation/public/workingDirectory";
import * as stylex from "@stylexjs/stylex";
import { ScrollRegion } from "@flame/ui";
import { color, face, space, type, weight } from "@flame/ui/tokens.stylex";

import { useT } from "@/lib/i18n/index";
import { InputDialog } from "@/ui/index";
import { useCreateSession } from "@/features/agent/public/session";
import { focusComposer } from "@/features/composer/public/focus";
import { runtimeCommandsAvailable } from "@/features/connection/public/serviceStatus";

const styles = stylex.create({
  title: { display: "grid", gap: space.s0_5, minWidth: 0 },
  endpoint: {
    minWidth: 0,
    maxHeight: "3lh",
    paddingBlock: 0,
    color: color.fgMuted,
    fontWeight: weight.regular,
    overflowWrap: "anywhere",
    textWrap: "wrap",
    userSelect: "text",
  },
});

export function WorkingDirectoryDialog() {
  const composerAccess = useComposer();
  const runtimeAccess = useRuntime();
  const directoryAccess = useWorkingDirectory();

  const t = useT();
  const create = useCreateSession();
  const selection = directoryAccess.useSelection();
  if (!selection) return null;
  const browse = async () => {
    const current = directoryAccess.selection();
    if (current?.owner !== selection.owner || current.busy) return;
    directoryAccess.update(selection, { busy: true, error: null });
    try {
      await directoryAccess.browse(selection.owner);
    } catch (error) {
      const title = t("session.error.chooseWorkingDirectory");
      const detail = error instanceof Error ? error.message : "";
      directoryAccess.update(selection, {
        error: detail ? `${title}\n${detail}` : title,
      });
    } finally {
      directoryAccess.update(selection, { busy: false });
    }
  };
  const submit = async () => {
    const current = directoryAccess.selection();
    if (
      !runtimeCommandsAvailable(runtimeAccess) ||
      current?.owner !== selection.owner ||
      current.busy
    )
      return;
    directoryAccess.update(selection, { busy: true, error: null });
    const session = await create({ cwd: selection.path.trim() });
    if (session) {
      if (directoryAccess.update(selection, null)) focusComposer(composerAccess);
    } else directoryAccess.update(selection, { busy: false, error: t("session.error.create") });
  };
  return (
    <InputDialog
      title={
        // The explicit space keeps the accessible name readable; the grid ignores it visually.
        <span {...stylex.props(styles.title)}>
          <span>{t("session.directory.title")}</span>{" "}
          <ScrollRegion
            as="span"
            label={selection.endpoint}
            styles={[styles.endpoint, type.uiSm, face.mono]}
          >
            {selection.endpoint}
          </ScrollRegion>
        </span>
      }
      label={t("session.directory.path")}
      value={selection.path}
      busy={selection.busy}
      error={selection.error}
      onChange={(path) => directoryAccess.update(selection, { path, error: null })}
      onConfirm={() => void submit()}
      onClose={() => directoryAccess.update(selection, null)}
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
