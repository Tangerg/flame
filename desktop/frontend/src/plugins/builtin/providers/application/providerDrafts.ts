import { useCallback } from "react";
import { create } from "zustand";
import { ProviderCredentialsDraft } from "./providerDraft";

interface ProviderDrafts {
  drafts: ReadonlyMap<string, ProviderCredentialsDraft>;
  change: (
    provider: { id: string; baseUrl?: string },
    next: (draft: ProviderCredentialsDraft) => ProviderCredentialsDraft,
  ) => void;
}

const useProviderDrafts = create<ProviderDrafts>()((set) => ({
  drafts: new Map(),
  change: (provider, next) =>
    set((state) => {
      const drafts = new Map(state.drafts);
      const draft = next(
        state.drafts.get(provider.id) ?? ProviderCredentialsDraft.initial(provider),
      );
      if (draft.dirty(provider)) drafts.set(provider.id, draft);
      else drafts.delete(provider.id);
      return { drafts };
    }),
}));

export function useProviderDraft(provider: { id: string; baseUrl?: string }) {
  const stored = useProviderDrafts((state) => state.drafts.get(provider.id));
  const change = useProviderDrafts((state) => state.change);
  const draft = stored ?? ProviderCredentialsDraft.initial(provider);
  const setDraft = useCallback(
    (next: (draft: ProviderCredentialsDraft) => ProviderCredentialsDraft) => change(provider, next),
    [change, provider],
  );
  return [draft, setDraft] as const;
}

export function resetProviderDraftsForTest(): void {
  useProviderDrafts.setState({ drafts: new Map() });
}
