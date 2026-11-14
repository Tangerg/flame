import { afterEach, describe, expect, it, vi } from "vitest";
import { Window as HappyWindow } from "happy-dom";
import { PointerSession } from "../dist/index.js";

afterEach(() => {
  document.body.replaceChildren();
  vi.restoreAllMocks();
});

function target(document = window.document) {
  const node = document.createElement("div");
  document.body.append(node);
  const captured = new Set<number>();
  node.setPointerCapture = vi.fn((id) => {
    captured.add(id);
  });
  node.hasPointerCapture = (id) => captured.has(id);
  node.releasePointerCapture = vi.fn((id) => {
    captured.delete(id);
    node.dispatchEvent(
      new PointerEvent("lostpointercapture", { pointerId: id }),
    );
  });
  return node;
}

function start(node = target(), overrides = {}) {
  const callbacks = {
    current: vi.fn(() => true),
    move: vi.fn(),
    finish: vi.fn(),
  };
  const session = PointerSession.start(
    node,
    {
      button: 0,
      isPrimary: true,
      pointerId: 7,
      clientX: 20,
      clientY: 30,
      ...overrides,
    },
    callbacks,
  );
  return { node, session, ...callbacks };
}

function pointer(type: string, pointerId = 7, x = 25, y = 40) {
  return new PointerEvent(type, { pointerId, clientX: x, clientY: y });
}

describe("PointerSession", () => {
  it.each([{ button: 2 }, { isPrimary: false }])(
    "rejects an inadmissible pointer %j",
    (input) => {
      const { node, session, finish } = start(target(), input);
      expect(session).toBeNull();
      expect(node.setPointerCapture).not.toHaveBeenCalled();
      expect(finish).not.toHaveBeenCalled();
    },
  );

  it("rejects a disconnected target", () => {
    const node = target();
    node.remove();
    expect(start(node).session).toBeNull();
  });

  it("filters other pointers and completes once before capture-release reentry", () => {
    const { node, session, move, finish } = start();
    window.dispatchEvent(pointer("pointermove", 8));
    window.dispatchEvent(pointer("pointercancel", 8));
    window.dispatchEvent(pointer("pointerup", 8));
    expect(move).not.toHaveBeenCalled();
    expect(session!.active).toBe(true);
    window.dispatchEvent(pointer("pointermove"));
    expect(move).toHaveBeenCalledExactlyOnceWith({ x: 5, y: 10 });
    window.dispatchEvent(pointer("pointerup"));
    session!.retire();
    window.dispatchEvent(pointer("pointermove"));
    window.dispatchEvent(pointer("pointerup"));
    expect(session!.active).toBe(false);
    expect(node.releasePointerCapture).toHaveBeenCalledExactlyOnceWith(7);
    expect(finish).toHaveBeenCalledExactlyOnceWith("complete");
    expect(move).toHaveBeenCalledTimes(1);
  });

  it.each([
    "pointercancel",
    "lostpointercapture",
    "Escape",
    "window blur",
    "target blur",
  ])("cancels on %s", (reason) => {
    const { node, session, finish } = start();
    if (reason === "Escape") {
      const event = new KeyboardEvent("keydown", {
        key: "Escape",
        cancelable: true,
      });
      window.dispatchEvent(event);
      expect(event.defaultPrevented).toBe(true);
    } else if (reason === "window blur")
      window.dispatchEvent(new Event("blur"));
    else if (reason === "target blur") node.dispatchEvent(new Event("blur"));
    else
      (reason === "lostpointercapture" ? node : window).dispatchEvent(
        pointer(reason),
      );
    session!.retire();
    expect(finish).toHaveBeenCalledExactlyOnceWith("cancel");
  });

  it.each(["pointermove", "pointerup", "pointercancel"])(
    "retires an invalid source before %s can write",
    (event) => {
      const { session, current, move, finish } = start();
      current.mockReturnValue(false);
      window.dispatchEvent(pointer(event));
      expect(session!.active).toBe(false);
      expect(move).not.toHaveBeenCalled();
      expect(finish).toHaveBeenCalledExactlyOnceWith("retire");
    },
  );

  it("retires for page lifecycle loss and leaves no later handler", () => {
    const { session, move, finish } = start();
    window.dispatchEvent(new Event("pagehide"));
    window.dispatchEvent(pointer("pointermove"));
    window.dispatchEvent(new Event("blur"));
    expect(session!.active).toBe(false);
    expect(move).not.toHaveBeenCalled();
    expect(finish).toHaveBeenCalledExactlyOnceWith("retire");
  });

  it("uses the target's window rather than the ambient window", async () => {
    const other = new HappyWindow();
    const node = target(other.document as unknown as Document);
    const { session, move, finish } = start(node);
    window.dispatchEvent(pointer("pointermove"));
    expect(move).not.toHaveBeenCalled();
    other.dispatchEvent(pointer("pointermove"));
    expect(move).toHaveBeenCalledExactlyOnceWith({ x: 5, y: 10 });
    other.dispatchEvent(pointer("pointerup"));
    expect(session!.active).toBe(false);
    expect(finish).toHaveBeenCalledExactlyOnceWith("complete");
    await other.happyDOM.close();
  });

  it("detaches an admission that fails to capture and exposes the failure", () => {
    const node = target();
    const error = new Error("capture rejected");
    node.setPointerCapture = () => {
      throw error;
    };
    const callbacks = { current: () => true, move: vi.fn(), finish: vi.fn() };
    expect(() =>
      PointerSession.start(
        node,
        { button: 0, isPrimary: true, pointerId: 7, clientX: 0, clientY: 0 },
        callbacks,
      ),
    ).toThrow(error);
    window.dispatchEvent(pointer("pointermove"));
    window.dispatchEvent(pointer("pointerup"));
    expect(callbacks.move).not.toHaveBeenCalled();
    expect(callbacks.finish).not.toHaveBeenCalled();
  });
});
