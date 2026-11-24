import { lazy } from "react";
import { type FlameClient } from "@flame/runtime-client";
import { definePlugin } from "@/extensions/definePlugin";
import { WORKSPACE_VIEW } from "@/features/workspace/public/views";
import { registerProjectDataProviders } from "@/features/projects/adapters/runtimeProjects";
import { PROJECT_HOME_VIEW } from "@/features/projects/application/projectsAccess";

export { createProjects } from "@/features/projects/adapters/projects";

export function createProjectsPlugin(runtimeClient: () => FlameClient) {
  return definePlugin({
    name: "flame.builtin.projects",
    setup(ctx) {
      registerProjectDataProviders(ctx.contributions, runtimeClient);
      ctx.contributions.add(WORKSPACE_VIEW, {
        id: PROJECT_HOME_VIEW,
        title: "projects.home.title",
        order: 30,
        component: lazy(() =>
          import("@/features/projects/ui/ProjectHome").then((m) => ({ default: m.ProjectHome })),
        ),
      });
    },
  });
}
