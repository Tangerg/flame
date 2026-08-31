// Tab badges load EAGERLY while the view bodies they belong to do not: a badge renders in
// the tab strip, which sits outside the view's Suspense boundary. Lazy badges would suspend
// the strip itself and flash every tab whenever one view's chunk resolved.

import { usePendingWork } from "@/plugins/builtin/agent/public/hitl";
import { useActiveSessionWorkspace } from "@/plugins/builtin/agent/public/session";
import { usePlanView } from "@/plugins/builtin/workspace/application/planViewModel";
import {
  useWorkspaceCapability,
  useWorkspaceFileChanges,
} from "@/plugins/builtin/workspace/public/queries";

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
  const gitEnabled = useWorkspaceCapability("git");
  const workspace = useActiveSessionWorkspace();
  const { data: files } = useWorkspaceFileChanges(
    gitEnabled && workspace.status === "ready" ? { cwd: workspace.cwd } : undefined,
  );
  if (!files || files.length === 0) return null;
  return String(files.length);
}
