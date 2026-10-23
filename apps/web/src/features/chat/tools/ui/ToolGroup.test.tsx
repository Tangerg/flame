import { act, fireEvent, render, screen } from "@testing-library/react";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";
import { type ToolCall } from "@/features/agent/public/sessionView";
import { ToolGroup } from "@/features/chat/tools/ui/ToolGroup";
const read: ToolCall = {
  id: "read",
  runId: "r1",
  name: "read",
  fn: "file.ts",
  args: "",
  status: "ok",
  result: "export const answer = 42;",
};
const searching: ToolCall = {
  id: "search",
  runId: "r1",
  name: "grep",
  fn: "pattern",
  args: "",
  status: "running",
};
const searched: ToolCall = { ...searching, status: "ok", result: "file.ts:1" };

function Turn({ tools, superseded }: { tools: ToolCall[]; superseded?: boolean }) {
  const [expandedIds, setExpandedIds] = useState<Set<string>>(() => new Set());
  const toggle = (id: string) =>
    setExpandedIds((current) => {
      const next = new Set(current);
      if (!next.delete(id)) next.add(id);
      return next;
    });
  return (
    <>
      <ToolGroup
        tools={tools}
        expandedIds={expandedIds}
        onToggleExpand={toggle}
        superseded={superseded}
      />
      <button type="button">Outside</button>
    </>
  );
}

function renderTurn() {
  const view = render(<Turn tools={[read, searching]} />);
  const settle = () => view.rerender(<Turn tools={[read, searched]} />);
  return { ...view, settle };
}

const groupHeader = () => screen.getByRole("button", { name: /2 calls/ });

function member(container: HTMLElement, name: string): HTMLElement {
  const row = container.querySelector<HTMLElement>(`button[data-tool="${name}"]`);
  if (!row) throw new Error(`missing ${name} row`);
  return row;
}

function activate(element: HTMLElement) {
  act(() => element.focus());
  fireEvent.click(element);
}

describe("ToolGroup", () => {
  it("keeps a focused result reader open and focused when the group settles and is superseded", () => {
    const { container, settle, rerender } = renderTurn();
    expect(groupHeader().getAttribute("aria-expanded")).toBe("true");
    activate(member(container, "read"));
    const copy = screen.getByRole("button", { name: "Copy code" });
    act(() => copy.focus());

    settle();

    expect(groupHeader().getAttribute("aria-expanded")).toBe("true");
    expect(screen.getByRole("region", { name: "Result" }).textContent).toBe(read.result);
    expect(document.activeElement).toBe(copy);

    rerender(<Turn tools={[read, searched]} superseded />);

    expect(groupHeader().getAttribute("aria-expanded")).toBe("true");
    expect(screen.getByRole("region", { name: "Result" }).textContent).toBe(read.result);
    expect(document.activeElement).toBe(copy);
  });

  it("keeps a focused member row reachable without an expanded preview", () => {
    const { container, settle } = renderTurn();
    const row = member(container, "read");
    act(() => row.focus());

    settle();

    expect(groupHeader().getAttribute("aria-expanded")).toBe("true");
    expect(row.isConnected).toBe(true);
    expect(document.activeElement).toBe(row);
  });

  it("keeps an engaged group open after focus moves outside", () => {
    const { container, settle } = renderTurn();
    activate(member(container, "read"));
    const outside = screen.getByRole("button", { name: "Outside" });
    act(() => outside.focus());

    settle();

    expect(groupHeader().getAttribute("aria-expanded")).toBe("true");
    expect(screen.getByRole("region", { name: "Result" }).textContent).toBe(read.result);
    expect(document.activeElement).toBe(outside);
  });

  it("keeps sibling rows available after closing the last child preview", () => {
    const { container, settle } = renderTurn();
    const row = member(container, "read");
    activate(row);
    activate(row);
    expect(screen.queryByRole("region", { name: "Result" })).toBeNull();

    settle();

    expect(groupHeader().getAttribute("aria-expanded")).toBe("true");
    expect(member(container, "grep").isConnected).toBe(true);
    expect(document.activeElement).toBe(row);
  });

  it("lets a manual group close win over earlier engagement", () => {
    const { container, settle } = renderTurn();
    activate(member(container, "read"));
    activate(groupHeader());
    expect(groupHeader().getAttribute("aria-expanded")).toBe("false");

    settle();

    expect(groupHeader().getAttribute("aria-expanded")).toBe("false");
    activate(groupHeader());
    expect(groupHeader().getAttribute("aria-expanded")).toBe("true");
  });

  it("still collapses automatically when untouched or only its header was focused", () => {
    for (const focusHeader of [false, true]) {
      const { settle, unmount } = renderTurn();
      if (focusHeader) act(() => groupHeader().focus());

      settle();

      expect(groupHeader().getAttribute("aria-expanded")).toBe("false");
      unmount();
    }
  });

  it("updates the folded summary as a call settles without overriding the user's fold", () => {
    const calls: ToolCall[] = [
      { id: "read", runId: "r1", name: "read", fn: "file.ts", args: "", status: "running" },
      { id: "search", runId: "r1", name: "grep", fn: "pattern", args: "", status: "ok" },
    ];
    const group = () => (
      <ToolGroup tools={calls} expandedIds={new Set()} onToggleExpand={vi.fn()} />
    );
    const { rerender } = render(group());
    const summary = screen.getByRole("button", { name: /1 running.*2 calls/ });
    expect(summary.getAttribute("aria-expanded")).toBe("true");

    fireEvent.click(summary);
    calls[0] = { ...calls[0]!, status: "err" };
    rerender(group());

    expect(summary.getAttribute("aria-expanded")).toBe("false");
    expect(screen.getByRole("button", { name: /1 issue.*2 calls/ })).toBe(summary);
    expect(screen.queryByText("1 running")).toBeNull();
  });
});
