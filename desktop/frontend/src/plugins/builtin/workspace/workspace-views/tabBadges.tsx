import { usePendingWork } from "@/plugins/builtin/agent/public/hitl";
import { usePlanView } from "@/plugins/builtin/workspace/application/planViewModel";
import { useWorkingTreeChanges } from "@/plugins/builtin/workspace/application/workingTreeChanges";

export function PlanTabBadge() {
  const view = usePlanView();
  if (view.total === 0) return null;
  return `${view.done}/${view.total}`;
}

export function InboxBadge() {
  const { data } = usePendingWork();
  const count = data?.length ?? 0;
  if (count === 0) return null;
  return <>{count}</>;
}

export function DiffTabBadge() {
  const changes = useWorkingTreeChanges();
  return changes ? String(changes.files) : null;
}
