import { wasGenerationRetired } from "@/lib/asyncOwnership";
import { useCallback, useSyncExternalStore } from "react";
import type { ProviderTestOutcome } from "./ports/providerGateway";
import { t } from "@/lib/i18n";
import {
  type ProviderConfiguration,
  providerRoleIsAvailable,
  useEmbeddingRole,
  useModels,
  useProviders,
  useUtilityRole,
} from "./providerQueries";
import type { ProviderRole } from "./providerModels";
import type { ProviderUpdate } from "./ports/providerGateway";
import { ProviderMutationOwner } from "./providerMutationOwner";

export type { ProviderConfiguration };

export function useProviderConfigs() {
  return useProviders();
}

export function useProviderMutationMaterialGeneration(): bigint {
  return useSyncExternalStore(
    ProviderMutationOwner.subscribeMaterialGeneration,
    ProviderMutationOwner.materialGeneration,
    ProviderMutationOwner.materialGeneration,
  );
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
  const selected =
    role?.provider && role.model
      ? (modelOptions.find(
          (model) => model.provider === role.provider && model.id === role.model,
        ) ?? null)
      : null;
  return {
    role,
    modelOptions,
    selected,
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

export async function updateProvider(input: ProviderUpdate): Promise<ProviderConfiguration> {
  return ProviderMutationOwner.current().updateProvider(input);
}

export function useUpdateProvider(): (input: ProviderUpdate) => Promise<ProviderConfiguration> {
  return useCallback((input) => {
    return updateProvider(input);
  }, []);
}

export async function setUtilityRole(role: ProviderRole): Promise<ProviderTestOutcome> {
  const owner = ProviderMutationOwner.current();
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

export async function setEmbeddingRole(role: ProviderRole): Promise<ProviderTestOutcome> {
  const owner = ProviderMutationOwner.current();
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
  return useCallback(async (provider) => {
    const res = await ProviderMutationOwner.current().testProvider(provider);
    return {
      ok: res.ok,
      error: res.ok ? undefined : (res.error ?? t("providers.error.test")),
    };
  }, []);
}
