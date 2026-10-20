import { describe, expect, it } from "vitest";
import { type Message } from "@/features/agent/public/sessionView";
import {
  messageContextMenuModel,
  type MessageContextMenuCopyState,
} from "@/features/chat/message-actions/application/messageContextMenuModel";
const message = ({ runId = null, ...overrides }: Partial<Message>): Message => ({
  blocks: [],
  id: "m",
  role: "user",
  createdAt: "2026-01-01T00:00:00.000Z",
  runId,
  ...overrides,
});

const copy = (
  overrides: Partial<MessageContextMenuCopyState> = {},
): MessageContextMenuCopyState => ({
  plain: "",
  canCopy: false,
  ...overrides,
});

describe("messageContextMenuModel", () => {
  it("projects copy payload availability independently from message actions", () => {
    const model = messageContextMenuModel({
      msg: message({ role: "assistant" }),
      copy: copy({ plain: "hi", canCopy: true }),
      canRestoreFiles: false,
      conversation: [],
    });

    expect(model.copyMarkdown).toBe(true);
    expect(model.copyPlain).toBe(true);
  });

  it("uses draftable content, not plain copy text, for user edit actions", () => {
    const model = messageContextMenuModel({
      msg: message({
        role: "user",
        blocks: [{ kind: "image", mime: "image/png", data: "abc" }],
      }),
      copy: copy(),
      canRestoreFiles: false,
      conversation: [],
    });

    expect(model.user.visible).toBe(true);
    expect(model.user.editInComposer).toBe(true);
    expect(model.copyPlain).toBe(false);
  });

  it("enables run-scoped user actions from a user run id", () => {
    const model = messageContextMenuModel({
      msg: message({
        role: "user",
        runId: "run_1",
        blocks: [{ kind: "text", status: "complete", text: "fix this" }],
      }),
      copy: copy({ plain: "fix this", canCopy: true }),
      canRestoreFiles: true,
      conversation: [],
    });

    expect(model.user).toMatchObject({
      visible: true,
      editRerun: true,
      editRerunRestore: true,
      restore: true,
      restoreFiles: true,
      restoreBoth: true,
      fork: true,
    });
  });

  it("enables assistant regenerate actions by restore capability", () => {
    expect(
      messageContextMenuModel({
        msg: message({ role: "assistant" }),
        copy: copy(),
        canRestoreFiles: true,
        conversation: [message({ role: "assistant" })],
      }).assistant,
    ).toEqual({ visible: true, regenerate: true, regenerateRestore: true });
  });

  it("keeps copying available for a child answer without offering a main-conversation replay", () => {
    const model = messageContextMenuModel({
      msg: message({ id: "child-answer", role: "assistant", runId: "child" }),
      copy: copy({ plain: "Child answer", canCopy: true }),
      canRestoreFiles: true,
      conversation: [message({ id: "root-answer", role: "assistant", runId: "root" })],
    });
    expect(model.copyMarkdown).toBe(true);
    expect(model.copyPlain).toBe(true);
    expect(model.assistant.visible).toBe(false);
  });
});
