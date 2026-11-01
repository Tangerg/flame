import type { PluginInstallation } from "@flame/runtime-client/wire";

export function backendAvailable(installation: PluginInstallation, server: string) {
  return (
    installation.enabled &&
    !installation.disabledServers.includes(server) &&
    installation.realization.type === "available" &&
    !installation.realization.unavailableBackends?.includes(server)
  );
}

export function workflowAuthors(installations: readonly PluginInstallation[]) {
  return installations.flatMap((installation) =>
    installation.selected.contributions
      .filter((c) => backendAvailable(installation, c.server))
      .map((c) => ({
        source: {
          installationId: installation.id,
          release: installation.selected.digest,
          contribution: c.id,
        },
        roles: c.selectionRoles ?? [],
      })),
  );
}
