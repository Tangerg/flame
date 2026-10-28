import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { stopFollowOnReadingFocus, useReadingFocusVisibility } from "./readingFollow";
import { act, cleanup, fireEvent, screen } from "@testing-library/react";
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
          <div
            ref={attached ? follow.contentRef : undefined}
            onFocus={(event) => stopFollowOnReadingFocus(event, follow)}
          >
            <div data-testid="nested">
              <button>Read original evidence</button>
            </div>
          </div>
        </div>
      </>
    );
  }
  const view = testClient.render(<Harness />);
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
  Object.defineProperty(viewport, "scrollTo", {
    configurable: true,
    value: (options: ScrollToOptions) =>
      move((options.top ?? viewport.scrollTop) - viewport.scrollTop),
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
  it("reveals a fitting group heading before the focused option's own label", () => {
    const s = scenario();
    s.nested.dataset.focusUnit = "";
    const label = document.createElement("label");
    s.nested.append(label);
    label.append(s.target);
    vi.spyOn(label, "getBoundingClientRect").mockImplementation(s.bounds);
    act(() => screen.getByRole("textbox", { name: "Later draft" }).focus());
    s.viewport.scrollTop = 140;

    act(() => s.target.focus({ preventScroll: true }));

    expect(s.nested.getBoundingClientRect().top).toBe(20);
    expect(document.activeElement).toBe(s.target);
  });

  it("keeps the lower edge of a fitting unit above the Composer even while its control is visible", () => {
    const s = scenario();
    s.nested.dataset.focusUnit = "";
    act(() => screen.getByRole("textbox", { name: "Later draft" }).focus());
    s.shape.height = 320;

    act(() => s.target.focus({ preventScroll: true }));

    expect(s.nested.getBoundingClientRect().bottom).toBe(240);
    expect(document.activeElement).toBe(s.target);
  });

  it("keeps a fitting unit together as the reading frame shrinks", () => {
    const s = scenario();
    s.nested.dataset.focusUnit = "";
    fireEvent.focusIn(s.target);
    s.shape.height = 300;

    s.resize();

    expect(s.nested.getBoundingClientRect().bottom).toBe(220);
    expect(document.activeElement).toBe(s.target);
  });

  it("preserves the focused control's CSS clearance when revealing its fitting context", () => {
    const s = scenario();
    s.nested.dataset.focusUnit = "";
    s.target.style.scrollMarginTop = "80px";
    act(() => screen.getByRole("textbox", { name: "Later draft" }).focus());
    s.viewport.scrollTop = 140;

    act(() => s.target.focus({ preventScroll: true }));

    expect(s.bounds().top).toBe(100);
    expect(s.nested.getBoundingClientRect().top).toBe(40);
    expect(document.activeElement).toBe(s.target);
  });

  it("falls back to the focused control when its complete unit exceeds the reading frame", () => {
    const s = scenario();
    s.nested.dataset.focusUnit = "";
    s.shape.height = 160;
    fireEvent.focusIn(s.target);
    expect(s.bounds().bottom).toBe(80);
    expect(document.activeElement).toBe(s.target);
  });

  it("leaves a group heading behind after deliberate scrolling while the option remains visible", () => {
    const s = scenario();
    s.nested.dataset.focusUnit = "";
    fireEvent.focusIn(s.target);
    fireEvent.wheel(s.viewport, { deltaY: 140 });
    s.viewport.scrollTop = 140;
    fireEvent.scroll(s.viewport);
    s.shape.height = 380;
    s.resize();
    expect(s.viewport.scrollTop).toBe(140);
    expect(document.activeElement).toBe(s.target);
  });

  it("keeps the caption around a nested reading region without testing it against its own inner clip", () => {
    const s = scenario();
    s.content.dataset.focusUnit = "";
    s.nested.style.overflowY = "auto";
    vi.spyOn(s.content, "getBoundingClientRect").mockImplementation(
      () => new DOMRect(0, 150 - s.viewport.scrollTop, 600, 150),
    );
    act(() => screen.getByRole("textbox", { name: "Later draft" }).focus());
    s.shape.height = 320;
    act(() => s.target.focus({ preventScroll: true }));
    expect(s.content.getBoundingClientRect().bottom).toBe(240);
    expect(s.nested.scrollTop).toBe(0);
    expect(document.activeElement).toBe(s.target);
  });

  it("uses the control when a group cannot fit its enclosing clip", () => {
    const s = scenario();
    s.nested.dataset.focusUnit = "";
    const clip = document.createElement("div");
    clip.style.overflowY = "hidden";
    s.content.append(clip);
    clip.append(s.nested);
    vi.spyOn(clip, "getBoundingClientRect").mockImplementation(
      () => new DOMRect(0, 150 - s.viewport.scrollTop, 600, 60),
    );
    act(() => screen.getByRole("textbox", { name: "Later draft" }).focus());
    s.shape.top = 180;
    s.shape.height = 260;
    act(() => s.target.focus({ preventScroll: true }));
    expect(s.viewport.scrollTop).toBe(24);
    expect(s.nested.scrollTop).toBe(0);
    expect(document.activeElement).toBe(s.target);
  });

  it("keeps group context inside the current reading viewport", () => {
    const s = scenario();
    const outside = s.viewport.parentElement!;
    outside.dataset.focusUnit = "";
    vi.spyOn(outside, "getBoundingClientRect").mockReturnValue(new DOMRect(0, 5, 600, 100));
    fireEvent.focusIn(s.target);
    expect(s.order).toEqual([]);
    expect(s.viewport.scrollTop).toBe(0);
  });

  it("retires queued following that would hide a group's heading while leaving its option visible", () => {
    const s = scenario();
    s.nested.dataset.focusUnit = "";
    act(() => screen.getByRole("textbox", { name: "Later draft" }).focus());
    s.viewport.scrollTop = 100;
    s.follow.state.isAtBottom = true;
    s.follow.state.targetScrollTop = 140;

    act(() => s.target.focus({ preventScroll: true }));

    expect(s.order).toEqual(["follow retired"]);
    expect(s.viewport.scrollTop).toBe(100);
    expect(document.activeElement).toBe(s.target);
  });

  it("keeps returned focus beyond its CSS disclosure clearance", () => {
    const s = scenario();
    act(() => screen.getByRole("textbox", { name: "Later draft" }).focus());
    s.viewport.scrollTop = 200;
    s.viewport.scrollLeft = 23;
    s.shape.top = 250;
    s.target.style.scrollMarginTop = "48px";

    act(() => s.target.focus({ preventScroll: true }));

    expect(s.bounds().top).toBe(68);
    expect(s.viewport.scrollLeft).toBe(23);
    expect(document.activeElement).toBe(s.target);
  });

  it("uses a changed CSS clearance when focused content reflows", () => {
    const s = scenario();
    s.target.style.scrollMarginTop = "48px";
    s.shape.top = 90;
    s.resize(s.content);
    expect(s.viewport.scrollTop).toBe(0);

    s.target.style.scrollMarginTop = "96px";
    s.resize(s.content);

    expect(s.bounds().top).toBe(116);
    expect(document.activeElement).toBe(s.target);
  });

  it("keeps visible focus when its margin extends outside a disclosure's clipping box", () => {
    const s = scenario();
    s.target.style.scrollMarginTop = "96px";
    s.nested.style.overflowY = "hidden";
    s.nested.scrollTop = 4;
    fireEvent.scroll(s.nested);
    s.shape.height = 240;

    s.resize();

    expect(s.bounds().bottom).toBeLessThanOrEqual(160);
    expect(document.activeElement).toBe(s.target);
  });

  it("keeps the clearance start when the complete focus area exceeds the reading frame", () => {
    const s = scenario();
    act(() => screen.getByRole("textbox", { name: "Later draft" }).focus());
    s.viewport.scrollTop = 200;
    s.shape.height = 123;
    s.shape.top = 250;
    s.target.style.scrollMarginTop = "56.5px";

    act(() => s.target.focus({ preventScroll: true }));

    expect(s.bounds().top).toBeGreaterThanOrEqual(76.5);
    expect(s.bounds().top).toBeLessThan(77.5);
    expect(document.activeElement).toBe(s.target);
  });

  it("reserves the focused control's CSS end margin above the Composer", () => {
    const s = scenario();
    s.target.style.scrollMarginBottom = "32px";
    s.shape.height = 240;
    s.shape.top = 145;

    s.resize();

    expect(s.bounds().bottom).toBe(128);
    expect(document.activeElement).toBe(s.target);
  });

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

  it("preserves the witness when a reflow-only scroll precedes the content resize", () => {
    const s = scenario();
    s.shape.height = 300;
    s.shape.top = 190;
    s.resize();
    expect(s.viewport.scrollTop).toBe(0);

    s.shape.top = 225;
    fireEvent.scroll(s.viewport);
    s.resize(s.content);

    expect(s.bounds().bottom).toBeLessThanOrEqual(220);
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
      Object.defineProperty(s.viewport, "scrollTo", {
        configurable: true,
        value: (options: ScrollToOptions) => {
          s.viewport.scrollTop = Math.trunc(options.top ?? s.viewport.scrollTop);
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

  it.each([
    { height: 24, top: 136, bottom: 160 },
    { height: 330, top: 20, bottom: 350 },
  ])(
    "reveals returned focus after geometry changed outside the reader ($height px target)",
    ({ height, top, bottom }) => {
      const s = scenario();
      act(() => screen.getByRole("textbox", { name: "Later draft" }).focus());
      s.shape.height = 240;
      s.shape.top = 350;
      s.shape.targetHeight = height;
      s.resize();
      expect(s.order).toEqual([]);

      act(() => s.target.focus({ preventScroll: true }));
      expect(s.bounds().top).toBe(top);
      expect(s.bounds().bottom).toBe(bottom);
      expect(document.activeElement).toBe(s.target);
      expect(s.order).toEqual(["target revealed"]);

      fireEvent.scroll(s.viewport);
      s.shape.top += 40;
      s.resize(s.content);
      expect(s.bounds().top).toBe(top);
      expect(s.bounds().bottom).toBe(bottom);
      expect(document.activeElement).toBe(s.target);
    },
  );

  it("keeps pointer focus at the pressed position and reveals a later programmatic return", () => {
    const s = scenario();
    const later = screen.getByRole("textbox", { name: "Later draft" });
    act(() => later.focus());
    s.shape.height = 240;
    s.shape.top = 150;
    s.shape.targetHeight = 100;
    fireEvent.pointerDown(s.target);
    act(() => s.target.focus());
    expect(s.order).toEqual([]);
    expect(s.viewport.scrollTop).toBe(0);
    fireEvent.pointerUp(s.target);

    act(() => later.focus());
    act(() => s.target.focus({ preventScroll: true }));
    expect(s.bounds().bottom).toBeLessThanOrEqual(160);
    expect(s.order).toEqual(["target revealed"]);
  });

  it.each([
    { scroll: 250, top: 700, height: 330 },
    { scroll: 600, top: 500, height: 24 },
  ])(
    "reveals returned focus from an existing reading offset ($scroll px)",
    ({ scroll, top, height }) => {
      const s = scenario();
      act(() => screen.getByRole("textbox", { name: "Later draft" }).focus());
      s.viewport.scrollTop = scroll;
      s.viewport.scrollLeft = 23;
      s.shape.height = 240;
      s.shape.top = top;
      s.shape.targetHeight = height;

      act(() => s.target.focus({ preventScroll: true }));

      expect(s.bounds().top).toBe(20);
      expect(s.viewport.scrollLeft).toBe(23);
      expect(document.activeElement).toBe(s.target);
      expect(s.order).toEqual(["target revealed"]);
    },
  );

  it("keeps the start of oversized focus visible through quantized scrolling and later reflow", () => {
    const s = scenario();
    act(() => screen.getByRole("textbox", { name: "Later draft" }).focus());
    s.shape.height = 240;
    s.shape.top = 350.75;
    s.shape.targetHeight = 330;
    act(() => s.target.focus({ preventScroll: true }));
    expect(s.bounds().top).toBe(20.75);
    fireEvent.scroll(s.viewport);
    s.shape.top += 40.25;
    s.resize(s.content);
    expect(s.bounds().top).toBeGreaterThanOrEqual(20);
    expect(s.bounds().top).toBeLessThan(21);
    expect(document.activeElement).toBe(s.target);
  });

  it("retires a virtual press when focus leaves the reader before pointerup", () => {
    const s = scenario();
    fireEvent.pointerDown(s.target);
    act(() => screen.getByRole("textbox", { name: "Later draft" }).focus());
    s.shape.height = 240;
    s.shape.top = 350;
    act(() => s.target.focus({ preventScroll: true }));
    expect(s.bounds().bottom).toBeLessThanOrEqual(160);
    expect(s.order).toEqual(["target revealed"]);
    expect(document.activeElement).toBe(s.target);
  });

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
    s.nested.dataset.focusUnit = "";
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
