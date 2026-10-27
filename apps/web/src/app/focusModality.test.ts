import { readFileSync } from "node:fs";
import { runInNewContext } from "node:vm";
import { describe, expect, it } from "vitest";

const bootstrap = readFileSync("public/focus-modality.js", "utf8");

function scene() {
  const page = document.implementation.createHTMLDocument();
  const events = new EventTarget();
  runInNewContext(bootstrap, {
    document: page,
    addEventListener: events.addEventListener.bind(events),
  });
  return {
    root: page.documentElement,
    key(key: string, options: KeyboardEventInit & { altGraph?: boolean } = {}) {
      const { altGraph = false, ...init } = options;
      const event = new KeyboardEvent("keydown", { key, cancelable: true, ...init });
      const modifierState = event.getModifierState.bind(event);
      // Happy DOM reports every Alt press as AltGraph; model the native modifier independently.
      Object.defineProperty(event, "getModifierState", {
        value: (name: string) => (name === "AltGraph" ? altGraph : modifierState(name)),
      });
      events.dispatchEvent(event);
      return event;
    },
    pointer() {
      events.dispatchEvent(new Event("pointerdown"));
    },
  };
}

describe("focus modality bootstrap", () => {
  it.each([
    "Tab",
    "ArrowUp",
    "ArrowDown",
    "ArrowLeft",
    "ArrowRight",
    "Home",
    "End",
    "PageUp",
    "PageDown",
    "Enter",
    " ",
    "Escape",
  ])("reveals keyboard focus for navigation or activation with %j", (key) => {
    const s = scene();
    expect(s.root.getAttribute("data-input-modality")).toBe("pointer");

    const event = s.key(key);

    expect(s.root.getAttribute("data-input-modality")).toBe("keyboard");
    expect(event.defaultPrevented).toBe(false);
  });

  it("returns to pointer focus without making ordinary typing reveal a ring", () => {
    const s = scene();
    s.key("Tab");
    s.pointer();

    for (const key of ["a", "你", "Shift", "Process"]) s.key(key);

    expect(s.root.getAttribute("data-input-modality")).toBe("pointer");
  });

  it.each(
    ["l", ",", "F6"].flatMap((key) => [
      { key, metaKey: true },
      { key, ctrlKey: true },
    ]),
  )("reveals keyboard focus for command stroke %j", ({ key, ...options }) => {
    const s = scene();

    const event = s.key(key, options);

    expect(s.root.getAttribute("data-input-modality")).toBe("keyboard");
    expect(event.defaultPrevented).toBe(false);
    s.pointer();
    expect(s.root.getAttribute("data-input-modality")).toBe("pointer");
  });

  it.each(["Meta", "Control", "Alt", "Shift", "AltGraph", "Dead", "Process", "Unidentified"])(
    "does not treat %s alone as a command stroke",
    (key) => {
      const s = scene();

      s.key(key, { metaKey: true, ctrlKey: true });

      expect(s.root.getAttribute("data-input-modality")).toBe("pointer");
    },
  );

  it("distinguishes AltGraph text from a Ctrl-Alt command", () => {
    const s = scene();

    s.key("@", { ctrlKey: true, altKey: true, altGraph: true });
    expect(s.root.getAttribute("data-input-modality")).toBe("pointer");

    s.key("k", { ctrlKey: true, altKey: true });
    expect(s.root.getAttribute("data-input-modality")).toBe("keyboard");
  });

  it.each(["Enter", "ArrowDown"])("keeps composition-owned %s out of focus navigation", (key) => {
    const s = scene();

    s.key(key, { isComposing: true });

    expect(s.root.getAttribute("data-input-modality")).toBe("pointer");
  });

  it.each(["l", ","])("keeps composition-owned %s out of command focus", (key) => {
    const s = scene();

    s.key(key, { metaKey: true, isComposing: true });

    expect(s.root.getAttribute("data-input-modality")).toBe("pointer");
  });

  it.each(["ArrowDown", "l"])(
    "keeps legacy native composition %s out of focus navigation",
    (key) => {
      const s = scene();

      s.key(key, { keyCode: 229, ctrlKey: true });

      expect(s.root.getAttribute("data-input-modality")).toBe("pointer");
    },
  );
});
