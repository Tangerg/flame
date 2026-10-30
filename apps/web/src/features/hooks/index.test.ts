import { HookTrustMutationOwner } from "@/features/hooks/application/hookTrust";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { type FlameClient } from "@flame/runtime-client";
import { definePlugin } from "@/extensions/definePlugin";
import { loadPluginsForTest, resetKernelForTest } from "@/test/kernel";
import { RuntimeConnectionGeneration, RUNTIME_STREAM } from "@/features/connection/public/services";
import { createHooksPlugin } from "@/features/hooks/index";
import { rejected } from "@/test/rejected";
let owner: HookTrustMutationOwner;
beforeEach(() => {
  owner = new HookTrustMutationOwner(null);
});

afterEach(async () => {
  owner.dispose();
  await resetKernelForTest();
});

describe("hooks plugin Runtime generation wiring", () => {
  it("retires an admitted trust command when the Runtime process generation changes", async () => {
    const retired = deferred();
    const setTrust = vi.fn(() => retired.promise);
    let runtimeClient = () => ({ hooks: { setTrust } }) as unknown as FlameClient;
    let generation: RuntimeConnectionGeneration | null = null;
    const subscribers = new Set<() => void>();
    const runtime = definePlugin({
      name: "test.runtime-generation",
      provides: { stream: RUNTIME_STREAM },
      setup() {
        return {
          stream: {
            connectionGeneration: () => generation,
            subscribeConnection(onChange: () => void) {
              subscribers.add(onChange);
              return () => subscribers.delete(onChange);
            },
            reportConnectionLoss: vi.fn(),
          },
        };
      },
    });
    await loadPluginsForTest(
      runtime,
      createHooksPlugin(() => runtimeClient(), owner),
    );

    expect(owner.capture()).toBeNull();
    generation = RuntimeConnectionGeneration.forProcess("runtime_1");
    for (const subscriber of subscribers) subscriber();
    const command = rejected(owner.capture()!.setProjectTrust("/repo", true));
    await vi.waitFor(() => expect(setTrust).toHaveBeenCalledOnce());

    const successorTrust = vi.fn().mockResolvedValue(undefined);
    runtimeClient = () => ({ hooks: { setTrust: successorTrust } }) as unknown as FlameClient;
    generation = null;
    for (const subscriber of subscribers) subscriber();
    expect(owner.capture()).toBeNull();
    generation = RuntimeConnectionGeneration.forProcess("runtime_2");
    for (const subscriber of subscribers) subscriber();
    await expect(command).resolves.toMatchObject({
      message: "hook_trust_mutation_generation_retired",
    });

    retired.resolve();
    await owner.capture()!.setProjectTrust("/repo", false);
    expect(successorTrust).toHaveBeenCalledExactlyOnceWith("/repo", false);
    expect(setTrust).toHaveBeenCalledOnce();
  });
});

function deferred() {
  let resolve!: () => void;
  const promise = new Promise<void>((settle) => {
    resolve = settle;
  });
  return { promise, resolve };
}
