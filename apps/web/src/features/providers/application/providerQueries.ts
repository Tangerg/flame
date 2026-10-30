import { createDataQuery } from "@/extensions/dataQuery";
import { SelectableModel } from "@/features/providers/application/selectableModel";
import { type ProviderCredentialSummary } from "@/features/providers/application/providerSetup";
export { SelectableModel } from "@/features/providers/application/selectableModel";

export const PROVIDERS_KEY = "providers";
export const MODELS_KEY = "models";
export const useModels = createDataQuery<SelectableModel[]>(MODELS_KEY);
export const useProviders = createDataQuery<ProviderCredentialSummary[]>(PROVIDERS_KEY);
