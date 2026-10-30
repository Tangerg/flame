import { createStore } from "zustand/vanilla";
import { ProviderCredentialsDraft } from "@/features/providers/application/providerDraft";
import { type ProviderMutationAccess } from "@/features/providers/application/providerMutationOwner";

export interface ProviderDrafts {
  readonly generation: bigint;
  readonly drafts: ReadonlyMap<string, ProviderCredentialsDraft>;
  readonly change: (
    generation: bigint,
    provider: { id: string; baseUrl?: string },
    next: (draft: ProviderCredentialsDraft) => ProviderCredentialsDraft,
  ) => void;
}

export function createProviderDrafts(mutations: ProviderMutationAccess) {
  const empty = () => ({
    generation: mutations.materialGeneration(),
    drafts: new Map<string, ProviderCredentialsDraft>(),
  });
  const store = createStore<ProviderDrafts>()((set) => ({
    ...empty(),
    change(generation, provider, next) {
      mutations.assertMaterialGeneration(generation);
      set((state) => {
        const drafts =
          state.generation === generation
            ? new Map(state.drafts)
            : new Map<string, ProviderCredentialsDraft>();
        const draft = next(drafts.get(provider.id) ?? ProviderCredentialsDraft.initial(provider));
        if (draft.dirty(provider)) drafts.set(provider.id, draft);
        else drafts.delete(provider.id);
        return { generation, drafts };
      });
    },
  }));
  const dispose = mutations.subscribeMaterialGeneration(() => store.setState(empty()));
  return { store, dispose };
}
