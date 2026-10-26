import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { act, fireEvent, screen } from "@testing-library/react";
import { useState } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { AgentDockTabs, type AgentDockTab } from "@/ui/agent/context-dock";
let nativeScrollIntoView: typeof HTMLElement.prototype.scrollIntoView | undefined;
let frames: FrameRequestCallback[] = [];

beforeEach(() => {
  nativeScrollIntoView = HTMLElement.prototype.scrollIntoView;
  frames = [];
  vi.spyOn(window, "requestAnimationFrame").mockImplementation((callback) => {
    frames.push(callback);
    return frames.length;
  });
});

afterEach(() => {
  if (nativeScrollIntoView) HTMLElement.prototype.scrollIntoView = nativeScrollIntoView;
  else Reflect.deleteProperty(HTMLElement.prototype, "scrollIntoView");
  vi.restoreAllMocks();
});

function flushFrames() {
  const pending = frames;
  frames = [];
  act(() => pending.forEach((callback) => callback(performance.now())));
}

function DockHarness({ initial }: { initial: string[] }) {
  const [ids, setIds] = useState(initial);
  const [activeId, setActiveId] = useState(initial[0]);
  const [draft, setDraft] = useState("");
  const close = (id: string) => {
    const next = ids.filter((candidate) => candidate !== id);
    setIds(next);
    if (id === activeId) setActiveId(next[Math.max(0, ids.indexOf(id) - 1)]);
  };
  const dockTabs: AgentDockTab[] = ids.map((id) => ({
    id,
    title: id,
    active: id === activeId,
    onSelect: () => setActiveId(id),
    onClose: () => close(id),
    closeLabel: `Close ${id}`,
    onCloseOthers: () => {
      setIds([id]);
      setActiveId(id);
    },
    closeOthersLabel: `Close others than ${id}`,
  }));
  return (
    <>
      <div>
        <AgentDockTabs
          tabs={dockTabs}
          ariaLabel="Workspace tabs"
          reorder={{
            onMove: (id, toIndex) => {
              const next = ids.filter((candidate) => candidate !== id);
              next.splice(toIndex, 0, id);
              setIds(next);
            },
            moveLeftLabel: "Move left",
            moveRightLabel: "Move right",
          }}
        />
        <button type="button">Browse panels</button>
      </div>
      <output aria-label="Selected view">{activeId ?? "none"}</output>
      <textarea
        aria-label="Message"
        value={draft}
        onChange={(event) => setDraft(event.target.value)}
      />
    </>
  );
}

function rowOf(id: string): HTMLElement {
  return screen.getByRole("tab", { name: id }).parentElement!;
}

async function chooseFromTabMenu(id: string, item: string) {
  fireEvent.contextMenu(rowOf(id));
  fireEvent.click(await screen.findByRole("menuitem", { name: item }));
}

function tabs(activeId: string): AgentDockTab[] {
  return ["skills", "file", "diff", "tools", "plan", "timeline"].map((id) => ({
    id,
    title: id,
    active: id === activeId,
    onClose: () => {},
    closeLabel: `Close ${id}`,
  }));
}

describe("AgentDockTabs", () => {
  it("brings a newly active overflow tab into the visible strip", () => {
    const scrollIntoView = vi.fn();
    HTMLElement.prototype.scrollIntoView = scrollIntoView;
    const view = testClient.render(
      <AgentDockTabs tabs={tabs("skills")} ariaLabel="Workspace tabs" />,
    );
    scrollIntoView.mockClear();

    view.rerender(<AgentDockTabs tabs={tabs("timeline")} ariaLabel="Workspace tabs" />);

    expect(scrollIntoView).toHaveBeenCalledOnce();
    expect(scrollIntoView).toHaveBeenCalledWith({ block: "nearest", inline: "nearest" });
  });

  it("reveals the same active tab after reordering without overriding later strip browsing", () => {
    const initial = tabs("skills");
    const view = testClient.render(<AgentDockTabs tabs={initial} ariaLabel="Workspace tabs" />);
    const strip = view.container.querySelector<HTMLElement>(".agent-dock-tabs")!;
    const active = screen.getByRole("tab", { name: "skills" });
    let offset = 0;
    // Happy DOM has no layout; these are the strip's external geometry and scroll bounds.
    Object.defineProperties(strip, {
      clientWidth: { configurable: true, value: 300 },
      scrollWidth: { configurable: true, value: 600 },
      scrollLeft: {
        configurable: true,
        get: () => offset,
        set: (value: number) => {
          offset = Math.max(0, Math.min(300, value));
        },
      },
    });
    strip.style.scrollPaddingInlineStart = "16px";
    vi.spyOn(strip, "getBoundingClientRect").mockReturnValue(new DOMRect(0, 0, 300, 28));
    for (const tab of screen.getAllByRole("tab")) {
      vi.spyOn(tab.parentElement!, "getBoundingClientRect").mockImplementation(() => {
        const position = Array.from(strip.querySelectorAll('[role="tab"]')).indexOf(tab);
        return new DOMRect(position * 100 - offset, 0, 100, 28);
      });
    }
    act(() => active.focus());
    const reordered = [...initial.slice(1), initial[0]!];

    view.rerender(<AgentDockTabs tabs={reordered} ariaLabel="Workspace tabs" />);

    expect(document.activeElement).toBe(active);
    expect(active.getAttribute("aria-selected")).toBe("true");
    const row = active.parentElement!.getBoundingClientRect();
    expect(row.left).toBeGreaterThanOrEqual(0);
    expect(row.right).toBeLessThanOrEqual(strip.clientWidth);

    strip.scrollLeft = 52;
    fireEvent.scroll(strip);
    view.rerender(<AgentDockTabs tabs={[...reordered]} ariaLabel="Workspace tabs" />);

    expect(strip.scrollLeft).toBe(52);
  });

  it("closes a tab on middle click without selecting it", () => {
    const onClose = vi.fn();
    const onSelect = vi.fn();
    const view = testClient.render(
      <AgentDockTabs
        tabs={[{ id: "diff", title: "diff", active: true, onSelect, onClose, closeLabel: "Close" }]}
        ariaLabel="Workspace tabs"
      />,
    );

    fireEvent(
      view.getByText("diff"),
      new MouseEvent("auxclick", { bubbles: true, cancelable: true, button: 1 }),
    );

    expect(onClose).toHaveBeenCalledOnce();
    expect(onSelect).not.toHaveBeenCalled();
  });

  it("reports the drop position when a tab is dragged onto a sibling", () => {
    const onReorder = vi.fn();
    const view = testClient.render(
      <AgentDockTabs
        tabs={tabs("skills")}
        ariaLabel="Workspace tabs"
        reorder={{ onMove: onReorder, moveLeftLabel: "Move left", moveRightLabel: "Move right" }}
      />,
    );
    const rows = view.container.querySelectorAll<HTMLElement>("[draggable=true]");
    const data = new Map<string, string>();
    const dataTransfer = {
      effectAllowed: "",
      dropEffect: "",
      setData: (format: string, value: string) => data.set(format, value),
      getData: (format: string) => data.get(format) ?? "",
    };

    fireEvent.dragStart(rows[0]!, { dataTransfer });
    fireEvent.drop(rows[3]!, { dataTransfer });

    expect(onReorder).toHaveBeenCalledWith("skills", 3);
  });

  it("leaves a single-tab strip undraggable", () => {
    const view = testClient.render(
      <AgentDockTabs
        tabs={[
          { id: "diff", title: "diff", active: true, onClose: () => {}, closeLabel: "Close diff" },
        ]}
        ariaLabel="Workspace tabs"
        reorder={{ onMove: vi.fn(), moveLeftLabel: "Move left", moveRightLabel: "Move right" }}
      />,
    );

    expect(view.container.querySelector("[draggable=true]")).toBeNull();
  });

  it("exposes which overflow edges still contain hidden tabs", () => {
    const view = testClient.render(
      <AgentDockTabs tabs={tabs("skills")} ariaLabel="Workspace tabs" />,
    );
    const strip = view.container.querySelector<HTMLElement>(".agent-dock-tabs")!;
    Object.defineProperties(strip, {
      clientWidth: { configurable: true, value: 300 },
      scrollWidth: { configurable: true, value: 600 },
    });

    strip.scrollLeft = 0;
    fireEvent.scroll(strip);
    expect(strip.hasAttribute("data-overflow-start")).toBe(false);
    expect(strip.hasAttribute("data-overflow-end")).toBe(true);

    strip.scrollLeft = 120;
    fireEvent.scroll(strip);
    expect(strip.hasAttribute("data-overflow-start")).toBe(true);
    expect(strip.hasAttribute("data-overflow-end")).toBe(true);

    strip.scrollLeft = 300;
    fireEvent.scroll(strip);
    expect(strip.hasAttribute("data-overflow-start")).toBe(true);
    expect(strip.hasAttribute("data-overflow-end")).toBe(false);
  });
});

describe("AgentDockTabs focus continuity", () => {
  it("middle-closes a background tab without interrupting the composer", () => {
    testClient.render(<DockHarness initial={["files", "diff", "plan"]} />);
    const composer = screen.getByRole("textbox", { name: "Message" });
    composer.focus();
    fireEvent.change(composer, { target: { value: "修复 dock focus 草稿" } });

    const pressTookFocus = fireEvent.mouseDown(rowOf("diff"), { button: 1 });
    fireEvent(
      screen.getByRole("tab", { name: "diff" }),
      new MouseEvent("auxclick", { bubbles: true, cancelable: true, button: 1 }),
    );
    flushFrames();

    expect(screen.queryByRole("tab", { name: "diff" })).toBeNull();
    expect(screen.getByRole("tab", { name: "files" }).getAttribute("aria-selected")).toBe("true");
    expect(screen.getByRole("status", { name: "Selected view" }).textContent).toBe("files");
    expect(document.activeElement).toBe(composer);
    expect((composer as HTMLTextAreaElement).value).toBe("修复 dock focus 草稿");
    expect(pressTookFocus).toBe(false);
  });

  it("leaves a primary press free to focus its tab", () => {
    testClient.render(<DockHarness initial={["files", "diff"]} />);

    expect(fireEvent.mouseDown(screen.getByRole("tab", { name: "diff" }), { button: 0 })).toBe(
      true,
    );
  });

  it("settles focus on the surviving active tab when the focused tab closes", () => {
    testClient.render(<DockHarness initial={["files", "diff", "plan"]} />);
    fireEvent.click(screen.getByRole("tab", { name: "diff" }));
    screen.getByRole("tab", { name: "diff" }).focus();

    fireEvent.keyDown(screen.getByRole("tab", { name: "diff" }), { key: "Delete" });
    flushFrames();

    expect(screen.queryByRole("tab", { name: "diff" })).toBeNull();
    expect(document.activeElement).toBe(screen.getByRole("tab", { name: "files" }));
    expect(screen.getByRole("tab", { name: "files" }).getAttribute("aria-selected")).toBe("true");
  });

  it("falls back to the header action when the last tab closes", () => {
    testClient.render(<DockHarness initial={["files"]} />);
    screen.getByRole("button", { name: "Close files" }).focus();

    fireEvent.click(screen.getByRole("button", { name: "Close files" }));
    flushFrames();

    expect(screen.queryByRole("tab")).toBeNull();
    expect(document.activeElement).toBe(screen.getByRole("button", { name: "Browse panels" }));
  });

  it("respects focus the user moves elsewhere before a queued close settles", () => {
    testClient.render(<DockHarness initial={["files", "diff"]} />);
    screen.getByRole("tab", { name: "files" }).focus();

    fireEvent.keyDown(screen.getByRole("tab", { name: "files" }), { key: "Backspace" });
    screen.getByRole("textbox", { name: "Message" }).focus();
    flushFrames();

    expect(screen.queryByRole("tab", { name: "files" })).toBeNull();
    expect(document.activeElement).toBe(screen.getByRole("textbox", { name: "Message" }));
  });

  it("returns focus from the menu to the surviving active tab after closing others", async () => {
    testClient.render(<DockHarness initial={["files", "diff", "plan"]} />);

    await chooseFromTabMenu("plan", "Close others than plan");
    flushFrames();

    expect(screen.getAllByRole("tab").map((tab) => tab.textContent)).toEqual(["plan"]);
    expect(document.activeElement).toBe(screen.getByRole("tab", { name: "plan" }));
  });

  it("names the menu after the inactive tab it was opened on", async () => {
    testClient.render(<DockHarness initial={["files", "diff", "plan"]} />);

    fireEvent.contextMenu(rowOf("plan"));

    expect(await screen.findByRole("menu", { name: "plan" })).toBeTruthy();
    expect(screen.queryByRole("menu", { name: "files" })).toBeNull();
  });

  it("keeps the moved tab focused after a menu reorder", async () => {
    testClient.render(<DockHarness initial={["files", "diff", "plan"]} />);

    await chooseFromTabMenu("files", "Move right");
    flushFrames();

    expect(screen.getAllByRole("tab").map((tab) => tab.textContent)).toEqual([
      "diff",
      "files",
      "plan",
    ]);
    expect(document.activeElement).toBe(screen.getByRole("tab", { name: "files" }));
  });

  it("respects focus the user moves elsewhere before a queued move settles", async () => {
    testClient.render(<DockHarness initial={["files", "diff", "plan"]} />);

    await chooseFromTabMenu("files", "Move right");
    screen.getByRole("textbox", { name: "Message" }).focus();
    flushFrames();

    expect(document.activeElement).toBe(screen.getByRole("textbox", { name: "Message" }));
  });

  it("preserves a different tab selected before a queued move settles", async () => {
    testClient.render(<DockHarness initial={["files", "diff", "plan"]} />);

    await chooseFromTabMenu("files", "Move right");
    const chosen = screen.getByRole("tab", { name: "plan" });
    fireEvent.click(chosen);
    chosen.focus();
    flushFrames();

    expect(document.activeElement).toBe(chosen);
    expect(chosen.getAttribute("aria-selected")).toBe("true");
    expect(screen.getByRole("status", { name: "Selected view" }).textContent).toBe("plan");
  });

  it("does not focus a moved tab that was retired before the move settled", async () => {
    testClient.render(<DockHarness initial={["files", "diff", "plan"]} />);
    const moved = screen.getByRole("tab", { name: "files" });

    await chooseFromTabMenu("files", "Move right");
    fireEvent.keyDown(screen.getByRole("tab", { name: "files" }), { key: "Delete" });
    flushFrames();

    expect(moved.isConnected).toBe(false);
    expect(screen.queryByRole("tab", { name: "files" })).toBeNull();
    expect(document.activeElement).toBe(screen.getByRole("tab", { name: "diff" }));
  });

  it("ignores Delete and Backspace that an IME or an earlier handler consumed", () => {
    testClient.render(<DockHarness initial={["files", "diff"]} />);
    const tab = screen.getByRole("tab", { name: "files" });
    tab.focus();

    fireEvent.keyDown(tab, { key: "Backspace", isComposing: true });
    fireEvent.keyDown(tab, { key: "Delete", keyCode: 229 });
    const consume = (event: Event) => event.preventDefault();
    tab.addEventListener("keydown", consume);
    fireEvent.keyDown(tab, { key: "Delete" });
    tab.removeEventListener("keydown", consume);
    flushFrames();

    expect(screen.getAllByRole("tab").map((element) => element.textContent)).toEqual([
      "files",
      "diff",
    ]);
    expect(document.activeElement).toBe(tab);

    fireEvent.keyDown(tab, { key: "Delete" });
    flushFrames();

    expect(screen.queryByRole("tab", { name: "files" })).toBeNull();
  });
});
