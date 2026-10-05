import { create } from "zustand";
import { createDataQuery } from "@/plugins/sdk";
import { createSingletonPort } from "@/lib/ports/singletonPort";
import type { Methods } from "@flame/runtime-contract/client/methods";
import type { PluginInstallation } from "@flame/runtime-contract/wire";

export const PACKAGES_KEY = "runtime-packages";
export const usePackages = createDataQuery<PluginInstallation[]>(PACKAGES_KEY);
export const packageOperations = createSingletonPort<Methods["plugins"] & { signal: AbortSignal }>(
  "Package operations are not installed",
);

interface PackageRealizationFailure {
  readonly reason: string;
  retry(): void;
}

export const usePackageRealization = create<{ failure: PackageRealizationFailure | null }>(() => ({
  failure: null,
}));
