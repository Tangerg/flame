import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { useEffect, useRef, type ReactNode } from "react";
import { act, cleanup, createEvent, fireEvent, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { isImeKey } from "@flame/ui";
import { selectAgentSession } from "@/features/agent/public/session";
import {
  captureComposerSubmission,
  replaceComposerDraft,
  useComposerText,
  useSetComposerText,
} from "@/features/composer/public/draft";
import {
  registerComposerFocusPresentation,
  registerComposerFocusTarget,
} from "@/features/composer/public/focus";
import { createComposerSendIntent } from "@/features/composer/domain/sendIntent";
import { SelectionQuote } from "@/features/composer/quote-selection/SelectionQuote";

const message = "Complete selected evidence — 保留全部内容.\nA second line with its final words.";
const quote = "> Complete selected evidence — 保留全部内容.\n> A second line with its final words.";
const frames = new Map<number, FrameRequestCallback>();
let nextFrame = 0;

function ComposerField() {
  const value = useComposerText();
  const setValue = useSetComposerText();
  const field = useRef<HTMLTextAreaElement>(null);
  useEffect(() => registerComposerFocusTarget(testClient.composer.access, field.current), []);
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
        <div role="region" aria-label="Message reader" tabIndex={-1}>
          <div data-quote-source="message">
            <p>{message}</p>
          </div>
        </div>
        <div data-quote-source="message">
          <p>Another message</p>
        </div>
        <div role="region" aria-label="File reader" tabIndex={-1}>
          <div data-quote-source="file" data-quote-path="src/complete source.ts">
            <span data-quote-line="4">{"const a = 1;\n"}</span>
            <span data-quote-line="5">return a;</span>
          </div>
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

function selectRange(range: Range) {
  act(() => {
    const selection = window.getSelection()!;
    selection.removeAllRanges();
    selection.addRange(range);
    document.dispatchEvent(new Event("selectionchange"));
  });
}

async function enterQuote() {
  const reader = screen.getByRole("region", { name: "Message reader" });
  act(() => reader.focus());
  const button = await showQuote();
  const event = createEvent.keyDown(reader, { key: "Tab" });
  fireEvent(reader, event);
  expect(event.defaultPrevented).toBe(true);
  expect(document.activeElement).toBe(button);
  return { reader, button };
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
  vi.spyOn(HTMLElement.prototype, "getClientRects").mockImplementation(function (
    this: HTMLElement,
  ) {
    return this.closest("[hidden], [inert]")
      ? ([] as unknown as DOMRectList)
      : ([new DOMRect(900, 400, 480, 140)] as unknown as DOMRectList);
  });
  selectAgentSession(testClient.agent.access, "selection-quote");
  replaceComposerDraft(testClient.composer.access, { text: "Existing draft" });
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
      testClient.render(<Harness />);
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
      ).toBe(`Existing draft\n\n${quote}\n\n`);
      expect(selection.isCollapsed).toBe(true);
    },
  );

  it("closes on a normal source click that collapses the selection", async () => {
    testClient.render(<Harness />);
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
    testClient.render(<Harness />);
    await showQuote();

    pressOutside(screen.getByText("Another message"));
    flushFrame();

    expect(window.getSelection()?.toString()).toBe(message);
    expect(screen.queryByRole("button", { name: "Quote in message" })).toBeNull();
  });

  it("preserves the native selection until pointer activation and appends the complete quote once", async () => {
    testClient.render(<Harness />);
    const origin = screen.getByRole("button", { name: "Back to conversation" });
    act(() => origin.focus());
    const present = vi.fn((focus: () => void) => focus());
    const unregister = registerComposerFocusPresentation(testClient.composer.access, present);
    const button = await showQuote();
    const field = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Composer draft" });
    expect(document.activeElement).toBe(origin);
    expect(fireEvent.mouseDown(button)).toBe(false);
    expect(window.getSelection()?.toString()).toBe(message);
    fireEvent(document, new Event("selectionchange"));

    fireEvent.click(button);
    flushFrame();

    expect(field.value).toBe(`Existing draft\n\n${quote}\n\n`);
    expect(present).toHaveBeenCalledTimes(1);
    expect(document.activeElement).toBe(field);
    expect(field.selectionStart).toBe(field.value.length);
    expect(field.selectionEnd).toBe(field.value.length);
    expect(window.getSelection()?.isCollapsed).toBe(true);
    expect(screen.queryByRole("button", { name: "Quote in message" })).toBeNull();
    expect(
      createComposerSendIntent(captureComposerSubmission(testClient.composer.access)),
    ).toMatchObject({
      body: `Existing draft\n\n${quote}`,
      historyText: `Existing draft\n\n${quote}`,
    });
    fireEvent.change(field, { target: { value: `${field.value} Continued.` } });
    expect(field.value).toBe(`Existing draft\n\n${quote}\n\n Continued.`);
    expect(screen.getByRole("textbox", { name: "Composer draft" })).toBe(field);
    expect(document.activeElement).toBe(field);
    expect(field.selectionEnd).toBe(field.value.length);
    expect(
      createComposerSendIntent(captureComposerSubmission(testClient.composer.access)),
    ).toMatchObject({
      body: `Existing draft\n\n${quote}\n\n Continued.`,
      historyText: `Existing draft\n\n${quote}\n\n Continued.`,
    });
    unregister();
  });

  it("keeps consecutive quotes and continuation in the same draft with one paragraph boundary", async () => {
    testClient.render(<Harness />);
    const field = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Composer draft" });
    const first = await showQuote();
    await act(async () => {
      fireEvent.click(first);
    });
    const range = document.createRange();
    range.selectNodeContents(
      screen.getByRole("region", { name: "Message reader" }).querySelector("p")!,
    );
    selectRange(range);
    flushFrame();
    const second = await screen.findByRole("button", { name: "Quote in message" });
    await act(async () => {
      fireEvent.click(second);
    });
    flushFrame();

    expect(field.value).toBe(`Existing draft\n\n${quote}\n\n${quote}\n\n`);
    expect(screen.getByRole("textbox", { name: "Composer draft" })).toBe(field);
    expect(document.activeElement).toBe(field);
    expect(field.selectionStart).toBe(field.value.length);
    expect(field.selectionEnd).toBe(field.value.length);
    expect(
      createComposerSendIntent(captureComposerSubmission(testClient.composer.access)),
    ).toMatchObject({
      body: `Existing draft\n\n${quote}\n\n${quote}`,
      historyText: `Existing draft\n\n${quote}\n\n${quote}`,
    });

    fireEvent.change(field, { target: { value: `${field.value}Continue after both.` } });

    expect(field.value).toBe(`Existing draft\n\n${quote}\n\n${quote}\n\nContinue after both.`);
    expect(
      createComposerSendIntent(captureComposerSubmission(testClient.composer.access)),
    ).toMatchObject({
      body: `Existing draft\n\n${quote}\n\n${quote}\n\nContinue after both.`,
      historyText: `Existing draft\n\n${quote}\n\n${quote}\n\nContinue after both.`,
    });
  });

  it.each(["Enter", " "])(
    "accepts the keyboard-generated click for %j without a dialog focus boundary",
    async (key) => {
      testClient.render(<Harness />);
      const button = await showQuote();
      const present = vi.fn((focus: () => void) => focus());
      const unregister = registerComposerFocusPresentation(testClient.composer.access, present);
      act(() => button.focus());
      expect(button.closest('[role="dialog"], [role="alertdialog"]')).toBeNull();
      fireEvent.keyDown(button, { key });
      fireEvent.keyUp(button, { key });

      fireEvent.click(button, { detail: 0 });
      flushFrame();

      const field = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Composer draft" });
      expect(field.value).toBe(`Existing draft\n\n${quote}\n\n`);
      expect(present).toHaveBeenCalledTimes(1);
      expect(document.activeElement).toBe(field);
      expect(field.selectionEnd).toBe(field.value.length);
      expect(window.getSelection()?.isCollapsed).toBe(true);
      expect(screen.queryByRole("button", { name: "Quote in message" })).toBeNull();
      unregister();
    },
  );

  it("keeps a file selection's path, line range, and complete source", async () => {
    testClient.render(<Harness />);
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
    const field = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Composer draft" });

    fireEvent.click(button);

    const quoted =
      "Existing draft\n\n`src/complete source.ts:4-5`\n```\nconst a = 1;\nreturn a;\n```";
    expect(field.value).toBe(`${quoted}\n\n`);
    expect(document.activeElement).toBe(field);
    expect(field.selectionStart).toBe(field.value.length);
    expect(field.selectionEnd).toBe(field.value.length);
    expect(
      createComposerSendIntent(captureComposerSubmission(testClient.composer.access)),
    ).toMatchObject({
      body: quoted,
      historyText: quoted,
    });

    fireEvent.change(field, { target: { value: `${field.value} Continue file.` } });

    expect(field.value).toBe(`${quoted}\n\n Continue file.`);
    expect(screen.getByRole("textbox", { name: "Composer draft" })).toBe(field);
    expect(document.activeElement).toBe(field);
    expect(field.selectionEnd).toBe(field.value.length);
    expect(
      createComposerSendIntent(captureComposerSubmission(testClient.composer.access)),
    ).toMatchObject({
      body: `${quoted}\n\n Continue file.`,
      historyText: `${quoted}\n\n Continue file.`,
    });
  });

  it("quotes Markdown file fences literally and continues in the same Composer", async () => {
    testClient.render(
      <Harness>
        <div role="region" aria-label="README source" tabIndex={-1}>
          <div data-quote-source="file" data-quote-path="README.md">
            <span data-quote-line="17">{"```ts\n"}</span>
            <span data-quote-line="18">{'const outcome = "recorded";\n'}</span>
            <span data-quote-line="19">{"```"}</span>
          </div>
        </div>
      </Harness>,
    );
    const field = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Composer draft" });
    const reader = screen.getByRole("region", { name: "README source" });
    act(() => reader.focus());
    const first = reader.querySelector('[data-quote-line="17"]')!.firstChild!;
    const last = reader.querySelector('[data-quote-line="19"]')!.firstChild!;
    const range = document.createRange();
    range.setStart(first, 0);
    range.setEnd(last, last.textContent!.length);
    selectRange(range);
    flushFrame();
    expect(window.getSelection()?.toString()).toBe('```ts\nconst outcome = "recorded";\n```');
    const button = await screen.findByRole("button", { name: "Quote in message" });
    fireEvent.keyDown(reader, { key: "Tab" });
    expect(document.activeElement).toBe(button);

    await act(async () => {
      fireEvent.click(button, { detail: 0 });
    });
    flushFrame();

    const quoted =
      'Existing draft\n\n`README.md:17-19`\n````\n```ts\nconst outcome = "recorded";\n```\n````';
    expect(field.value).toBe(`${quoted}\n\n`);
    expect(screen.getByRole("textbox", { name: "Composer draft" })).toBe(field);
    expect(document.activeElement).toBe(field);
    expect(field.selectionStart).toBe(field.value.length);
    expect(field.selectionEnd).toBe(field.value.length);
    expect(
      createComposerSendIntent(captureComposerSubmission(testClient.composer.access)),
    ).toMatchObject({
      body: quoted,
      historyText: quoted,
    });

    fireEvent.change(field, { target: { value: `${field.value}Explain this setup.` } });

    expect(field.value).toBe(`${quoted}\n\nExplain this setup.`);
    expect(document.activeElement).toBe(field);
    expect(field.selectionEnd).toBe(field.value.length);
    expect(
      createComposerSendIntent(captureComposerSubmission(testClient.composer.access)),
    ).toMatchObject({
      body: `${quoted}\n\nExplain this setup.`,
      historyText: `${quoted}\n\nExplain this setup.`,
    });
  });

  it("does not quote a selection spanning separate sources", () => {
    testClient.render(<Harness />);
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
    testClient.render(<Harness />);
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
    testClient.render(<Harness onBack={back} />);
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
    testClient.render(<Harness onBack={back} />);
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
    testClient.render(<Harness />);
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
    testClient.render(<Harness onBack={back} />);
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
    testClient.render(<Harness />);
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
    const view = testClient.render(<Harness />);
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
    testClient.render(<Harness />);
    await showQuote();
    fireEvent(document, new Event("selectionchange"));

    pressOutside(screen.getByRole("button", { name: "Other task" }));
    flushFrame();

    expect(screen.queryByRole("button", { name: "Quote in message" })).toBeNull();
    expect(screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Composer draft" }).value).toBe(
      "Existing draft",
    );
  });

  it("uses the current full selection when Tab precedes its queued refresh", async () => {
    testClient.render(<Harness />);
    const reader = screen.getByRole("region", { name: "Message reader" });
    act(() => reader.focus());
    const text = screen.getByText("Complete selected evidence", { exact: false }).firstChild!;
    const word = document.createRange();
    word.setStart(text, 0);
    word.setEnd(text, "Complete".length);
    selectRange(word);
    flushFrame();
    const button = await screen.findByRole("button", { name: "Quote in message" });
    selectMessage();

    const event = createEvent.keyDown(reader, { key: "Tab" });
    fireEvent(reader, event);
    flushFrame();

    expect(event.defaultPrevented).toBe(true);
    expect(document.activeElement).toBe(button);
    fireEvent.click(button, { detail: 0 });
    expect(screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Composer draft" }).value).toBe(
      `Existing draft\n\n${quote}\n\n`,
    );
  });

  it("keeps rendered selection separators distinct from DOM range material", async () => {
    testClient.render(<Harness />);
    const rendered = message.replace("\n", "\n\n");
    vi.spyOn(window.getSelection()!, "toString").mockReturnValue(rendered);
    const { button } = await enterQuote();
    expect(window.getSelection()!.getRangeAt(0).toString()).toBe(message);
    expect(window.getSelection()!.toString()).toBe(rendered);

    fireEvent.click(button, { detail: 0 });

    expect(screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Composer draft" }).value).toBe(
      "Existing draft\n\n> Complete selected evidence — 保留全部内容.\n>\n> A second line with its final words.\n\n",
    );
  });

  it.each([
    ["Enter", false],
    ["Enter", true],
    [" ", false],
    [" ", true],
  ])("activates the Tab-entered Quote with %j after collapsed=%j", async (key, collapsed) => {
    testClient.render(<Harness />);
    const present = vi.fn((focus: () => void) => focus());
    const unregister = registerComposerFocusPresentation(testClient.composer.access, present);
    const { reader, button } = await enterQuote();
    const readerFocus = vi.spyOn(reader, "focus");
    act(() => {
      if (collapsed) window.getSelection()!.removeAllRanges();
      document.dispatchEvent(new Event("selectionchange"));
    });
    flushFrame();

    expect(screen.getByRole("button", { name: "Quote in message" })).toBe(button);
    expect(document.activeElement).toBe(button);
    fireEvent.keyDown(button, { key });
    fireEvent.keyUp(button, { key });
    fireEvent.click(button, { detail: 0 });
    flushFrame();

    const field = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Composer draft" });
    expect(field.value).toBe(`Existing draft\n\n${quote}\n\n`);
    expect(present).toHaveBeenCalledOnce();
    expect(readerFocus).not.toHaveBeenCalled();
    expect(document.activeElement).toBe(field);
    expect(field.selectionEnd).toBe(field.value.length);
    expect(screen.queryByRole("button", { name: "Quote in message" })).toBeNull();
    unregister();
  });

  it.each([
    { key: "Tab", shiftKey: false },
    { key: "Tab", shiftKey: true },
    { key: "Escape", shiftKey: false },
  ])("returns to the reader on $key shift=$shiftKey and releases the next Tab", async (keys) => {
    testClient.render(<Harness />);
    const { reader, button } = await enterQuote();
    const readerFocus = vi.spyOn(reader, "focus");
    fireEvent(document, new Event("selectionchange"));

    const event = createEvent.keyDown(button, keys);
    fireEvent(button, event);
    fireEvent(document, new Event("selectionchange"));
    flushFrame();

    expect(event.defaultPrevented).toBe(true);
    expect(document.activeElement).toBe(reader);
    expect(readerFocus).toHaveBeenCalledWith({ preventScroll: true });
    expect(window.getSelection()?.isCollapsed).toBe(true);
    expect(screen.queryByRole("button", { name: "Quote in message" })).toBeNull();
    expect(screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Composer draft" }).value).toBe(
      "Existing draft",
    );
    const next = createEvent.keyDown(reader, { key: "Tab" });
    fireEvent(reader, next);
    expect(next.defaultPrevented).toBe(false);
    expect(screen.queryByRole("button", { name: "Quote in message" })).toBeNull();
  });

  it("leaves modified, composing, handled, and later controls' Tab to their owners", async () => {
    testClient.render(<Harness />);
    const reader = screen.getByRole("region", { name: "Message reader" });
    act(() => reader.focus());
    await showQuote();
    for (const extra of [
      { shiftKey: true },
      { ctrlKey: true },
      { altKey: true },
      { metaKey: true },
      { isComposing: true },
      { keyCode: 229 },
    ]) {
      const event = createEvent.keyDown(reader, { key: "Tab", ...extra });
      fireEvent(reader, event);
      expect(event.defaultPrevented).toBe(false);
      expect(document.activeElement).toBe(reader);
    }
    const handled = createEvent.keyDown(reader, { key: "Tab" });
    handled.preventDefault();
    fireEvent(reader, handled);
    expect(document.activeElement).toBe(reader);

    selectMessage();
    const other = screen.getByRole("button", { name: "Other task" });
    act(() => other.focus());
    flushFrame();
    const later = createEvent.keyDown(other, { key: "Tab" });
    fireEvent(other, later);
    expect(later.defaultPrevented).toBe(false);
    expect(document.activeElement).toBe(other);
    fireEvent(document, new Event("selectionchange"));
    flushFrame();
    const refreshed = createEvent.keyDown(other, { key: "Tab" });
    fireEvent(other, refreshed);
    expect(refreshed.defaultPrevented).toBe(false);
    expect(document.activeElement).toBe(other);
  });

  it("retires the keyboard task when a later modal takes focus without reclaiming it", async () => {
    const view = testClient.render(<Harness />);
    const { reader } = await enterQuote();
    const readerFocus = vi.spyOn(reader, "focus");
    fireEvent(document, new Event("selectionchange"));
    view.rerender(
      <Harness>
        <div role="dialog" aria-label="Later task">
          <input aria-label="Later task input" />
        </div>
      </Harness>,
    );
    const field = screen.getByRole("textbox", { name: "Later task input" });
    act(() => field.focus());
    flushFrame();

    expect(screen.queryByRole("button", { name: "Quote in message" })).toBeNull();
    expect(document.activeElement).toBe(field);
    expect(readerFocus).not.toHaveBeenCalled();
    const event = createEvent.keyDown(field, { key: "Tab" });
    fireEvent(field, event);
    expect(event.defaultPrevented).toBe(false);
  });

  it.each(["Tab", "click", "refresh"])(
    "does not clear or quote a new equal-text range on %s",
    async (action) => {
      testClient.render(<Harness />);
      const reader = screen.getByRole("region", { name: "Message reader" });
      act(() => reader.focus());
      const text = screen.getByText("Complete selected evidence", { exact: false }).firstChild!;
      const firstOffset = message.indexOf("e");
      const secondOffset = message.indexOf("e", firstOffset + 1);
      const first = document.createRange();
      first.setStart(text, firstOffset);
      first.setEnd(text, firstOffset + 1);
      selectRange(first);
      flushFrame();
      const button = await screen.findByRole("button", { name: "Quote in message" });
      fireEvent.keyDown(reader, { key: "Tab" });
      expect(document.activeElement).toBe(button);
      const replacement = document.createRange();
      replacement.setStart(text, secondOffset);
      replacement.setEnd(text, secondOffset + 1);
      selectRange(replacement);

      if (action === "click") fireEvent.click(button, { detail: 0 });
      else if (action === "Tab") fireEvent.keyDown(button, { key: "Tab" });
      flushFrame();

      const remaining = window.getSelection()!.getRangeAt(0);
      expect(remaining.startContainer).toBe(text);
      expect(remaining.startOffset).toBe(secondOffset);
      expect(remaining.endOffset).toBe(secondOffset + 1);
      expect(window.getSelection()?.toString()).toBe("e");
      expect(screen.queryByRole("button", { name: "Quote in message" })).toBeNull();
      expect(
        screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Composer draft" }).value,
      ).toBe("Existing draft");
    },
  );

  it.each(["hidden", "replaced", "changed"])(
    "retires the keyboard snapshot on selectionchange after its source is %s",
    async (change) => {
      testClient.render(<Harness />);
      const { reader } = await enterQuote();
      const host = screen.getByText("Complete selected evidence", { exact: false }).parentElement!;
      const readerFocus = vi.spyOn(reader, "focus");

      await act(async () => {
        if (change === "hidden") reader.hidden = true;
        else if (change === "replaced") host.replaceWith(host.cloneNode(true));
        else host.firstChild!.textContent = "Replaced source content";
        document.dispatchEvent(new Event("selectionchange"));
      });
      flushFrame();

      expect(screen.queryByRole("button", { name: "Quote in message" })).toBeNull();
      expect(readerFocus).not.toHaveBeenCalled();
      expect(
        screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Composer draft" }).value,
      ).toBe("Existing draft");
    },
  );

  it("preserves file boundaries through keyboard entry and rejects a changed file identity", async () => {
    testClient.render(<Harness />);
    const reader = screen.getByRole("region", { name: "File reader" });
    act(() => reader.focus());
    const first = screen.getByText("const a = 1;").firstChild!;
    const last = screen.getByText("return a;").firstChild!;
    const range = document.createRange();
    range.setStart(first, 0);
    range.setEnd(last, last.textContent!.length);
    selectRange(range);
    flushFrame();
    const button = await screen.findByRole("button", { name: "Quote in message" });
    fireEvent.keyDown(reader, { key: "Tab" });
    expect(document.activeElement).toBe(button);

    await act(async () => {
      fireEvent.click(button, { detail: 0 });
    });

    const field = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Composer draft" });
    const expected =
      "Existing draft\n\n`src/complete source.ts:4-5`\n```\nconst a = 1;\nreturn a;\n```\n\n";
    expect(field.value).toBe(expected);
    act(() => reader.focus());
    selectRange(range);
    flushFrame();
    const second = await screen.findByRole("button", { name: "Quote in message" });
    expect(window.getSelection()?.toString()).toBe("const a = 1;\nreturn a;");
    expect(document.activeElement).toBe(reader);
    expect(second.isConnected).toBe(true);
    const entering = createEvent.keyDown(reader, { key: "Tab" });
    fireEvent(reader, entering);
    expect(entering.defaultPrevented).toBe(true);
    expect(document.activeElement).toBe(second);
    first.parentElement!.parentElement!.dataset.quotePath = "src/another source.ts";

    fireEvent.click(second, { detail: 0 });
    flushFrame();

    expect(field.value).toBe(expected);
    expect(screen.queryByRole("button", { name: "Quote in message" })).toBeNull();
    expect(window.getSelection()?.toString()).toBe("const a = 1;\nreturn a;");
  });

  it.each(["scroll", "resize"])("retires an entered keyboard task on %s", async (event) => {
    testClient.render(<Harness />);
    await enterQuote();
    fireEvent(document, new Event("selectionchange"));

    fireEvent(window, new Event(event));
    flushFrame();

    expect(screen.queryByRole("button", { name: "Quote in message" })).toBeNull();
    expect(screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Composer draft" }).value).toBe(
      "Existing draft",
    );
  });
});
