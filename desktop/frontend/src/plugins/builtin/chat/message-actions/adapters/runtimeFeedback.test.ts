import { afterEach, describe, expect, it, vi } from "vitest";
import type { FlameClient } from "@flame/runtime-contract/client";
import { submitMessageFeedback } from "../application/feedback";
import { installRuntimeFeedbackGateway } from "./runtimeFeedback";

let runtimeClient: () => FlameClient = () => {
  throw new Error("Runtime test client is not configured");
};
const getRuntimeClient = () => runtimeClient();

let dispose: (() => void) | undefined;

afterEach(() => {
  dispose?.();
  dispose = undefined;
});

describe("runtimeFeedbackGateway", () => {
  it("captures the exact client and submits the Session identity admitted with the message", async () => {
    const create = vi.fn().mockResolvedValue(undefined);
    const successorCreate = vi.fn().mockResolvedValue(undefined);
    runtimeClient = () => ({ feedback: { create } }) as unknown as FlameClient;
    const installation = installRuntimeFeedbackGateway(getRuntimeClient);
    dispose = installation.dispose;
    runtimeClient = () => ({ feedback: { create: successorCreate } }) as unknown as FlameClient;

    await submitMessageFeedback(
      {
        sessionId: "ses_original",
        messageId: "item_feedback",
        runId: "run_feedback",
      },
      "positive",
    );

    expect(create).toHaveBeenCalledWith({
      sessionId: "ses_original",
      runId: "run_feedback",
      itemId: "item_feedback",
      rating: "positive",
    });
    expect(successorCreate).not.toHaveBeenCalled();
  });
});

it("replaces the captured feedback client only at its Runtime generation boundary", async () => {
  const first = vi.fn().mockResolvedValue(undefined);
  const successor = vi.fn().mockResolvedValue(undefined);
  let client = { feedback: { create: first } } as unknown as FlameClient;
  const provider = vi.fn(() => client);
  const installation = installRuntimeFeedbackGateway(provider);
  dispose = installation.dispose;
  const target = { sessionId: "ses_feedback", messageId: "item_feedback" };
  await submitMessageFeedback(target, "positive");
  client = { feedback: { create: successor } } as unknown as FlameClient;
  installation.replaceRuntimeGeneration();
  await submitMessageFeedback(target, "negative");
  expect(first).toHaveBeenCalledOnce();
  expect(successor).toHaveBeenCalledOnce();
  installation.dispose();
  provider.mockClear();
  installation.replaceRuntimeGeneration();
  expect(provider).not.toHaveBeenCalled();
});
