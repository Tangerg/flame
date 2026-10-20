import { useMemo } from "react";
import { useWorkspaceViews } from "@/features/workspace/public/views";
import {
  groupContextDockDestinations,
  resolveContextDockItems,
  type ContextDockDestinationGroup,
} from "@/features/workspace/application/contextDockDestinationGroups";
export function useContextDockCatalog(): ContextDockDestinationGroup[] {
  const views = useWorkspaceViews();
  return useMemo(() => groupContextDockDestinations(resolveContextDockItems(views)), [views]);
}
