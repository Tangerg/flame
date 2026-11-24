import { useActiveWorkspaceViewId } from "@/features/workspace/public/navigation";
import { useProjectsAccess } from "@/features/projects/application/projectsContext";
import { useProjectHomeTarget } from "@/features/projects/application/projectHome";
import { PROJECT_HOME_VIEW } from "@/features/projects/application/projectsAccess";

export function useShownProjectHome(): string | undefined {
  const access = useProjectsAccess();
  const target = useProjectHomeTarget(access.home);
  return useActiveWorkspaceViewId() === PROJECT_HOME_VIEW ? target : undefined;
}
