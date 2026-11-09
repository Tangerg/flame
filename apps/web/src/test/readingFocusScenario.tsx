import { act, screen } from "@testing-library/react";
import { vi } from "vitest";
import type { StickToBottomInstance } from "use-stick-to-bottom";
import { type ReactNode } from "react";
import { stopFollowOnReadingFocus, useReadingFocusVisibility } from "@/ui/agent/readingFollow";
import { type TestClient } from "@/test/renderClient";
import { ButtonPrimitive, TextField } from "@flame/ui";

export function readingFocusScenario(
  testClient: TestClient,
  {
    children,
    targetLabel,
    draftLabel,
    draftValue,
  }: {
    children?: ReactNode;
    targetLabel: string;
    draftLabel: string;
    draftValue: string;
  },
) {
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
  const frames = new Map<number, FrameRequestCallback>();
  const frameIds = { last: 0 };
  vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => {
    frames.set(++frameIds.last, callback);
    return frameIds.last;
  });
  vi.stubGlobal("cancelAnimationFrame", (id: number) => frames.delete(id));
  const frame = () =>
    act(() => {
      const queued = [...frames.values()];
      frames.clear();
      for (const callback of queued) callback(performance.now());
    });
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
        <TextField aria-label={draftLabel} defaultValue={draftValue} />
        <div ref={attached ? follow.scrollRef : undefined}>
          <div
            ref={attached ? follow.contentRef : undefined}
            onFocus={(event) => stopFollowOnReadingFocus(event, follow)}
          >
            <div data-testid="nested">
              {children ?? <ButtonPrimitive>{targetLabel}</ButtonPrimitive>}
            </div>
          </div>
        </div>
      </>
    );
  }
  const view = testClient.render(<Harness />);
  const viewport = follow.scrollRef.current!;
  const content = follow.contentRef.current!;
  const target = screen.getByRole("button", { name: targetLabel });
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
  // Delivers the observed layout frame unless a test needs to act between notification and frame.
  const resize = (node = viewport, { deliverFrame = true } = {}) => {
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
    if (deliverFrame) frame();
  };
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
    frame,
    order,
  };
}
