import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { type Message } from "@/features/agent/public/sessionView";
import { selectAgentSession } from "@/features/agent/public/session";
import {
  editAndRerunMessage,
  editMessageInComposer,
  forkFromMessage,
  regenerateMessage,
  restoreCheckpoint,
} from "@/features/chat/message-actions/application/messageActions";
import { notifyInfo } from "@/features/notifications/public/notifications";
import * as imageInput from "@/features/composer/public/input";
import {
  appendToComposerDraft,
  captureComposerSubmission,
  replaceComposerDraft,
} from "@/features/composer/public/draft";
import {
  useAddComposerImageFiles,
  useAddComposerPaste,
} from "@/features/composer/public/attachments";
import { registerComposerFocusTarget } from "@/features/composer/public/focus";
const model = vi.hoisted(() => ({
  conversation: null as { sessionId: string; messages: Message[] } | null,
  rollback: vi.fn(),
  fork: vi.fn(),
  send: vi.fn(),
}));

vi.mock("@/features/agent/public/session", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/features/agent/public/session")>()),
  activeAgentConversation: () => model.conversation,
  forkAgentSessionAtRun: model.fork,
  rollbackSessionToBeforeRun: model.rollback,
  sendToAgentSession: model.send,
}));

vi.mock("@/features/notifications/public/notifications", async (importOriginal) => {
  const actual =
    await importOriginal<typeof import("@/features/notifications/public/notifications")>();
  return { ...actual, notifyError: vi.fn(), notifyInfo: vi.fn() };
});

vi.mock("@/lib/i18n/index", () => ({ t: (key: string) => key }));

vi.mock("@/lib/rpcErrors", () => ({ describeRpcError: () => undefined }));

beforeEach(() => {
  model.rollback.mockReset();
  model.fork.mockReset().mockResolvedValue(undefined);
  model.send.mockReset();
  model.send.mockReturnValue(true);
  vi.mocked(notifyInfo).mockClear();
  selectAgentSession("ses_1");
  replaceComposerDraft({ text: "waiting draft" });
  const prompt = message({
    id: "user_1",
    role: "user",
    runId: "run_1",
    blocks: [{ kind: "text", status: "complete", text: "stale projection" }],
  });
  model.conversation = {
    sessionId: "ses_1",
    messages: [prompt, message({ id: "assistant_1", runId: "run_1" })],
  };
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

function readDraft() {
  const { value, images, pastes } = captureComposerSubmission();
  return { value, images, pastes };
}

describe("message history actions", () => {
  it.each([
    { label: "a child user message", id: "child-user", role: "user", runId: "child-run" },
    { label: "a missing message", id: "missing-user", role: "user", runId: "run_1" },
    { label: "a stale run", id: "user_1", role: "user", runId: "retired-run" },
    { label: "a user message without a run", id: "user_1", role: "user", runId: null },
    { label: "an assistant message", id: "assistant_1", role: "assistant", runId: "run_1" },
  ] as const)(
    "refuses history actions for $label before changing draft or focus",
    async ({ id, role, runId }) => {
      const msg = message({
        id,
        role,
        runId,
        blocks: [{ kind: "text", status: "complete", text: "do not load this prompt" }],
      });
      const input = document.createElement("textarea");
      const origin = document.createElement("button");
      document.body.append(input, origin);
      const unregister = registerComposerFocusTarget(input);
      model.rollback.mockResolvedValue({ status: "unavailable" });
      replaceComposerDraft({
        text: "keep the current draft",
        images: [{ mime: "image/png", data: "retained-image" }],
      });
      const before = readDraft();
      origin.focus();
      expect(document.activeElement).toBe(origin);
      try {
        editAndRerunMessage(msg);
        editAndRerunMessage(msg, { restoreFiles: true });
        restoreCheckpoint(msg, "history");
        restoreCheckpoint(msg, "files");
        restoreCheckpoint(msg, "both");
        await Promise.resolve();

        expect(readDraft()).toEqual(before);
        expect(document.activeElement).toBe(origin);
        expect(model.rollback).not.toHaveBeenCalled();
        expect(notifyInfo).not.toHaveBeenCalled();
      } finally {
        unregister();
        input.remove();
        origin.remove();
      }
    },
  );

  it("keeps ordinary editing available for a user message without a run", () => {
    editMessageInComposer(
      message({
        role: "user",
        blocks: [
          { kind: "text", status: "complete", text: "local prompt" },
          { kind: "image", mime: "image/png", data: "local-image" },
        ],
      }),
    );

    expect(readDraft().value).toBe("local prompt");
    expect(readDraft().images).toMatchObject([{ mime: "image/png", data: "local-image" }]);
    expect(model.rollback).not.toHaveBeenCalled();
  });

  it.each(["run_1", "child-run"])("keeps forking available at user run %s", (runId) => {
    const before = readDraft();

    forkFromMessage(message({ id: "fork-target", role: "user", runId }));

    expect(model.fork).toHaveBeenCalledWith("ses_1", runId);
    expect(model.rollback).not.toHaveBeenCalled();
    expect(readDraft()).toEqual(before);
  });

  it.each(["history", "files", "both"] as const)(
    "restores %s before an earlier root prompt",
    async (restoreType) => {
      const prompt = model.conversation!.messages[0]!;
      model.conversation!.messages.push(
        message({ id: "later-user", role: "user", runId: "run_2" }),
      );
      model.rollback.mockResolvedValue({ status: "committed" });
      const before = readDraft();

      restoreCheckpoint(prompt, restoreType);
      await model.rollback.mock.results[0]!.value;

      expect(model.rollback).toHaveBeenCalledWith("ses_1", "run_1", restoreType);
      expect(readDraft()).toEqual(before);
      expect(notifyInfo).toHaveBeenCalledWith(`session.restore.${restoreType}`, {
        source: "session",
      });
    },
  );

  it("regenerates from the Runtime-authored dropped input", async () => {
    model.rollback.mockResolvedValue({
      status: "committed",
      userInput: {
        parts: [
          { kind: "text", text: "authoritative prompt" },
          { kind: "image", mime: "image/png", data: "aW1hZ2U=" },
        ],
      },
    });

    regenerateMessage(model.conversation!.messages[1]!);

    await vi.waitFor(() =>
      expect(model.send).toHaveBeenCalledWith("ses_1", {
        parts: [
          { kind: "text", text: "authoritative prompt" },
          { kind: "image", mime: "image/png", data: "aW1hZ2U=" },
        ],
      }),
    );
  });

  it("does not run a second follow-up for a joined history rewrite", async () => {
    model.rollback.mockResolvedValue({ status: "inFlight" });

    regenerateMessage(model.conversation!.messages[1]!);

    await vi.waitFor(() => expect(model.rollback).toHaveBeenCalledOnce());
    expect(model.send).not.toHaveBeenCalled();
  });

  it("prefills an earlier root edit-and-rerun from the Runtime-authored dropped input", async () => {
    model.conversation!.messages.push(message({ id: "later-user", role: "user", runId: "run_2" }));
    model.rollback.mockResolvedValue({
      status: "committed",
      userInput: {
        parts: [
          { kind: "text", text: "canonical edit" },
          { kind: "image", mime: "image/jpeg", data: "anBlZw==" },
        ],
      },
    });

    editAndRerunMessage(model.conversation!.messages[0]!);

    await vi.waitFor(() => expect(readDraft().value).toBe("canonical edit"));
    expect(readDraft().images).toMatchObject([{ mime: "image/jpeg", data: "anBlZw==" }]);
    expect(model.rollback).toHaveBeenCalledWith("ses_1", "run_1", "history");
    expect(notifyInfo).not.toHaveBeenCalled();
  });

  it("uses the message content when a committed rollback has no dropped input", async () => {
    model.rollback.mockResolvedValue({ status: "committed" });

    editAndRerunMessage(model.conversation!.messages[0]!);

    await vi.waitFor(() => expect(readDraft().value).toBe("stale projection"));
  });

  it.each(["unavailable", "inFlight"])("does not prefill an %s rollback", async (status) => {
    model.rollback.mockResolvedValue({ status });
    const before = readDraft();

    editAndRerunMessage(model.conversation!.messages[0]!);
    await model.rollback.mock.results[0]!.value;

    expect(readDraft()).toEqual(before);
    expect(notifyInfo).not.toHaveBeenCalled();
  });

  it.each(["text", "paste", "image"])(
    "preserves later %s authoring when rollback finishes",
    async (kind) => {
      const rollback = Promise.withResolvers<{ status: "committed" }>();
      const { result } = renderHook(() => ({
        addPaste: useAddComposerPaste(),
        addImageFiles: useAddComposerImageFiles(),
      }));
      vi.spyOn(imageInput, "fileToInputImage").mockResolvedValue({
        mime: "image/png",
        data: "later-image",
        name: "later.png",
      });
      model.rollback.mockReturnValue(rollback.promise);
      editAndRerunMessage(model.conversation!.messages[0]!);
      await act(async () => {
        if (kind === "text") appendToComposerDraft("later draft");
        else if (kind === "paste") result.current.addPaste("later source text");
        else result.current.addImageFiles([new File([""], "later.png")]);
      });
      const later = readDraft();
      expect(later.images).toHaveLength(kind === "image" ? 1 : 0);
      expect(later.pastes).toHaveLength(kind === "paste" ? 1 : 0);

      rollback.resolve({ status: "committed" });
      await rollback.promise;

      expect(readDraft()).toEqual(later);
      expect(notifyInfo).toHaveBeenCalledWith("session.editRerun.draftRetained", {
        source: "session",
      });
    },
  );

  it("does not replace either Session's draft after switching away", async () => {
    const rollback = Promise.withResolvers<{ status: "committed" }>();
    model.rollback.mockReturnValue(rollback.promise);
    const original = readDraft();
    editAndRerunMessage(model.conversation!.messages[0]!);
    selectAgentSession("ses_2");
    replaceComposerDraft({ text: "keep this second draft" });
    const current = readDraft();

    rollback.resolve({ status: "committed" });
    await rollback.promise;

    expect(readDraft()).toEqual(current);
    selectAgentSession("ses_1");
    expect(readDraft()).toEqual(original);
    expect(notifyInfo).toHaveBeenCalledOnce();
  });

  it("preserves an image selected while rollback is pending", async () => {
    const rollback = Promise.withResolvers<{ status: "committed" }>();
    const reading =
      Promise.withResolvers<Awaited<ReturnType<typeof imageInput.fileToInputImage>>>();
    vi.spyOn(imageInput, "fileToInputImage").mockReturnValue(reading.promise);
    const { result } = renderHook(useAddComposerImageFiles);
    model.rollback.mockReturnValue(rollback.promise);
    editAndRerunMessage(model.conversation!.messages[0]!);
    act(() => result.current([new File([""], "later.png")]));
    const waiting = readDraft();

    rollback.resolve({ status: "committed" });
    await rollback.promise;

    expect(readDraft()).toEqual(waiting);
    expect(notifyInfo).toHaveBeenCalledOnce();
    reading.resolve({ mime: "image/png", data: "later-image", name: "later.png" });
    await vi.waitFor(() => expect(readDraft().images).toHaveLength(1));

    expect(readDraft().value).toBe("waiting draft");
    expect(readDraft().images[0]?.data).toBe("later-image");
  });
});

function message({ runId = null, ...overrides }: Partial<Message>): Message {
  return {
    blocks: [],
    id: "message",
    role: "assistant",
    createdAt: "2026-08-12T00:00:00.000Z",
    runId,
    ...overrides,
  };
}
