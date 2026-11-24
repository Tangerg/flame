import { type Contributor } from "@/extensions/definePlugin";
import { DATA_PROVIDER } from "@/extensions/dataProvider";
import { type FlameClient, type Project } from "@flame/runtime-client";
import {
  PROJECTS_KEY,
  commitProjectSummary,
  type ProjectSummary,
} from "@/features/projects/application/projectQueries";
import { type ProjectCommands } from "@/features/projects/application/projectCommands";

export function toProjectSummary(project: Project): ProjectSummary {
  return {
    id: project.id,
    name: project.name,
    instructions: project.instructions ?? "",
    archived: project.archived ?? false,
    revision: project.revision,
    time: project.updatedAt,
  };
}

export function registerProjectDataProviders(
  ctx: Contributor,
  runtimeClient: () => FlameClient,
): void {
  ctx.add(DATA_PROVIDER, {
    key: PROJECTS_KEY,
    fetcher: async (_params, signal) => {
      const client = runtimeClient();
      const [active, archived] = await Promise.all([
        client.projects.list(undefined, signal).autoPagingToArray(),
        client.projects.list({ archived: true }, signal).autoPagingToArray(),
      ]);
      return [...active, ...archived].map(toProjectSummary);
    },
  });
}

export function createRuntimeProjectCommands(runtimeClient: () => FlameClient): ProjectCommands {
  const settle = async (pending: Promise<Project>) => {
    const project = toProjectSummary(await pending);
    commitProjectSummary(project);
    return project;
  };
  return {
    create: (name) => settle(runtimeClient().projects.create({ name })),
    update: (project, patch) =>
      settle(
        runtimeClient().projects.update({
          projectId: project.id,
          expectedRevision: project.revision,
          ...patch,
        }),
      ),
  };
}
