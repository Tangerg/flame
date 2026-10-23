import { useEffect, useRef, type ReactNode } from "react";
import { act, cleanup, createEvent, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { isImeKey } from "@flame/ui";
import { selectAgentSession } from "@/features/agent/public/session";
import {
  replaceComposerDraft,
  useComposerText,
  useSetComposerText,
} from "@/features/composer/public/draft";
import {
  registerComposerFocusPresentation,
  registerComposerFocusTarget,
} from "@/features/composer/public/focus";
import { SelectionQuote } from "@/features/composer/quote-selection/ui/SelectionQuote";

const message = "Complete selected evidence — 保留全部内容.\nA second line with its final words.";
const quote = "> Complete selected evidence — 保留全部内容.\n> A second line with its final words.";
const frames = new Map<number, FrameRequestCallback>();
let nextFrame = 0;

function ComposerField() {
  const value = useComposerText();
  const setValue = useSetComposerText();
  const field = useRef<HTMLTextAreaElement>(null);
  useEffect(() => registerComposerFocusTarget(field.current), []);
  return (
    <textarea
      ref={field}
      aria-label="Composer draft"
      value={value}
      onChange={(event) => setValue(event.target.value)}
    />
  );
}

function Harness({ onBack = () => {}, children }: { onBack?: () => void; children?: ReactNode }) {
  return (
    <>
      <section aria-label="Inspection">
        <button
          onKeyDown={(event) => {
            if (event.key !== "Escape" || event.defaultPrevented || isImeKey(event.nativeEvent))
              return;
            event.preventDefault();
            event.stopPropagation();
            onBack();
          }}
        >
          Back to conversation
        </button>
        <div data-quote-source="message">
          <p>{message}</p>
        </div>
        <div data-quote-source="message">
          <p>Another message</p>
        </div>
        <div data-quote-source="file" data-quote-path="src/complete source.ts">
          <span data-quote-line="4">{"const a = 1;\n"}</span>
          <span data-quote-line="5">return a;</span>
        </div>
      </section>
      <ComposerField />
      <button>Other task</button>
      {children}
      <SelectionQuote />
    </>
  );
}

function flushFrame() {
  act(() => {
    const queued = [...frames.values()];
    frames.clear();
    for (const callback of queued) callback(0);
  });
}

function selectMessage() {
  const range = document.createRange();
  range.selectNodeContents(screen.getByText("Complete selected evidence", { exact: false }));
  act(() => {
    const selection = window.getSelection()!;
    selection.removeAllRanges();
    selection.addRange(range);
    document.dispatchEvent(new Event("selectionchange"));
  });
}

async function showQuote() {
  selectMessage();
  flushFrame();
  return screen.findByRole("button", { name: "Quote in message" });
}

function beginMousePress(element: HTMLElement, detail = 1) {
  fireEvent.pointerDown(element, { pointerType: "mouse", button: 0, detail });
  fireEvent.mouseDown(element, { button: 0, detail });
}

function endMousePress(element: HTMLElement, detail = 1) {
  fireEvent.pointerUp(element, { pointerType: "mouse", button: 0, detail });
  fireEvent.mouseUp(element, { button: 0, detail });
  fireEvent.click(element, { detail });
}

function pressOutside(element: HTMLElement) {
  beginMousePress(element);
  endMousePress(element);
}

beforeEach(() => {
  nextFrame = 0;
  frames.clear();
  vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => {
    const frame = ++nextFrame;
    frames.set(frame, callback);
    return frame;
  });
  vi.stubGlobal("cancelAnimationFrame", (frame: number) => frames.delete(frame));
  vi.spyOn(Range.prototype, "getBoundingClientRect").mockReturnValue(
    new DOMRect(933, 442, 451, 70),
  );
  selectAgentSession("selection-quote");
  replaceComposerDraft({ text: "Existing draft" });
});

afterEach(() => {
  cleanup();
  window.getSelection()?.removeAllRanges();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe("SelectionQuote", () => {
  it.each(["before", "after"])(
    "retains a paragraph selected by multiple clicks when its refresh runs %s the final click",
    async (refresh) => {
      render(<Harness />);
      const source = screen.getByText("Complete selected evidence", { exact: false });
      const text = source.firstChild!;
      const selection = window.getSelection()!;
      beginMousePress(source);
      act(() => {
        selection.collapse(text, 0);
        document.dispatchEvent(new Event("selectionchange"));
      });
      flushFrame();
      endMousePress(source);
      expect(screen.queryByRole("button", { name: "Quote in message" })).toBeNull();

      beginMousePress(source, 2);
      const word = document.createRange();
      word.setStart(text, 0);
      word.setEnd(text, "Complete".length);
      act(() => {
        selection.removeAllRanges();
        selection.addRange(word);
        document.dispatchEvent(new Event("selectionchange"));
      });
      flushFrame();
      await screen.findByRole("button", { name: "Quote in message" });
      endMousePress(source, 2);
      fireEvent.doubleClick(source, { detail: 2 });
      expect(selection.toString()).toBe("Complete");
      expect(screen.getByRole("button", { name: "Quote in message" })).toBeTruthy();

      beginMousePress(source, 3);
      selectMessage();
      if (refresh === "before") flushFrame();
      endMousePress(source, 3);
      if (refresh === "after") flushFrame();

      expect(selection.toString()).toBe(message);
      const button = screen.getByRole("button", { name: "Quote in message" });
      fireEvent.mouseDown(button);
      fireEvent.click(button);
      expect(
        screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Composer draft" }).value,
      ).toBe(`Existing draft\n\n${quote}`);
      expect(selection.isCollapsed).toBe(true);
    },
  );

  it("closes on a normal source click that collapses the selection", async () => {
    render(<Harness />);
    await showQuote();
    const source = screen.getByText("Complete selected evidence", { exact: false });
    beginMousePress(source);
    act(() => {
      window.getSelection()!.collapse(source.firstChild!, 3);
      document.dispatchEvent(new Event("selectionchange"));
    });

    endMousePress(source);
    flushFrame();

    expect(window.getSelection()?.isCollapsed).toBe(true);
    expect(screen.queryByRole("button", { name: "Quote in message" })).toBeNull();
  });

  it("closes when another source is pressed while the original text remains selected", async () => {
    render(<Harness />);
    await showQuote();

    pressOutside(screen.getByText("Another message"));
    flushFrame();

    expect(window.getSelection()?.toString()).toBe(message);
    expect(screen.queryByRole("button", { name: "Quote in message" })).toBeNull();
  });

  it("preserves the native selection until pointer activation and appends the complete quote once", async () => {
    render(<Harness />);
    const origin = screen.getByRole("button", { name: "Back to conversation" });
    act(() => origin.focus());
    const present = vi.fn((focus: () => void) => focus());
    const unregister = registerComposerFocusPresentation(present);
    const button = await showQuote();
    const field = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Composer draft" });
    expect(document.activeElement).toBe(origin);
    expect(fireEvent.mouseDown(button)).toBe(false);
    expect(window.getSelection()?.toString()).toBe(message);
    fireEvent(document, new Event("selectionchange"));

    fireEvent.click(button);
    flushFrame();

    expect(field.value).toBe(`Existing draft\n\n${quote}`);
    expect(present).toHaveBeenCalledTimes(1);
    expect(document.activeElement).toBe(field);
    expect(field.selectionStart).toBe(field.value.length);
    expect(field.selectionEnd).toBe(field.value.length);
    expect(window.getSelection()?.isCollapsed).toBe(true);
    expect(screen.queryByRole("button", { name: "Quote in message" })).toBeNull();
    fireEvent.change(field, { target: { value: `${field.value} Continued.` } });
    expect(field.value).toBe(`Existing draft\n\n${quote} Continued.`);
    unregister();
  });

  it.each(["Enter", " "])(
    "accepts the keyboard-generated click for %j without a dialog focus boundary",
    async (key) => {
      render(<Harness />);
      const button = await showQuote();
      const present = vi.fn((focus: () => void) => focus());
      const unregister = registerComposerFocusPresentation(present);
      act(() => button.focus());
      expect(button.closest('[role="dialog"], [role="alertdialog"]')).toBeNull();
      fireEvent.keyDown(button, { key });
      fireEvent.keyUp(button, { key });

      fireEvent.click(button, { detail: 0 });
      flushFrame();

      const field = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Composer draft" });
      expect(field.value).toBe(`Existing draft\n\n${quote}`);
      expect(present).toHaveBeenCalledTimes(1);
      expect(document.activeElement).toBe(field);
      expect(field.selectionEnd).toBe(field.value.length);
      expect(window.getSelection()?.isCollapsed).toBe(true);
      expect(screen.queryByRole("button", { name: "Quote in message" })).toBeNull();
      unregister();
    },
  );

  it("keeps a file selection's path, line range, and complete source", async () => {
    render(<Harness />);
    const first = screen.getByText("const a = 1;").firstChild!;
    const last = screen.getByText("return a;").firstChild!;
    const range = document.createRange();
    range.setStart(first, 0);
    range.setEnd(last, last.textContent!.length);
    act(() => {
      window.getSelection()!.addRange(range);
      document.dispatchEvent(new Event("selectionchange"));
    });
    flushFrame();
    const button = await screen.findByRole("button", { name: "Quote in message" });

    fireEvent.click(button);

    expect(screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Composer draft" }).value).toBe(
      "Existing draft\n\n`src/complete source.ts:4-5`\n```\nconst a = 1;\nreturn a;\n```",
    );
  });

  it("does not quote a selection spanning separate sources", () => {
    render(<Harness />);
    const range = document.createRange();
    range.setStart(screen.getByText("Complete selected evidence", { exact: false }).firstChild!, 0);
    range.setEnd(screen.getByText("Another message").firstChild!, 5);
    act(() => {
      window.getSelection()!.addRange(range);
      document.dispatchEvent(new Event("selectionchange"));
    });
    flushFrame();

    expect(screen.queryByRole("button", { name: "Quote in message" })).toBeNull();
  });

  it.each(["scroll", "resize"])("retires a selection refresh queued before %s", async (event) => {
    render(<Harness />);
    selectMessage();

    fireEvent(window, new Event(event));
    flushFrame();

    expect(screen.queryByRole("button", { name: "Quote in message" })).toBeNull();
    fireEvent(document, new Event("selectionchange"));
    flushFrame();
    expect(await screen.findByRole("button", { name: "Quote in message" })).toBeTruthy();
  });

  it("consumes the selection's first Escape before workspace return and retires queued refresh", async () => {
    const back = vi.fn();
    render(<Harness onBack={back} />);
    const origin = screen.getByRole("button", { name: "Back to conversation" });
    act(() => origin.focus());
    await showQuote();
    fireEvent(document, new Event("selectionchange"));

    fireEvent.keyDown(origin, { key: "Escape" });
    flushFrame();

    expect(screen.queryByRole("button", { name: "Quote in message" })).toBeNull();
    expect(back).not.toHaveBeenCalled();
    expect(document.activeElement).toBe(origin);
    expect(window.getSelection()?.toString()).toBe(message);
    fireEvent.keyDown(origin, { key: "Escape" });
    expect(back).toHaveBeenCalledTimes(1);
  });

  it("dismisses on the Quote button's Escape without returning focus elsewhere", async () => {
    const back = vi.fn();
    render(<Harness onBack={back} />);
    const button = await showQuote();
    act(() => button.focus());

    fireEvent.keyDown(button, { key: "Escape" });
    flushFrame();

    expect(screen.queryByRole("button", { name: "Quote in message" })).toBeNull();
    expect(back).not.toHaveBeenCalled();
    expect(document.activeElement).not.toBe(
      screen.getByRole("textbox", { name: "Composer draft" }),
    );
    expect(document.activeElement).not.toBe(
      screen.getByRole("button", { name: "Back to conversation" }),
    );
  });

  it("consumes Escape for a native selection that leaves focus on the body", async () => {
    render(<Harness />);
    expect(document.activeElement).toBe(document.body);
    await showQuote();
    const escaped = vi.fn();
    window.addEventListener("keydown", escaped);

    fireEvent.keyDown(document.body, { key: "Escape" });
    flushFrame();

    expect(screen.queryByRole("button", { name: "Quote in message" })).toBeNull();
    expect(escaped).not.toHaveBeenCalled();
    expect(document.activeElement).toBe(document.body);
    window.removeEventListener("keydown", escaped);
  });

  it("leaves composing and already handled Escape to their owners", async () => {
    const back = vi.fn();
    render(<Harness onBack={back} />);
    const origin = screen.getByRole("button", { name: "Back to conversation" });
    act(() => origin.focus());
    const button = await showQuote();

    fireEvent.keyDown(origin, { key: "Escape", isComposing: true });
    fireEvent.keyDown(origin, { key: "Escape", keyCode: 229 });
    const handled = createEvent.keyDown(origin, { key: "Escape" });
    handled.preventDefault();
    fireEvent(origin, handled);
    flushFrame();

    expect(screen.getByRole("button", { name: "Quote in message" })).toBe(button);
    expect(back).not.toHaveBeenCalled();
  });

  it("leaves Escape to a task focused after the selection request", async () => {
    render(<Harness />);
    const origin = screen.getByRole("button", { name: "Back to conversation" });
    const other = screen.getByRole("button", { name: "Other task" });
    act(() => origin.focus());
    selectMessage();
    act(() => other.focus());
    flushFrame();
    await screen.findByRole("button", { name: "Quote in message" });
    const escaped = vi.fn();
    window.addEventListener("keydown", escaped);

    const event = createEvent.keyDown(other, { key: "Escape" });
    fireEvent(other, event);

    expect(escaped).toHaveBeenCalledOnce();
    expect(event.defaultPrevented).toBe(false);
    expect(document.activeElement).toBe(other);
    window.removeEventListener("keydown", escaped);
  });

  it("leaves Escape to a subsequently focused modal", async () => {
    const escaped = vi.fn();
    const view = render(<Harness />);
    await showQuote();
    view.rerender(
      <Harness>
        <div role="dialog" aria-label="Other task">
          <input aria-label="Modal task" onKeyDown={escaped} />
        </div>
      </Harness>,
    );
    const field = screen.getByRole("textbox", { name: "Modal task" });
    act(() => field.focus());

    const event = createEvent.keyDown(field, { key: "Escape" });
    fireEvent(field, event);

    expect(escaped).toHaveBeenCalledOnce();
    expect(event.defaultPrevented).toBe(false);
    expect(document.activeElement).toBe(field);
  });

  it("lets outside press dismiss the popup and cancels an older queued selection refresh", async () => {
    render(<Harness />);
    await showQuote();
    fireEvent(document, new Event("selectionchange"));

    pressOutside(screen.getByRole("button", { name: "Other task" }));
    flushFrame();

    expect(screen.queryByRole("button", { name: "Quote in message" })).toBeNull();
    expect(screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Composer draft" }).value).toBe(
      "Existing draft",
    );
  });
});
