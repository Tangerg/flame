import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { type FlameClient } from "@flame/runtime-client";
import { definePlugin } from "@/extensions/definePlugin";
import { loadPluginsForTest, resetKernelForTest } from "@/test/kernel";
import { RuntimeConnectionGeneration, RUNTIME_STREAM } from "@/features/connection/public/services";
import { MessageFeedbackOwner } from "@/features/chat/message-actions/application/feedback";
import { createMessageFeedbackPlugin } from "@/features/chat/message-actions/feedback";
import { rejected } from "@/test/rejected";
let runtimeClient: () => FlameClient = () => {
  throw new Error("Runtime test client is not configured");
};

const getRuntimeClient = () => runtimeClient();

let owner: MessageFeedbackOwner;
beforeEach(() => {
  owner = new MessageFeedbackOwner(null);
});

afterEach(async () => {
  owner.dispose();
  await resetKernelForTest();
});

describe("message feedback Runtime generation wiring", () => {
  it("retires an admitted command when the Runtime process generation changes", async () => {
    const pending = Promise.withResolvers<void>();
    const create = vi.fn(() => pending.promise);
    runtimeClient = () => ({ feedback: { create } }) as unknown as FlameClient;
    let generation: RuntimeConnectionGeneration | null =
      RuntimeConnectionGeneration.forProcess("runtime_1");
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
    await loadPluginsForTest(runtime, createMessageFeedbackPlugin(getRuntimeClient, owner));

    const command = rejected(
      owner.capture()!.submit(
        {
          sessionId: "ses_feedback",
          messageId: "item_feedback",
          runId: "run_feedback",
        },
        "positive",
      ),
    );
    await vi.waitFor(() => expect(create).toHaveBeenCalledOnce());

    generation = RuntimeConnectionGeneration.forProcess("runtime_2");
    for (const subscriber of subscribers) subscriber();
    await expect(command).resolves.toMatchObject({
      message: "message_feedback_generation_retired",
    });

    runtimeClient = () => {
      throw new Error("Runtime connection is withdrawn");
    };
    generation = null;
    expect(() => {
      for (const subscriber of subscribers) subscriber();
    }).not.toThrow();
    expect(owner.capture()).toBeNull();
    pending.resolve();
  });
});
