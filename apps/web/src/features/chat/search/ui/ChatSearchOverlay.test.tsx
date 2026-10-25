import { act, fireEvent, render, screen } from "@testing-library/react";
import { type ReactNode } from "react";
import { ContentVisibility } from "@flame/ui";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { selectAgentSession } from "@/features/agent/public/session";
import { MESSAGE_CONTENT_SELECTOR } from "@/features/chat/public/message/rendering";
import { openChatSearch } from "@/features/chat/search/application/openChatSearch";
import { ChatSearchOverlay } from "@/features/chat/search/ui/ChatSearchOverlay";
import { motionScale, publishMotionScale } from "@/lib/appearance";
const highlights = vi.hoisted(() => ({ paint: vi.fn(), clear: vi.fn() }));

vi.mock("@/features/chat/search/adapters/searchHighlights", () => ({
  paintChatSearchHighlights: highlights.paint,
  clearChatSearchHighlights: highlights.clear,
}));
const nativeScrollIntoView = Object.getOwnPropertyDescriptor(
  HTMLElement.prototype,
  "scrollIntoView",
);

beforeEach(() => {
  vi.clearAllMocks();
  selectAgentSession("session-a");
  Object.defineProperty(HTMLElement.prototype, "scrollIntoView", {
    configurable: true,
    value: vi.fn(),
  });
});

afterEach(() => {
  if (nativeScrollIntoView) {
    Object.defineProperty(HTMLElement.prototype, "scrollIntoView", nativeScrollIntoView);
  } else {
    Reflect.deleteProperty(HTMLElement.prototype, "scrollIntoView");
  }
});

function SearchFixture({
  visible = true,
  removeComposer = false,
  hideComposer = false,
  inertComposer = false,
  content = "Alpha beta ALPHA a+b",
  earlierContent,
}: {
  visible?: boolean;
  removeComposer?: boolean;
  hideComposer?: boolean;
  inertComposer?: boolean;
  content?: ReactNode;
  earlierContent?: string;
}) {
  return (
    <>
      <ContentVisibility visible={visible}>
        <div role="region" aria-label="Conversation" tabIndex={-1} hidden={!visible}>
          {!removeComposer && (
            <div hidden={hideComposer} inert={inertComposer}>
              <textarea aria-label="Composer draft" defaultValue="Keep this draft." />
            </div>
          )}
          {earlierContent && (
            <article key="earlier" className={MESSAGE_CONTENT_SELECTOR.slice(1)}>
              {earlierContent}
            </article>
          )}
          <article key="answer" className={MESSAGE_CONTENT_SELECTOR.slice(1)}>
            {content}
          </article>
          <ChatSearchOverlay />
        </div>
      </ContentVisibility>
      <aside role="region" aria-label="Dock">
        <article className={MESSAGE_CONTENT_SELECTOR.slice(1)}>
          alpha outside the conversation
        </article>
        <button type="button">Outside</button>
      </aside>
    </>
  );
}

function renderSearch() {
  return render(<SearchFixture />);
}

function openSearch(): HTMLInputElement {
  act(() => openChatSearch());
  return screen.getByRole<HTMLInputElement>("textbox", { name: "Search in chat" });
}

function composer() {
  return screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Composer draft" });
}

function searchInput() {
  return screen.queryByRole<HTMLInputElement>("textbox", { name: "Search in chat" });
}

function searchFor(query: string) {
  const input = openSearch();
  fireEvent.change(input, { target: { value: query } });
  return input;
}

function expectDraftFocus(
  draft: HTMLTextAreaElement,
  value = "Keep this draft.",
  selection = [5, 10],
) {
  expect(document.activeElement).toBe(draft);
  expect(draft.value).toBe(value);
  expect([draft.selectionStart, draft.selectionEnd]).toEqual(selection);
}

describe("ChatSearchOverlay", () => {
  it("updates and navigates matches directly from user events", () => {
    renderSearch();
    const input = openSearch();

    fireEvent.change(input, { target: { value: "alpha" } });
    expect(screen.getByText("1 / 2")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Next match" }));
    expect(screen.getByText("2 / 2")).toBeTruthy();

    fireEvent.keyDown(input, { key: "Enter", shiftKey: true });
    expect(screen.getByText("1 / 2")).toBeTruthy();
  });

  it("owns reset semantics at close and session boundaries", () => {
    renderSearch();
    const input = openSearch();
    fireEvent.change(input, { target: { value: "alpha" } });

    fireEvent.keyDown(input, { key: "Escape" });
    expect(searchInput()).toBeNull();
    expect(openSearch().value).toBe("");

    fireEvent.change(searchInput()!, { target: { value: "beta" } });
    act(() => selectAgentSession("session-b"));
    expect(searchInput()).toBeNull();
    expect(openSearch().value).toBe("");
  });

  it("refreshes a growing reply without moving the query, caret or reading position", async () => {
    const { rerender } = renderSearch();
    const input = searchFor("alpha");
    const status = screen.getByRole("status");
    input.setSelectionRange(1, 3);
    vi.mocked(HTMLElement.prototype.scrollIntoView).mockClear();

    rerender(<SearchFixture content="Alpha beta ALPHA a+b alpha" />);
    await act(async () => {});

    expect(screen.getByRole("status")).toBe(status);
    expect(status.textContent).toBe("1 / 3");
    expect(searchInput()).toBe(input);
    expect(document.activeElement).toBe(input);
    expect([input.selectionStart, input.selectionEnd]).toEqual([1, 3]);
    expect(HTMLElement.prototype.scrollIntoView).not.toHaveBeenCalled();
    const ranges = highlights.paint.mock.lastCall![0] as Range[];
    expect(ranges.map((range) => range.toString())).toEqual(["Alpha", "ALPHA", "alpha"]);
  });

  it("keeps the selected occurrence in its message when earlier material arrives", async () => {
    const { rerender } = renderSearch();
    const input = searchFor("alpha");
    fireEvent.keyDown(input, { key: "Enter" });
    expect(screen.getByRole("status").textContent).toBe("2 / 2");
    vi.mocked(HTMLElement.prototype.scrollIntoView).mockClear();

    rerender(<SearchFixture earlierContent="alpha from an earlier reply" />);
    await act(async () => {});

    expect(screen.getByRole("status").textContent).toBe("3 / 3");
    const [ranges, active] = highlights.paint.mock.lastCall as [Range[], number];
    expect(ranges[active]?.toString()).toBe("ALPHA");
    expect(HTMLElement.prototype.scrollIntoView).not.toHaveBeenCalled();
    fireEvent.keyDown(input, { key: "Enter", shiftKey: true });
    expect(screen.getByRole("status").textContent).toBe("2 / 3");
  });

  it("finds a later result for the same previously unmatched query", async () => {
    const { rerender } = renderSearch();
    const input = searchFor("arriving");
    expect(screen.getByRole("status").textContent).toBe("0 / 0");

    rerender(<SearchFixture content="The arriving result." />);
    await act(async () => {});

    expect(screen.getByRole("status").textContent).toBe("1 / 1");
    expect(searchInput()).toBe(input);
    expect(screen.getByRole<HTMLButtonElement>("button", { name: "Next match" }).disabled).toBe(
      false,
    );
  });

  it("replaces detached ranges after rendered formatting changes", async () => {
    const { rerender } = renderSearch();
    searchFor("alpha");
    const original = (highlights.paint.mock.lastCall![0] as Range[])[0]!;

    rerender(
      <SearchFixture
        content={
          <p>
            <strong>Alpha</strong> beta <em>ALPHA</em>
          </p>
        }
      />,
    );
    await act(async () => {});

    const ranges = highlights.paint.mock.lastCall![0] as Range[];
    expect(ranges[0]).not.toBe(original);
    expect(ranges.map((range) => range.toString())).toEqual(["Alpha", "ALPHA"]);
    expect(ranges.every((range) => range.startContainer.isConnected)).toBe(true);
  });

  it("reconciles removed matches and clears the result when none remain", async () => {
    const { rerender } = renderSearch();
    const input = searchFor("alpha");
    fireEvent.keyDown(input, { key: "Enter" });

    rerender(<SearchFixture content="Only alpha remains" />);
    await act(async () => {});
    expect(screen.getByRole("status").textContent).toBe("1 / 1");
    expect((highlights.paint.mock.lastCall![0] as Range[])[0]?.toString()).toBe("alpha");

    rerender(<SearchFixture content="No matching content" />);
    await act(async () => {});
    expect(screen.getByRole("status").textContent).toBe("0 / 0");
    expect(highlights.paint.mock.lastCall![0]).toEqual([]);
    expect(document.activeElement).toBe(input);
  });

  it("keeps later outside focus while new matching content arrives", async () => {
    const { rerender } = renderSearch();
    searchFor("alpha");
    const outside = screen.getByRole("button", { name: "Outside" });
    outside.focus();
    vi.mocked(HTMLElement.prototype.scrollIntoView).mockClear();

    rerender(<SearchFixture content="alpha alpha alpha" />);
    await act(async () => {});

    expect(screen.getByRole("status").textContent).toBe("1 / 3");
    expect(document.activeElement).toBe(outside);
    expect(HTMLElement.prototype.scrollIntoView).not.toHaveBeenCalled();
  });

  it.each(["Previous match", "Next match"])(
    "keeps %s focused and locally closable when its last result disappears",
    async (name) => {
      const { rerender } = renderSearch();
      const draft = composer();
      draft.focus();
      draft.setSelectionRange(5, 10);
      searchFor("alpha");
      const control = screen.getByRole<HTMLButtonElement>("button", { name });
      act(() => control.focus());

      rerender(<SearchFixture content="No matching content" />);
      await act(async () => {});

      expect(screen.getByRole("status").textContent).toBe("0 / 0");
      expect(document.activeElement).toBe(control);
      expect(control.disabled).toBe(false);
      expect(control.getAttribute("aria-disabled")).toBe("true");
      highlights.paint.mockClear();
      fireEvent.click(control);
      fireEvent.keyDown(control, { key: "Enter" });
      fireEvent.keyUp(control, { key: "Enter" });
      fireEvent.keyDown(control, { key: " " });
      fireEvent.keyUp(control, { key: " " });
      expect(highlights.paint).not.toHaveBeenCalled();

      fireEvent.keyDown(control, { key: "Escape" });
      expect(searchInput()).toBeNull();
      expectDraftFocus(draft);
    },
  );

  it("ignores unrelated chrome and stops refreshing after the query is cleared", async () => {
    const { rerender } = renderSearch();
    const input = searchFor("alpha");
    const chrome = document.createElement("p");
    chrome.textContent = "alpha in unrelated chrome";
    highlights.paint.mockClear();
    await act(async () => {
      screen.getByRole("region", { name: "Conversation" }).append(chrome);
    });
    expect(highlights.paint).not.toHaveBeenCalled();
    expect(screen.getByRole("status").textContent).toBe("1 / 2");

    fireEvent.change(input, { target: { value: "" } });
    highlights.paint.mockClear();
    rerender(<SearchFixture content="alpha alpha alpha" />);
    await act(async () => {});
    expect(highlights.paint).not.toHaveBeenCalled();
    expect(screen.getByRole("status").textContent).toBe("");
  });

  it("does not repaint a retired search from a queued content update", async () => {
    const { rerender } = renderSearch();
    const draft = composer();
    draft.focus();
    searchFor("alpha");
    rerender(<SearchFixture content="alpha alpha alpha" />);
    fireEvent.click(screen.getByRole("button", { name: "Close" }));
    highlights.paint.mockClear();
    await act(async () => {});

    expect(searchInput()).toBeNull();
    expect(document.activeElement).toBe(draft);
    expect(highlights.paint).not.toHaveBeenCalled();
  });

  it.each(["Previous match", "Next match", "Close"])(
    "closes locally from %s and returns to the same draft selection",
    (name) => {
      const parentKey = vi.fn();
      window.addEventListener("keydown", parentKey);
      try {
        renderSearch();
        const draft = composer();
        draft.focus();
        draft.setSelectionRange(5, 10);
        searchFor("alpha");
        const control = screen.getByRole("button", { name });
        control.focus();
        fireEvent.keyDown(control, { key: "Escape" });

        expect(searchInput()).toBeNull();
        expect(parentKey).not.toHaveBeenCalled();
        expectDraftFocus(draft);
      } finally {
        window.removeEventListener("keydown", parentKey);
      }
    },
  );

  it("returns the original Composer selection after Escape in the query", () => {
    renderSearch();
    const draft = composer();
    draft.focus();
    draft.setSelectionRange(5, 10);
    const input = searchFor("alpha");
    fireEvent.keyDown(input, { key: "Escape" });

    expect(searchInput()).toBeNull();
    expectDraftFocus(draft);
    expect(highlights.clear).toHaveBeenCalled();
  });

  it("refocuses and selects an existing query without replacing its opening target", () => {
    renderSearch();
    const draft = composer();
    draft.focus();
    draft.setSelectionRange(5, 10);
    const input = searchFor("alpha");
    screen.getByRole("button", { name: "Close" }).focus();

    expect(openSearch()).toBe(input);
    expect(input.value).toBe("alpha");
    expect(document.activeElement).toBe(input);
    expect([input.selectionStart, input.selectionEnd]).toEqual([0, 5]);
    fireEvent.keyDown(input, { key: "Escape" });
    expectDraftFocus(draft);
  });

  it("retains later draft edits and their current caret when returning from Find", () => {
    renderSearch();
    const draft = composer();
    draft.focus();
    draft.setSelectionRange(5, 10);
    const input = searchFor("alpha");

    draft.focus();
    fireEvent.change(draft, { target: { value: "Keep this updated draft." } });
    draft.setSelectionRange(22, 22);
    input.focus();
    fireEvent.keyDown(input, { key: "Escape" });

    expectDraftFocus(draft, "Keep this updated draft.", [22, 22]);
  });

  it("keeps a later outside focus choice when closure is delivered", () => {
    renderSearch();
    composer().focus();
    searchFor("alpha");
    const outside = screen.getByRole("button", { name: "Outside" });
    outside.focus();
    fireEvent.click(screen.getByRole("button", { name: "Close" }));

    expect(searchInput()).toBeNull();
    expect(document.activeElement).toBe(outside);
  });

  it.each(["removed", "hidden", "inert"])(
    "returns to Conversation when the opener becomes %s",
    (state) => {
      const { rerender } = renderSearch();
      const draft = composer();
      draft.focus();
      const input = searchFor("alpha");
      rerender(
        <SearchFixture
          removeComposer={state === "removed"}
          hideComposer={state === "hidden"}
          inertComposer={state === "inert"}
        />,
      );
      input.focus();
      fireEvent.keyDown(input, { key: "Escape" });

      expect(document.activeElement).toBe(screen.getByRole("region", { name: "Conversation" }));
    },
  );

  it("counts and highlights only the owning conversation", () => {
    renderSearch();
    searchFor("alpha");
    expect(screen.getByRole("search", { name: "Search in chat" })).toBeTruthy();
    expect(screen.getByRole("status").textContent).toBe("1 / 2");
    const ranges = highlights.paint.mock.lastCall![0] as Range[];
    const region = screen.getByRole("region", { name: "Conversation" });
    expect(ranges.map((range) => range.toString())).toEqual(["Alpha", "ALPHA"]);
    expect(ranges.every((range) => region.contains(range.startContainer))).toBe(true);
  });

  it("does not search the document when its conversation boundary is missing", () => {
    render(
      <>
        <article className={MESSAGE_CONTENT_SELECTOR.slice(1)}>alpha</article>
        <ChatSearchOverlay />
      </>,
    );
    searchFor("alpha");
    expect(screen.getByRole("status").textContent).toBe("0 / 0");
    expect(highlights.paint.mock.lastCall![0]).toEqual([]);
  });

  it("keeps one status node from the empty query through navigation and no matches", () => {
    renderSearch();
    const input = openSearch();
    const status = screen.getByRole("status");
    expect(status.textContent).toBe("");
    fireEvent.change(input, { target: { value: "alpha" } });
    expect(screen.getByRole("status")).toBe(status);
    fireEvent.keyDown(input, { key: "Enter" });
    expect(status.textContent).toBe("2 / 2");
    fireEvent.keyDown(input, { key: "Enter" });
    expect(status.textContent).toBe("1 / 2");
    fireEvent.change(input, { target: { value: "missing" } });
    expect(status.textContent).toBe("0 / 0");
    expect(screen.getByRole("button", { name: "Next match" }).getAttribute("aria-disabled")).toBe(
      "true",
    );
  });

  it("keeps regular-expression punctuation literal", () => {
    renderSearch();
    searchFor("a+b");
    expect(screen.getByRole("status").textContent).toBe("1 / 1");
  });

  it("leaves composing and already-handled keys with their owner", () => {
    renderSearch();
    const input = searchFor("alpha");
    for (const key of ["Enter", "Escape"]) {
      fireEvent.keyDown(input, { key, isComposing: true, keyCode: 229 });
      const handled = new KeyboardEvent("keydown", { key, bubbles: true, cancelable: true });
      handled.preventDefault();
      fireEvent(input, handled);
    }
    expect(searchInput()).toBe(input);
    expect(screen.getByRole("status").textContent).toBe("1 / 2");
  });

  it.each([
    { scale: 1, reduced: false, behavior: "smooth" },
    { scale: 0, reduced: false, behavior: "instant" },
    { scale: 1, reduced: true, behavior: "instant" },
  ])("honors application and system motion preferences: %j", ({ scale, reduced, behavior }) => {
    const previousScale = motionScale();
    const media = vi.spyOn(window, "matchMedia").mockReturnValue({
      matches: reduced,
      media: "(prefers-reduced-motion: reduce)",
      onchange: null,
      addListener: vi.fn(),
      removeListener: vi.fn(),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn(() => true),
    });
    try {
      publishMotionScale(scale);
      renderSearch();
      const input = searchFor("alpha");
      expect(HTMLElement.prototype.scrollIntoView).toHaveBeenLastCalledWith({
        block: "center",
        behavior,
      });
      fireEvent.keyDown(input, { key: "Enter" });
      expect(HTMLElement.prototype.scrollIntoView).toHaveBeenLastCalledWith({
        block: "center",
        behavior,
      });
      expect(document.activeElement).toBe(input);
    } finally {
      publishMotionScale(previousScale);
      media.mockRestore();
    }
  });

  it("reads a later application motion preference at navigation without replacing Find", () => {
    const previousScale = motionScale();
    try {
      publishMotionScale(1);
      renderSearch();
      const input = searchFor("alpha");
      publishMotionScale(0);
      fireEvent.keyDown(input, { key: "Enter" });
      expect(HTMLElement.prototype.scrollIntoView).toHaveBeenLastCalledWith({
        block: "center",
        behavior: "instant",
      });
      expect(searchInput()).toBe(input);
    } finally {
      publishMotionScale(previousScale);
    }
  });

  it("does not open for hidden content", () => {
    render(<SearchFixture visible={false} />);
    const outside = screen.getByRole("button", { name: "Outside" });
    outside.focus();
    act(() => openChatSearch());
    expect(searchInput()).toBeNull();
    expect(document.activeElement).toBe(outside);
    expect(highlights.paint).not.toHaveBeenCalled();
  });

  it("retires ranges on hiding without reclaiming host focus or reopening on reveal", () => {
    const { rerender } = renderSearch();
    composer().focus();
    searchFor("alpha");
    const clearCount = highlights.clear.mock.calls.length;
    const outside = screen.getByRole("button", { name: "Outside" });
    outside.focus();
    rerender(<SearchFixture visible={false} />);
    expect(searchInput()).toBeNull();
    expect(highlights.clear.mock.calls.length).toBeGreaterThan(clearCount);
    expect(document.activeElement).toBe(outside);
    rerender(<SearchFixture />);
    expect(searchInput()).toBeNull();
    expect(document.activeElement).toBe(outside);
    expect(openSearch().value).toBe("");
  });

  it("clears a retired Session search without returning focus to its prior opener", () => {
    renderSearch();
    const draft = composer();
    draft.focus();
    searchFor("alpha");
    const outside = screen.getByRole("button", { name: "Outside" });
    outside.focus();
    const clearCount = highlights.clear.mock.calls.length;
    act(() => selectAgentSession("session-b"));
    expect(searchInput()).toBeNull();
    expect(document.activeElement).toBe(outside);
    expect(highlights.clear.mock.calls.length).toBeGreaterThan(clearCount);
    expect(openSearch().value).toBe("");
  });
});
