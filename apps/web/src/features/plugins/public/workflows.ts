import { useSyncExternalStore } from "react";
import { useInstalledPlugins } from "@/features/plugins/context";
import {
  GOAL_WORKFLOW,
  type WorkflowAuthor,
} from "@/features/plugins/application/workflowProviders";

export {
  GOAL_WORKFLOW,
  type WorkflowProviders,
  type WorkflowAuthor,
} from "@/features/plugins/application/workflowProviders";

export function useGoalAuthorAvailable(source: WorkflowAuthor | undefined) {
  const { workflows } = useInstalledPlugins();
  return useSyncExternalStore(
    workflows.subscribe,
    () => workflows.admits(GOAL_WORKFLOW, source),
    () => false,
  );
}
