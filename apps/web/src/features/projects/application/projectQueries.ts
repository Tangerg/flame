import { createDataQuery } from "@/extensions/dataQuery";
import { queryClient } from "@/lib/queryClient";

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

// An accepted command reply is newer than any list read still in flight, so it
// replaces that Project and the next change notification rereads the rest.
export function commitProjectSummary(project: ProjectSummary): void {
  void queryClient.cancelQueries({ queryKey: [PROJECTS_KEY] });
  queryClient.setQueryData<ProjectSummary[]>([PROJECTS_KEY], (projects) => [
    project,
    ...(projects ?? []).filter((candidate) => candidate.id !== project.id),
  ]);
}
