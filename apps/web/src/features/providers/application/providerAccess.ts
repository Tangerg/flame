import { type StoreApi } from "zustand/vanilla";
import { type ProviderDrafts } from "@/features/providers/application/providerDrafts";
import { type ProviderMutationAccess } from "@/features/providers/application/providerMutationOwner";

export interface ProviderAccess {
  readonly mutations: ProviderMutationAccess;
  readonly drafts: Pick<StoreApi<ProviderDrafts>, "getState" | "getInitialState" | "subscribe">;
}
