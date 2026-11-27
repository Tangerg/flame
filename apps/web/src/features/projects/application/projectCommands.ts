import { type ProjectSummary } from "@/features/projects/application/projectQueries";

export interface ProjectPatch {
  name?: string;
  instructions?: string;
  archived?: boolean;
}

export type ProjectCommandTarget = Pick<ProjectSummary, "id" | "revision">;

export interface ProjectCommands {
  create(name: string): Promise<string>;
  update(project: ProjectCommandTarget, patch: ProjectPatch): Promise<void>;
}
