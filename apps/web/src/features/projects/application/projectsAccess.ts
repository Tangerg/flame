import { type ProjectCommands } from "@/features/projects/application/projectCommands";
import { type ProjectHomeTarget } from "@/features/projects/application/projectHome";

export interface ProjectsAccess {
  readonly commands: ProjectCommands;
  readonly home: ProjectHomeTarget;
  openHome(projectId: string): void;
}

export const PROJECT_HOME_VIEW = "project";
