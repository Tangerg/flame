import { fireEvent, render } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { AgentDockTabs, type AgentDockTab } from "./context-dock";

let nativeScrollIntoView: typeof HTMLElement.prototype.scrollIntoView | undefined;

beforeEach(() => {
  nativeScrollIntoView = HTMLElement.prototype.scrollIntoView;
});

afterEach(() => {
  if (nativeScrollIntoView) HTMLElement.prototype.scrollIntoView = nativeScrollIntoView;
  else Reflect.deleteProperty(HTMLElement.prototype, "scrollIntoView");
});

function tabs(activeId: string): AgentDockTab[] {
  return ["explorer", "file", "diff", "terminal", "plan", "timeline"].map((id) => ({
    id,
    title: id,
    active: id === activeId,
  }));
}

describe("AgentDockTabs", () => {
  it("brings a newly active overflow tab into the visible strip", () => {
    const scrollIntoView = vi.fn();
    HTMLElement.prototype.scrollIntoView = scrollIntoView;
    const view = render(<AgentDockTabs tabs={tabs("explorer")} ariaLabel="Workspace tabs" />);
    scrollIntoView.mockClear();

    view.rerender(<AgentDockTabs tabs={tabs("timeline")} ariaLabel="Workspace tabs" />);

    expect(scrollIntoView).toHaveBeenCalledOnce();
    expect(scrollIntoView).toHaveBeenCalledWith({ block: "nearest", inline: "nearest" });
  });

  it("closes a tab on middle click without selecting it", () => {
    const onClose = vi.fn();
    const onSelect = vi.fn();
    const view = render(
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
    const view = render(
      <AgentDockTabs tabs={tabs("explorer")} ariaLabel="Workspace tabs" onReorder={onReorder} />,
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

    expect(onReorder).toHaveBeenCalledWith("explorer", 3);
  });

  it("leaves a single-tab strip undraggable", () => {
    const view = render(
      <AgentDockTabs
        tabs={[{ id: "diff", title: "diff", active: true }]}
        ariaLabel="Workspace tabs"
        onReorder={vi.fn()}
      />,
    );

    expect(view.container.querySelector("[draggable=true]")).toBeNull();
  });

  it("exposes which overflow edges still contain hidden tabs", () => {
    const view = render(<AgentDockTabs tabs={tabs("explorer")} ariaLabel="Workspace tabs" />);
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
