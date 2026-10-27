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
    key(key: string, options: KeyboardEventInit = {}) {
      const event = new KeyboardEvent("keydown", { key, cancelable: true, ...options });
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
    expect(s.root.hasAttribute("data-pointer")).toBe(true);

    const event = s.key(key);

    expect(s.root.hasAttribute("data-pointer")).toBe(false);
    expect(event.defaultPrevented).toBe(false);
  });

  it("returns to pointer focus without making ordinary typing reveal a ring", () => {
    const s = scene();
    s.key("Tab");
    s.pointer();

    for (const key of ["a", "你", "Shift", "Process"]) s.key(key);

    expect(s.root.hasAttribute("data-pointer")).toBe(true);
  });

  it.each(["Enter", "ArrowDown"])("keeps composition-owned %s out of focus navigation", (key) => {
    const s = scene();

    s.key(key, { isComposing: true });

    expect(s.root.hasAttribute("data-pointer")).toBe(true);
  });

  it("keeps legacy native composition events out of focus navigation", () => {
    const s = scene();

    s.key("ArrowDown", { keyCode: 229 });

    expect(s.root.hasAttribute("data-pointer")).toBe(true);
  });
});
