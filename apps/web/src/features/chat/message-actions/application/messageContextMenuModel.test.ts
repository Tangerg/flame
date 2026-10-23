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

  it.each([false, true])(
    "enables historical root actions with file restore capability %s",
    (canRestoreFiles) => {
      const prompt = message({
        role: "user",
        runId: "run_1",
        blocks: [{ kind: "text", status: "complete", text: "fix this" }],
      });
      const model = messageContextMenuModel({
        msg: prompt,
        copy: copy({ plain: "fix this", canCopy: true }),
        canRestoreFiles,
        conversation: [prompt, message({ id: "later-user", role: "user", runId: "run_2" })],
      });

      expect(model.user).toMatchObject({
        visible: true,
        editRerun: true,
        editRerunRestore: canRestoreFiles,
        restore: true,
        restoreFiles: canRestoreFiles,
        restoreBoth: canRestoreFiles,
        fork: true,
      });
    },
  );

  it("keeps child user copy, editing, and fork separate from root history actions", () => {
    const model = messageContextMenuModel({
      msg: message({
        id: "child-user",
        runId: "child",
        blocks: [{ kind: "text", status: "complete", text: "child prompt" }],
      }),
      copy: copy({ plain: "child prompt", canCopy: true }),
      canRestoreFiles: true,
      conversation: [message({ id: "root-user", runId: "root" })],
    });

    expect(model.copyMarkdown).toBe(true);
    expect(model.copyPlain).toBe(true);
    expect(model.user).toEqual({
      visible: true,
      editInComposer: true,
      editRerun: false,
      editRerunRestore: false,
      restore: false,
      restoreFiles: false,
      restoreBoth: false,
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
