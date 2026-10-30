import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { act, fireEvent, screen, waitFor, within } from "@testing-library/react";
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
  const onRename = vi.fn().mockResolvedValue(true);
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
  it("admits Enter and blur once while a title is saving and keeps a refused draft editable", async () => {
    const pending = Promise.withResolvers<boolean>();
    const onRename = vi.fn(() => pending.promise);
    testClient.render(
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
        onSelect={vi.fn()}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "Rename" }));
    const input = screen.getByRole<HTMLInputElement>("textbox");
    fireEvent.change(input, { target: { value: "Draft while saving" } });
    await act(async () => {
      fireEvent.keyDown(input, { key: "Enter" });
      fireEvent.blur(input);
      fireEvent.keyDown(input, { key: "Enter" });
    });
    expect(onRename).toHaveBeenCalledExactlyOnceWith("s", 1, "Draft while saving");
    expect(input.readOnly).toBe(true);
    fireEvent.keyDown(input, { key: "Escape" });
    expect(screen.getByRole("textbox")).toBe(input);
    await act(async () => {
      pending.resolve(false);
    });
    expect(input.readOnly).toBe(false);
    expect(input.value).toBe("Draft while saving");
    expect(input.getAttribute("aria-invalid")).toBe("true");
    expect(
      document.getElementById(input.getAttribute("aria-describedby")!)?.getAttribute("role"),
    ).toBe("alert");
    fireEvent.blur(input);
    expect(onRename).toHaveBeenCalledOnce();
    fireEvent.keyDown(input, { key: "Escape" });
    expect(screen.queryByRole("textbox")).toBeNull();
  });

  it("keeps the opened target when the row receives a newer revision", async () => {
    const onRename = vi.fn().mockResolvedValue(true);
    const row = (revision: number, title: string) => (
      <SessionRow
        session={{
          id: "s",
          revision,
          title,
          time: new Date().toISOString(),
          attention: "none",
          favorite: false,
        }}
        active
        onRename={onRename}
        onSelect={vi.fn()}
      />
    );
    const view = testClient.render(row(1, "Original"));
    fireEvent.click(screen.getByRole("button", { name: "Rename" }));
    const input = screen.getByRole<HTMLInputElement>("textbox");
    fireEvent.change(input, { target: { value: "Local draft" } });
    view.rerender(row(2, "Remote title"));
    await act(async () => {
      fireEvent.keyDown(input, { key: "Enter" });
    });
    expect(onRename).toHaveBeenCalledExactlyOnceWith("s", 1, "Local draft");
  });

  it("does not submit an untouched original title over a remote update on blur", () => {
    const onRename = vi.fn().mockResolvedValue(true);
    const row = (revision: number, title: string) => (
      <SessionRow
        session={{
          id: "s",
          revision,
          title,
          time: new Date().toISOString(),
          attention: "none",
          favorite: false,
        }}
        active
        onRename={onRename}
        onSelect={vi.fn()}
      />
    );
    const view = testClient.render(row(1, "Original"));
    fireEvent.click(screen.getByRole("button", { name: "Rename" }));
    const input = screen.getByRole<HTMLInputElement>("textbox");
    view.rerender(row(2, "Remote title"));
    fireEvent.blur(input);
    expect(onRename).not.toHaveBeenCalled();
  });

  it("keeps a refused draft until the user explicitly cancels and opens a new edit", async () => {
    const onRename = vi.fn().mockResolvedValueOnce(false).mockResolvedValueOnce(true);
    const row = (revision: number, title: string) => (
      <SessionRow
        session={{
          id: "s",
          revision,
          title,
          time: new Date().toISOString(),
          attention: "none",
          favorite: false,
        }}
        active
        onRename={onRename}
        onSelect={vi.fn()}
      />
    );
    const view = testClient.render(row(1, "Original"));
    fireEvent.click(screen.getByRole("button", { name: "Rename" }));
    const input = screen.getByRole<HTMLInputElement>("textbox");
    fireEvent.change(input, { target: { value: "Local draft" } });
    view.rerender(row(2, "Remote title"));
    await act(async () => {
      fireEvent.keyDown(input, { key: "Enter" });
    });
    await screen.findByRole("alert");
    expect(screen.getByRole<HTMLInputElement>("textbox").value).toBe("Local draft");
    expect(onRename).toHaveBeenCalledExactlyOnceWith("s", 1, "Local draft");
    fireEvent.keyDown(input, { key: "Escape" });
    fireEvent.click(screen.getByRole("button", { name: "Rename" }));
    const next = screen.getByRole<HTMLInputElement>("textbox");
    expect(next.value).toBe("Remote title");
    fireEvent.change(next, { target: { value: "New intent" } });
    await act(async () => {
      fireEvent.keyDown(next, { key: "Enter" });
    });
    await waitFor(() => expect(onRename).toHaveBeenLastCalledWith("s", 2, "New intent"));
  });

  it("edits outside activation buttons and submits once with an explicit focus destination", async () => {
    const { input, onRename, onSelect } = edit();
    expect(input.closest("button")).toBeNull();
    await act(async () => {
      fireEvent.keyDown(input, { key: "Enter" });
      fireEvent.blur(input);
    });
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
  it("leaves IME confirmation inside the field", async () => {
    const { input, onRename } = edit();
    fireEvent.keyDown(input, { key: "Enter", isComposing: true });
    fireEvent.keyDown(input, { key: "Enter", keyCode: 229 });
    expect(onRename).not.toHaveBeenCalled();
    expect(screen.getByRole("textbox")).toBe(input);
    await act(async () => {
      fireEvent.blur(input);
    });
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
