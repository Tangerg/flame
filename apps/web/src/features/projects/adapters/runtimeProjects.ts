import { type Contributor } from "@/extensions/definePlugin";
import { DATA_PROVIDER } from "@/extensions/dataProvider";
import { type FlameClient, type Project } from "@flame/runtime-client";
import {
  PROJECTS_KEY,
  invalidateProjects,
  type ProjectSummary,
} from "@/features/projects/application/projectQueries";
import { type ProjectCommands } from "@/features/projects/application/projectCommands";

function toProjectSummary(project: Project): ProjectSummary {
  return {
    id: project.id,
    name: project.name,
    instructions: project.instructions ?? "",
    archived: project.archived ?? false,
    revision: project.revision,
    time: project.updatedAt,
  };
}

export async function readRuntimeProjectSummaries(
  client: Pick<FlameClient, "projects">,
  signal?: AbortSignal,
): Promise<ProjectSummary[]> {
  const projects = await client.projects.list(undefined, signal).autoPagingToArray();
  return projects.map(toProjectSummary);
}

export function registerProjectDataProviders(
  ctx: Contributor,
  runtimeClient: () => FlameClient,
): void {
  ctx.add(DATA_PROVIDER, {
    key: PROJECTS_KEY,
    fetcher: (_params, signal) => readRuntimeProjectSummaries(runtimeClient(), signal),
  });
}

export function createRuntimeProjectCommands(runtimeClient: () => FlameClient): ProjectCommands {
  return {
    async create(name) {
      const { id } = await runtimeClient().projects.create({ name });
      await invalidateProjects();
      return id;
    },
    async update(project, patch) {
      await runtimeClient().projects.update({
        projectId: project.id,
        expectedRevision: project.revision,
        ...patch,
      });
      await invalidateProjects();
    },
  };
}
