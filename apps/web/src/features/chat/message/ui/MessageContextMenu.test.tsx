import { type Message } from "@/features/agent/public/sessionView";
import { useEffect, useRef } from "react";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  captureComposerSubmission,
  replaceComposerDraft,
  useComposerText,
  useSetComposerText,
} from "@/features/composer/public/draft";
import { registerComposerFocusTarget } from "@/features/composer/public/focus";
import { selectAgentSession } from "@/features/agent/public/session";
import { writeToClipboard } from "@/lib/clipboard";
import { drainBrowserTasks } from "@/test/browserTasks";
import { MessageContextMenu } from "@/features/chat/message/ui/MessageContextMenu";
const material = vi.hoisted(() => ({ messages: [] as Message[], rollback: vi.fn() }));

vi.mock("@/features/agent/public/conversation", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/features/agent/public/conversation")>()),
  useActiveConversationMessages: () => material.messages,
}));

vi.mock("@/features/agent/public/session", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/features/agent/public/session")>()),
  activeAgentConversation: () => ({ sessionId: "menu-session", messages: material.messages }),
  rollbackSessionToBeforeRun: material.rollback,
}));

vi.mock("@/features/connection/public/capabilities", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/features/connection/public/capabilities")>()),
  runtimeCapability: () => true,
}));

vi.mock("@/lib/clipboard", () => ({ writeToClipboard: vi.fn().mockResolvedValue(true) }));

function buildMessage({ runId = null, ...overrides }: Partial<Message> = {}): Message {
  return {
    id: "m1",
    role: "user",
    createdAt: "2026-01-01T00:00:00.000Z",
    runId,
    blocks: [{ kind: "text", text: "hello world", status: "complete" }],
    ...overrides,
  };
}

function openMenu(triggerLabel: string): void {
  fireEvent.contextMenu(screen.getByText(triggerLabel));
}

function DraftInput() {
  const input = useRef<HTMLTextAreaElement>(null);
  const value = useComposerText();
  const setValue = useSetComposerText();
  useEffect(() => registerComposerFocusTarget(input.current), []);
  return (
    <textarea
      ref={input}
      aria-label="Composer draft"
      value={value}
      onChange={(event) => setValue(event.target.value)}
    />
  );
}

function mountFocusMenu() {
  const msg = buildMessage({ runId: "run-1" });
  material.messages = [msg];
  render(
    <>
      <button>Copy message</button>
      <MessageContextMenu msg={msg} label="You">
        <div>message body</div>
      </MessageContextMenu>
      <DraftInput />
      <button>Later control</button>
    </>,
  );
  return screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Composer draft" });
}

async function settleMenu() {
  await act(async () => {
    await drainBrowserTasks();
  });
}

async function openFocusMenu() {
  const origin = screen.getByRole("button", { name: "Copy message" });
  act(() => origin.focus());
  openMenu("message body");
  await settleMenu();
  return origin;
}

describe("messageContextMenu", () => {
  beforeEach(() => {
    material.messages = [];
    material.rollback.mockReset().mockResolvedValue({
      status: "committed",
      userInput: { parts: [{ kind: "text", text: "history edit" }] },
    });
    vi.mocked(writeToClipboard).mockClear();
    selectAgentSession("menu-session");
    replaceComposerDraft({ text: "waiting draft" });
  });
  afterEach(async () => {
    cleanup();
    await drainBrowserTasks();
  });

  it("shows Edit in composer + copy items for a user message", () => {
    render(
      <MessageContextMenu msg={buildMessage({ role: "user" })} label="You">
        <div>user message</div>
      </MessageContextMenu>,
    );
    openMenu("user message");
    expect(screen.getByText("Edit in composer")).toBeTruthy();
    expect(screen.getByText("Copy plain text")).toBeTruthy();
    expect(screen.queryByText("Regenerate response")).toBeNull();
  });

  it("shows Regenerate + copy items for an assistant message", () => {
    const assistant = buildMessage({ role: "assistant" });
    material.messages = [assistant];
    render(
      <MessageContextMenu msg={assistant} label="Assistant">
        <div>assistant message</div>
      </MessageContextMenu>,
    );
    openMenu("assistant message");
    expect(screen.getByText("Regenerate response")).toBeTruthy();
    expect(screen.getByText("Copy plain text")).toBeTruthy();
    expect(screen.queryByText("Edit in composer")).toBeNull();
  });

  it("names the menu after the displayed message author", async () => {
    render(
      <MessageContextMenu msg={buildMessage({ role: "user" })} label="You">
        <div>user message</div>
      </MessageContextMenu>,
    );
    openMenu("user message");
    expect(await screen.findByRole("menu", { name: "You" })).toBeTruthy();
  });

  it("does not show copy items when the message body is empty", () => {
    render(
      <MessageContextMenu
        msg={buildMessage({
          role: "user",
          blocks: [{ kind: "text", text: "", status: "complete" }],
        })}
        label="You"
      >
        <div>empty</div>
      </MessageContextMenu>,
    );
    openMenu("empty");
    expect(screen.queryByText("Copy markdown")).toBeNull();
    expect(screen.queryByText("Copy plain text")).toBeNull();
    expect(screen.queryByText("Edit in composer")).toBeNull();
  });

  it("shows Edit in composer for image-only user messages", () => {
    render(
      <MessageContextMenu
        msg={buildMessage({
          role: "user",
          blocks: [{ kind: "image", mime: "image/png", data: "abc" }],
        })}
        label="You"
      >
        <div>image only</div>
      </MessageContextMenu>,
    );
    openMenu("image only");
    expect(screen.getByText("Edit in composer")).toBeTruthy();
    expect(screen.queryByText("Copy plain text")).toBeNull();
  });

  it("Edit in composer loads the message text into composerStore", async () => {
    replaceComposerDraft({ text: "", images: [] });
    render(
      <MessageContextMenu
        msg={buildMessage({
          role: "user",
          blocks: [{ kind: "text", text: "draft me again", status: "complete" }],
        })}
        label="You"
      >
        <div>user msg</div>
      </MessageContextMenu>,
    );
    openMenu("user msg");
    fireEvent.click(screen.getByText("Edit in composer"));
    await settleMenu();
    expect(captureComposerSubmission().value).toBe("draft me again");
  });

  it.each([
    { label: "Edit in composer", text: "hello world", rollback: [] },
    {
      label: "Edit & rerun from here",
      text: "history edit",
      rollback: [["menu-session", "run-1", "history"]],
    },
    {
      label: "Edit & rerun, restore files",
      text: "history edit",
      rollback: [["menu-session", "run-1", "both"]],
    },
  ])("leaves focus in the composer after $label closes the menu", async (action) => {
    const draft = mountFocusMenu();
    await openFocusMenu();

    fireEvent.click(screen.getByRole("menuitem", { name: action.label }));
    await settleMenu();

    expect(screen.queryByRole("menu")).toBeNull();
    expect(draft.value).toBe(action.text);
    expect(document.activeElement).toBe(draft);
    expect(material.rollback.mock.calls).toEqual(action.rollback);
  });

  it.each(["Escape", "Copy plain text"])(
    "restores ordinary close focus after reopening and choosing %s",
    async (action) => {
      const draft = mountFocusMenu();
      await openFocusMenu();
      fireEvent.click(screen.getByRole("menuitem", { name: "Edit in composer" }));
      await settleMenu();
      expect(document.activeElement).toBe(draft);
      const origin = await openFocusMenu();

      if (action === "Escape") fireEvent.keyDown(screen.getByRole("menu"), { key: "Escape" });
      else fireEvent.click(screen.getByRole("menuitem", { name: action }));
      await settleMenu();

      expect(screen.queryByRole("menu")).toBeNull();
      expect(document.activeElement).toBe(origin);
      expect(writeToClipboard).toHaveBeenCalledTimes(action === "Escape" ? 0 : 1);
    },
  );

  it("keeps later outside focus when a delayed edit-and-rerun succeeds", async () => {
    const rollback = Promise.withResolvers<{
      status: "committed";
      userInput: { parts: [{ kind: "text"; text: string }] };
    }>();
    material.rollback.mockReturnValue(rollback.promise);
    const draft = mountFocusMenu();
    await openFocusMenu();
    fireEvent.click(screen.getByRole("menuitem", { name: "Edit & rerun from here" }));
    await settleMenu();
    expect(document.activeElement).toBe(draft);
    const outside = screen.getByRole("button", { name: "Later control" });
    act(() => outside.focus());

    await act(async () => {
      rollback.resolve({
        status: "committed",
        userInput: { parts: [{ kind: "text", text: "delayed history input" }] },
      });
      await rollback.promise;
    });

    expect(draft.value).toBe("delayed history input");
    expect(document.activeElement).toBe(outside);
  });
});
