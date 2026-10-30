import { GenerationRetiredError, wasGenerationRetired } from "@/lib/asyncOwnership";
import { useCallback, useSyncExternalStore } from "react";
import { type ProviderTestOutcome } from "@/features/providers/application/ports/providerGateway";
import { t } from "@/lib/i18n/index";
import { type ProviderConfiguration } from "@/features/providers/application/providerQueries";
import {
  providerRoleIsAvailable,
  useEmbeddingRole,
  useModels,
  useProviders,
  useUtilityRole,
} from "@/features/providers/application/providerQueries";
import { type ProviderRole } from "@/features/providers/application/providerModels";
import { type ProviderUpdate } from "@/features/providers/application/ports/providerGateway";
import { type ProviderCommands } from "@/features/providers/application/providerMutationOwner";
import { useProviderAccess } from "@/features/providers/public/providers";
export type { ProviderConfiguration };

export function useProviderConfigs() {
  return useProviders();
}

export function useProviderMutationMaterialGeneration(): bigint {
  const { mutations } = useProviderAccess();
  return useSyncExternalStore(
    mutations.subscribeMaterialGeneration,
    mutations.materialGeneration,
    mutations.materialGeneration,
  );
}

function useProviderCommandBinding() {
  const { mutations } = useProviderAccess();
  const generation = useProviderMutationMaterialGeneration();
  const captured = mutations.capture();
  return useCallback(() => {
    mutations.assertMaterialGeneration(generation);
    if (!captured) throw new GenerationRetiredError("provider_mutation_generation");
    return captured;
  }, [mutations, generation, captured]);
}

export type RoleConfigState =
  { kind: "loading" } | { kind: "error"; retry: () => void } | { kind: "ready"; stale: boolean };

interface FactQuery {
  data?: unknown;
  isError: boolean;
  refetch: () => unknown;
}

function roleConfigState(queries: readonly FactQuery[]): RoleConfigState {
  const retry = () => {
    for (const query of queries) if (query.isError) void query.refetch();
  };
  if (queries.some((query) => query.data === undefined)) {
    return queries.some((query) => query.isError) ? { kind: "error", retry } : { kind: "loading" };
  }
  return { kind: "ready", stale: queries.some((query) => query.isError) };
}

function useProviderRoleConfig() {
  const utilityRole = useUtilityRole();
  const embeddingRole = useEmbeddingRole();
  const models = useModels();
  const providers = useProviders();
  return { utilityRole, embeddingRole, models, providers };
}

export function useUtilityModelConfig() {
  const { utilityRole, models, providers } = useProviderRoleConfig();
  const role = utilityRole.data;
  const modelOptions = models.data ?? [];
  const providerConfigs = providers.data ?? [];
  return {
    role,
    modelOptions,
    isSet: Boolean(role?.model),
    isAvailable: providerRoleIsAvailable(role, providerConfigs),
    isError: models.isError,
    state: roleConfigState([utilityRole, providers]),
  };
}

export function useEmbeddingModelConfig() {
  const { embeddingRole, providers } = useProviderRoleConfig();
  const role = embeddingRole.data;
  const providerConfigs = providers.data ?? [];
  return {
    role,
    providers: providerConfigs,
    capableProviders: providerConfigs.filter(
      (provider) => provider.embeddingCapable && provider.configured,
    ),
    isSet: Boolean(role?.model),
    isAvailable: providerRoleIsAvailable(role, providerConfigs),
    state: roleConfigState([embeddingRole, providers]),
  };
}

export function useUpdateProvider(): (input: ProviderUpdate) => Promise<ProviderConfiguration> {
  const commands = useProviderCommandBinding();
  return useCallback((input) => commands().updateProvider(input), [commands]);
}

export async function setUtilityRole(
  owner: ProviderCommands,
  role: ProviderRole,
): Promise<ProviderTestOutcome> {
  try {
    await owner.setUtilityRole(role);
    return { ok: true };
  } catch (error) {
    if (wasGenerationRetired(error)) throw error;
    const detail = owner.errorMessage(error);
    return {
      ok: false,
      error: detail ?? (error instanceof Error ? error.message : t("providers.utility.error")),
    };
  }
}

export async function setEmbeddingRole(
  owner: ProviderCommands,
  role: ProviderRole,
): Promise<ProviderTestOutcome> {
  try {
    await owner.setEmbeddingRole(role);
    return { ok: true };
  } catch (error) {
    if (wasGenerationRetired(error)) throw error;
    const detail = owner.errorMessage(error);
    return {
      ok: false,
      error: detail ?? (error instanceof Error ? error.message : t("providers.embedding.error")),
    };
  }
}

export function useTestProvider(): (provider: string) => Promise<ProviderTestOutcome> {
  const commands = useProviderCommandBinding();
  return useCallback(
    async (provider) => {
      const res = await commands().testProvider(provider);
      return {
        ok: res.ok,
        error: res.ok ? undefined : (res.error ?? t("providers.error.test")),
      };
    },
    [commands],
  );
}

export function useSetUtilityRole() {
  const commands = useProviderCommandBinding();
  return useCallback((role: ProviderRole) => setUtilityRole(commands(), role), [commands]);
}

export function useSetEmbeddingRole() {
  const commands = useProviderCommandBinding();
  return useCallback((role: ProviderRole) => setEmbeddingRole(commands(), role), [commands]);
}
