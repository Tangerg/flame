import { describe, expect, it } from "vitest";
import type { Message } from "@/plugins/sdk/types/agentSessionView";
import {
  canCopyMessage,
  canEditMessage,
  canRateMessage,
  canRegenerateMessage,
  canUseMessageRunCheckpoint,
} from "./messageActionAvailability";

const message = ({ runId = null, ...overrides }: Partial<Message>): Message => ({
  blocks: [],
  id: "m",
  role: "assistant",
  createdAt: "2026-01-01T00:00:00.000Z",
  runId,
  ...overrides,
});

describe("message action availability", () => {
  it("uses copy payload availability for copy actions", () => {
    expect(canCopyMessage({ canCopy: true })).toBe(true);
    expect(canCopyMessage({ canCopy: false })).toBe(false);
  });

  it("allows editing draftable user messages, including image-only content", () => {
    expect(
      canEditMessage(
        message({
          role: "user",
          blocks: [{ kind: "image", mime: "image/png", data: "abc" }],
        }),
      ),
    ).toBe(true);
    expect(canEditMessage(message({ role: "assistant", blocks: [] }))).toBe(false);
  });

  it("uses user run ids for checkpoint actions", () => {
    expect(canUseMessageRunCheckpoint(message({ role: "user", runId: "run_1" }))).toBe(true);
    expect(canUseMessageRunCheckpoint(message({ role: "assistant", runId: "run_1" }))).toBe(false);
  });

  it("regenerates only an assistant in the main conversation, while child feedback stays available", () => {
    const assistant = message({ role: "assistant" });
    const user = message({ id: "user", role: "user" });
    const conversation = [user, assistant];
    expect(canRegenerateMessage(assistant, conversation)).toBe(true);
    expect(canRegenerateMessage(user, conversation)).toBe(false);
    expect(canRegenerateMessage(message({ id: "child-answer" }), conversation)).toBe(false);
    expect(canRateMessage(message({ role: "assistant" }))).toBe(true);
    expect(canRateMessage(message({ role: "user" }))).toBe(false);
  });
});
