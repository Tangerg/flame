import * as stylex from "@stylexjs/stylex";
import { Button, DiffStat, vocab } from "@flame/ui";
import { useT } from "@/lib/i18n";
import { openDiffViewInDock } from "@/plugins/builtin/workspace/public/deeplinks";
import { useWorkingTreeChanges } from "@/plugins/builtin/workspace/public/queries";

export function HeaderDiffStat({ className }: { className?: string }) {
  const t = useT();
  const changes = useWorkingTreeChanges();
  if (!changes) return null;
  const counted =
    (changes.added !== undefined || changes.removed !== undefined) &&
    (changes.added !== 0 || changes.removed !== 0);

  return (
    <Button
      size="sm"
      aria-label={t("diff.worktree.open", { count: changes.files })}
      title={t("diff.worktree.open", { count: changes.files })}
      onClick={openDiffViewInDock}
      chip
      face="mono"
      className={className}
    >
      {counted ? (
        <DiffStat added={changes.added} removed={changes.removed} />
      ) : (
        <span {...stylex.props(vocab.muted)}>{t("diff.fileCount", { count: changes.files })}</span>
      )}
    </Button>
  );
}
