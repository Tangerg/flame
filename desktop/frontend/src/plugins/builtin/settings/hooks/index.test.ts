import { afterEach, describe, expect, it, vi } from "vitest";
import type { FlameClient } from "@flame/runtime-contract/client";
import { definePlugin } from "@/plugins/sdk";
import { loadPluginsForTest, resetKernelForTest } from "@/plugins/sdk/testKernel";
import {
  RuntimeConnectionGeneration,
  RUNTIME_STREAM,
} from "@/plugins/builtin/runtime/public/services";
import { setHookTrust } from "./application/hookTrust";
import { createHooksPlugin } from "./index";
import { rejected } from "@/test/rejected";

afterEach(async () => {
  await resetKernelForTest();
});

describe("hooks plugin Runtime generation wiring", () => {
  it("retires an admitted trust command when the Runtime process generation changes", async () => {
    const retired = deferred();
    const setTrust = vi.fn(() => retired.promise);
    let runtimeClient = () => ({ hooks: { setTrust } }) as unknown as FlameClient;
    let generation = RuntimeConnectionGeneration.forProcess("runtime_1");
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
      createHooksPlugin(() => runtimeClient()),
    );

    const command = rejected(setHookTrust("/repo", true));
    await vi.waitFor(() => expect(setTrust).toHaveBeenCalledOnce());

    const successorTrust = vi.fn().mockResolvedValue(undefined);
    runtimeClient = () => ({ hooks: { setTrust: successorTrust } }) as unknown as FlameClient;
    generation = RuntimeConnectionGeneration.forProcess("runtime_2");
    for (const subscriber of subscribers) subscriber();
    await expect(command).resolves.toMatchObject({
      message: "hook_trust_mutation_generation_retired",
    });

    retired.resolve();
    await setHookTrust("/repo", false);
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
