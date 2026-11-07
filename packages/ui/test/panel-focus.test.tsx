import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import {
  Button,
  ContentVisibility,
  CodeBlock,
  Checkbox,
  Disclosure,
  FormActions,
  FormGroup,
  Panel,
  RecordBody,
  RecordForm,
  Tile,
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
    show: () => view.rerender(frame(true)),
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

it.each([false, true])(
  "sizes nested readers to the clipped host without requiring body focus (controls: %s)",
  (sticky) => {
    const { viewport, field } = scene("unit", sticky);
    const panel = field.closest<HTMLElement>('[data-slot="panel"]')!;
    resize(viewport);
    expect(panel.style.getPropertyValue("--reading-frame-height")).toBe(
      "300px",
    );

    const outside = document.createElement("button");
    document.body.append(outside);
    act(() => outside.focus());
    rectangles.set(viewport, new DOMRect(0, 200, 400, 220));
    fireEvent(window, new Event("resize"));
    expect(panel.style.getPropertyValue("--reading-frame-height")).toBe(
      "220px",
    );
    expect(document.activeElement).toBe(outside);
    expect(revealed).toEqual([]);
    outside.remove();
  },
);

it("intersects nested host clips and the window when sizing source readers", () => {
  render(
    <div data-testid="outer" style={{ overflowY: "hidden" }}>
      <div data-testid="inner" style={{ overflowY: "auto" }}>
        <Panel label="Captured evidence">
          <CodeBlock label="Original source">
            Complete original source
          </CodeBlock>
        </Panel>
      </div>
    </div>,
  );
  const outer = screen.getByTestId("outer");
  const inner = screen.getByTestId("inner");
  const panel = screen.getByRole("region", { name: "Captured evidence" });
  rectangles.set(outer, new DOMRect(0, 80, 800, 380));
  rectangles.set(inner, new DOMRect(0, 140, 800, 340));
  resize(inner);
  expect(panel.style.getPropertyValue("--reading-frame-height")).toBe("320px");

  rectangles.set(outer, new DOMRect(0, 80, 800, 580));
  rectangles.set(inner, new DOMRect(0, 140, 800, 560));
  resize(outer);
  expect(panel.style.getPropertyValue("--reading-frame-height")).toBe("460px");
  expect(revealed).toEqual([]);
});

it("budgets nested readers inside each host's border and scroll padding", () => {
  render(
    <div
      data-testid="outer"
      style={{
        overflowY: "hidden",
        borderStyle: "solid",
        borderTopWidth: 2,
        borderBottomWidth: 2,
      }}
    >
      <div
        data-testid="inner"
        style={{
          overflowY: "auto",
          scrollPaddingTop: 16,
          scrollPaddingBottom: 16,
        }}
      >
        <Panel label="Captured evidence">
          <CodeBlock label="Original source">
            Complete original source
          </CodeBlock>
        </Panel>
      </div>
    </div>,
  );
  const panel = screen.getByRole("region", { name: "Captured evidence" });
  // The inner padding bounds the start; the outer border bounds the end.
  rectangles.set(screen.getByTestId("outer"), new DOMRect(0, 80, 800, 380));
  rectangles.set(screen.getByTestId("inner"), new DOMRect(0, 70, 800, 410));
  resize(screen.getByTestId("inner"));
  expect(panel.style.getPropertyValue("--reading-frame-height")).toBe("372px");
  expect(revealed).toEqual([]);
});

function paddedScene() {
  render(
    <div
      data-testid="viewport"
      style={{
        overflowY: "auto",
        borderStyle: "solid",
        borderTopWidth: 2,
        borderBottomWidth: 2,
        scrollPaddingTop: 16,
        scrollPaddingBottom: 16,
      }}
    >
      <Panel label="Providers">
        <div data-focus-unit="" data-testid="unit">
          <span>Embedding model</span>
          <input aria-label="Embedding model" />
        </div>
      </Panel>
    </div>,
  );
  const viewport = screen.getByTestId("viewport");
  const unit = screen.getByTestId("unit");
  const field = screen.getByRole("textbox", {
    name: "Embedding model",
  }) as HTMLInputElement;
  rectangles.set(viewport, new DOMRect(0, 200, 800, 300));
  rectangles.set(unit, new DOMRect(0, 230, 300, 172));
  rectangles.set(field, new DOMRect(0, 360, 200, 32));
  return {
    viewport,
    unit,
    field,
    panel: field.closest<HTMLElement>('[data-slot="panel"]')!,
  };
}

it("reveals a visible field whose context outgrows the padded host frame during reflow", () => {
  const { viewport, unit, field, panel } = paddedScene();
  field.value = "text-embedding-3-large";
  field.setSelectionRange(5, 14, "backward");
  act(() => field.focus());
  expect(revealed).toEqual([]);

  // The 172px context fits the 189px border box but not its 153px reading frame.
  rectangles.set(viewport, new DOMRect(0, 174, 560, 189));
  rectangles.set(unit, new DOMRect(0, 192, 300, 172));
  rectangles.set(field, new DOMRect(0, 332, 200, 32));
  resize(viewport);
  expect(revealed).toEqual([field]);
  expect(panel.style.getPropertyValue("--reading-frame-height")).toBe("153px");
  expect(document.activeElement).toBe(field);
  expect(field.value).toBe("text-embedding-3-large");
  expect([
    field.selectionStart,
    field.selectionEnd,
    field.selectionDirection,
  ]).toEqual([5, 14, "backward"]);
});

it("reveals a context that fits the padded host frame but starts inside its padding", () => {
  const { viewport, unit, field } = paddedScene();
  act(() => field.focus());
  rectangles.set(viewport, new DOMRect(0, 174, 560, 220));
  rectangles.set(unit, new DOMRect(0, 180, 300, 172));
  rectangles.set(field, new DOMRect(0, 320, 200, 32));
  resize(viewport);
  expect(revealed).toEqual([unit]);
  expect(document.activeElement).toBe(field);
});

it("retires and renews the reader budget with retained content visibility", () => {
  const { viewport, field, hide, show, unmount } = scene("unit", true);
  const panel = field.closest<HTMLElement>('[data-slot="panel"]')!;
  resize(viewport);
  expect(panel.style.getPropertyValue("--reading-frame-height")).toBe("300px");
  hide();
  expect(panel.style.getPropertyValue("--reading-frame-height")).toBe("");
  rectangles.set(viewport, new DOMRect(0, 200, 400, 140));
  resize(viewport);
  expect(panel.style.getPropertyValue("--reading-frame-height")).toBe("");
  show();
  expect(panel.style.getPropertyValue("--reading-frame-height")).toBe("140px");
  unmount();
  expect(panel.style.getPropertyValue("--reading-frame-height")).toBe("");
  expect(observations.every(({ nodes }) => !nodes.has(viewport))).toBe(true);
});

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

it("keeps the reading witness through its own queued scroll during native reflow", () => {
  const { unit, field, viewport } = scene();
  rectangles.set(unit, new DOMRect(0, 220, 300, 100));
  act(() => field.focus());
  vi.spyOn(HTMLElement.prototype, "scrollIntoView").mockImplementation(
    function (this: HTMLElement) {
      revealed.push(this);
      viewport.scrollTop += 40;
      rectangles.set(field, new DOMRect(0, 180, 200, 32));
    },
  );
  rectangles.set(unit, new DOMRect(0, 100, 300, 400));
  rectangles.set(field, new DOMRect(0, 340, 200, 32));
  rectangles.set(viewport, new DOMRect(0, 200, 400, 100));
  resize(viewport);
  fireEvent.scroll(viewport);

  rectangles.set(field, new DOMRect(0, 340, 200, 32));
  rectangles.set(viewport, new DOMRect(0, 200, 420, 80));
  resize(viewport);
  expect(revealed).toEqual([field, field]);
  expect(document.activeElement).toBe(field);
});
it.each([
  { controlsHeight: 75, wheel: false, reveal: true },
  { controlsHeight: 100, wheel: false, reveal: true },
  { controlsHeight: 150, wheel: false, reveal: true },
  { controlsHeight: 150, wheel: true, reveal: false },
])(
  "reveals past oversized controls and retains reflow follow with controls=$controlsHeight and wheel=$wheel",
  ({ controlsHeight, wheel, reveal }) => {
    const { unit, field, viewport } = scene("unit", true);
    const controls = field
      .closest('[data-slot="panel"]')!
      .querySelector('[data-slot="panel-controls"]')!;
    const body = field.closest<HTMLElement>('[data-slot="panel-body"]')!;
    body.style.lineHeight = "20px";
    body.parentElement!.style.rowGap = "10px";
    rectangles.set(viewport, new DOMRect(0, 200, 800, 100));
    rectangles.set(controls, new DOMRect(0, 200, 800, controlsHeight));
    rectangles.set(unit, new DOMRect(0, 210, 300, 200));
    rectangles.set(field, new DOMRect(0, 220, 200, 100));
    act(() => field.focus());
    expect(revealed).toEqual([field]);
    rectangles.set(field, new DOMRect(0, 200, 200, 100));
    viewport.scrollTop = 80;
    if (wheel) fireEvent.wheel(viewport, { deltaY: 80 });
    fireEvent.scroll(viewport);
    rectangles.set(controls, new DOMRect(0, 200, 800, 50));
    resize(controls);
    expect(revealed).toEqual(reveal ? [field, field] : [field]);
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

function formScene(enclosed = false) {
  const content = (feedback: string | null) => {
    const panel = (
      <Panel label="Memory" controls={<button>Add</button>}>
        <RecordForm aria-label="Delete memory" data-focus-unit="">
          <h3>Delete memory</h3>
          <p>Prefer concise answers.</p>
          <FormActions
            label="Delete actions"
            feedback={feedback && <p role="alert">{feedback}</p>}
          >
            <button>Cancel</button>
            <button>Delete</button>
          </FormActions>
        </RecordForm>
      </Panel>
    );
    return (
      <div data-testid="viewport" style={{ overflowY: "auto", height: 300 }}>
        {enclosed ? (
          <div
            data-focus-unit=""
            data-testid="enclosing"
            style={{ overflowY: "visible" }}
          >
            {panel}
          </div>
        ) : (
          panel
        )}
      </div>
    );
  };
  const view = render(content(null));
  const form = screen.getByRole("region", { name: "Delete memory" });
  const actions = screen.getByRole("group", { name: "Delete actions" });
  const cancel = screen.getByRole("button", { name: "Cancel" });
  rectangles.set(screen.getByTestId("viewport"), new DOMRect(0, 200, 800, 300));
  rectangles.set(
    view.container.querySelector('[data-slot="panel-controls"]')!,
    new DOMRect(0, 200, 800, 50),
  );
  return {
    form,
    actions,
    cancel,
    update: () => view.rerender(content("The memory changed elsewhere.")),
  };
}

it("reveals the complete fitting form even when its focused actions already fit", () => {
  const { form, actions, cancel } = formScene();
  rectangles.set(form, new DOMRect(0, 180, 300, 200));
  rectangles.set(actions, new DOMRect(0, 300, 300, 80));
  rectangles.set(cancel, new DOMRect(0, 310, 100, 32));
  act(() => cancel.focus());
  expect(revealed).toEqual([form]);
  expect(document.activeElement).toBe(cancel);
});

it("falls back to the fitting actions when the complete form is too tall", () => {
  const { form, actions, cancel } = formScene();
  rectangles.set(form, new DOMRect(0, 100, 300, 400));
  rectangles.set(actions, new DOMRect(0, 440, 300, 80));
  rectangles.set(cancel, new DOMRect(0, 450, 100, 32));
  act(() => cancel.focus());
  expect(revealed).toEqual([actions]);
  expect(document.activeElement).toBe(cancel);
});

it("narrows the reading context when feedback outgrows the form without moving focus", () => {
  const { form, actions, cancel, update } = formScene();
  rectangles.set(form, new DOMRect(0, 260, 300, 200));
  rectangles.set(actions, new DOMRect(0, 380, 300, 80));
  rectangles.set(cancel, new DOMRect(0, 390, 100, 32));
  act(() => cancel.focus());
  expect(revealed).toEqual([]);
  rectangles.set(form, new DOMRect(0, 260, 300, 300));
  rectangles.set(actions, new DOMRect(0, 440, 300, 110));
  rectangles.set(cancel, new DOMRect(0, 450, 100, 32));
  update();
  expect(revealed).toEqual([actions]);
  expect(document.activeElement).toBe(cancel);
  expect(actions.contains(screen.getByRole("alert"))).toBe(true);
});

it("does not choose a fitting declared unit outside the panel body", () => {
  const { form, actions, cancel } = formScene(true);
  rectangles.set(
    screen.getByTestId("enclosing"),
    new DOMRect(0, 180, 300, 100),
  );
  rectangles.set(form, new DOMRect(0, 180, 300, 200));
  rectangles.set(actions, new DOMRect(0, 300, 300, 80));
  rectangles.set(cancel, new DOMRect(0, 310, 100, 32));
  act(() => cancel.focus());
  expect(revealed).toEqual([form]);
});

function disclosureScene(variant: "plain" | "row") {
  const view = render(
    <div data-testid="viewport" style={{ overflowY: "auto", height: 300 }}>
      <Panel label="Skills" controls={<button>Refresh</button>}>
        <Disclosure
          variant={variant}
          title="Literal proposal name"
          defaultOpen
          actions={<button>Approve</button>}
        >
          <input aria-label="Review note" />
        </Disclosure>
      </Panel>
    </div>,
  );
  const header = screen
    .getByText("Literal proposal name")
    .closest("button")!.parentElement!;
  const approve = screen.getByRole("button", { name: "Approve" });
  const note = screen.getByRole("textbox", { name: "Review note" });
  rectangles.set(screen.getByTestId("viewport"), new DOMRect(0, 200, 800, 300));
  rectangles.set(
    view.container.querySelector('[data-slot="panel-controls"]')!,
    new DOMRect(0, 200, 800, 50),
  );
  return { header, approve, note };
}

it.each(["plain", "row"] as const)(
  "reveals the complete %s disclosure summary with its focused action",
  (variant) => {
    const { header, approve } = disclosureScene(variant);
    rectangles.set(header, new DOMRect(0, 230, 300, 64));
    rectangles.set(approve, new DOMRect(200, 260, 100, 32));
    act(() => approve.focus());
    expect(revealed).toEqual([header]);
    expect(document.activeElement).toBe(approve);
  },
);

it.each(["plain", "row"] as const)(
  "keeps the focused action fallback for an oversized %s disclosure header",
  (variant) => {
    const { header, approve } = disclosureScene(variant);
    rectangles.set(header, new DOMRect(0, 100, 300, 360));
    rectangles.set(approve, new DOMRect(200, 300, 100, 32));
    act(() => approve.focus());
    expect(revealed).toEqual([]);
    act(() => approve.blur());
    rectangles.set(approve, new DOMRect(200, 230, 100, 32));
    act(() => approve.focus());
    expect(revealed).toEqual([approve]);
    expect(document.activeElement).toBe(approve);
  },
);

it.each(["plain", "row"] as const)(
  "does not read expanded %s disclosure content through its header",
  (variant) => {
    const { header, note } = disclosureScene(variant);
    rectangles.set(header, new DOMRect(0, 180, 300, 40));
    rectangles.set(note, new DOMRect(0, 230, 200, 32));
    act(() => note.focus());
    expect(revealed).toEqual([note]);
    expect(document.activeElement).toBe(note);
  },
);

it("reveals a tile's text and action without its oversized visual", () => {
  const download = vi.fn();
  render(
    <div data-testid="viewport" style={{ overflowY: "auto", height: 300 }}>
      <Panel label="Brand">
        <Tile
          data-testid="tile"
          visual={<img alt="Primary logo preview" src="data:," />}
        >
          <strong>Primary logo</strong>
          <RecordBody>For light surfaces.</RecordBody>
          <Button onClick={download}>Download primary logo</Button>
        </Tile>
      </Panel>
    </div>,
  );
  const image = screen.getByRole("img", { name: "Primary logo preview" });
  const title = screen.getByText("Primary logo");
  const body = title.parentElement!;
  const button = screen.getByRole("button", { name: "Download primary logo" });
  rectangles.set(screen.getByTestId("viewport"), new DOMRect(0, 200, 800, 300));
  rectangles.set(screen.getByTestId("tile"), new DOMRect(0, -320, 300, 680));
  rectangles.set(image, new DOMRect(0, -300, 300, 460));
  rectangles.set(body, new DOMRect(0, 180, 300, 160));
  rectangles.set(button, new DOMRect(0, 300, 200, 32));
  act(() => button.focus());
  expect(revealed).toEqual([body]);
  expect(document.activeElement).toBe(button);
  expect(body.contains(screen.getByText("For light surfaces."))).toBe(true);
  expect(body.contains(button)).toBe(true);
  expect(body.contains(image)).toBe(false);

  act(() => button.blur());
  revealed.length = 0;
  rectangles.set(body, new DOMRect(0, 100, 300, 400));
  act(() => button.focus());
  expect(revealed).toEqual([]);
  act(() => button.blur());
  rectangles.set(button, new DOMRect(0, 160, 200, 32));
  act(() => button.focus());
  expect(revealed).toEqual([button]);
  expect(document.activeElement).toBe(button);
  expect(download).not.toHaveBeenCalled();
});
