import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import { SessionRow } from "./SessionRow";

vi.mock("@/ui", async (original) => ({
  ...(await original<typeof import("@/ui")>()),
  ContextMenu: {
    Root: ({ children }: { children: ReactNode }) => children,
    Trigger: ({ render: element }: { render: ReactNode }) => element,
    Content: ({ children }: { children: ReactNode }) => children,
    IconItem: ({ children, onSelect }: { children: ReactNode; onSelect: () => void }) => (
      <button onClick={onSelect}>{children}</button>
    ),
  },
}));

function edit() {
  const onRename = vi.fn();
  const onSelect = vi.fn();
  render(
    <SessionRow
      session={{
        id: "s",
        revision: 1,
        title: "Original",
        time: new Date().toISOString(),
        attention: "none",
        favorite: false,
      }}
      active
      onRename={onRename}
      onSelect={onSelect}
    />,
  );
  fireEvent.click(screen.getByRole("button", { name: "Rename" }));
  const input = screen.getByRole("textbox");
  fireEvent.change(input, { target: { value: "新的标题" } });
  return { input, onRename, onSelect };
}

describe("session title editing", () => {
  it("edits outside activation buttons and submits once with an explicit focus destination", async () => {
    const { input, onRename, onSelect } = edit();
    expect(input.closest("button")).toBeNull();
    fireEvent.keyDown(input, { key: "Enter" });
    fireEvent.blur(input);
    expect(onRename).toHaveBeenCalledExactlyOnceWith("s", 1, "新的标题");
    expect(onSelect).not.toHaveBeenCalled();
    await waitFor(() => expect(document.activeElement?.getAttribute("aria-current")).toBe("page"));
  });
  it("cancels without allowing blur to submit", () => {
    const { input, onRename } = edit();
    fireEvent.keyDown(input, { key: "Escape" });
    fireEvent.blur(input);
    expect(onRename).not.toHaveBeenCalled();
  });
  it("leaves IME confirmation inside the field", () => {
    const { input, onRename } = edit();
    fireEvent.keyDown(input, { key: "Enter", isComposing: true });
    fireEvent.keyDown(input, { key: "Enter", keyCode: 229 });
    expect(onRename).not.toHaveBeenCalled();
    expect(screen.getByRole("textbox")).toBe(input);
    fireEvent.blur(input);
    expect(onRename).toHaveBeenCalledExactlyOnceWith("s", 1, "新的标题");
  });
});
