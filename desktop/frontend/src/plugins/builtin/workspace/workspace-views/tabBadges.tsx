import { useWorkingTreeChanges } from "@/plugins/builtin/workspace/application/workingTreeChanges";

export function DiffTabBadge() {
  const changes = useWorkingTreeChanges();
  return changes ? String(changes.files) : null;
}
