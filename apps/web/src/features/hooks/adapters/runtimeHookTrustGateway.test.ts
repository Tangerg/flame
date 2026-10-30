import { afterEach, describe, expect, it, vi } from "vitest";
import { queryClient } from "@/lib/queryClient";
import { type FlameClient } from "@flame/runtime-client";
import { HookTrustMutationOwner } from "@/features/hooks/application/hookTrust";
import { HOOKS_KEY } from "@/features/hooks/application/hookQueries";
import { createRuntimeHookTrustGateway } from "@/features/hooks/adapters/runtimeHookTrustGateway";
import { rejected } from "@/test/rejected";
let uninstall: (() => void) | undefined;

afterEach(() => {
  uninstall?.();
  uninstall = undefined;

  queryClient.removeQueries({ queryKey: [HOOKS_KEY] });
});

describe("runtimeHookTrustGateway", () => {
  it("retires in-flight and queued trust intents before installing a successor", async () => {
    const retiredWrite = deferred();
    const setTrustRetired = vi.fn(() => retiredWrite.promise);
    const setTrustSuccessor = vi.fn().mockResolvedValue(undefined);
    let runtimeClient = () => ({ hooks: { setTrust: setTrustRetired } }) as unknown as FlameClient;
    const owner = new HookTrustMutationOwner(createRuntimeHookTrustGateway(runtimeClient()));
    uninstall = () => owner.dispose();
    const commands = owner.capture()!;

    const inFlight = rejected(commands.setProjectTrust("/repo", true));
    const queued = rejected(commands.setProjectTrust("/repo", false));
    await vi.waitFor(() => expect(setTrustRetired).toHaveBeenCalledOnce());

    runtimeClient = () => ({ hooks: { setTrust: setTrustSuccessor } }) as unknown as FlameClient;
    owner.replaceRuntimeGeneration(() => createRuntimeHookTrustGateway(runtimeClient()));
    retiredWrite.resolve();

    await expect(inFlight).resolves.toMatchObject({
      message: "hook_trust_mutation_generation_retired",
    });
    await expect(queued).resolves.toMatchObject({
      message: "hook_trust_mutation_generation_retired",
    });
    expect(setTrustSuccessor).not.toHaveBeenCalled();
  });
});

function deferred() {
  let resolve!: () => void;
  const promise = new Promise<void>((settle) => {
    resolve = settle;
  });
  return { promise, resolve };
}
