import { Button } from "@/ui";
import { cn } from "@/lib/classNames";
import { useT } from "@/lib/i18n";
import { useActiveSessionWorkspace } from "@/plugins/builtin/agent/public/session";
import { openDiffViewInDock } from "@/plugins/builtin/workspace/public/deeplinks";
import {
  useWorkspaceCapability,
  useWorkspaceFileChanges,
} from "@/plugins/builtin/workspace/public/queries";

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
      press={false}
      aria-label={t("workspace.view.title.diff")}
      onClick={openDiffViewInDock}
      className={cn("gap-1.5 px-1.5 font-mono text-ui-sm", className)}
    >
      <span className="text-success">+{totals.added}</span>
      <span className="text-negative">−{totals.removed}</span>
    </Button>
  );
}
