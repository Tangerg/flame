import { describe, expect, it } from "vitest";
import { type Message } from "@/features/agent/public/sessionView";
import {
  canCopyMessage,
  canEditMessage,
  canForkMessage,
  canRateMessage,
  canRegenerateMessage,
  canRollbackMessage,
} from "@/features/chat/message-actions/application/messageActionAvailability";
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

  it("allows rollback at every root user prompt, while excluding child and local messages", () => {
    const earlier = message({ id: "earlier", role: "user", runId: "run_1" });
    const latest = message({ id: "latest", role: "user", runId: "run_2" });
    const local = message({ id: "local", role: "user" });
    const assistant = message({ id: "answer", role: "assistant", runId: "run_2" });
    const roots = [earlier, latest, local, assistant];

    expect(canRollbackMessage(earlier, roots)).toBe(true);
    expect(canRollbackMessage(latest, roots)).toBe(true);
    expect(canRollbackMessage(local, roots)).toBe(false);
    expect(canRollbackMessage(assistant, roots)).toBe(false);
    expect(canRollbackMessage(message({ id: "child", role: "user", runId: "child" }), roots)).toBe(
      false,
    );
  });

  it("requires both the message and its run to match the root narrative", () => {
    const prompt = message({ role: "user", runId: "root" });

    expect(canRollbackMessage(prompt, [])).toBe(false);
    expect(canRollbackMessage({ ...prompt, id: "missing" }, [prompt])).toBe(false);
    expect(canRollbackMessage({ ...prompt, runId: "stale" }, [prompt])).toBe(false);
  });

  it("allows a child user run to fork independently of root rollback eligibility", () => {
    expect(canForkMessage(message({ role: "user", runId: "root" }))).toBe(true);
    expect(canForkMessage(message({ role: "user", runId: "child" }))).toBe(true);
    expect(canForkMessage(message({ role: "user" }))).toBe(false);
    expect(canForkMessage(message({ role: "assistant", runId: "root" }))).toBe(false);
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
