import { createProviderDrafts } from "@/features/providers/application/providerDrafts";
import { ProviderMutationOwner } from "@/features/providers/application/providerMutationOwner";
import { type ProviderAccess } from "@/features/providers/application/providerAccess";

export function createProviders() {
  const mutations = new ProviderMutationOwner(null);
  const drafts = createProviderDrafts(mutations);
  const access: ProviderAccess = { mutations, drafts: drafts.store };
  return {
    mutations,
    access,
    dispose() {
      mutations.dispose();
      drafts.dispose();
    },
  };
}

export type Providers = ReturnType<typeof createProviders>;
