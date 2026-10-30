import { expect, it, vi } from "vitest";
import { createProviders } from "@/features/providers/adapters/providers";
import { type ProviderGateway } from "@/features/providers/application/ports/providerGateway";

it("keeps commands and sensitive drafts with their explicit feature instance", async () => {
  const first = createProviders();
  const second = createProviders();
  const probe = vi.fn(async () => ({ ok: true }));
  const gateway = { testProvider: probe } as unknown as ProviderGateway;
  try {
    first.mutations.replaceRuntimeGeneration(() => gateway);
    const firstGeneration = first.mutations.materialGeneration();
    const provider = { id: "shared-name" };
    first.access.drafts
      .getState()
      .change(firstGeneration, provider, (draft) => draft.withAPIKey("synthetic-first-secret"));
    second.mutations.replaceRuntimeGeneration(() => gateway);
    expect(first.mutations.materialGeneration()).toBe(firstGeneration);
    expect(second.access.drafts.getState().drafts.get(provider.id)).toBeUndefined();
    const commands = second.mutations.capture()!;
    first.dispose();
    expect(first.access.drafts.getState().drafts.get(provider.id)).toBeUndefined();
    await expect(commands.testProvider(provider.id)).resolves.toEqual({ ok: true });
    expect(probe).toHaveBeenCalledOnce();
  } finally {
    first.dispose();
    second.dispose();
  }
});
