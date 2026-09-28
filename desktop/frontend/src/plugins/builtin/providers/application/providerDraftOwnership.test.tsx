import { act, renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { GenerationRetiredError } from "@/lib/asyncOwnership";
import type { ProviderGateway } from "./ports/providerGateway";
import { ProviderMutationOwner } from "./providerMutationOwner";
import { useUpdateProvider, useTestProvider } from "./providerConfig";
import { observeProviderDraftLifetime, useProviderDraft } from "./providerDrafts";

function gateway(): ProviderGateway {
  return {
    updateProvider: vi.fn<ProviderGateway["updateProvider"]>(),
    setUtilityRole: async (role) => role,
    setEmbeddingRole: async (role) => role,
    testProvider: vi.fn(async () => ({ ok: true })),
    errorMessage: () => undefined,
  };
}

describe("provider draft ownership", () => {
  it("retires sensitive drafts and every stale submission when Runtime changes", async () => {
    const first = gateway();
    const second = gateway();
    const owner = ProviderMutationOwner.install(first);
    const stop = observeProviderDraftLifetime();
    const provider = { id: "same-provider", baseUrl: "https://provider.example" };
    const hook = renderHook(() => ({
      draft: useProviderDraft(provider),
      update: useUpdateProvider(),
      test: useTestProvider(),
    }));
    try {
      act(() => hook.result.current.draft[1]((draft) => draft.withAPIKey("synthetic-A-secret")));
      const stale = hook.result.current;
      const input = stale.draft[0].toUpdate(provider);
      act(() => owner.replaceRuntimeGeneration(() => second));
      expect(hook.result.current.draft[0].apiKey).toBe("");
      expect(() => stale.update(input)).toThrow(GenerationRetiredError);
      await expect(stale.test(provider.id)).rejects.toBeInstanceOf(GenerationRetiredError);
      expect(() => stale.draft[1]((draft) => draft.withAPIKey("late-A-secret"))).toThrow(
        GenerationRetiredError,
      );
      expect(second.updateProvider).not.toHaveBeenCalled();
      expect(second.testProvider).not.toHaveBeenCalled();
      act(() => owner.replaceRuntimeGeneration(() => first));
      expect(hook.result.current.draft[0].apiKey).toBe("");
    } finally {
      hook.unmount();
      owner.dispose();
      stop();
    }
  });
});
