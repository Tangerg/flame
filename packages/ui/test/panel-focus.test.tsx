import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import {
  ContentVisibility,
  CodeBlock,
  Checkbox,
  FormActions,
  FormGroup,
  Panel,
} from "../dist/index.js";

let rectangles: Map<Element, DOMRect>;
let revealed: Element[];
let observations: { callback: ResizeObserverCallback; nodes: Set<Element> }[];
const nativeScroll = Object.getOwnPropertyDescriptor(
  HTMLElement.prototype,
  "scrollIntoView",
);

beforeEach(() => {
  rectangles = new Map();
  revealed = [];
  observations = [];
  vi.stubGlobal(
    "ResizeObserver",
    class {
      readonly nodes = new Set<Element>();
      constructor(readonly callback: ResizeObserverCallback) {
        observations.push(this);
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
  vi.unstubAllGlobals();
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
  kind: "unit" | "label" | "group" | "trigger" | "choice" = "unit",
  sticky = false,
) {
  const input =
    kind === "choice" ? (
      <Checkbox
        checked={false}
        onCheckedChange={() => {}}
        label="Generative UI"
      />
    ) : kind === "trigger" ? (
      <button aria-label="Interface font">
        <span>System</span>
      </button>
    ) : (
      <input aria-label="Interface font" />
    );
  const content =
    kind === "label" ? (
      <label data-testid="unit">Interface font{input}</label>
    ) : kind === "group" || kind === "choice" ? (
      <FormGroup label={kind === "choice" ? "Capabilities" : "Interface font"}>
        {input}
      </FormGroup>
    ) : (
      <div data-focus-unit="" data-testid="unit">
        <span>Interface font</span>
        {input}
      </div>
    );
  const frame = (visible: boolean) => (
    <ContentVisibility visible={visible}>
      <div data-testid="viewport" style={{ overflowY: "auto", height: 300 }}>
        <Panel
          label="Appearance"
          controls={sticky ? <button>Refresh</button> : undefined}
        >
          {content}
        </Panel>
      </div>
    </ContentVisibility>
  );
  const view = render(frame(true));
  const unit =
    kind === "group" || kind === "choice"
      ? screen.getByRole("group")
      : screen.getByTestId("unit");
  const field = screen.getByRole(
    kind === "choice" ? "checkbox" : kind === "trigger" ? "button" : "textbox",
    {
      name: kind === "choice" ? "Generative UI" : "Interface font",
    },
  );
  rectangles.set(screen.getByTestId("viewport"), new DOMRect(0, 200, 800, 300));
  rectangles.set(unit, new DOMRect(0, 180, 300, 100));
  rectangles.set(field, new DOMRect(0, 240, 200, 32));
  if (kind === "choice")
    rectangles.set(field.closest("label")!, new DOMRect(0, 240, 300, 32));
  if (sticky) {
    const controls = view.container.querySelector(
      '[data-slot="panel-controls"]',
    )!;
    rectangles.set(controls, new DOMRect(0, 200, 800, 50));
  }
  return {
    unit,
    field,
    viewport: screen.getByTestId("viewport"),
    hide: () => view.rerender(frame(false)),
    unmount: view.unmount,
  };
}

function resize(node: Element) {
  act(() => {
    for (const observation of observations) {
      if (observation.nodes.has(node))
        observation.callback(
          [{ target: node } as ResizeObserverEntry],
          {} as ResizeObserver,
        );
    }
  });
}

it("keeps its DOM ref attached across ordinary content updates", () => {
  const projected: (HTMLElement | null)[] = [];
  const reference = (node: HTMLElement | null) => {
    projected.push(node);
  };
  const view = render(
    <Panel ref={reference} label="Draft">
      Before editing
    </Panel>,
  );
  const panel = screen.getByRole("region", { name: "Draft" });
  view.rerender(
    <Panel ref={reference} label="Draft">
      During editing
    </Panel>,
  );
  expect(projected).toEqual([panel]);
  view.unmount();
  expect(projected).toEqual([panel, null]);
});

it("reveals a field focused while new children are committed", () => {
  const frame = (editing: boolean) => (
    <div data-testid="viewport" style={{ overflowY: "auto", height: 300 }}>
      <Panel label="Draft">
        {editing ? (
          <input
            aria-label="New draft"
            ref={(node) => {
              node?.focus();
            }}
          />
        ) : (
          <p>No draft</p>
        )}
      </Panel>
    </div>
  );
  const view = render(frame(false));
  rectangles.set(screen.getByTestId("viewport"), new DOMRect(0, 200, 800, 300));
  view.rerender(frame(true));
  const field = screen.getByRole("textbox", { name: "New draft" });
  expect(document.activeElement).toBe(field);
  expect(revealed).toEqual([field]);
});

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

it("reveals the code caption with its focused reader when the complete block fits", () => {
  const view = render(
    <div data-testid="viewport" style={{ overflowY: "auto", height: 300 }}>
      <Panel label="Captured evidence" controls={<button>Refresh</button>}>
        <CodeBlock label="Original source">Complete original source</CodeBlock>
      </Panel>
    </div>,
  );
  const reader = screen.getByRole("region", { name: "Original source" });
  const block = view.container.querySelector('[data-slot="code-block"]')!;
  const controls = view.container.querySelector(
    '[data-slot="panel-controls"]',
  )!;
  rectangles.set(screen.getByTestId("viewport"), new DOMRect(0, 200, 800, 300));
  rectangles.set(controls, new DOMRect(0, 200, 800, 50));
  rectangles.set(block, new DOMRect(0, 230, 800, 200));
  rectangles.set(reader, new DOMRect(0, 270, 800, 160));
  act(() => reader.focus());
  expect(revealed).toEqual([block]);
  expect(document.activeElement).toBe(reader);

  act(() => reader.blur());
  revealed.length = 0;
  rectangles.set(block, new DOMRect(0, 230, 800, 400));
  act(() => reader.focus());
  expect(revealed).toEqual([]);
  expect(document.activeElement).toBe(reader);
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

it("keeps a fitting choice group's heading with its focused option", () => {
  const { unit, field } = scene("choice");
  act(() => field.focus());
  expect(revealed).toEqual([unit]);
  expect(document.activeElement).toBe(field);
  expect(field.getAttribute("aria-checked")).toBe("false");
});

it("preserves a choice press before revealing the group on a later keyboard return", () => {
  const { unit, field, viewport } = scene("choice");
  fireEvent.pointerDown(field.closest("label")!);
  act(() => field.focus());
  resize(viewport);
  expect(revealed).toEqual([]);
  fireEvent.pointerUp(field);
  act(() => field.blur());
  act(() => field.focus());
  expect(revealed).toEqual([unit]);
  expect(document.activeElement).toBe(field);
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

it("keeps a visible field, draft and selection through host viewport reflow", () => {
  const { unit, field, viewport } = scene();
  rectangles.set(unit, new DOMRect(0, 220, 300, 100));
  const input = field as HTMLInputElement;
  input.value = "Continue the unfinished setup";
  input.setSelectionRange(3, 9, "backward");
  act(() => input.focus());
  rectangles.set(viewport, new DOMRect(0, 200, 400, 40));
  resize(viewport);
  expect(revealed).toEqual([field]);
  expect(document.activeElement).toBe(field);
  expect(input.value).toBe("Continue the unfinished setup");
  expect([
    input.selectionStart,
    input.selectionEnd,
    input.selectionDirection,
  ]).toEqual([3, 9, "backward"]);
});

it("keeps a fitting label and field below a moved viewport edge", () => {
  const { unit, field, viewport } = scene();
  rectangles.set(unit, new DOMRect(0, 220, 300, 100));
  act(() => field.focus());
  rectangles.set(viewport, new DOMRect(0, 250, 800, 250));
  resize(viewport);
  expect(revealed).toEqual([unit]);
});

it("does not move focus that still fits after a viewport resize", () => {
  const { unit, field, viewport } = scene();
  rectangles.set(unit, new DOMRect(0, 220, 300, 100));
  act(() => field.focus());
  rectangles.set(viewport, new DOMRect(0, 200, 400, 250));
  resize(viewport);
  expect(revealed).toEqual([]);
  expect(document.activeElement).toBe(field);
});

it("keeps its reading witness through a zero-displacement reflow notification", () => {
  const { unit, field, viewport } = scene();
  rectangles.set(unit, new DOMRect(0, 220, 300, 100));
  act(() => field.focus());
  rectangles.set(unit, new DOMRect(0, 340, 300, 400));
  rectangles.set(field, new DOMRect(0, 380, 200, 32));
  rectangles.set(viewport, new DOMRect(0, 200, 400, 150));
  fireEvent.scroll(viewport);
  resize(viewport);
  expect(revealed).toEqual([field]);
});

it("keeps focus through scroll anchoring before the resized frame is observed", () => {
  const { unit, field, viewport } = scene();
  rectangles.set(unit, new DOMRect(0, 220, 300, 100));
  act(() => field.focus());
  viewport.scrollTop = 80;
  rectangles.set(unit, new DOMRect(0, 340, 300, 400));
  rectangles.set(field, new DOMRect(0, 380, 200, 32));
  rectangles.set(viewport, new DOMRect(0, 200, 400, 150));
  fireEvent.scroll(viewport);
  resize(viewport);
  expect(revealed).toEqual([field]);
});

it.each([
  { controlsHeight: 100, wheel: false, reveal: true },
  { controlsHeight: 150, wheel: false, reveal: true },
  { controlsHeight: 150, wheel: true, reveal: false },
])(
  "retains viewport space recovery with controls=$controlsHeight and wheel=$wheel",
  ({ controlsHeight, wheel, reveal }) => {
    const { unit, field, viewport } = scene("unit", true);
    const controls = field
      .closest('[data-slot="panel"]')!
      .querySelector('[data-slot="panel-controls"]')!;
    rectangles.set(viewport, new DOMRect(0, 200, 800, 100));
    rectangles.set(controls, new DOMRect(0, 200, 800, controlsHeight));
    rectangles.set(unit, new DOMRect(0, 210, 300, 200));
    rectangles.set(field, new DOMRect(0, 220, 200, 100));
    act(() => field.focus());
    expect(revealed).toEqual([]);
    viewport.scrollTop = 80;
    if (wheel) fireEvent.wheel(viewport, { deltaY: 80 });
    fireEvent.scroll(viewport);
    rectangles.set(controls, new DOMRect(0, 200, 800, 50));
    resize(controls);
    expect(revealed).toEqual(reveal ? [field] : []);
    expect(document.activeElement).toBe(field);
  },
);

it("responds to native window reflow before an element observer notifies", () => {
  const { unit, field, viewport } = scene();
  rectangles.set(unit, new DOMRect(0, 220, 300, 100));
  act(() => field.focus());
  rectangles.set(viewport, new DOMRect(0, 200, 400, 40));
  fireEvent(window, new Event("resize"));
  expect(revealed).toEqual([field]);
});

it("does not pull a reader back after a deliberate wheel gesture", () => {
  const { unit, field, viewport } = scene();
  rectangles.set(unit, new DOMRect(0, 220, 300, 100));
  act(() => field.focus());
  fireEvent.wheel(viewport, { deltaY: 200 });
  rectangles.set(unit, new DOMRect(0, 430, 300, 100));
  rectangles.set(field, new DOMRect(0, 440, 200, 32));
  rectangles.set(viewport, new DOMRect(0, 200, 400, 200));
  resize(viewport);
  expect(revealed).toEqual([]);
  expect(document.activeElement).toBe(field);
});

it("retires focus follow when a moved scrollbar leaves the field behind", () => {
  const { unit, field, viewport } = scene();
  rectangles.set(unit, new DOMRect(0, 220, 300, 100));
  act(() => field.focus());
  viewport.scrollTop = 150;
  rectangles.set(unit, new DOMRect(0, 530, 300, 100));
  rectangles.set(field, new DOMRect(0, 540, 200, 32));
  fireEvent.scroll(viewport);
  rectangles.set(viewport, new DOMRect(0, 200, 400, 200));
  resize(viewport);
  expect(revealed).toEqual([]);
});

it("preserves a later outside focus during host reflow", () => {
  const { unit, field, viewport } = scene();
  rectangles.set(unit, new DOMRect(0, 220, 300, 100));
  act(() => field.focus());
  const outside = document.createElement("button");
  document.body.append(outside);
  act(() => outside.focus());
  rectangles.set(viewport, new DOMRect(0, 200, 400, 40));
  resize(viewport);
  expect(revealed).toEqual([]);
  expect(document.activeElement).toBe(outside);
  outside.remove();
});

it("does not reveal a retained inactive owner when its controls resize", () => {
  const { unit, field, hide } = scene("unit", true);
  rectangles.set(unit, new DOMRect(0, 270, 300, 100));
  rectangles.set(field, new DOMRect(0, 280, 200, 32));
  act(() => field.focus());
  hide();
  const controls = document.querySelector('[data-slot="panel-controls"]')!;
  rectangles.set(controls, new DOMRect(0, 200, 800, 150));
  resize(controls);
  expect(revealed).toEqual([]);
});

it("defers reflow movement while the focused control's press is unfinished", () => {
  const { unit, field, viewport } = scene();
  rectangles.set(unit, new DOMRect(0, 220, 300, 100));
  fireEvent.pointerDown(field);
  act(() => field.focus());
  rectangles.set(viewport, new DOMRect(0, 200, 400, 40));
  resize(viewport);
  expect(revealed).toEqual([]);
  fireEvent.pointerUp(field);
  resize(viewport);
  expect(revealed).toEqual([field]);
});

it("retires a cancelled pointer press before subsequent reflow", () => {
  const { unit, field, viewport } = scene();
  rectangles.set(unit, new DOMRect(0, 220, 300, 100));
  fireEvent.pointerDown(field);
  act(() => field.focus());
  fireEvent.pointerCancel(field);
  rectangles.set(viewport, new DOMRect(0, 200, 400, 40));
  resize(viewport);
  expect(revealed).toEqual([field]);
});

it("keeps the visible beginning of an oversized field still through reflow", () => {
  const { unit, field, viewport } = scene();
  rectangles.set(unit, new DOMRect(0, 220, 300, 500));
  rectangles.set(field, new DOMRect(0, 240, 200, 420));
  act(() => field.focus());
  rectangles.set(viewport, new DOMRect(0, 200, 400, 200));
  resize(viewport);
  expect(revealed).toEqual([]);
  expect(document.activeElement).toBe(field);
});

it("retires its viewport observation when the panel unmounts", () => {
  const { unit, field, viewport, unmount } = scene();
  rectangles.set(unit, new DOMRect(0, 220, 300, 100));
  act(() => field.focus());
  unmount();
  rectangles.set(viewport, new DOMRect(0, 200, 400, 40));
  resize(viewport);
  expect(revealed).toEqual([]);
});

function actionScene() {
  const content = (feedback: string | null) => (
    <>
      <button>Outside editor</button>
      <div data-testid="viewport" style={{ overflowY: "auto", height: 300 }}>
        <Panel label="MCP servers" controls={<button>Add server</button>}>
          <FormActions
            label="Save server"
            feedback={feedback && <p role="alert">{feedback}</p>}
          >
            <button>Cancel</button>
            <button>Save</button>
          </FormActions>
        </Panel>
      </div>
    </>
  );
  const view = render(content(null));
  const unit = screen.getByRole("group", { name: "Save server" });
  const save = screen.getByRole("button", { name: "Save" });
  rectangles.set(screen.getByTestId("viewport"), new DOMRect(0, 200, 800, 300));
  rectangles.set(
    view.container.querySelector('[data-slot="panel-controls"]')!,
    new DOMRect(0, 200, 800, 50),
  );
  rectangles.set(unit, new DOMRect(0, 300, 300, 80));
  rectangles.set(save, new DOMRect(0, 310, 100, 32));
  act(() => save.focus());
  revealed.length = 0;
  return {
    unit,
    save,
    update: () => view.rerender(content("The server rejected the update.")),
  };
}

it("reveals changed feedback with the still-focused form actions", () => {
  const { unit, save, update } = actionScene();
  rectangles.set(unit, new DOMRect(0, 440, 300, 110));
  rectangles.set(save, new DOMRect(0, 450, 100, 32));
  update();
  expect(revealed).toEqual([unit]);
  expect(document.activeElement).toBe(save);
  expect(unit.contains(screen.getByRole("alert"))).toBe(true);
});

it("does not move already-visible form feedback", () => {
  const { unit, save, update } = actionScene();
  update();
  expect(unit.contains(screen.getByRole("alert"))).toBe(true);
  expect(revealed).toEqual([]);
  expect(document.activeElement).toBe(save);
});

it("does not reveal asynchronous form feedback after focus moved outside", () => {
  const { update } = actionScene();
  const outside = screen.getByRole("button", { name: "Outside editor" });
  act(() => outside.focus());
  update();
  expect(revealed).toEqual([]);
  expect(document.activeElement).toBe(outside);
});

it("keeps the focused action visible when feedback exceeds the viewport", () => {
  const { unit, save, update } = actionScene();
  rectangles.set(unit, new DOMRect(0, 440, 300, 400));
  rectangles.set(save, new DOMRect(0, 510, 100, 32));
  update();
  expect(revealed).toEqual([save]);
  expect(document.activeElement).toBe(save);
});
