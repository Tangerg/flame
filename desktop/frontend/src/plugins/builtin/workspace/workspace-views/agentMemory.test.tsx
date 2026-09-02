import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { AgentMemoryEntry } from "../application/agentMemoryConfig";
import { AgentMemoryTab } from "./agentMemory";

const model = vi.hoisted(() => ({
  addAgentMemory: vi.fn(async () => {}),
  updateAgentMemoryContent: vi.fn(async () => {}),
  reviewAgentMemory: vi.fn(async () => {}),
  setAgentMemoryPinned: vi.fn(async () => {}),
  deleteAgentMemory: vi.fn(async () => {}),
  items: [] as AgentMemoryEntry[],
}));

vi.mock("@/plugins/builtin/workspace/application/agentMemoryConfig", async (importOriginal) => ({
  ...(await importOriginal<
    typeof import("@/plugins/builtin/workspace/application/agentMemoryConfig")
  >()),
  addAgentMemory: model.addAgentMemory,
  updateAgentMemoryContent: model.updateAgentMemoryContent,
  reviewAgentMemory: model.reviewAgentMemory,
  setAgentMemoryPinned: model.setAgentMemoryPinned,
  deleteAgentMemory: model.deleteAgentMemory,
  useAgentMemory: () => ({ data: model.items, isLoading: false, isError: false }),
}));

vi.mock("@/plugins/builtin/agent/public/session", () => ({
  useActiveSessionWorkspace: () => ({ status: "ready", cwd: "/work/alpha" }),
}));

vi.mock("@/plugins/builtin/runtime/public/capabilities", () => ({
  useRuntimeCapability: () => true,
}));

function memory(overrides: Partial<AgentMemoryEntry> = {}): AgentMemoryEntry {
  return {
    id: "mem_0123456789abcdef0123456789abcdef",
    scope: "project",
    content: "Prefers pnpm",
    origin: "user",
    status: "active",
    pinned: false,
    sessionId: "",
    day: "",
    createdAt: "2026-08-12T12:00:00Z",
    updatedAt: "2026-08-12T12:00:00Z",
    ...overrides,
  };
}

beforeEach(() => {
  model.items = [memory()];
  for (const command of [
    model.addAgentMemory,
    model.updateAgentMemoryContent,
    model.reviewAgentMemory,
    model.setAgentMemoryPinned,
    model.deleteAgentMemory,
  ]) {
    command.mockClear();
  }
});

afterEach(cleanup);

function openEditor(): HTMLTextAreaElement {
  fireEvent.click(screen.getByRole("button", { name: "Edit memory" }));
  return screen.getByRole("textbox", { name: "Edit memory content" }) as HTMLTextAreaElement;
}

describe("Agent memory tab", () => {
  // The Runtime rejects blank content outright, and this view is the only thing that stops it
  // being sent — there is no second check between here and the wire.
  it("refuses to add a memory that is only whitespace", () => {
    render(<AgentMemoryTab />);

    fireEvent.click(screen.getByRole("button", { name: "Add memory" }));
    fireEvent.change(screen.getByRole("textbox", { name: "Add memory" }), {
      target: { value: "   " },
    });

    expect(screen.getByRole("button", { name: "Save" }).getAttribute("disabled")).not.toBeNull();
    expect(model.addAgentMemory).not.toHaveBeenCalled();
  });

  it("refuses to save an edit that empties an existing memory", () => {
    render(<AgentMemoryTab />);

    fireEvent.change(openEditor(), { target: { value: "  " } });

    expect(screen.getByRole("button", { name: "Save" }).getAttribute("disabled")).not.toBeNull();
    expect(model.updateAgentMemoryContent).not.toHaveBeenCalled();
  });

  it("keeps Save inert until an edit actually changes the memory", () => {
    render(<AgentMemoryTab />);
    const editor = openEditor();

    expect(screen.getByRole("button", { name: "Save" }).getAttribute("disabled")).not.toBeNull();

    fireEvent.change(editor, { target: { value: "Prefers bun" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    expect(model.updateAgentMemoryContent).toHaveBeenCalledWith(
      "mem_0123456789abcdef0123456789abcdef",
      "Prefers bun",
    );
  });

  it("adds a trimmed memory bound to the active workspace", async () => {
    render(<AgentMemoryTab />);

    fireEvent.click(screen.getByRole("button", { name: "Add memory" }));
    fireEvent.change(screen.getByRole("textbox", { name: "Add memory" }), {
      target: { value: "  Prefers bun  " },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await vi.waitFor(() =>
      expect(model.addAgentMemory).toHaveBeenCalledWith({
        scope: "project",
        cwd: "/work/alpha",
        content: "Prefers bun",
      }),
    );
  });

  it("reviews a mined memory in one decision per press", async () => {
    model.items = [memory({ status: "pending", origin: "auto" })];
    render(<AgentMemoryTab />);

    fireEvent.click(screen.getByRole("button", { name: "Approve" }));

    await vi.waitFor(() =>
      expect(model.reviewAgentMemory).toHaveBeenCalledWith(
        "mem_0123456789abcdef0123456789abcdef",
        "approve",
      ),
    );
    expect(model.reviewAgentMemory).toHaveBeenCalledOnce();
  });
});
