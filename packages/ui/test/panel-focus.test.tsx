import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { FormGroup, Panel } from "../dist/index.js";

let rectangles: Map<Element, DOMRect>;
let revealed: Element[];
const nativeScroll = Object.getOwnPropertyDescriptor(
  HTMLElement.prototype,
  "scrollIntoView",
);

beforeEach(() => {
  rectangles = new Map();
  revealed = [];
  vi.spyOn(document.documentElement, "clientHeight", "get").mockReturnValue(
    600,
  );
  vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(
    function (this: HTMLElement) {
      return rectangles.get(this) ?? new DOMRect(0, 0, 800, 600);
    },
  );
  Object.defineProperty(HTMLElement.prototype, "scrollIntoView", {
    configurable: true,
    value(this: HTMLElement) {
      revealed.push(this);
    },
  });
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  if (nativeScroll)
    Object.defineProperty(
      HTMLElement.prototype,
      "scrollIntoView",
      nativeScroll,
    );
  else Reflect.deleteProperty(HTMLElement.prototype, "scrollIntoView");
});

function scene(
  kind: "unit" | "label" | "group" | "trigger" = "unit",
  sticky = false,
) {
  const input =
    kind === "trigger" ? (
      <button aria-label="Interface font">
        <span>System</span>
      </button>
    ) : (
      <input aria-label="Interface font" />
    );
  const content =
    kind === "label" ? (
      <label data-testid="unit">Interface font{input}</label>
    ) : kind === "group" ? (
      <FormGroup label="Interface font">{input}</FormGroup>
    ) : (
      <div data-focus-unit="" data-testid="unit">
        <span>Interface font</span>
        {input}
      </div>
    );
  const view = render(
    <div data-testid="viewport" style={{ overflowY: "auto", height: 300 }}>
      <Panel
        label="Appearance"
        controls={sticky ? <button>Refresh</button> : undefined}
      >
        {content}
      </Panel>
    </div>,
  );
  const unit =
    kind === "group" ? screen.getByRole("group") : screen.getByTestId("unit");
  const field = screen.getByRole(kind === "trigger" ? "button" : "textbox", {
    name: "Interface font",
  });
  rectangles.set(screen.getByTestId("viewport"), new DOMRect(0, 200, 800, 300));
  rectangles.set(unit, new DOMRect(0, 180, 300, 100));
  rectangles.set(field, new DOMRect(0, 240, 200, 32));
  if (sticky) {
    const controls = view.container.querySelector(
      '[data-slot="panel-controls"]',
    )!;
    rectangles.set(controls, new DOMRect(0, 200, 800, 50));
  }
  return { unit, field };
}

it.each(["unit", "label", "group"] as const)(
  "reveals the complete %s when its control is visible but its label is above the viewport",
  (kind) => {
    const { unit, field } = scene(kind);
    act(() => field.focus());
    expect(revealed).toEqual([unit]);
    expect(document.activeElement).toBe(field);
  },
);

it("reveals the whole unit when its lower edge is clipped", () => {
  const { unit, field } = scene();
  rectangles.set(unit, new DOMRect(0, 430, 300, 90));
  rectangles.set(field, new DOMRect(0, 440, 200, 32));
  act(() => field.focus());
  expect(revealed).toEqual([unit]);
});

it("keeps a fully visible unit still", () => {
  const { unit, field } = scene();
  rectangles.set(unit, new DOMRect(0, 220, 300, 100));
  act(() => field.focus());
  expect(revealed).toEqual([]);
});

it("uses the control when the complete unit is taller than the available viewport", () => {
  const { unit, field } = scene();
  rectangles.set(unit, new DOMRect(0, 100, 300, 500));
  act(() => field.focus());
  expect(revealed).toEqual([]);
  act(() => field.blur());
  rectangles.set(field, new DOMRect(0, 160, 200, 32));
  act(() => field.focus());
  expect(revealed).toEqual([field]);
});

it("keeps a fitting unit below the panel controls", () => {
  const { unit, field } = scene("unit", true);
  rectangles.set(unit, new DOMRect(0, 230, 300, 100));
  rectangles.set(field, new DOMRect(0, 270, 200, 32));
  act(() => field.focus());
  expect(revealed).toEqual([unit]);
});

it("uses the nearest native label inside a larger unit", () => {
  const { unit, field } = scene();
  const label = document.createElement("label");
  label.textContent = "Font family";
  unit.append(label);
  label.append(field);
  rectangles.set(unit, new DOMRect(0, 100, 300, 350));
  rectangles.set(label, new DOMRect(0, 180, 300, 100));
  act(() => field.focus());
  expect(revealed).toEqual([label]);
});

it("keeps the pressed control still until pointer activation finishes", () => {
  const { field } = scene();
  fireEvent.pointerDown(field);
  act(() => field.focus());
  expect(revealed).toEqual([]);
  expect(document.activeElement).toBe(field);
});

it("recognizes a press on a trigger's child", () => {
  const { field } = scene("trigger");
  fireEvent.pointerDown(screen.getByText("System"));
  act(() => field.focus());
  expect(revealed).toEqual([]);
});

it("keeps a pressed slider proxy still when its native input receives focus", () => {
  const { unit, field } = scene();
  const proxy = document.createElement("span");
  proxy.dataset.focusProxy = "";
  unit.append(proxy);
  proxy.append(field);
  fireEvent.pointerDown(proxy);
  act(() => field.focus());
  expect(revealed).toEqual([]);
});

it("keeps a pressed native label still when it focuses its input", () => {
  const { unit, field } = scene("label");
  fireEvent.pointerDown(unit);
  act(() => field.focus());
  expect(revealed).toEqual([]);
});

it("still reveals a different field autofocused by a pointer action", () => {
  const { unit, field } = scene();
  const opener = document.createElement("button");
  opener.textContent = "Add field";
  unit.parentElement!.append(opener);
  fireEvent.pointerDown(opener);
  act(() => field.focus());
  expect(revealed).toEqual([unit]);
});

it("does not let an old press suppress keyboard return after focus leaves the body", () => {
  const { unit, field } = scene();
  rectangles.set(unit, new DOMRect(0, 220, 300, 100));
  act(() => field.focus());
  fireEvent.pointerDown(field);
  const outside = document.createElement("button");
  document.body.append(outside);
  act(() => outside.focus());
  rectangles.set(unit, new DOMRect(0, 180, 300, 100));
  act(() => field.focus());
  outside.remove();
  expect(revealed).toEqual([unit]);
});
