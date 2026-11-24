import { type FlameClient } from "@flame/runtime-client";
import { type WorkspaceAccess } from "@/features/workspace/public/workspace";
import { openWorkspaceView } from "@/features/workspace/public/navigation";
import { createRuntimeProjectCommands } from "@/features/projects/adapters/runtimeProjects";
import { createProjectHomeTarget } from "@/features/projects/application/projectHome";
import {
  PROJECT_HOME_VIEW,
  type ProjectsAccess,
} from "@/features/projects/application/projectsAccess";

export function createProjects(
  runtimeClient: () => FlameClient,
  workspaceAccess: WorkspaceAccess,
): { access: ProjectsAccess } {
  const home = createProjectHomeTarget();
  return {
    access: {
      commands: createRuntimeProjectCommands(runtimeClient),
      home,
      openHome(projectId) {
        home.show(projectId);
        openWorkspaceView(workspaceAccess, PROJECT_HOME_VIEW);
      },
    },
  };
}
