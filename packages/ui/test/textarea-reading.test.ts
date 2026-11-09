import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { followTextAreaCaret } from "../dist/textarea-reading.js";

let notify: ResizeObserverCallback;
let disconnected: boolean;
let frames: Map<number, FrameRequestCallback>;
let nextFrame: number;
let dispose: (() => void) | undefined;

beforeEach(() => {
  frames = new Map();
  nextFrame = 0;
  disconnected = false;
  vi.spyOn(window, "requestAnimationFrame").mockImplementation((callback) => {
    frames.set(++nextFrame, callback);
    return nextFrame;
  });
  vi.spyOn(window, "cancelAnimationFrame").mockImplementation((id) => {
    frames.delete(id);
  });
  vi.stubGlobal(
    "ResizeObserver",
    class {
      constructor(callback: ResizeObserverCallback) {
        notify = callback;
      }
      observe() {}
      disconnect() {
        disconnected = true;
      }
    },
  );
});

afterEach(() => {
  dispose?.();
  dispose = undefined;
  document.body.replaceChildren();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

function paint() {
  const pending = [...frames.values()];
  frames.clear();
  for (const callback of pending) callback(0);
}

function editor() {
  const area = document.createElement("textarea");
  area.style.cssText =
    "box-sizing:border-box;width:200px;padding:8px;border:1px solid;";
  area.value = "A draft with a middle selection and an unchanged tail.";
  const later = document.createElement("button");
  document.body.append(area, later);
  let height = 100;
  Object.defineProperties(area, {
    clientWidth: { get: () => 198 },
    offsetWidth: { get: () => 200 },
    clientHeight: { get: () => height },
    scrollHeight: { get: () => 500 },
  });
  area.getBoundingClientRect = () => new DOMRect(0, 0, 200, height + 2);
  area.focus();
  area.setSelectionRange(3, 8, "backward");
  let measuredOffset: number | undefined;
  let caretTop = 300;
  const createRange = document.createRange.bind(document);
  vi.spyOn(document, "createRange").mockImplementation(() => {
    const range = createRange();
    range.getBoundingClientRect = () => {
      measuredOffset = range.startOffset;
      return new DOMRect(0, caretTop, 0, 18);
    };
    return range;
  });
  dispose = followTextAreaCaret(area);
  return {
    area,
    later,
    measuredOffset: () => measuredOffset,
    setCaretTop: (top: number) => {
      caretTop = top;
    },
    resize: (next: number) => {
      height = next;
      notify([], {} as ResizeObserver);
    },
    anchor: (next: number, top: number) => {
      height = next;
      area.scrollTop = top;
      area.dispatchEvent(new Event("scroll"));
    },
  };
}

describe("TextArea caret reading through reflow", () => {
  it("reveals the backward endpoint without changing the draft, selection or focus", () => {
    const { area, resize, measuredOffset } = editor();
    const value = area.value;
    const focus = vi.spyOn(area, "focus");
    const setSelection = vi.spyOn(area, "setSelectionRange");
    resize(80);
    paint();

    expect(area.scrollTop).toBe(254);
    expect(measuredOffset()).toBe(3);
    expect(area.value).toBe(value);
    expect([
      area.selectionStart,
      area.selectionEnd,
      area.selectionDirection,
    ]).toEqual([3, 8, "backward"]);
    expect(document.activeElement).toBe(area);
    expect(focus).not.toHaveBeenCalled();
    expect(setSelection).not.toHaveBeenCalled();
    expect(document.body.children).toHaveLength(2);
  });

  it("measures the forward endpoint and can reveal an earlier middle line", () => {
    const { area, resize, measuredOffset, setCaretTop } = editor();
    area.setSelectionRange(3, 8, "forward");
    area.scrollTop = 250;
    setCaretTop(40);
    resize(80);
    paint();

    expect(measuredOffset()).toBe(8);
    expect(area.scrollTop).toBe(40);
  });

  it("keeps manual wheel reading through resize and same-control reactivation", () => {
    const { area, resize } = editor();
    resize(80);
    paint();
    area.dispatchEvent(new WheelEvent("wheel", { deltaY: -200 }));
    area.scrollTop = 0;
    area.dispatchEvent(new Event("scroll"));
    area.dispatchEvent(new FocusEvent("focus"));
    area.dispatchEvent(new Event("selectionchange", { bubbles: true }));
    resize(60);
    paint();

    expect(area.scrollTop).toBe(0);
    area.dispatchEvent(new InputEvent("input"));
    paint();
    expect(area.scrollTop).toBe(274);
  });

  it("retires following after a scrollbar moves without a layout change", () => {
    const { area, resize } = editor();
    resize(80);
    paint();
    area.dispatchEvent(new PointerEvent("pointerdown"));
    area.scrollTop = 0;
    area.dispatchEvent(new Event("scroll"));
    document.dispatchEvent(new PointerEvent("pointerup"));
    resize(60);
    paint();

    expect(area.scrollTop).toBe(0);
  });

  it("keeps following through native reflow anchoring and its own queued scroll", () => {
    const { area, anchor, resize } = editor();
    anchor(80, 30);
    paint();
    expect(area.scrollTop).toBe(254);
    area.dispatchEvent(new Event("scroll"));
    resize(60);
    paint();

    expect(area.scrollTop).toBe(274);
  });

  it("lets an actual selection change resume manual reading", () => {
    const { area, resize, measuredOffset } = editor();
    area.dispatchEvent(new WheelEvent("wheel"));
    area.setSelectionRange(5, 9, "backward");
    area.dispatchEvent(new Event("selectionchange", { bubbles: true }));
    resize(80);
    paint();

    expect(measuredOffset()).toBe(5);
    expect(area.scrollTop).toBe(254);
  });

  it.each(["control", "document"])(
    "resumes a collapsed caret from a %s selectionchange",
    (target) => {
      const { area, setCaretTop, measuredOffset } = editor();
      const value = area.value;
      area.dispatchEvent(new WheelEvent("wheel"));
      area.setSelectionRange(0, 0);
      setCaretTop(0);
      area.scrollTop = 10;
      (target === "control" ? area : document).dispatchEvent(
        new Event("selectionchange", { bubbles: true }),
      );
      area.dispatchEvent(new Event("scroll"));
      paint();

      expect(measuredOffset()).toBe(0);
      expect(area.scrollTop).toBe(0);
      expect(area.value).toBe(value);
      expect([area.selectionStart, area.selectionEnd]).toEqual([0, 0]);
      expect(document.activeElement).toBe(area);
    },
  );

  it("defers pointer-caused reflow until the press has ended", () => {
    const { area, resize } = editor();
    area.dispatchEvent(new PointerEvent("pointerdown"));
    resize(80);
    paint();
    expect(area.scrollTop).toBe(0);
    document.dispatchEvent(new PointerEvent("pointerup"));
    paint();

    expect(area.scrollTop).toBe(254);
  });

  it("defers composition and reveals only while the editor still owns focus", () => {
    const { area, resize } = editor();
    area.dispatchEvent(new CompositionEvent("compositionstart"));
    resize(80);
    paint();
    expect(area.scrollTop).toBe(0);
    area.dispatchEvent(new CompositionEvent("compositionend"));
    paint();

    expect(area.scrollTop).toBe(254);
  });

  it("does not reclaim later focus after queued resize or composition completion", () => {
    const { area, later, resize, measuredOffset } = editor();
    area.dispatchEvent(new CompositionEvent("compositionstart"));
    resize(80);
    later.focus();
    area.dispatchEvent(new CompositionEvent("compositionend"));
    paint();

    expect(document.activeElement).toBe(later);
    expect(area.scrollTop).toBe(0);
    expect(measuredOffset()).toBeUndefined();
  });

  it("cancels observation, queued work and listeners when retired", () => {
    const { area, resize, measuredOffset } = editor();
    resize(80);
    dispose!();
    dispose = undefined;
    resize(60);
    window.dispatchEvent(new Event("resize"));
    area.dispatchEvent(new InputEvent("input"));
    area.setSelectionRange(0, 0);
    document.dispatchEvent(new Event("selectionchange"));
    paint();

    expect(disconnected).toBe(true);
    expect(frames.size).toBe(0);
    expect(area.scrollTop).toBe(0);
    expect(measuredOffset()).toBeUndefined();
  });

  it("retires an abandoned composition when focus leaves and permits a new entry", () => {
    const { area, later, resize } = editor();
    area.dispatchEvent(new CompositionEvent("compositionstart"));
    later.focus();
    area.focus();
    resize(80);
    paint();

    expect(area.scrollTop).toBe(254);
  });
});
