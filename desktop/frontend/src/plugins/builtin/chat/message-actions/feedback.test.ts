import { afterEach, describe, expect, it, vi } from "vitest";
import { resetContainer, setContainer } from "@/main/container";
import type { FlameClient } from "@/rpc";
import { definePlugin } from "@/plugins/sdk";
import { loadPluginsForTest, resetKernelForTest } from "@/plugins/sdk/testKernel";
import {
  RuntimeConnectionGeneration,
  RUNTIME_STREAM,
} from "@/plugins/builtin/runtime/public/services";
import { submitMessageFeedback } from "./application/feedback";
import { messageFeedback } from "./feedback";
import { rejected } from "@/test/rejected";

afterEach(async () => {
  await resetKernelForTest();
  resetContainer();
});

describe("message feedback Runtime generation wiring", () => {
  it("retires an admitted command when the Runtime process generation changes", async () => {
    const pending = Promise.withResolvers<void>();
    const create = vi.fn(() => pending.promise);
    setContainer({ client: () => ({ feedback: { create } }) as unknown as FlameClient });
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
    await loadPluginsForTest(runtime, messageFeedback);

    const command = rejected(
      submitMessageFeedback(
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

    pending.resolve();
  });
});
