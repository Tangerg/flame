import { createDataQuery } from "@/extensions/dataQuery";
import {
  ProviderConfiguration,
  type ProviderRole,
} from "@/features/providers/application/providerModels";
import { SelectableModel } from "@/features/providers/application/selectableModel";
export {
  ProviderConfiguration,
  type ProviderRole,
} from "@/features/providers/application/providerModels";
export { SelectableModel } from "@/features/providers/application/selectableModel";

export function providerRoleIsAvailable(
  role: ProviderRole | undefined,
  providers: readonly ProviderConfiguration[],
): boolean {
  return Boolean(
    role?.provider &&
    role.model &&
    providers.some((provider) => provider.id === role.provider && provider.configured),
  );
}

export const PROVIDERS_KEY = "providers";

export const MODELS_KEY = "models";

export const UTILITY_ROLE_KEY = "utility-role";

export const EMBEDDING_ROLE_KEY = "embedding-role";

export const useModels = createDataQuery<SelectableModel[]>(MODELS_KEY);

export const useProviders = createDataQuery<ProviderConfiguration[]>(PROVIDERS_KEY);

export const useUtilityRole = createDataQuery<ProviderRole>(UTILITY_ROLE_KEY);

export const useEmbeddingRole = createDataQuery<ProviderRole>(EMBEDDING_ROLE_KEY);
