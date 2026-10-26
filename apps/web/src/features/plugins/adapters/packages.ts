import { create } from "zustand";
import { createDataQuery } from "@/extensions/dataQuery";
import { type Methods } from "@flame/runtime-client";
import { type PluginProviderSelection, type PluginInstallation } from "@flame/runtime-client/wire";
export const PACKAGES_KEY = "runtime-packages";

export const usePackages = createDataQuery<PluginInstallation[]>(PACKAGES_KEY);

export const PROVIDERS_KEY = "runtime-plugin-providers";

export const usePluginProviders = createDataQuery<PluginProviderSelection[]>(PROVIDERS_KEY);

export type PackageOperations = Methods["plugins"] & {
  signal: AbortSignal;
  track<P extends Promise<unknown>>(operation: P): P;
};

export interface PackageRealizationFailure {
  readonly reason: string;
  retry(): void;
}

export function createInstalledPackageState() {
  const useState = create<{
    operations: PackageOperations | null;
    failure: PackageRealizationFailure | null;
  }>(() => ({ operations: null, failure: null }));
  return {
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
      useState.setState({ operations: null, failure: null });
    },
  };
}
export type InstalledPackageState = ReturnType<typeof createInstalledPackageState>;
