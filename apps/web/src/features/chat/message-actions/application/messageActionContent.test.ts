import { describe, expect, it } from "vitest";
import { type Message } from "@/features/agent/public/sessionView";
import {
  messageDraftContent,
  messageHasDraftContent,
} from "@/features/chat/message-actions/application/messageActionContent";
const message = ({ runId = null, ...overrides }: Partial<Message>): Message => ({
  blocks: [],
  id: "m",
  role: "assistant",
  createdAt: "2026-01-01T00:00:00.000Z",
  runId,
  ...overrides,
});

describe("messageActionContent", () => {
  it("extracts text and inline images for composer drafts", () => {
    expect(
      messageDraftContent(
        message({
          blocks: [
            { kind: "text", status: "complete", text: "hello" },
            { kind: "image", mime: "image/png", data: "abc" },
          ],
        }),
      ),
    ).toEqual({ text: "hello", images: [{ mime: "image/png", data: "abc" }] });
  });

  it("treats image-only messages as draftable content", () => {
    expect(
      messageHasDraftContent(
        message({ blocks: [{ kind: "image", mime: "image/png", data: "x" }] }),
      ),
    ).toBe(true);
    expect(messageHasDraftContent(message({ blocks: [] }))).toBe(false);
  });
});
