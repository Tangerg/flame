import { WorkflowProviders } from "../application/workflowProviders";
import { createPresentationPreferences } from "./presentationPreferences";
import { create } from "zustand";
import { createDataQuery } from "@/extensions/dataQuery";
import { type Methods } from "@flame/runtime-client";
import { type PluginProviderSelection, type PluginInstallation } from "@flame/runtime-client/wire";
// Package ids namespace an installation's contributions, themes and diagnostics
// so two packages cannot collide on one extension-point or selection id.
export const PACKAGE_ID_PREFIX = "package:";

export function packageId(installationId: string): string {
  return `${PACKAGE_ID_PREFIX}${installationId}`;
}

export function packageScopedId(installationId: string, localId: string): string {
  return `${packageId(installationId)}:${localId}`;
}

export const INSTALLED_PLUGIN_LOADER = "flame.builtin.installed-plugin-loader";

export const PACKAGES_KEY = "runtime-packages";

export const usePackages = createDataQuery<PluginInstallation[]>(PACKAGES_KEY);

export const PROVIDERS_KEY = "runtime-plugin-providers";

export const usePluginProviders = createDataQuery<PluginProviderSelection[]>(PROVIDERS_KEY);

export type PackageOperations = Methods["plugins"] & {
  signal: AbortSignal;
  track<P extends Promise<unknown>>(operation: (signal: AbortSignal) => P): P;
};

export interface PackageRealizationFailure {
  readonly reason: string;
  readonly retry?: () => void;
}

export function createInstalledPackageState(endpoint: () => string) {
  const useState = create<{
    operations: PackageOperations | null;
    failure: PackageRealizationFailure | null;
  }>(() => ({ operations: null, failure: null }));
  const workflows = new WorkflowProviders();
  return {
    workflows,
    presentationPreferences: createPresentationPreferences(endpoint),
    useOperations: () => useState((state) => state.operations),
    useFailure: () => useState((state) => state.failure),
    captureOperations: () => useState.getState().operations,
    publishOperations(operations: PackageOperations) {
      useState.setState({ operations });
      return () => {
        if (useState.getState().operations === operations) useState.setState({ operations: null });
      };
    },
    failure: () => useState.getState().failure,
    setFailure: (failure: PackageRealizationFailure | null) => useState.setState({ failure }),
    clear() {
      workflows.publish([], []);
      useState.setState({ operations: null, failure: null });
    },
  };
}
export type InstalledPackageState = ReturnType<typeof createInstalledPackageState>;
