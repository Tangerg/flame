import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from "@testing-library/react";
import {
  createRef,
  useRef,
  useState,
  type ComponentProps,
  type ReactNode,
  type RefObject,
} from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  ContentVisibility,
  FormActions,
  ModalDialog,
  Panel,
  Select,
} from "../dist/index.js";

type FinalFocus = ComponentProps<typeof ModalDialog>["finalFocus"];

let rectangles: Map<Element, DOMRect>;

beforeEach(() => {
  rectangles = new Map();
  // Base UI's clipping signal needs visible anchor geometry in layout-free Happy DOM.
  vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(
    function (this: HTMLElement) {
      const known = rectangles.get(this);
      if (known) return known;
      if (this.closest("[hidden]")) return new DOMRect();
      return this === document.documentElement || this === document.body
        ? new DOMRect(0, 0, 1024, 768)
        : new DOMRect(20, 20, 160, 32);
    },
  );
  vi.spyOn(document.documentElement, "clientWidth", "get").mockReturnValue(
    1024,
  );
  vi.spyOn(document.documentElement, "clientHeight", "get").mockReturnValue(
    768,
  );
  vi.spyOn(HTMLElement.prototype, "getClientRects").mockImplementation(
    function (this: HTMLElement) {
      return (this.closest("[hidden]") ? [] : [{}]) as unknown as DOMRectList;
    },
  );
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const options = [
  { value: "fast", label: "Fast" },
  { value: "deep", label: "Deep" },
];

function Scene({
  visible,
  finalFocus,
  dismissible = true,
  withFallback = true,
  innerHidden = false,
  alternateFallback = false,
}: {
  visible: boolean;
  finalFocus?: FinalFocus;
  dismissible?: boolean;
  withFallback?: boolean;
  innerHidden?: boolean;
  alternateFallback?: boolean;
}) {
  const fallback = useRef<HTMLButtonElement>(null);
  const [open, setOpen] = useState(false);
  const [title, setTitle] = useState("");
  const [model, setModel] = useState("fast");
  return (
    <>
      <button ref={alternateFallback ? null : fallback}>Conversation</button>
      <button ref={alternateFallback ? fallback : null}>Other material</button>
      <button>Outside</button>
      <div hidden={!visible}>
        <ContentVisibility
          visible={visible}
          fallbackFocus={withFallback ? fallback : undefined}
        >
          <button onClick={() => setOpen(true)}>Add schedule</button>
          <ModalDialog
            open={open}
            onOpenChange={setOpen}
            title="Add schedule"
            finalFocus={finalFocus}
            dismissible={dismissible}
            footer={<button onClick={() => setOpen(false)}>Cancel</button>}
          >
            <input
              aria-label="Title"
              value={title}
              onChange={(event) => setTitle(event.target.value)}
            />
            <ContentVisibility visible={!innerHidden}>
              <Select
                label="Model"
                value={model}
                onValueChange={setModel}
                options={options}
              />
            </ContentVisibility>
          </ModalDialog>
        </ContentVisibility>
      </div>
    </>
  );
}

async function openDialog(props: Partial<ComponentProps<typeof Scene>> = {}) {
  const view = render(<Scene visible {...props} />);
  const opener = screen.getByRole("button", { name: "Add schedule" });
  act(() => opener.focus());
  fireEvent.click(opener);
  const dialog = await screen.findByRole("dialog");
  fireEvent.change(screen.getByRole("textbox", { name: "Title" }), {
    target: { value: "Nightly" },
  });
  return {
    dialog,
    opener,
    hide: () => view.rerender(<Scene {...props} visible={false} />),
    reveal: () => view.rerender(<Scene {...props} visible />),
    showOtherMaterial: () =>
      view.rerender(<Scene {...props} visible={false} alternateFallback />),
  };
}

async function openSelect() {
  const trigger = screen.getByRole("combobox", { name: "Model" });
  act(() => trigger.focus());
  fireEvent.click(trigger);
  const listbox = await screen.findByRole("listbox");
  await vi.waitFor(() =>
    expect(listbox.contains(document.activeElement)).toBe(true),
  );
  return listbox;
}

function pressEscape() {
  fireEvent.keyDown(document.activeElement ?? document.body, {
    key: "Escape",
  });
}

describe("ModalDialog over a hidden owner", () => {
  it("keeps the dialog, draft and open Select through hide and reveal", async () => {
    const scene = await openDialog();
    const listbox = await openSelect();
    const focused = document.activeElement;

    scene.hide();

    expect(screen.getByRole("dialog")).toBe(scene.dialog);
    expect(screen.getByRole("listbox")).toBe(listbox);
    expect(document.activeElement).toBe(focused);
    expect(screen.getByRole("textbox", { name: "Title" })).toHaveProperty(
      "value",
      "Nightly",
    );

    scene.reveal();

    expect(screen.getByRole("dialog")).toBe(scene.dialog);
    expect(screen.getByRole("listbox")).toBe(listbox);
  });

  it("closes the nested popup on Escape before the dialog", async () => {
    const scene = await openDialog();
    await openSelect();
    scene.hide();

    pressEscape();
    await vi.waitFor(() => expect(screen.queryByRole("listbox")).toBeNull());
    expect(screen.getByRole("dialog")).toBe(scene.dialog);

    pressEscape();
    await vi.waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  it("returns focus to the host fallback instead of the hidden opener", async () => {
    const scene = await openDialog();
    scene.hide();

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    await vi.waitFor(() =>
      expect(document.activeElement).toBe(
        screen.getByRole("button", { name: "Conversation" }),
      ),
    );
    expect(document.activeElement).not.toBe(scene.opener);
  });

  it("does not focus the hidden opener without a fallback", async () => {
    const scene = await openDialog({ withFallback: false });
    scene.hide();

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    await vi.waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(document.activeElement).not.toBe(scene.opener);
  });

  it("resolves the current fallback target only when the task closes", async () => {
    const scene = await openDialog();
    scene.hide();
    scene.showOtherMaterial();

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    await vi.waitFor(() =>
      expect(document.activeElement).toBe(
        screen.getByRole("button", { name: "Other material" }),
      ),
    );
  });

  it("passes a visible caller's resolver through unchanged", async () => {
    const resolve = vi.fn(() => true);
    const scene = await openDialog({ finalFocus: resolve });

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    await vi.waitFor(() => expect(document.activeElement).toBe(scene.opener));
    expect(resolve).toHaveBeenCalledTimes(1);
  });

  it("honors do-nothing and visible explicit targets while hidden", async () => {
    const none = vi.fn(() => undefined);
    const first = await openDialog({ finalFocus: none });
    first.hide();
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    await vi.waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(none).toHaveBeenCalledTimes(1);
    expect(document.activeElement).not.toBe(
      screen.getByRole("button", { name: "Conversation" }),
    );
    cleanup();

    const second = await openDialog({ finalFocus: false });
    second.hide();
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    await vi.waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(document.activeElement).not.toBe(
      screen.getByRole("button", { name: "Conversation" }),
    );
    cleanup();

    const outside = vi.fn(() =>
      screen.getByRole("button", { name: "Outside" }),
    );
    const third = await openDialog({ finalFocus: outside });
    third.hide();
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    await vi.waitFor(() =>
      expect(document.activeElement).toBe(
        screen.getByRole("button", { name: "Outside" }),
      ),
    );
    expect(outside).toHaveBeenCalledTimes(1);
  });

  it("lets a hidden provider inside the dialog still retire its popup", async () => {
    const view = render(<Scene visible />);
    fireEvent.click(screen.getByRole("button", { name: "Add schedule" }));
    await screen.findByRole("dialog");
    await openSelect();

    view.rerender(<Scene visible innerHidden />);

    await vi.waitFor(() => expect(screen.queryByRole("listbox")).toBeNull());
    expect(screen.getByRole("dialog")).toBeTruthy();
  });

  it("keeps a non-dismissible dialog open while its owner is hidden", async () => {
    const scene = await openDialog({ dismissible: false });
    scene.hide();

    act(() => scene.dialog.focus());
    pressEscape();

    expect(screen.getByRole("dialog")).toBe(scene.dialog);
  });
});

function StyleHiddenScene({
  visibility,
  returnTo,
}: {
  visibility: "visible" | "hidden" | "collapse";
  returnTo: "ref" | "resolver";
}) {
  const fallback = useRef<HTMLButtonElement>(null);
  const opener = useRef<HTMLButtonElement>(null);
  const [open, setOpen] = useState(false);
  return (
    <>
      <button ref={fallback}>Conversation</button>
      <div style={{ visibility }}>
        <ContentVisibility
          visible={visibility === "visible"}
          fallbackFocus={fallback}
        >
          <button ref={opener} onClick={() => setOpen(true)}>
            Remove report
          </button>
          <ModalDialog
            open={open}
            onOpenChange={setOpen}
            title="Remove report"
            finalFocus={returnTo === "ref" ? opener : () => opener.current}
            footer={<button onClick={() => setOpen(false)}>Cancel</button>}
          />
        </ContentVisibility>
      </div>
    </>
  );
}

describe("ModalDialog over a style-hidden owner", () => {
  it.each([
    ["hidden", "ref"],
    ["hidden", "resolver"],
    ["collapse", "ref"],
    ["collapse", "resolver"],
  ] as const)(
    "rejects a visibility:%s %s target that keeps its layout boxes",
    async (visibility, returnTo) => {
      const view = render(
        <StyleHiddenScene visibility="visible" returnTo={returnTo} />,
      );
      const conversation = screen.getByRole("button", {
        name: "Conversation",
      });
      const opener = screen.getByRole("button", { name: "Remove report" });
      act(() => opener.focus());
      fireEvent.click(opener);
      await screen.findByRole("dialog");

      view.rerender(
        <StyleHiddenScene visibility={visibility} returnTo={returnTo} />,
      );

      expect(getComputedStyle(opener).visibility).toBe(visibility);
      expect(opener.getClientRects().length).toBeGreaterThan(0);
      expect(opener.getBoundingClientRect().width).toBeGreaterThan(0);
      expect(getComputedStyle(conversation).visibility).not.toMatch(
        /^(hidden|collapse)$/,
      );

      fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

      await vi.waitFor(() => expect(document.activeElement).toBe(conversation));
      expect(document.activeElement).not.toBe(opener);
    },
  );
});

function ReadingScene({
  title,
  focusSource = false,
  body,
}: {
  title: ReactNode;
  focusSource?: boolean;
  body?: RefObject<HTMLDivElement | null>;
}) {
  const source = useRef<HTMLInputElement>(null);
  const [open, setOpen] = useState(false);
  return (
    <>
      <button onClick={() => setOpen(true)}>Import servers</button>
      <ModalDialog
        open={open}
        onOpenChange={setOpen}
        title={title}
        initialFocus={focusSource ? source : body}
        bodyRef={body}
        footer={<button onClick={() => setOpen(false)}>Cancel</button>}
      >
        <input ref={source} aria-label="Source" />
        <p>Imported 2 servers.</p>
      </ModalDialog>
    </>
  );
}

async function openReading(props: ComponentProps<typeof ReadingScene>) {
  const view = render(<ReadingScene {...props} />);
  const opener = screen.getByRole("button", { name: "Import servers" });
  act(() => opener.focus());
  fireEvent.click(opener);
  const dialog = await screen.findByRole("dialog");
  await vi.waitFor(() =>
    expect(dialog.contains(document.activeElement)).toBe(true),
  );
  return { view, dialog, opener };
}

describe("ModalDialog body reading", () => {
  it("names its focusable body after the current title", async () => {
    const { view, dialog } = await openReading({
      title: (
        <>
          <em>Import</em> servers
        </>
      ),
    });
    const body = screen.getByRole("region", { name: "Import servers" });
    expect(body.dataset.slot).toBe("dialog-body");
    expect(body.tabIndex).toBe(0);
    expect(dialog.contains(body)).toBe(true);

    view.rerender(<ReadingScene title="Review servers" />);

    expect(screen.getByRole("region", { name: "Review servers" })).toBe(body);
  });

  it("keeps an explicit initial focus on its field", async () => {
    await openReading({ title: "Import servers", focusSource: true });
    await vi.waitFor(() =>
      expect(document.activeElement).toBe(
        screen.getByRole("textbox", { name: "Source" }),
      ),
    );
  });

  it("lets the body hold focus and Escape return to the opener", async () => {
    const { opener } = await openReading({ title: "Import servers" });
    const body = screen.getByRole("region", { name: "Import servers" });

    act(() => body.focus());
    expect(document.activeElement).toBe(body);

    pressEscape();

    await vi.waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    await vi.waitFor(() => expect(document.activeElement).toBe(opener));
  });

  it("projects its body ref as the initial focus and clears it on close", async () => {
    const body = createRef<HTMLDivElement>();
    const { opener } = await openReading({ title: "Import servers", body });
    const region = screen.getByRole("region", { name: "Import servers" });

    expect(body.current).toBe(region);
    await vi.waitFor(() => expect(document.activeElement).toBe(region));

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    await vi.waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(body.current).toBeNull();
    await vi.waitFor(() => expect(document.activeElement).toBe(opener));
  });
});

function ModelEditor({ result }: { result: string | null }) {
  const [title, setTitle] = useState("");
  return (
    <Panel label="Schedules">
      <p>Saved schedules</p>
      <ModalDialog
        open
        onOpenChange={() => {}}
        title="Edit schedule"
        footer={<button>Save</button>}
      >
        <input
          aria-label="Title"
          value={title}
          onChange={(event) => setTitle(event.target.value)}
        />
        <FormActions
          label="Refresh model choices"
          feedback={result && <p role="alert">{result}</p>}
        >
          <button type="button">Refresh</button>
        </FormActions>
      </ModalDialog>
    </Panel>
  );
}

describe("ModalDialog body results", () => {
  const nativeScroll = Object.getOwnPropertyDescriptor(
    HTMLElement.prototype,
    "scrollIntoView",
  );
  let revealed: Element[];

  beforeEach(() => {
    revealed = [];
    vi.stubGlobal(
      "ResizeObserver",
      class {
        observe() {}
        unobserve() {}
        disconnect() {}
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
    vi.unstubAllGlobals();
    if (nativeScroll)
      Object.defineProperty(
        HTMLElement.prototype,
        "scrollIntoView",
        nativeScroll,
      );
    else Reflect.deleteProperty(HTMLElement.prototype, "scrollIntoView");
  });

  async function openEditor() {
    const view = render(<ModelEditor result={null} />);
    const dialog = await screen.findByRole("dialog");
    await vi.waitFor(() =>
      expect(dialog.contains(document.activeElement)).toBe(true),
    );
    const title = screen.getByRole<HTMLInputElement>("textbox", {
      name: "Title",
    });
    fireEvent.change(title, { target: { value: "Nightly review" } });
    const group = screen.getByRole("group", { name: "Refresh model choices" });
    const refresh = screen.getByRole("button", { name: "Refresh" });
    const body = dialog.querySelector<HTMLElement>(
      '[data-slot="dialog-body"]',
    )!;
    rectangles.set(body, new DOMRect(0, 100, 600, 300));
    rectangles.set(group, new DOMRect(0, 300, 600, 40));
    rectangles.set(refresh, new DOMRect(480, 300, 100, 32));
    act(() => refresh.focus());
    revealed.length = 0;
    return {
      body,
      title,
      group,
      refresh,
      settle: (result: string) =>
        view.rerender(<ModelEditor result={result} />),
    };
  }

  it("reveals a delayed and a replaced result with the still-focused action", async () => {
    const { title, group, refresh, settle } = await openEditor();
    rectangles.set(group, new DOMRect(0, 300, 600, 160));
    settle("The model catalog is unavailable.");
    expect(revealed).toEqual([group]);
    expect(group.contains(screen.getByRole("alert"))).toBe(true);
    expect(document.activeElement).toBe(refresh);

    revealed.length = 0;
    rectangles.set(group, new DOMRect(0, 200, 600, 240));
    rectangles.set(refresh, new DOMRect(480, 200, 100, 32));
    settle("The provider refused the catalog read. Check its credentials.");
    expect(revealed).toEqual([group]);
    expect(document.activeElement).toBe(refresh);
    expect(title.value).toBe("Nightly review");
  });

  it("keeps the focused action visible when its result exceeds the body", async () => {
    const { group, refresh, settle } = await openEditor();
    rectangles.set(group, new DOMRect(0, 390, 600, 420));
    rectangles.set(refresh, new DOMRect(480, 390, 100, 32));
    settle("The model catalog is unavailable.");
    expect(revealed).toEqual([refresh]);
    expect(document.activeElement).toBe(refresh);
  });

  it("leaves an already visible result still", async () => {
    const { group, refresh, settle } = await openEditor();
    rectangles.set(group, new DOMRect(0, 300.25, 600, 100.2));
    settle("The model catalog is unavailable.");
    expect(group.contains(screen.getByRole("alert"))).toBe(true);
    expect(revealed).toEqual([]);
    expect(document.activeElement).toBe(refresh);
  });

  it("reserves the dialog body scroll padding when choosing a fitting result", async () => {
    const { body, group, refresh, settle } = await openEditor();
    body.style.scrollPaddingBlockStart = "12px";
    body.style.scrollPaddingBlockEnd = "12px";
    rectangles.set(group, new DOMRect(0, 370, 600, 280));
    rectangles.set(refresh, new DOMRect(480, 370, 100, 32));
    settle("The model catalog is unavailable.");
    expect(revealed).toEqual([refresh]);
    expect(document.activeElement).toBe(refresh);
  });

  it("leaves a later field focus, draft and selection alone when the result arrives", async () => {
    const { title, group, settle } = await openEditor();
    act(() => title.focus());
    title.setSelectionRange(2, 7, "backward");
    rectangles.set(group, new DOMRect(0, 300, 600, 160));
    settle("The model catalog is unavailable.");
    expect(revealed).toEqual([]);
    expect(document.activeElement).toBe(title);
    expect(title.value).toBe("Nightly review");
    expect([
      title.selectionStart,
      title.selectionEnd,
      title.selectionDirection,
    ]).toEqual([2, 7, "backward"]);
  });
});

describe("ModalDialog body focus through reflow", () => {
  const nativeScroll = Object.getOwnPropertyDescriptor(
    HTMLElement.prototype,
    "scrollIntoView",
  );
  type Observer = {
    callback: () => void;
    targets: Set<Element>;
  };
  let observers: Set<Observer>;
  let revealed: Element[];

  beforeEach(() => {
    observers = new Set();
    revealed = [];
    vi.stubGlobal(
      "ResizeObserver",
      class {
        targets = new Set<Element>();
        callback: () => void;
        constructor(callback: () => void) {
          this.callback = callback;
          observers.add(this);
        }
        observe(target: Element) {
          this.targets.add(target);
        }
        unobserve(target: Element) {
          this.targets.delete(target);
        }
        disconnect() {
          this.targets.clear();
          observers.delete(this);
        }
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
    vi.unstubAllGlobals();
    if (nativeScroll)
      Object.defineProperty(
        HTMLElement.prototype,
        "scrollIntoView",
        nativeScroll,
      );
    else Reflect.deleteProperty(HTMLElement.prototype, "scrollIntoView");
  });

  function resize(target: Element) {
    act(() => {
      for (const observer of observers)
        if (observer.targets.has(target)) observer.callback();
    });
  }

  async function focusTitle() {
    const view = render(<ModelEditor result={null} />);
    const dialog = await screen.findByRole("dialog");
    await vi.waitFor(() =>
      expect(dialog.contains(document.activeElement)).toBe(true),
    );
    const title = screen.getByRole<HTMLInputElement>("textbox", {
      name: "Title",
    });
    const body = dialog.querySelector<HTMLElement>(
      '[data-slot="dialog-body"]',
    )!;
    rectangles.set(body, new DOMRect(0, 100, 600, 300));
    rectangles.set(title, new DOMRect(0, 120, 600, 32));
    act(() => title.focus());
    fireEvent.change(title, { target: { value: "Nightly review" } });
    title.setSelectionRange(2, 7, "backward");
    resize(title);
    expect(revealed).toEqual([]);
    return { view, body, title };
  }

  function shrink(body: HTMLElement, title: HTMLElement) {
    rectangles.set(body, new DOMRect(0, 100, 600, 60));
    rectangles.set(title, new DOMRect(0, 180, 600, 32));
  }

  it("keeps a focused field, its draft and selection inside the body through resizes", async () => {
    const { body, title } = await focusTitle();

    shrink(body, title);
    resize(body);
    expect(revealed).toEqual([title]);

    rectangles.set(title, new DOMRect(0, 120, 600, 32));
    fireEvent.scroll(body);
    rectangles.set(title, new DOMRect(0, 240, 600, 32));
    fireEvent(window, new Event("resize"));
    expect(revealed).toEqual([title, title]);

    expect(document.activeElement).toBe(title);
    expect(title.value).toBe("Nightly review");
    expect([
      title.selectionStart,
      title.selectionEnd,
      title.selectionDirection,
    ]).toEqual([2, 7, "backward"]);
  });

  it("stops following once the reader scrolls the field away", async () => {
    const { body, title } = await focusTitle();
    resize(title);

    body.scrollTop = 80;
    rectangles.set(title, new DOMRect(0, 40, 600, 32));
    fireEvent.scroll(body);
    shrink(body, title);
    resize(body);

    expect(revealed).toEqual([]);
    expect(document.activeElement).toBe(title);
  });

  it("keeps following through a queued reveal scroll before layout settles", async () => {
    const { body, title } = await focusTitle();
    vi.spyOn(HTMLElement.prototype, "scrollIntoView").mockImplementation(
      function (this: HTMLElement) {
        revealed.push(this);
        body.scrollTop = 80;
      },
    );
    shrink(body, title);
    resize(body);
    fireEvent.scroll(body);
    fireEvent(window, new Event("resize"));

    expect(revealed).toEqual([title, title]);
    expect(document.activeElement).toBe(title);
  });

  it("does not mistake fractional frame reflow for a scrollbar departure", async () => {
    const { body, title } = await focusTitle();
    rectangles.set(body, new DOMRect(0, 100, 600.25, 300));
    rectangles.set(title, new DOMRect(0, 40, 600, 32));
    body.scrollTop = 80;
    fireEvent.scroll(body);

    expect(revealed).toEqual([title]);
    expect(document.activeElement).toBe(title);
  });

  it("stops following on wheel, body reading and later outside focus", async () => {
    const { body, title } = await focusTitle();

    fireEvent.wheel(body, { deltaY: -80 });
    shrink(body, title);
    resize(body);
    expect(revealed).toEqual([]);

    act(() => body.focus());
    act(() => title.focus());
    resize(title);
    expect(revealed).toEqual([title]);

    revealed.length = 0;
    act(() => body.focus());
    resize(body);
    expect(revealed).toEqual([]);

    act(() => title.focus());
    act(() => screen.getByRole("button", { name: "Save" }).focus());
    resize(body);
    expect(revealed).toEqual([]);
  });

  it("leaves a pressed field in place until the press ends", async () => {
    const { body, title } = await focusTitle();
    act(() => body.focus());

    fireEvent.pointerDown(title);
    act(() => title.focus());
    shrink(body, title);
    resize(title);
    expect(revealed).toEqual([]);

    fireEvent.pointerUp(title);
    resize(body);
    expect(revealed).toEqual([title]);
  });

  it("releases its observer and listeners when the dialog unmounts", async () => {
    const { view, body, title } = await focusTitle();
    view.unmount();

    for (const observer of observers) {
      expect(observer.targets.has(body)).toBe(false);
      expect(observer.targets.has(title)).toBe(false);
    }
    shrink(body, title);
    fireEvent(window, new Event("resize"));
    expect(revealed).toEqual([]);
  });
});
