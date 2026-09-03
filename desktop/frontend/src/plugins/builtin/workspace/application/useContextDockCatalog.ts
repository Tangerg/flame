import { useMemo } from "react";
import { useWorkspaceViews } from "@/plugins/sdk";
import {
  groupContextDockDestinations,
  resolveContextDockItems,
  type ContextDockDestinationGroup,
} from "./contextDockDestinationGroups";

export function useContextDockCatalog(): ContextDockDestinationGroup[] {
  const views = useWorkspaceViews();
  return useMemo(() => groupContextDockDestinations(resolveContextDockItems(views)), [views]);
}
