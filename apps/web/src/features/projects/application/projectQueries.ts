import { createDataQuery } from "@/extensions/dataQuery";
import { replaceCachedRead } from "@/lib/queryClient";

export interface ProjectSummary {
  id: string;
  name: string;
  instructions: string;
  archived: boolean;
  revision: number;
  time: string;
}

export const PROJECTS_KEY = "projects";

export const useProjects = createDataQuery<ProjectSummary[]>(PROJECTS_KEY);

export function invalidateProjects(): Promise<void> {
  return replaceCachedRead({ queryKey: [PROJECTS_KEY] });
}
