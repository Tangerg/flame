import { type ProjectSummary } from "@/features/projects/application/projectQueries";

export interface ProjectPatch {
  name?: string;
  instructions?: string;
  archived?: boolean;
}

export interface ProjectCommands {
  create(name: string): Promise<ProjectSummary>;
  update(project: ProjectSummary, patch: ProjectPatch): Promise<ProjectSummary>;
}
