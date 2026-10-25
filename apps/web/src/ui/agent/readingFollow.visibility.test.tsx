import { useReadingFocusVisibility } from "./readingFollow";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { StickToBottomInstance } from "use-stick-to-bottom";

function scenario() {
  const observers: Array<{ nodes: Set<Element>; callback: ResizeObserverCallback }> = [];
  vi.stubGlobal(
    "ResizeObserver",
    class {
      nodes = new Set<Element>();
      constructor(callback: ResizeObserverCallback) {
        observers.push({ nodes: this.nodes, callback });
      }
      observe(node: Element) {
        this.nodes.add(node);
      }
      unobserve(node: Element) {
        this.nodes.delete(node);
      }
      disconnect() {
        this.nodes.clear();
      }
    },
  );
  const ref = () => {
    const callback = (node: HTMLElement | null) => {
      callback.current = node;
    };
    callback.current = null as HTMLElement | null;
    return callback;
  };
  const stop = vi.fn();
  const follow = {
    scrollRef: ref(),
    contentRef: ref(),
    state: { isAtBottom: false },
    stopScroll: stop,
  } as unknown as Pick<StickToBottomInstance, "scrollRef" | "contentRef" | "state" | "stopScroll">;
  const shape = { height: 400, top: 210, targetHeight: 24, rendered: true };
  function Harness({ attached = true }: { attached?: boolean }) {
    useReadingFocusVisibility(follow, attached);
    return (
      <>
        <input aria-label="Later draft" defaultValue="Keep this draft" />
        <div ref={attached ? follow.scrollRef : undefined}>
          <div ref={attached ? follow.contentRef : undefined}>
            <div data-testid="nested">
              <button>Read original evidence</button>
            </div>
          </div>
        </div>
      </>
    );
  }
  const view = render(<Harness />);
  const viewport = follow.scrollRef.current!;
  const content = follow.contentRef.current!;
  const target = screen.getByRole("button", { name: "Read original evidence" });
  const nested = screen.getByTestId("nested");
  viewport.style.scrollPaddingTop = "20px";
  viewport.style.scrollPaddingBottom = "80px";
  vi.spyOn(viewport, "getBoundingClientRect").mockImplementation(
    () => new DOMRect(0, 0, 600, shape.height),
  );
  vi.spyOn(nested, "getBoundingClientRect").mockImplementation(
    () => new DOMRect(0, 150 - viewport.scrollTop, 600, 100),
  );
  const bounds = () =>
    new DOMRect(0, shape.top - viewport.scrollTop - nested.scrollTop, 100, shape.targetHeight);
  vi.spyOn(target, "getBoundingClientRect").mockImplementation(bounds);
  vi.spyOn(target, "getClientRects").mockImplementation(
    () => (shape.rendered ? [bounds()] : []) as unknown as DOMRectList,
  );
  const order: string[] = [];
  stop.mockImplementation(() => {
    order.push("follow retired");
    follow.state.isAtBottom = false;
  });
  const move = (delta: number) => {
    order.push("target revealed");
    viewport.scrollTop += delta;
  };
  Object.defineProperty(viewport, "scrollBy", {
    configurable: true,
    value: (options: ScrollToOptions) => move(options.top ?? 0),
  });
  Object.defineProperty(target, "scrollIntoView", {
    configurable: true,
    value: (options: ScrollIntoViewOptions) => {
      const rect = bounds(),
        top = 20,
        bottom = shape.height - parseFloat(viewport.style.scrollPaddingBottom);
      move(
        options.block === "start" || rect.top < top
          ? rect.top - top
          : Math.max(0, rect.bottom - bottom),
      );
    },
  });
  const resize = (node = viewport) =>
    act(() => {
      for (const observer of observers)
        if (observer.nodes.has(node)) {
          observer.callback(
            [
              {
                target: node,
                contentRect: node.getBoundingClientRect(),
                borderBoxSize: [],
                contentBoxSize: [],
                devicePixelContentBoxSize: [],
              },
            ],
            {} as ResizeObserver,
          );
        }
    });
  act(() => target.focus());
  return {
    ...view,
    Harness,
    follow,
    viewport,
    content,
    target,
    nested,
    shape,
    bounds,
    resize,
    order,
  };
}

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe("reading focus geometry", () => {
  it("keeps the same focused target through successive resizes when scroll precedes ResizeObserver", () => {
    const s = scenario();
    s.shape.height = 300;
    s.resize();
    fireEvent.scroll(s.viewport);
    expect(s.bounds().bottom).toBeLessThanOrEqual(220);
    s.shape.height = 240;
    fireEvent.scroll(s.viewport);
    s.resize();
    expect(s.bounds().bottom).toBeLessThanOrEqual(160);
    expect(document.activeElement).toBe(s.target);
  });

  it("does not reclaim a target that the reader scrolled away inside nested evidence", () => {
    const s = scenario();
    s.nested.style.overflowY = "auto";
    s.nested.scrollTop = 120;
    fireEvent.scroll(s.nested, { bubbles: false });
    s.shape.height = 180;
    s.resize();
    expect(s.viewport.scrollTop).toBe(0);
    expect(s.nested.scrollTop).toBe(120);
    expect(document.activeElement).toBe(s.target);
  });

  it.each(["below", "above"] as const)(
    "keeps repeated reveals inside the safe frame with quantized native scroll offsets (%s)",
    (edge) => {
      const s = scenario();
      Object.defineProperty(s.viewport, "scrollBy", {
        configurable: true,
        value: (options: ScrollToOptions) => {
          s.viewport.scrollTop += Math.trunc(options.top ?? 0);
        },
      });
      s.shape.top = 210.75;
      fireEvent.focusIn(s.target);
      if (edge === "below") {
        s.shape.height = 300;
        s.resize();
        fireEvent.scroll(s.viewport);
        s.shape.height = 240;
        fireEvent.scroll(s.viewport);
        s.resize();
      } else {
        s.viewport.scrollTop = 100;
        fireEvent.scroll(s.viewport);
        s.shape.top = 100.25;
        s.resize(s.content);
      }
      expect(s.bounds().top).toBeGreaterThanOrEqual(20);
      expect(s.bounds().bottom).toBeLessThanOrEqual(s.shape.height - 80);
      expect(document.activeElement).toBe(s.target);
    },
  );

  it("keeps visible focus and an ordinary content append in place", () => {
    const s = scenario();
    s.resize(s.content);
    expect(s.viewport.scrollTop).toBe(0);
    expect(s.order).toEqual([]);
    expect(document.activeElement).toBe(s.target);
  });

  it("uses the current reserved clearance without changing focus", () => {
    const s = scenario();
    s.viewport.style.scrollPaddingBottom = "180px";
    fireEvent.scroll(s.viewport);
    s.resize(s.content);
    expect(s.bounds().bottom).toBeLessThanOrEqual(220);
    expect(document.activeElement).toBe(s.target);
  });

  it("retires follow before revealing content displacement", () => {
    const s = scenario();
    s.follow.state.isAtBottom = true;
    s.shape.top = 350;
    s.resize(s.content);
    expect(s.order).toEqual(["follow retired", "target revealed"]);
    expect(s.bounds().bottom).toBeLessThanOrEqual(320);
    expect(document.activeElement).toBe(s.target);
  });

  it("leaves deliberate reading scroll untouched by a later resize", () => {
    const s = scenario();
    s.viewport.scrollTop = 250;
    fireEvent.scroll(s.viewport);
    s.shape.height = 240;
    s.resize();
    expect(s.viewport.scrollTop).toBe(250);
    expect(s.order).toEqual([]);
    expect(document.activeElement).toBe(s.target);
  });

  it("respects a wheel gesture while a new geometry frame is pending", () => {
    const s = scenario();
    s.shape.height = 240;
    fireEvent.wheel(s.viewport, { deltaY: 100 });
    s.viewport.scrollTop = 250;
    fireEvent.scroll(s.viewport);
    s.resize();
    expect(s.viewport.scrollTop).toBe(250);
    expect(s.order).toEqual([]);
    expect(document.activeElement).toBe(s.target);
  });

  it("preserves a later draft focus, value and selection", () => {
    const s = scenario();
    const draft = screen.getByRole<HTMLInputElement>("textbox", { name: "Later draft" });
    act(() => {
      draft.focus();
      draft.setSelectionRange(5, 9);
    });
    s.shape.height = 240;
    s.resize();
    expect(document.activeElement).toBe(draft);
    expect(draft.value).toBe("Keep this draft");
    expect([draft.selectionStart, draft.selectionEnd]).toEqual([5, 9]);
    expect(s.order).toEqual([]);
  });

  it("does not move reading for a removed control or a replacement", () => {
    const s = scenario();
    s.target.remove();
    const replacement = document.createElement("button");
    replacement.textContent = "Read original evidence";
    s.content.append(replacement);
    s.shape.height = 240;
    s.resize();
    expect(s.order).toEqual([]);
    expect(document.activeElement).not.toBe(replacement);
  });

  it("ignores a control that becomes unrendered", () => {
    const s = scenario();
    s.shape.rendered = false;
    s.shape.height = 240;
    s.resize();
    expect(s.order).toEqual([]);
  });

  it("aligns the start of a target taller than the remaining reading area", () => {
    const s = scenario();
    s.shape.top = 100;
    s.shape.targetHeight = 100;
    fireEvent.focusIn(s.target);
    s.shape.height = 170;
    s.resize();
    expect(s.bounds().top).toBe(20);
    expect(document.activeElement).toBe(s.target);
  });

  it("retires hidden measurement and reattaches the same visible reader", () => {
    const s = scenario();
    s.rerender(<s.Harness attached={false} />);
    s.shape.height = 240;
    s.resize();
    expect(s.order).toEqual([]);
    s.shape.height = 400;
    s.rerender(<s.Harness />);
    s.shape.height = 240;
    s.resize();
    expect(s.bounds().bottom).toBeLessThanOrEqual(160);
    expect(document.activeElement).toBe(s.target);
    s.unmount();
    s.resize();
  });

  it("ignores a fractional edge within native pixel rounding", () => {
    const s = scenario();
    s.shape.top = 296.25;
    s.resize();
    expect(s.viewport.scrollTop).toBe(0);
    expect(s.order).toEqual([]);
  });
});
