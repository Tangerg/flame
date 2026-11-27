import { type FlameClient } from "@flame/runtime-client";
import { type ProjectCommands } from "@/features/projects/application/projectCommands";
import { type ProjectHomeTarget } from "@/features/projects/application/projectHome";

export interface ProjectsAccess {
  readonly commands: ProjectCommands;
  readonly background: FlameClient["projects"]["background"];
  readonly home: ProjectHomeTarget;
  readPullRequest(
    ref: string,
    signal?: AbortSignal,
  ): Promise<{
    title: string;
    head: string;
    recheck?: string;
    pendingFindings: number;
  }>;
  openHome(projectId: string): void;
}

export const PROJECT_HOME_VIEW = "project";
