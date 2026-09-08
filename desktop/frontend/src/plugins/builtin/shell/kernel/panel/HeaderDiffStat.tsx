import * as stylex from "@stylexjs/stylex";
import { Button } from "@/ui";
import { useT } from "@/lib/i18n";
import { useActiveSessionWorkspace } from "@/plugins/builtin/agent/public/session";
import { openDiffViewInDock } from "@/plugins/builtin/workspace/public/deeplinks";
import {
  useWorkspaceCapability,
  useWorkspaceFileChanges,
} from "@/plugins/builtin/workspace/public/queries";
import { shellStyles as sh } from "../shellStyles";

export function HeaderDiffStat({ className }: { className?: string }) {
  const t = useT();
  const gitEnabled = useWorkspaceCapability("git");
  const workspace = useActiveSessionWorkspace();
  const { data: files } = useWorkspaceFileChanges(
    gitEnabled && workspace.status === "ready" ? { cwd: workspace.cwd } : undefined,
  );

  const totals = (files ?? []).reduce(
    (sum, file) => ({
      added: sum.added + (file.added ?? 0),
      removed: sum.removed + (file.removed ?? 0),
    }),
    { added: 0, removed: 0 },
  );
  if (totals.added === 0 && totals.removed === 0) return null;

  return (
    <Button
      size="sm"
      aria-label={t("workspace.view.title.diff")}
      onClick={openDiffViewInDock}
      chip
      face="mono"
      className={className}
    >
      <span {...stylex.props(sh.success)}>+{totals.added}</span>
      <span {...stylex.props(sh.negative)}>−{totals.removed}</span>
    </Button>
  );
}
