import { INSTALLATION_SCOPE, type FlameClient } from "@flame/runtime-client";
import { parseJSON } from "@frame/sdk/json";
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
      background: {
        get: (id, signal) => runtimeClient().projects.background.get(id, signal),
        setPolicy: (params) => runtimeClient().projects.background.setPolicy(params),
      },
      home,
      async readPullRequest(ref, signal) {
        const match = /^([\w.-]+\/[\w.-]+)#([1-9][0-9]*)(?:@([0-9a-f]{40}:[0-9a-f]{40}))?$/u.exec(
          ref,
        );
        if (!match) throw new Error("Invalid pull request reference");
        const client = runtimeClient();
        const found = await client.plugins.list(signal);
        const installations = found.data.filter(
          (value) => value.enabled && value.selected.name === "flame.github",
        );
        if (installations.length !== 1)
          throw new Error("One GitHub Reviews installation is required");
        const installation = installations[0]!;
        const contribution = installation.selected.contributions.find(
          (value) => value.id === "github-operations",
        );
        if (!contribution?.contract) throw new Error("GitHub Reviews is unavailable");
        const response = await client.plugins.call(
          {
            route: {
              installationId: installation.id,
              digest: installation.selected.digest,
              contribution: contribution.id,
              contract: contribution.contract,
              method: "flame.github/review-work.v1",
              target: { scope: INSTALLATION_SCOPE, reference: installation.id },
            },
            input: JSON.stringify({
              repository: match[1],
              number: Number(match[2]),
              revision: match[3],
            }),
          },
          { signal },
        );
        if (response.failure) throw new Error(response.failure.message);
        if (response.output === undefined) throw new Error("GitHub Reviews returned no work");
        const work = parseJSON(response.output) as {
          currentHead: string;
          selected?: { title: string; recheck?: { status: string } };
          needsRecheck: unknown[];
        };
        return {
          title: work.selected?.title ?? ref,
          head: work.currentHead,
          recheck: work.selected?.recheck?.status,
          pendingFindings: work.needsRecheck.length,
        };
      },
      openHome(projectId) {
        home.show(projectId);
        openWorkspaceView(workspaceAccess, PROJECT_HOME_VIEW);
      },
    },
  };
}
