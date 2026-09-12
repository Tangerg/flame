import type { Message } from "@/plugins/sdk/types/agentSessionView";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import {
  getComposerText,
  replaceComposerDraft,
} from "@/plugins/builtin/chat/composer/public/draft";
import { drainBrowserTasks } from "@/test/browserTasks";
import { MessageContextMenu } from "./MessageContextMenu";

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

describe("messageContextMenu", () => {
  afterEach(async () => {
    cleanup();
    await drainBrowserTasks();
  });

  it("shows Edit in composer + copy items for a user message", () => {
    render(
      <MessageContextMenu msg={buildMessage({ role: "user" })}>
        <div>user message</div>
      </MessageContextMenu>,
    );
    openMenu("user message");
    expect(screen.getByText("Edit in composer")).toBeTruthy();
    expect(screen.getByText("Copy plain text")).toBeTruthy();
    expect(screen.queryByText("Regenerate response")).toBeNull();
  });

  it("shows Regenerate + copy items for an assistant message", () => {
    render(
      <MessageContextMenu msg={buildMessage({ role: "assistant" })}>
        <div>assistant message</div>
      </MessageContextMenu>,
    );
    openMenu("assistant message");
    expect(screen.getByText("Regenerate response")).toBeTruthy();
    expect(screen.getByText("Copy plain text")).toBeTruthy();
    expect(screen.queryByText("Edit in composer")).toBeNull();
  });

  it("does not show copy items when the message body is empty", () => {
    render(
      <MessageContextMenu
        msg={buildMessage({
          role: "user",
          blocks: [{ kind: "text", text: "", status: "complete" }],
        })}
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
      >
        <div>image only</div>
      </MessageContextMenu>,
    );
    openMenu("image only");
    expect(screen.getByText("Edit in composer")).toBeTruthy();
    expect(screen.queryByText("Copy plain text")).toBeNull();
  });

  it("Edit in composer loads the message text into composerStore", () => {
    replaceComposerDraft({ text: "", images: [] });
    render(
      <MessageContextMenu
        msg={buildMessage({
          role: "user",
          blocks: [{ kind: "text", text: "draft me again", status: "complete" }],
        })}
      >
        <div>user msg</div>
      </MessageContextMenu>,
    );
    openMenu("user msg");
    fireEvent.click(screen.getByText("Edit in composer"));
    expect(getComposerText()).toBe("draft me again");
  });
});
