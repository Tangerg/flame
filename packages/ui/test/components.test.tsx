import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { createRef, useRef, useState } from "react";

import {
  CodeBlock,
  ClipboardReceiptProvider,
  type CopyReceipt,
  CodeCaption,
  CodeFrame,
  CodeHighlightProvider,
  type CodeSourceProps,
  FilterBar,
  copyReaderSelection,
  SearchField,
  ScrollRegion,
  scrollWithHorizontalArrow,
  selectAllInReader,
} from "../dist/index.js";

afterEach(cleanup);

describe("named scroll readers", () => {
  function horizontalRange(region: HTMLElement, rtl = false) {
    let offset = 0;
    Object.defineProperties(region, {
      clientWidth: { configurable: true, value: 200 },
      scrollWidth: { configurable: true, value: 800 },
      scrollLeft: {
        configurable: true,
        get: () => offset,
        set: (value: number) => {
          offset = Math.max(rtl ? -600 : 0, Math.min(rtl ? 0 : 600, value));
        },
      },
    });
  }

  it("keeps inline identity inside a heading keyboard reachable and complete", () => {
    const identity = `https://runtime.example/${"workspace/".repeat(50)}`;
    const reader = createRef<HTMLSpanElement>();
    render(
      <h2>
        New session{" "}
        <ScrollRegion as="span" ref={reader} label={identity}>
          {identity}
        </ScrollRegion>
      </h2>,
    );
    const heading = screen.getByRole("heading", {
      level: 2,
      name: `New session ${identity}`,
    });
    const region = screen.getByRole("region", { name: identity });
    expect(heading.contains(region)).toBe(true);
    expect(region.tagName).toBe("SPAN");
    expect(region.textContent).toBe(identity);
    expect(reader.current).toBe(region);
    expect(region.tabIndex).toBe(0);
    region.focus();
    expect(document.activeElement).toBe(region);
  });

  it("preserves the default region ref and caller focusability", () => {
    const reader = createRef<HTMLDivElement>();
    render(
      <ScrollRegion label="Tool output" ref={reader} tabIndex={-1}>
        Complete output
      </ScrollRegion>,
    );
    const region = screen.getByRole("region", { name: "Tool output" });
    expect(region.tagName).toBe("DIV");
    expect(reader.current).toBe(region);
    expect(region.tabIndex).toBe(-1);
  });

  it("pans the focused reader without moving a sibling or focus", () => {
    const parentKeyDown = vi.fn();
    render(
      <div onKeyDown={parentKeyDown}>
        <ScrollRegion label="Plot">Wide figure</ScrollRegion>
        <ScrollRegion label="Values">Exact values</ScrollRegion>
      </div>,
    );
    const plot = screen.getByRole("region", { name: "Plot" });
    const values = screen.getByRole("region", { name: "Values" });
    horizontalRange(plot);
    horizontalRange(values);
    plot.focus();

    expect(fireEvent.keyDown(plot, { key: "ArrowRight" })).toBe(false);
    expect(plot.scrollLeft).toBeGreaterThan(0);
    expect(values.scrollLeft).toBe(0);
    expect(document.activeElement).toBe(plot);
    expect(fireEvent.keyDown(plot, { key: "ArrowLeft" })).toBe(false);
    expect(plot.scrollLeft).toBe(0);

    expect(fireEvent.keyDown(plot, { key: "ArrowLeft" })).toBe(false);
    plot.scrollLeft = 600;
    expect(fireEvent.keyDown(plot, { key: "ArrowRight" })).toBe(false);
    expect(plot.scrollLeft).toBe(600);
    expect(parentKeyDown).not.toHaveBeenCalled();
  });

  it("lets the caller handle a key first and invokes it once", () => {
    const caller = vi.fn((event: { preventDefault(): void }) => {
      event.preventDefault();
    });
    render(
      <ScrollRegion as="span" label="Source identity" onKeyDown={caller}>
        Long source identity
      </ScrollRegion>,
    );
    const region = screen.getByRole("region", { name: "Source identity" });
    horizontalRange(region);
    expect(fireEvent.keyDown(region, { key: "ArrowRight" })).toBe(false);
    expect(caller).toHaveBeenCalledOnce();
    expect(region.scrollLeft).toBe(0);
  });

  it("leaves child controls, modified keys, composition and vertical reading alone", () => {
    const caller = vi.fn();
    render(
      <ScrollRegion label="Editable results" onKeyDown={caller}>
        <input aria-label="Filter values" />
        <button>Next result</button>
      </ScrollRegion>,
    );
    const region = screen.getByRole("region", { name: "Editable results" });
    horizontalRange(region);
    const input = screen.getByRole("textbox", { name: "Filter values" });
    const button = screen.getByRole("button", { name: "Next result" });
    expect(fireEvent.keyDown(input, { key: "ArrowRight" })).toBe(true);
    expect(fireEvent.keyDown(button, { key: "ArrowRight" })).toBe(true);
    const untouched = [
      { key: "ArrowRight", altKey: true },
      { key: "ArrowRight", ctrlKey: true },
      { key: "ArrowRight", metaKey: true },
      { key: "ArrowRight", shiftKey: true },
      { key: "ArrowRight", isComposing: true },
      { key: "ArrowUp" },
      { key: "ArrowDown" },
      { key: "Home" },
      { key: "End" },
      { key: "Tab" },
    ];
    for (const key of untouched) {
      expect(fireEvent.keyDown(region, key)).toBe(true);
    }
    expect(region.scrollLeft).toBe(0);
    expect(caller).toHaveBeenCalledTimes(untouched.length + 2);
  });

  it("delegates keyboard panning to the renderer that owns its columns", () => {
    const scrollBy = vi.fn();
    render(
      <ScrollRegion
        label="Rendered source"
        onKeyDown={(event) => scrollWithHorizontalArrow(event, scrollBy)}
      >
        <button>Source action</button>
      </ScrollRegion>,
    );
    const region = screen.getByRole("region", { name: "Rendered source" });
    Object.defineProperties(region, {
      clientWidth: { configurable: true, value: 200 },
      scrollWidth: { configurable: true, value: 200 },
    });
    expect(fireEvent.keyDown(region, { key: "ArrowRight" })).toBe(false);
    expect(scrollBy).toHaveBeenCalledExactlyOnceWith(20);
    expect(region.scrollLeft).toBe(0);
    expect(fireEvent.keyDown(region, { key: "ArrowLeft", metaKey: true })).toBe(
      true,
    );
    expect(
      fireEvent.keyDown(screen.getByRole("button"), { key: "ArrowLeft" }),
    ).toBe(true);
    expect(scrollBy).toHaveBeenCalledOnce();
  });

  it("keeps native keys when the reader has no horizontal range", () => {
    render(<ScrollRegion label="Short result">Complete result</ScrollRegion>);
    const region = screen.getByRole("region", { name: "Short result" });
    Object.defineProperties(region, {
      clientWidth: { configurable: true, value: 200 },
      scrollWidth: { configurable: true, value: 200 },
    });
    expect(fireEvent.keyDown(region, { key: "ArrowRight" })).toBe(true);
    Object.defineProperty(region, "clientWidth", { value: 0 });
    expect(fireEvent.keyDown(region, { key: "ArrowLeft" })).toBe(true);
  });

  it("pans in the physical arrow direction with native RTL offsets", () => {
    render(
      <ScrollRegion label="RTL source" dir="rtl">
        Wide source
      </ScrollRegion>,
    );
    const region = screen.getByRole("region", { name: "RTL source" });
    horizontalRange(region, true);
    expect(fireEvent.keyDown(region, { key: "ArrowLeft" })).toBe(false);
    expect(region.scrollLeft).toBeLessThan(0);
    expect(fireEvent.keyDown(region, { key: "ArrowRight" })).toBe(false);
    expect(region.scrollLeft).toBe(0);
  });
});

describe("recorded code", () => {
  afterEach(() => {
    cleanup();
    const reset = render(
      <CodeBlock label="Wrap preference">fixture</CodeBlock>,
    );
    const toggle = screen.queryByRole("button", { name: "Disable word wrap" });
    if (toggle) fireEvent.click(toggle);
    reset.unmount();
  });

  it("scopes each platform's select-all command to the active literal reader", () => {
    render(
      <>
        <p>Surrounding application text</p>
        <CodeBlock label="First source">{"first\n  literal"}</CodeBlock>
        <CodeBlock label="Second source">{"second <literal>"}</CodeBlock>
      </>,
    );
    const first = screen.getByRole("region", { name: "First source" });
    const second = screen.getByRole("region", { name: "Second source" });
    first.focus();
    fireEvent.keyDown(first, { key: "a", ctrlKey: true });
    expect(document.getSelection()?.toString()).toBe("first\n  literal");
    expect(document.activeElement).toBe(first);

    second.focus();
    fireEvent.keyDown(second, { key: "a", metaKey: true });
    expect(document.getSelection()?.toString()).toBe("second <literal>");
    expect(document.activeElement).toBe(second);
    document.getSelection()?.removeAllRanges();
  });

  it.each([{ isComposing: true }, { keyCode: 229 }])(
    "does not replace the selection during composition %o",
    (composition) => {
      render(<CodeBlock label="Captured source">{"recorded text"}</CodeBlock>);
      const reader = screen.getByRole("region", { name: "Captured source" });
      document.getSelection()?.removeAllRanges();
      reader.focus();
      expect(
        fireEvent.keyDown(reader, {
          key: "a",
          metaKey: true,
          ...composition,
        }),
      ).toBe(true);
      expect(document.getSelection()?.toString()).toBe("");
      expect(document.activeElement).toBe(reader);
    },
  );

  it.each([
    { key: "a", metaKey: true, altKey: true },
    { key: "a", ctrlKey: true, shiftKey: true },
    { key: "b", metaKey: true },
    { key: "a" },
  ])("leaves other keys and modified Select All to the platform %o", (init) => {
    render(<CodeBlock label="Captured source">{"recorded text"}</CodeBlock>);
    const reader = screen.getByRole("region", { name: "Captured source" });
    document.getSelection()?.removeAllRanges();
    reader.focus();
    expect(fireEvent.keyDown(reader, init)).toBe(true);
    expect(document.getSelection()?.rangeCount).toBe(0);
  });

  it("does not replace a Select All an earlier handler already owns", () => {
    render(
      <div onKeyDownCapture={(event) => event.preventDefault()}>
        <CodeBlock label="Captured source">{"recorded text"}</CodeBlock>
      </div>,
    );
    const reader = screen.getByRole("region", { name: "Captured source" });
    document.getSelection()?.removeAllRanges();
    reader.focus();
    fireEvent.keyDown(reader, { key: "a", ctrlKey: true });
    expect(document.getSelection()?.rangeCount).toBe(0);
  });

  it("leaves Select All to a descendant editable and writes presented text only for a whole-reader copy", () => {
    render(
      <>
        <p>Surrounding application text</p>
        <div
          role="region"
          aria-label="Reader"
          tabIndex={0}
          onKeyDown={selectAllInReader}
          onCopy={(event) => copyReaderSelection(event, "presented text")}
        >
          <span>captured</span>
          <input aria-label="Nested field" defaultValue="draft" />
          <span contentEditable suppressContentEditableWarning>
            <b>editable</b>
          </span>
        </div>
      </>,
    );
    const region = screen.getByRole("region", { name: "Reader" });
    const selection = document.getSelection()!;
    const copy = () => {
      const setData = vi.fn();
      const proceeded = fireEvent.copy(region, { clipboardData: { setData } });
      return { proceeded, setData };
    };
    const expectNativeCopy = () => {
      const { proceeded, setData } = copy();
      expect(proceeded).toBe(true);
      expect(setData).not.toHaveBeenCalled();
    };
    selection.removeAllRanges();
    for (const editable of [
      screen.getByRole("textbox", { name: "Nested field" }),
      screen.getByText("editable"),
    ]) {
      expect(fireEvent.keyDown(editable, { key: "a", metaKey: true })).toBe(
        true,
      );
      expect(selection.rangeCount).toBe(0);
    }
    expectNativeCopy();

    expect(fireEvent.keyDown(region, { key: "a", metaKey: true })).toBe(false);
    expect(selection.getRangeAt(0).startContainer).toBe(region);
    expect(selection.toString()).not.toContain("Surrounding");
    const whole = copy();
    expect(whole.proceeded).toBe(false);
    expect(whole.setData).toHaveBeenCalledExactlyOnceWith(
      "text/plain",
      "presented text",
    );
    for (const editable of [
      screen.getByRole("textbox", { name: "Nested field" }),
      screen.getByText("editable"),
    ]) {
      const setData = vi.fn();
      expect(fireEvent.copy(editable, { clipboardData: { setData } })).toBe(
        true,
      );
      expect(setData).not.toHaveBeenCalled();
    }

    selection.selectAllChildren(screen.getByText("captured"));
    expectNativeCopy();
    selection.setBaseAndExtent(region, 0, region, 0);
    expectNativeCopy();
    selection.setBaseAndExtent(
      screen.getByText("Surrounding application text"),
      0,
      region,
      region.childNodes.length,
    );
    expectNativeCopy();
    selection.removeAllRanges();
  });

  it("selects and copies a content boundary within its focused reader without its framing", () => {
    let handled = false;
    function Reader({ rows }: { rows: string[] }) {
      const content = useRef<HTMLDivElement>(null);
      return (
        <div
          onKeyDownCapture={(event) => {
            if (handled) event.preventDefault();
          }}
          onCopyCapture={(event) => {
            if (handled) event.preventDefault();
          }}
        >
          <p>Surrounding application text</p>
          <div
            role="region"
            aria-label="Reader"
            tabIndex={0}
            onKeyDown={(event) => selectAllInReader(event, content.current!)}
            onCopy={(event) =>
              copyReaderSelection(event, rows.join("\n"), content.current!)
            }
          >
            <p>src/review.go</p>
            <input aria-label="Reader field" defaultValue="draft" />
            <div ref={content}>
              <div>
                {rows.map((row, index) => (
                  <div key={index}>{row}</div>
                ))}
              </div>
            </div>
          </div>
        </div>
      );
    }
    const view = render(<Reader rows={["first", "", "\tthird"]} />);
    const region = screen.getByRole("region", { name: "Reader" });
    const content = screen.getByText("first").parentElement!.parentElement!;
    const field = screen.getByRole("textbox", { name: "Reader field" });
    const selection = document.getSelection()!;
    const copy = () => {
      const setData = vi.fn();
      const proceeded = fireEvent.copy(region, { clipboardData: { setData } });
      return { proceeded, setData };
    };
    const expectNativeCopy = () => {
      const { proceeded, setData } = copy();
      expect(proceeded).toBe(true);
      expect(setData).not.toHaveBeenCalled();
    };
    selection.removeAllRanges();

    expect(fireEvent.keyDown(field, { key: "a", metaKey: true })).toBe(true);
    expect(selection.rangeCount).toBe(0);
    for (const init of [
      { key: "a", metaKey: true, isComposing: true },
      { key: "a", metaKey: true, keyCode: 229 },
      { key: "a", metaKey: true, shiftKey: true },
      { key: "a" },
    ]) {
      expect(fireEvent.keyDown(region, init)).toBe(true);
      expect(selection.rangeCount).toBe(0);
    }
    handled = true;
    expect(fireEvent.keyDown(region, { key: "a", ctrlKey: true })).toBe(false);
    expect(selection.rangeCount).toBe(0);
    handled = false;

    expect(fireEvent.keyDown(region, { key: "a", metaKey: true })).toBe(false);
    const range = selection.getRangeAt(0);
    expect(range.startContainer).toBe(content);
    expect(range.endContainer).toBe(content);
    expect(selection.toString()).not.toContain("src/review.go");
    expect(selection.toString()).not.toContain("Surrounding");
    const whole = copy();
    expect(whole.proceeded).toBe(false);
    expect(whole.setData).toHaveBeenCalledExactlyOnceWith(
      "text/plain",
      "first\n\n\tthird",
    );

    view.rerender(<Reader rows={["first", "", "\tthird", "", "fifth"]} />);
    expect(copy().setData).toHaveBeenCalledExactlyOnceWith(
      "text/plain",
      "first\n\n\tthird\n\nfifth",
    );
    const fromField = vi.fn();
    expect(
      fireEvent.copy(field, { clipboardData: { setData: fromField } }),
    ).toBe(true);
    expect(fromField).not.toHaveBeenCalled();
    handled = true;
    expect(copy().setData).not.toHaveBeenCalled();
    handled = false;

    selection.selectAllChildren(screen.getByText("first"));
    expectNativeCopy();
    selection.setBaseAndExtent(content, 0, content, 0);
    expectNativeCopy();
    selection.setBaseAndExtent(
      screen.getByText("src/review.go"),
      0,
      content,
      content.childNodes.length,
    );
    expectNativeCopy();
    selection.removeAllRanges();
  });

  it("selects an owned shadow reader without crossing into another reader", () => {
    const host = document.createElement("div");
    const source = host.attachShadow({ mode: "open" });
    source.textContent = "captured source";
    const outside = document.createElement("div");
    const other = outside.attachShadow({ mode: "open" });
    other.textContent = "outside source";
    document.body.append(outside);
    let content = source;
    render(
      <div
        role="region"
        aria-label="Shadow reader"
        tabIndex={0}
        ref={(node) => {
          node?.append(host);
        }}
        onKeyDown={(event) => selectAllInReader(event, content)}
      />,
    );
    const region = screen.getByRole("region", { name: "Shadow reader" });
    const selection = document.getSelection()!;
    selection.removeAllRanges();
    expect(fireEvent.keyDown(region, { key: "a", ctrlKey: true })).toBe(false);
    expect(selection.getRangeAt(0).startContainer).toBe(source);
    expect(selection.getRangeAt(0).endContainer).toBe(source);
    selection.removeAllRanges();
    content = other;
    expect(fireEvent.keyDown(region, { key: "a", ctrlKey: true })).toBe(true);
    expect(selection.rangeCount).toBe(0);
    outside.remove();
  });

  it("leaves Select All and copy native for a boundary outside the reader", () => {
    render(
      <>
        <p>Outside source</p>
        <div
          role="region"
          aria-label="Reader"
          tabIndex={0}
          onKeyDown={(event) =>
            selectAllInReader(event, screen.getByText("Outside source"))
          }
          onCopy={(event) =>
            copyReaderSelection(
              event,
              "outside",
              screen.getByText("Outside source"),
            )
          }
        >
          captured
        </div>
      </>,
    );
    const region = screen.getByRole("region", { name: "Reader" });
    const selection = document.getSelection()!;
    selection.removeAllRanges();
    expect(fireEvent.keyDown(region, { key: "a", metaKey: true })).toBe(true);
    expect(selection.rangeCount).toBe(0);

    selection.selectAllChildren(screen.getByText("Outside source"));
    const setData = vi.fn();
    expect(fireEvent.copy(region, { clipboardData: { setData } })).toBe(true);
    expect(setData).not.toHaveBeenCalled();
    selection.removeAllRanges();
  });

  it("keeps the caption outside the literal keyboard-scrollable source", () => {
    const source =
      '<script>window.example = "literal"</script>\n  <img src="recorded">';
    const { rerender } = render(
      <CodeBlock label="Captured source" language="html">
        {source}
      </CodeBlock>,
    );
    const region = screen.getByRole("region", { name: "Captured source" });
    expect(region.tagName).toBe("DIV");
    expect(region.tabIndex).toBe(0);
    expect(region.textContent).toBe(source);
    expect(region.contains(screen.getByText("html", { exact: true }))).toBe(
      false,
    );
    expect(region.querySelector("script, img")).toBeNull();
    rerender(<CodeBlock label="Captured source">{source}</CodeBlock>);
    expect(screen.getByText("text", { exact: true })).toBeTruthy();
    expect(
      screen.getByRole("region", { name: "Captured source" }).textContent,
    ).toBe(source);
  });

  it("shows and copies the literal child an installed renderer passes, reporting success only after acceptance", async () => {
    const writeText = vi.fn<(text: string) => Promise<void>>();
    const accepted = Promise.withResolvers<void>();
    writeText.mockReturnValue(accepted.promise);
    vi.stubGlobal("navigator", { clipboard: { writeText } });
    const source = "type Executor interface {\n\tRun()\n}";
    const receipts: CopyReceipt[] = [];
    render(
      <ClipboardReceiptProvider onReceipt={(receipt) => receipts.push(receipt)}>
        <CodeBlock label="Go source" language="go">
          {source}
        </CodeBlock>
      </ClipboardReceiptProvider>,
    );
    expect(screen.getByRole("region", { name: "Go source" }).textContent).toBe(
      source,
    );

    fireEvent.click(screen.getByRole("button", { name: "Copy code" }));
    expect(writeText).toHaveBeenCalledWith(source);
    expect(screen.queryByRole("button", { name: "Copied" })).toBeNull();
    await act(async () => accepted.resolve());
    expect(receipts).toMatchObject([{ status: "copied" }]);
    expect(screen.getByRole("button", { name: "Copy code" })).toBeTruthy();
    vi.unstubAllGlobals();
  });

  it("shows a captured caption without changing its source language", () => {
    const languages: string[] = [];
    function Source({ source, language }: CodeSourceProps) {
      languages.push(language);
      return <code>{source}</code>;
    }
    render(
      <CodeHighlightProvider component={Source}>
        <CodeBlock
          label="Captured request"
          caption="Original input"
          language="json"
        >
          {"{}"}
        </CodeBlock>
      </CodeHighlightProvider>,
    );
    expect(screen.getByText("Original input")).not.toBeNull();
    expect(languages).toContain("json");
    expect(
      screen.getByRole("region", { name: "Captured request" }).textContent,
    ).toBe("{}");
  });

  it("paints a host source component for the same literal and keeps copy on the original text", async () => {
    const writeText = vi
      .fn<(text: string) => Promise<void>>()
      .mockResolvedValue();
    vi.stubGlobal("navigator", { clipboard: { writeText } });
    const seen: [string, string][] = [];
    function TintedSource({ source, language }: CodeSourceProps) {
      seen.push([source, language]);
      return (
        <code>
          <span style={{ color: "#0550ae" }}>{source}</span>
        </code>
      );
    }
    render(
      <CodeHighlightProvider component={TintedSource}>
        <CodeBlock label="Highlighted" language="ts">
          {"a < b"}
        </CodeBlock>
        <CodeBlock label="Untagged">{"plain"}</CodeBlock>
      </CodeHighlightProvider>,
    );
    const highlighted = screen.getByRole("region", { name: "Highlighted" });
    expect(highlighted.textContent).toBe("a < b");
    expect(highlighted.querySelector("span")).not.toBeNull();
    expect(seen).toContainEqual(["a < b", "ts"]);
    expect(seen).toContainEqual(["plain", "text"]);

    fireEvent.click(screen.getAllByRole("button", { name: "Copy code" })[0]!);
    await act(async () => {});
    expect(writeText).toHaveBeenCalledWith("a < b");
    vi.unstubAllGlobals();
  });

  it("keeps the same source surface when the host highlighter changes under a mounted block", () => {
    function PlainSource({ source }: CodeSourceProps) {
      return <code>{source}</code>;
    }
    function StatefulSource({ source }: CodeSourceProps) {
      const [tint] = useState("#0550ae");
      return (
        <code>
          <span style={{ color: tint }}>{source}</span>
        </code>
      );
    }
    const block = (
      <CodeBlock label="Mounted" language="go">
        {"func main() {}"}
      </CodeBlock>
    );
    const view = render(
      <CodeHighlightProvider component={PlainSource}>
        {block}
      </CodeHighlightProvider>,
    );
    const region = screen.getByRole("region", { name: "Mounted" });

    view.rerender(
      <CodeHighlightProvider component={StatefulSource}>
        {block}
      </CodeHighlightProvider>,
    );
    expect(screen.getByRole("region", { name: "Mounted" })).toBe(region);
    expect(region.textContent).toBe("func main() {}");
    expect(region.querySelector("span")).not.toBeNull();

    view.rerender(
      <CodeHighlightProvider component={PlainSource}>
        {block}
      </CodeHighlightProvider>,
    );
    expect(screen.getByRole("region", { name: "Mounted" })).toBe(region);
    expect(region.textContent).toBe("func main() {}");
    expect(region.querySelector("span")).toBeNull();
  });

  it("keeps changing wrap labels as ordinary actions without an inverted pressed state", () => {
    render(
      <CodeBlock label="Captured source">
        {"first line\nsecond line"}
      </CodeBlock>,
    );
    const action = screen.getByRole("button", { name: "Enable word wrap" });
    expect(action.getAttribute("aria-pressed")).toBeNull();
    action.focus();
    fireEvent.click(action);
    expect(screen.getByRole("button", { name: "Disable word wrap" })).toBe(
      action,
    );
    expect(action.getAttribute("aria-pressed")).toBeNull();
    expect(document.activeElement).toBe(action);
    expect(
      screen
        .getByRole("region", { name: "Captured source" })
        .getAttribute("data-wrap"),
    ).toBe("true");
    fireEvent.click(action);
    expect(screen.getByRole("button", { name: "Enable word wrap" })).toBe(
      action,
    );
    expect(action.getAttribute("aria-pressed")).toBeNull();
    expect(document.activeElement).toBe(action);
    expect(
      screen
        .getByRole("region", { name: "Captured source" })
        .getAttribute("data-wrap"),
    ).toBe("false");
  });

  it("shares one wrap preference across every block on the page", () => {
    render(
      <>
        <CodeBlock label="First">a</CodeBlock>
        <CodeBlock label="Second">b</CodeBlock>
      </>,
    );
    const [first] = screen.getAllByRole("button", { name: "Enable word wrap" });
    fireEvent.click(first!);
    expect(
      screen
        .getAllByRole("button", { name: "Disable word wrap" })
        .map((button) => button.getAttribute("aria-pressed")),
    ).toEqual([null, null]);
    expect(
      screen
        .getAllByRole("region")
        .map((region) => region.getAttribute("data-wrap")),
    ).toEqual(["true", "true"]);
  });

  it("frames a consumer body under one caption that leads with consumer actions", () => {
    render(
      <CodeFrame>
        <CodeCaption language="output" code="ok" wrappable={false}>
          <button type="button">Find</button>
        </CodeCaption>
        <div role="region" aria-label="Tool output">
          ok
        </div>
      </CodeFrame>,
    );
    const caption = screen.getByText("output").parentElement!;
    expect(
      screen
        .getAllByRole("button")
        .map(
          (button) => button.getAttribute("aria-label") ?? button.textContent,
        ),
    ).toEqual(["Find", "Copy code"]);
    expect(caption.nextElementSibling).toBe(
      screen.getByRole("region", { name: "Tool output" }),
    );
  });
});

describe("filter bar", () => {
  it("reads search, then page actions, then filters", () => {
    render(
      <FilterBar
        label="Page controls"
        search={<SearchField aria-label="Search this page" />}
        actions={<button type="button">Refresh</button>}
      >
        <button type="button">Status</button>
      </FilterBar>,
    );
    const zone = screen.getByRole("group", { name: "Page controls" });
    const order = Array.from(
      zone.querySelectorAll("input, button"),
      (node) => node.getAttribute("aria-label") ?? node.textContent,
    );
    expect(order.filter((name) => name !== "Clear search")).toEqual([
      "Search this page",
      "Refresh",
      "Status",
    ]);
  });
});
