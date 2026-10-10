import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { AgentMemoryEntry } from "@/plugins/builtin/workspace/public/memory";
import { MemoryActions } from "./MemoryActions";
const model = vi.hoisted(() => ({
  addAgentMemory: vi.fn(async () => {}),
  updateAgentMemoryContent: vi.fn(async () => {}),
  reviewAgentMemory: vi.fn(async () => {}),
  setAgentMemoryPinned: vi.fn(async () => {}),
  deleteAgentMemory: vi.fn(async () => {}),
  items: [] as AgentMemoryEntry[],
  error: undefined as Error | undefined,
}));
vi.mock("@/plugins/builtin/workspace/public/memory", () => ({
  ...model,
  useAgentMemory: () => ({
    data: model.items,
    isLoading: false,
    error: model.error,
    refetch: vi.fn(),
  }),
}));
const id = "mem_0123456789abcdef0123456789abcdef";
beforeEach(() => {
  model.error = undefined;
  model.items = [
    {
      id,
      scope: "project",
      content: "Prefers pnpm",
      origin: "user",
      status: "active",
      pinned: false,
      createdAt: "2026-08-12T12:00:00Z",
      updatedAt: "2026-08-12T12:00:00Z",
    },
  ];
  vi.clearAllMocks();
});
afterEach(cleanup);
function fixture() {
  const saved = vi.fn();
  render(
    <MemoryActions
      scope="project"
      cwd="/work/alpha"
      signal={new AbortController().signal}
      onSaved={saved}
    />,
  );
  return saved;
}
function select() {
  fireEvent.click(screen.getByRole("button", { name: "Select memory to manage" }));
  fireEvent.click(screen.getByRole("menuitem", { name: new RegExp(id) }));
}
it("keeps whitespace drafts out of Runtime commands", () => {
  fixture();
  fireEvent.click(screen.getByRole("button", { name: "Add memory" }));
  fireEvent.change(screen.getByRole("textbox", { name: "Add memory" }), {
    target: { value: "  " },
  });
  expect(screen.getByRole("button", { name: "Save" }).hasAttribute("disabled")).toBe(true);
  expect(model.addAgentMemory).not.toHaveBeenCalled();
});
it("captures the target and refreshes only after the Runtime command settles", async () => {
  let resolve!: () => void;
  model.addAgentMemory.mockImplementationOnce(() => new Promise<void>((done) => (resolve = done)));
  const saved = fixture();
  fireEvent.click(screen.getByRole("button", { name: "Add memory" }));
  fireEvent.change(screen.getByRole("textbox", { name: "Add memory" }), {
    target: { value: " durable fact " },
  });
  fireEvent.click(screen.getByRole("button", { name: "Save" }));
  expect(model.addAgentMemory).toHaveBeenCalledWith({
    scope: "project",
    cwd: "/work/alpha",
    content: "durable fact",
  });
  expect(saved).not.toHaveBeenCalled();
  resolve();
  await waitFor(() => expect(saved).toHaveBeenCalledOnce());
});
it("uses canonical IDs for edit, pin and delete", async () => {
  const saved = fixture();
  select();
  fireEvent.click(screen.getByRole("button", { name: "Edit memory" }));
  fireEvent.change(screen.getByRole("textbox", { name: "Edit memory content" }), {
    target: { value: "edited" },
  });
  fireEvent.click(screen.getByRole("button", { name: "Save" }));
  await waitFor(() => expect(model.updateAgentMemoryContent).toHaveBeenCalledWith(id, "edited"));
  await waitFor(() => expect(saved).toHaveBeenCalledTimes(1));
  fireEvent.click(screen.getByRole("button", { name: "Pin" }));
  await waitFor(() => expect(model.setAgentMemoryPinned).toHaveBeenCalledWith(id, true));
  await waitFor(() => expect(saved).toHaveBeenCalledTimes(2));
  fireEvent.click(screen.getByRole("button", { name: "Delete memory" }));
  await waitFor(() => expect(model.deleteAgentMemory).toHaveBeenCalledWith(id));
});
it.each(["approve", "reject"] as const)(
  "reviews pending memory through the trusted owner: %s",
  async (decision) => {
    model.items[0]!.status = "pending";
    model.items[0]!.origin = "auto";
    fixture();
    select();
    fireEvent.click(
      screen.getByRole("button", { name: decision === "approve" ? "Approve" : "Reject" }),
    );
    await waitFor(() => expect(model.reviewAgentMemory).toHaveBeenCalledWith(id, decision));
  },
);
it("preserves a rejected draft without refreshing the plugin", async () => {
  model.addAgentMemory.mockRejectedValueOnce(new Error("quota reached"));
  const saved = fixture();
  fireEvent.click(screen.getByRole("button", { name: "Add memory" }));
  fireEvent.change(screen.getByRole("textbox", { name: "Add memory" }), {
    target: { value: "fact" },
  });
  fireEvent.click(screen.getByRole("button", { name: "Save" }));
  await screen.findByText("quota reached");
  expect(saved).not.toHaveBeenCalled();
  expect((screen.getByRole("textbox", { name: "Add memory" }) as HTMLTextAreaElement).value).toBe(
    "fact",
  );
});
it("makes read failure explicit and refuses to operate on stale list items", () => {
  model.error = new Error("memory read failed");
  fixture();
  expect(screen.getByText("memory read failed")).toBeTruthy();
  expect(
    screen.getByRole("button", { name: "Select memory to manage" }).hasAttribute("disabled"),
  ).toBe(true);
});
