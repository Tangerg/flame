import {
  createContext,
  useCallback,
  useContext,
  useSyncExternalStore,
  type ReactNode,
} from "react";
import { useStore } from "zustand";
import { type ProviderAccess } from "@/features/providers/application/providerAccess";
import { ProviderCredentialsDraft } from "@/features/providers/application/providerDraft";

const ProvidersContext = createContext<ProviderAccess | null>(null);

export function ProvidersProvider({
  value,
  children,
}: {
  value: ProviderAccess;
  children: ReactNode;
}) {
  return <ProvidersContext value={value}>{children}</ProvidersContext>;
}

export function useProviderAccess(): ProviderAccess {
  const value = useContext(ProvidersContext);
  if (!value) throw new Error("ProvidersProvider is missing");
  return value;
}

export function useProviderDraft(provider: { id: string; baseUrl?: string }) {
  const { mutations, drafts } = useProviderAccess();
  const generation = useSyncExternalStore(
    mutations.subscribeMaterialGeneration,
    mutations.materialGeneration,
    mutations.materialGeneration,
  );
  const stored = useStore(drafts, (state) =>
    state.generation === generation ? state.drafts.get(provider.id) : undefined,
  );
  const change = useStore(drafts, (state) => state.change);
  const draft = stored ?? ProviderCredentialsDraft.initial(provider);
  const setDraft = useCallback(
    (next: (draft: ProviderCredentialsDraft) => ProviderCredentialsDraft) =>
      change(generation, provider, next),
    [change, generation, provider],
  );
  return [draft, setDraft] as const;
}
