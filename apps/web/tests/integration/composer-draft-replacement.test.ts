import { expect, it, onTestFinished } from "vitest";
import { editMessageInComposer } from "@/features/chat/message-actions/public/messageActions";
import { createComposer } from "@/features/composer/adapters/composerStatePorts";
import { DraftContent } from "@/features/composer/domain/draft";

it("retains a draft with a reference until replacing it from a sent message is confirmed", () => {
  const unexpectedRuntimeCall = () => {
    throw new Error("unexpected Runtime call while editing a local draft");
  };
  const { access, dispose } = createComposer(
    {
      getActiveSessionId: unexpectedRuntimeCall,
      getLifecycleSnapshot: unexpectedRuntimeCall,
      subscribeActiveSessionId: unexpectedRuntimeCall,
      subscribeLifecycle: unexpectedRuntimeCall,
      subscribeDeleted: unexpectedRuntimeCall,
    },
    unexpectedRuntimeCall,
    { subscribeReplacement: unexpectedRuntimeCall },
    {
      connectionGeneration: unexpectedRuntimeCall,
      subscribeConnection: unexpectedRuntimeCall,
      reportConnectionLoss: unexpectedRuntimeCall,
    },
  );
  onTestFinished(dispose);

  const draftText = "Unsent draft";
  const draftReference = { label: "notes.txt:1", text: "Unsent quote" };
  access.checkpoint(DraftContent.fromText(draftText));
  expect(access.captureAddReference()(draftReference)).toBe(true);
  const draft = access.captureSubmission();
  expect(draft.references).toMatchObject([draftReference]);

  const historyText = "Previously sent message";
  const historyReference = { label: "notes.txt:12", text: "Sent quote" };
  const confirmations: Array<() => boolean> = [];
  editMessageInComposer(
    access,
    {
      id: "previous-message",
      role: "user",
      runId: "previous-run",
      blocks: [
        { kind: "text", status: "complete", text: historyText },
        { kind: "reference", ...historyReference },
      ],
    },
    (apply) => {
      confirmations.push(apply);
    },
  );

  expect(access.captureSubmission()).toMatchObject({
    value: draftText,
    references: draft.references,
  });
  expect(confirmations).toHaveLength(1);
  expect(confirmations[0]?.()).toBe(true);

  expect(access.captureSubmission()).toMatchObject({
    value: historyText,
    references: [expect.objectContaining(historyReference)],
  });
});
