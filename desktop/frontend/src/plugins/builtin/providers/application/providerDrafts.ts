import { useCallback, useSyncExternalStore } from "react";
import { create } from "zustand";
import { ProviderCredentialsDraft } from "./providerDraft";
import { ProviderMutationOwner } from "./providerMutationOwner";

interface ProviderDrafts {
  generation: bigint;
  drafts: ReadonlyMap<string, ProviderCredentialsDraft>;
  change: (
    generation: bigint,
    provider: { id: string; baseUrl?: string },
    next: (draft: ProviderCredentialsDraft) => ProviderCredentialsDraft,
  ) => void;
}

function emptyDrafts() {
  return {
    generation: ProviderMutationOwner.materialGeneration(),
    drafts: new Map<string, ProviderCredentialsDraft>(),
  };
}

const useProviderDrafts = create<ProviderDrafts>()((set) => ({
  ...emptyDrafts(),
  change: (generation, provider, next) => {
    ProviderMutationOwner.assertMaterialGeneration(generation);
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

// The Host owns this observation; retiring a Runtime also retires its unsubmitted credentials.
export function observeProviderDraftLifetime(): () => void {
  const clear = () => useProviderDrafts.setState(emptyDrafts());
  clear();
  return ProviderMutationOwner.subscribeMaterialGeneration(clear);
}

export function useProviderDraft(provider: { id: string; baseUrl?: string }) {
  const generation = useSyncExternalStore(
    ProviderMutationOwner.subscribeMaterialGeneration,
    ProviderMutationOwner.materialGeneration,
    ProviderMutationOwner.materialGeneration,
  );
  const stored = useProviderDrafts((state) =>
    state.generation === generation ? state.drafts.get(provider.id) : undefined,
  );
  const change = useProviderDrafts((state) => state.change);
  const draft = stored ?? ProviderCredentialsDraft.initial(provider);
  const setDraft = useCallback(
    (next: (draft: ProviderCredentialsDraft) => ProviderCredentialsDraft) =>
      change(generation, provider, next),
    [change, generation, provider],
  );
  return [draft, setDraft] as const;
}

export function resetProviderDraftsForTest(): void {
  useProviderDrafts.setState(emptyDrafts());
}
