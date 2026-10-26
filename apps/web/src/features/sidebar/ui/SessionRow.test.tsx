import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { type ReactNode } from "react";
import { type WorkSession } from "@/features/navigation/public/workIndex";
import { SessionRow } from "@/features/sidebar/ui/SessionRow";
vi.mock("@/ui/index", async (original) => ({
  ...(await original<typeof import("@/ui/index")>()),
  ContextMenu: {
    Root: ({ children }: { children: ReactNode }) => children,
    Trigger: ({ render: element }: { render: ReactNode }) => element,
    Content: ({ children }: { children: ReactNode }) => children,
    IconItem: ({ children, onSelect }: { children: ReactNode; onSelect: () => void }) => (
      <button onClick={onSelect}>{children}</button>
    ),
  },
}));

function edit(title = "Original") {
  const onRename = vi.fn();
  const onSelect = vi.fn();
  testClient.render(
    <SessionRow
      session={{
        id: "s",
        revision: 1,
        title,
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
  const input = screen.getByRole<HTMLInputElement>("textbox");
  const initial = input.value;
  fireEvent.change(input, { target: { value: "新的标题" } });
  return { input, initial, onRename, onSelect };
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
  it("opens an untitled Session's editor empty instead of with its display fallback", () => {
    const { initial } = edit("");
    expect(initial).toBe("");
  });
});

describe("session row pin state", () => {
  const now = Date.parse("2026-10-23T12:00:00Z");
  const session = (overrides: Partial<WorkSession>): WorkSession => ({
    id: "s-pinned",
    revision: 7,
    title: "Refine Runtime protocol",
    time: new Date(now - 5 * 60_000).toISOString(),
    attention: "none",
    favorite: false,
    ...overrides,
  });

  it("names a pinned active Session with its title, pin state, attention and time", () => {
    const clock = vi.spyOn(Date, "now").mockReturnValue(now);
    try {
      const onSelect = vi.fn();
      testClient.render(
        <SessionRow
          session={session({ attention: "running", favorite: true })}
          active
          onSelect={onSelect}
          onToggleFavorite={vi.fn()}
        />,
      );
      const row = screen.getByRole("button", {
        name: "Refine Runtime protocol — Pinned — Running · 5 minutes ago",
      });
      expect(row.getAttribute("aria-current")).toBe("page");
      within(row).getByRole("img", { name: "Running · 5 minutes ago" });
      screen.getByRole("button", { name: "More actions for Refine Runtime protocol" });
      fireEvent.click(row);
      expect(onSelect).toHaveBeenCalledExactlyOnceWith("s-pinned");
    } finally {
      clock.mockRestore();
    }
  });

  it("follows the canonical favorite projection without claiming a requested toggle", () => {
    const clock = vi.spyOn(Date, "now").mockReturnValue(now);
    try {
      const onToggleFavorite = vi.fn();
      const view = (overrides: Partial<WorkSession>) => (
        <SessionRow
          session={session(overrides)}
          active
          onSelect={vi.fn()}
          onToggleFavorite={onToggleFavorite}
        />
      );
      const unpinned = "Refine Runtime protocol — 5 minutes ago";
      const pinned = "Refine Runtime protocol — Pinned — 5 minutes ago";
      const { rerender } = testClient.render(view({}));
      const row = screen.getByRole("button", { name: unpinned });

      fireEvent.click(screen.getByRole("button", { name: "Pin to top" }));
      expect(onToggleFavorite).toHaveBeenCalledExactlyOnceWith("s-pinned", 7, true);
      expect(row.getAttribute("aria-label")).toBe(unpinned);
      expect(screen.queryByRole("button", { name: "Unpin" })).toBeNull();

      rerender(view({ revision: 8, favorite: true }));
      expect(row.getAttribute("aria-label")).toBe(pinned);
      expect(row.getAttribute("aria-current")).toBe("page");

      fireEvent.click(screen.getByRole("button", { name: "Unpin" }));
      expect(onToggleFavorite).toHaveBeenCalledTimes(2);
      expect(onToggleFavorite).toHaveBeenLastCalledWith("s-pinned", 8, false);
      expect(row.getAttribute("aria-label")).toBe(pinned);

      rerender(view({ revision: 9 }));
      expect(row.getAttribute("aria-label")).toBe(unpinned);
      expect(row.getAttribute("aria-current")).toBe("page");
    } finally {
      clock.mockRestore();
    }
  });
});
