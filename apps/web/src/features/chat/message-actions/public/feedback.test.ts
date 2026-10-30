import { act, cleanup } from "@testing-library/react";
import { expect, it, onTestFinished, vi } from "vitest";
import { createTestClient } from "@/test/renderClient";
import { useMessageFeedback } from "@/features/chat/message-actions/public/feedback";

it("renders the current rating while a captured submission retains its original generation", async () => {
  const client = createTestClient({ start: false });
  onTestFinished(async () => {
    cleanup();
    await client.dispose();
  });
  const first = vi.fn().mockResolvedValue(undefined);
  const successor = vi.fn().mockResolvedValue(undefined);
  client.feedback.replaceRuntimeGeneration(() => ({ createMessageFeedback: first }));
  const { result } = client.renderHook(() =>
    useMessageFeedback("ses_feedback", { id: "item_feedback", runId: "run_feedback" }),
  );
  await act(async () => {
    await result.current.submit("positive");
  });
  expect(result.current.rating).toBe("positive");
  const captured = result.current.submit;

  act(() => {
    client.feedback.replaceRuntimeGeneration(() => ({ createMessageFeedback: successor }));
  });

  expect(result.current.rating).toBeUndefined();
  await expect(captured("negative")).rejects.toMatchObject({
    message: "message_feedback_generation_retired",
  });
  expect(first).toHaveBeenCalledOnce();
  expect(successor).not.toHaveBeenCalled();
  await act(async () => {
    await result.current.submit("negative");
  });
  expect(result.current.rating).toBe("negative");
  expect(successor).toHaveBeenCalledOnce();
});
