import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ContentVisibility, useScrollPan } from "../dist/index.js";

let resize: () => void;
let disconnect: ReturnType<typeof vi.fn>;
beforeEach(() => {
  disconnect = vi.fn();
  vi.stubGlobal(
    "ResizeObserver",
    class {
      constructor(callback: () => void) {
        resize = callback;
      }
      observe() {}
      disconnect = disconnect;
    },
  );
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function Pan({
  source = "a",
  enabled = true,
}: {
  source?: string;
  enabled?: boolean;
}) {
  const pan = useScrollPan(source, enabled);
  return (
    <div
      data-testid="pan"
      tabIndex={0}
      ref={pan.ref}
      onPointerDown={pan.onPointerDown}
      onKeyDownCapture={pan.retire}
    />
  );
}

function begin() {
  const node = screen.getByTestId("pan");
  node.setPointerCapture = vi.fn();
  node.hasPointerCapture = () => true;
  node.releasePointerCapture = vi.fn();
  node.scrollLeft = 100;
  node.scrollTop = 200;
  fireEvent.pointerDown(node, {
    button: 0,
    isPrimary: true,
    pointerId: 7,
    clientX: 100,
    clientY: 100,
  });
  fireEvent.pointerMove(window, { pointerId: 7, clientX: 80, clientY: 50 });
  expect([node.scrollLeft, node.scrollTop]).toEqual([120, 250]);
  expect(node.hasAttribute("data-panning")).toBe(true);
  return node;
}

describe("native scroll pan", () => {
  it("retains a completed position and ignores other pointers", () => {
    render(<Pan />);
    const node = begin();
    fireEvent.pointerCancel(window, { pointerId: 8 });
    expect(node.hasAttribute("data-panning")).toBe(true);
    fireEvent.pointerUp(window, { pointerId: 7 });
    expect([node.scrollLeft, node.scrollTop]).toEqual([120, 250]);
    expect(node.hasAttribute("data-panning")).toBe(false);
    expect(disconnect).toHaveBeenCalledTimes(1);
  });

  it("restores the captured position on cancellation", () => {
    render(<Pan />);
    const node = begin();
    fireEvent.keyDown(window, { key: "Escape" });
    expect([node.scrollLeft, node.scrollTop]).toEqual([100, 200]);
    expect(node.hasAttribute("data-panning")).toBe(false);
    fireEvent.pointerMove(window, { pointerId: 7, clientX: 0 });
    expect(node.scrollLeft).toBe(100);
  });

  it.each([
    { deltaX: 60, deltaY: 0 },
    { deltaX: 0, deltaY: 60 },
  ])(
    "yields to native wheel scrolling %j without a later rollback",
    (delta) => {
      render(<Pan />);
      const node = begin();
      const wheel = new WheelEvent("wheel", {
        ...delta,
        bubbles: true,
        cancelable: true,
      });
      fireEvent(node, wheel);
      expect(wheel.defaultPrevented).toBe(false);
      expect(node.hasAttribute("data-panning")).toBe(false);
      expect([node.scrollLeft, node.scrollTop]).toEqual([120, 250]);
      node.scrollLeft = 420;
      node.scrollTop = 650;
      fireEvent.pointerMove(window, { pointerId: 7, clientX: 0, clientY: 0 });
      fireEvent.pointerCancel(window, { pointerId: 7 });
      expect([node.scrollLeft, node.scrollTop]).toEqual([420, 650]);
      expect(disconnect).toHaveBeenCalledTimes(1);
    },
  );

  it.each(["source", "disabled", "hidden", "keyboard", "viewport", "unmount"])(
    "retires without rollback on %s takeover",
    (reason) => {
      const view = render(
        <ContentVisibility visible>
          <Pan />
        </ContentVisibility>,
      );
      const node = begin();
      if (reason === "source")
        view.rerender(
          <ContentVisibility visible>
            <Pan source="b" />
          </ContentVisibility>,
        );
      else if (reason === "disabled")
        view.rerender(
          <ContentVisibility visible>
            <Pan enabled={false} />
          </ContentVisibility>,
        );
      else if (reason === "hidden")
        view.rerender(
          <ContentVisibility visible={false}>
            <Pan />
          </ContentVisibility>,
        );
      else if (reason === "keyboard")
        fireEvent.keyDown(node, { key: "ArrowRight" });
      else if (reason === "viewport") {
        Object.defineProperty(node, "clientWidth", {
          value: 400,
          configurable: true,
        });
        act(() => resize());
      } else view.unmount();
      expect([node.scrollLeft, node.scrollTop]).toEqual([120, 250]);
      expect(node.hasAttribute("data-panning")).toBe(false);
      node.scrollLeft = 420;
      fireEvent.pointerMove(window, { pointerId: 7, clientX: 0 });
      fireEvent.pointerCancel(window, { pointerId: 7 });
      expect(node.scrollLeft).toBe(420);
      expect(disconnect).toHaveBeenCalledTimes(1);
    },
  );
});
