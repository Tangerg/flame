import { fireEvent, render, screen } from "@testing-library/react";
import { type CSSProperties } from "react";
import { describe, expect, it, vi } from "vitest";
import { ResizeHandle } from "@/ui/atoms/resize-handle";
const PROPERTY = "--pane-width";

function Harness({
  edge = "end",
  value = 320,
  onCommit,
}: {
  edge?: "start" | "end";
  value?: number;
  onCommit: (width: number, containerWidth: number) => void;
}) {
  return (
    <div data-testid="container">
      <ResizeHandle
        aria-label="Resize panel"
        edge={edge}
        value={value}
        container={(handle) => handle.parentElement}
        property={PROPERTY}
        read={(container) => Number.parseFloat(container.style.getPropertyValue(PROPERTY)) || value}
        minWidth={() => 240}
        maxWidth={() => 800}
        onCommit={onCommit}
        resizingAttribute="data-resizing"
      />
    </div>
  );
}

function ControlledHarness({
  value,
  onCommit,
}: {
  value: number;
  onCommit: (width: number, containerWidth: number) => void;
}) {
  return (
    <div data-testid="container" style={{ [PROPERTY]: `${value}px` } as CSSProperties}>
      <ResizeHandle
        aria-label="Resize panel"
        edge="end"
        value={value}
        container={(handle) => handle.parentElement}
        property={PROPERTY}
        read={(container) => Number.parseFloat(container.style.getPropertyValue(PROPERTY))}
        minWidth={() => 240}
        maxWidth={() => 800}
        onCommit={onCommit}
        resizingAttribute="data-resizing"
      />
    </div>
  );
}

function paneWidth(): string {
  return screen.getByTestId("container").style.getPropertyValue(PROPERTY);
}

describe("ResizeHandle", () => {
  it("owns accessible separator semantics", () => {
    render(<Harness onCommit={() => {}} />);

    const handle = screen.getByRole("separator", { name: "Resize panel" });
    expect(handle.getAttribute("aria-orientation")).toBe("vertical");
    expect(handle.getAttribute("tabindex")).toBe("0");
    expect(handle.getAttribute("aria-valuemin")).toBe("240");
    expect(handle.getAttribute("aria-valuenow")).toBe("320");
    expect(handle.getAttribute("aria-valuemax")).toBe("800");
  });

  it("reports the committed width while its surface is still transitioning", () => {
    render(
      <div>
        <ResizeHandle
          aria-label="Resize panel"
          edge="end"
          value={500}
          container={(handle) => handle.parentElement}
          property={PROPERTY}
          read={() => 375}
          minWidth={() => 240}
          maxWidth={() => 800}
          onCommit={() => {}}
        />
      </div>,
    );

    expect(screen.getByRole("separator").getAttribute("aria-valuenow")).toBe("500");
  });

  it("writes the width straight onto the container during a drag and commits on release", () => {
    const onCommit = vi.fn();
    render(<Harness onCommit={onCommit} />);
    const handle = screen.getByRole("separator", { name: "Resize panel" });

    fireEvent.pointerDown(handle, { button: 0, clientX: 320 });
    expect(screen.getByTestId("container").hasAttribute("data-resizing")).toBe(true);

    fireEvent(window, new PointerEvent("pointermove", { clientX: 400 }));
    expect(paneWidth()).toBe("400px");
    expect(handle.getAttribute("aria-valuenow")).toBe("400");
    expect(onCommit).not.toHaveBeenCalled();

    fireEvent(window, new PointerEvent("pointerup"));
    expect(onCommit).toHaveBeenCalledExactlyOnceWith(400, expect.any(Number));
    expect(screen.getByTestId("container").hasAttribute("data-resizing")).toBe(false);
  });

  it("does not commit a press that never moved", () => {
    const onCommit = vi.fn();
    render(<Harness onCommit={onCommit} />);
    const handle = screen.getByRole("separator", { name: "Resize panel" });

    fireEvent.pointerDown(handle, { button: 0, clientX: 320 });
    fireEvent(window, new PointerEvent("pointermove", { clientX: 320 }));
    fireEvent(window, new PointerEvent("pointerup"));

    expect(onCommit).not.toHaveBeenCalled();
    expect(paneWidth()).toBe("");
  });

  it("grows toward the pane, whichever edge it sits on", () => {
    const onCommit = vi.fn();
    const { unmount } = render(<Harness edge="end" onCommit={onCommit} />);
    fireEvent.keyDown(screen.getByRole("separator", { name: "Resize panel" }), {
      key: "ArrowRight",
    });
    expect(onCommit).toHaveBeenLastCalledWith(328, expect.any(Number));
    unmount();

    render(<Harness edge="start" onCommit={onCommit} />);
    fireEvent.keyDown(screen.getByRole("separator", { name: "Resize panel" }), {
      key: "ArrowRight",
    });
    expect(onCommit).toHaveBeenLastCalledWith(312, expect.any(Number));
  });

  it("takes a coarse step with Shift and jumps to the range ends", () => {
    const onCommit = vi.fn();
    render(<Harness onCommit={onCommit} />);
    const handle = screen.getByRole("separator", { name: "Resize panel" });

    fireEvent.keyDown(handle, { key: "ArrowRight", shiftKey: true });
    expect(onCommit).toHaveBeenLastCalledWith(344, expect.any(Number));
    fireEvent.keyDown(handle, { key: "Home" });
    expect(onCommit).toHaveBeenLastCalledWith(240, expect.any(Number));
    fireEvent.keyDown(handle, { key: "End" });
    expect(onCommit).toHaveBeenLastCalledWith(800, expect.any(Number));
  });

  it("holds the resizing mark across key repeats and releases it on blur", () => {
    render(<Harness onCommit={() => {}} />);
    const handle = screen.getByRole("separator", { name: "Resize panel" });
    const container = screen.getByTestId("container");

    fireEvent.keyDown(handle, { key: "ArrowRight" });
    fireEvent.keyDown(handle, { key: "ArrowRight" });
    expect(container.hasAttribute("data-resizing")).toBe(true);

    fireEvent.blur(handle);
    expect(container.hasAttribute("data-resizing")).toBe(false);
  });

  it("leaves nothing attached when unmounted mid-drag", () => {
    const onCommit = vi.fn();
    const { unmount } = render(<Harness onCommit={onCommit} />);
    const container = screen.getByTestId("container");

    fireEvent.pointerDown(screen.getByRole("separator", { name: "Resize panel" }), {
      button: 0,
      clientX: 320,
    });
    unmount();

    expect(container.hasAttribute("data-resizing")).toBe(false);
    fireEvent(window, new PointerEvent("pointermove", { clientX: 500 }));
    fireEvent(window, new PointerEvent("pointerup"));
    expect(onCommit).not.toHaveBeenCalled();
  });
});

describe("ResizeHandle gesture settlement", () => {
  it.each(["pointercancel", "Escape", "lostpointercapture", "blur"])(
    "rolls back %s without committing the preference",
    (reason) => {
      const onCommit = vi.fn();
      render(<Harness onCommit={onCommit} />);
      const handle = screen.getByRole("separator", { name: "Resize panel" });
      const pane = screen.getByTestId("container");
      pane.style.setProperty(PROPERTY, "320px", "important");
      fireEvent.pointerDown(handle, { button: 0, pointerId: 7, clientX: 320 });
      fireEvent.pointerMove(window, { pointerId: 7, clientX: 450 });
      expect(paneWidth()).toBe("450px");
      if (reason === "Escape") fireEvent.keyDown(window, { key: "Escape" });
      else if (reason === "blur") fireEvent.blur(window);
      else
        fireEvent(
          reason === "lostpointercapture" ? handle : window,
          new PointerEvent(reason, { pointerId: 7 }),
        );
      expect(paneWidth()).toBe("320px");
      expect(pane.style.getPropertyPriority(PROPERTY)).toBe("important");
      expect(handle.getAttribute("aria-valuenow")).toBe("320");
      expect(pane.hasAttribute("data-resizing")).toBe(false);
      fireEvent.pointerMove(window, { pointerId: 7, clientX: 600 });
      fireEvent.pointerUp(window, { pointerId: 7 });
      expect(onCommit).not.toHaveBeenCalled();
      expect(paneWidth()).toBe("320px");
    },
  );

  it("ignores a second pointer throughout the gesture", () => {
    const onCommit = vi.fn();
    render(<Harness onCommit={onCommit} />);
    const handle = screen.getByRole("separator", { name: "Resize panel" });
    fireEvent.pointerDown(handle, { button: 0, pointerId: 7, clientX: 320 });
    fireEvent.pointerMove(window, { pointerId: 8, clientX: 800 });
    fireEvent.pointerCancel(window, { pointerId: 8 });
    fireEvent.pointerUp(window, { pointerId: 8 });
    expect(paneWidth()).toBe("");
    fireEvent.pointerMove(window, { pointerId: 7, clientX: 400 });
    fireEvent.pointerUp(window, { pointerId: 7 });
    expect(onCommit).toHaveBeenCalledExactlyOnceWith(400, expect.any(Number));
  });

  it("restores the starting property when unmounted after a preview", () => {
    const onCommit = vi.fn();
    const { unmount } = render(<Harness onCommit={onCommit} />);
    const pane = screen.getByTestId("container");
    fireEvent.pointerDown(screen.getByRole("separator"), { button: 0, clientX: 320 });
    fireEvent.pointerMove(window, { clientX: 450 });
    unmount();
    expect(pane.style.getPropertyValue(PROPERTY)).toBe("");
    fireEvent.pointerUp(window);
    expect(onCommit).not.toHaveBeenCalled();
  });

  it("retires a drag when an external width replaces it", () => {
    const onCommit = vi.fn();
    const view = render(<Harness onCommit={onCommit} />);
    fireEvent.pointerDown(screen.getByRole("separator"), { button: 0, clientX: 320 });
    fireEvent.pointerMove(window, { clientX: 450 });
    view.rerender(<Harness value={500} onCommit={onCommit} />);
    fireEvent.pointerUp(window);
    expect(onCommit).not.toHaveBeenCalled();
    expect(paneWidth()).toBe("500px");
    expect(screen.getByRole("separator").getAttribute("aria-valuenow")).toBe("500");
  });

  it("keeps a controlled parent's committed width over the retired preview", () => {
    const onCommit = vi.fn();
    const view = render(<ControlledHarness value={320} onCommit={onCommit} />);
    const pane = screen.getByTestId("container");
    fireEvent.pointerDown(screen.getByRole("separator"), { button: 0, clientX: 320 });
    fireEvent.pointerMove(window, { clientX: 450 });
    expect(paneWidth()).toBe("450px");

    view.rerender(<ControlledHarness value={500} onCommit={onCommit} />);

    expect(paneWidth()).toBe("500px");
    expect(pane.hasAttribute("data-resizing")).toBe(false);
    fireEvent.pointerMove(window, { clientX: 600 });
    fireEvent.pointerUp(window);
    expect(onCommit).not.toHaveBeenCalled();
    expect(paneWidth()).toBe("500px");
    expect(screen.getByRole("separator").getAttribute("aria-valuenow")).toBe("500");
  });

  it("leaves the parent's projection alone when a container resize retired the gesture first", () => {
    const onCommit = vi.fn();
    let containerWidth = 1200;
    const width = vi
      .spyOn(HTMLElement.prototype, "clientWidth", "get")
      .mockImplementation(() => containerWidth);
    try {
      const view = render(<ControlledHarness value={320} onCommit={onCommit} />);
      fireEvent.pointerDown(screen.getByRole("separator"), { button: 0, clientX: 320 });
      fireEvent.pointerMove(window, { clientX: 450 });
      expect(paneWidth()).toBe("450px");

      containerWidth = 1000;
      fireEvent.pointerMove(window, { clientX: 460 });
      expect(paneWidth()).toBe("320px");

      view.rerender(<ControlledHarness value={500} onCommit={onCommit} />);
      expect(paneWidth()).toBe("500px");
      fireEvent.pointerUp(window);
      expect(onCommit).not.toHaveBeenCalled();
      expect(paneWidth()).toBe("500px");
    } finally {
      width.mockRestore();
    }
  });
});
