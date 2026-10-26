import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { FormEvent, ReactNode } from "react";
import { createRef, useRef, useState } from "react";
import { Brain } from "@flame/icons/react";
import {
  Button,
  CodeBlock,
  CodeCaption,
  CodeFrame,
  CodeHighlightProvider,
  type CodeSourceProps,
  ImageNotice,
  readCalloutMarker,
  useCopyFeedback,
  Disclosure,
  DiffTable,
  FilterBar,
  Checkbox,
  Form,
  FormActions,
  FormField,
  FormGroup,
  FormSection,
  FontScaleProvider,
  copyWholeReader,
  Facts,
  HeadingLevel,
  IconButton,
  ListRow,
  Metadata,
  ModalDialog,
  Panel,
  PanelHeader,
  PanelHost,
  PanelSection,
  RecordGroup,
  RecordHeading,
  Stats,
  RecordForm,
  SearchField,
  ScrollRegion,
  type SearchFieldProps,
  Segmented,
  Select,
  selectAllInReader,
  TextArea,
  TextField,
  TooltipProvider,
} from "../dist/index.js";
import { vocab } from "../dist/vocabulary.js";

afterEach(cleanup);

describe("named scroll readers", () => {
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
});

describe("record form focus ownership", () => {
  it("resolves the current return target only after an eligible form removal", () => {
    let target: HTMLElement | null = null;
    const resolveOpener = vi.fn(() => target);
    const content = (open: boolean) => (
      <>
        <input aria-label="Search records" />
        <button>Outside action</button>
        {open && (
          <RecordForm aria-label="Edit record" opener={resolveOpener}>
            <input aria-label="Draft content" />
          </RecordForm>
        )}
      </>
    );
    const view = render(content(true));
    expect(resolveOpener).not.toHaveBeenCalled();
    view.rerender(content(true));
    expect(resolveOpener).not.toHaveBeenCalled();

    target = screen.getByRole("textbox", { name: "Search records" });
    screen.getByRole<HTMLElement>("textbox", { name: "Draft content" }).focus();
    view.rerender(content(false));
    expect(resolveOpener).toHaveBeenCalledOnce();
    expect(document.activeElement).toBe(target);

    view.rerender(content(true));
    screen.getByRole<HTMLElement>("textbox", { name: "Draft content" }).focus();
    const outside = screen.getByRole<HTMLElement>("button", {
      name: "Outside action",
    });
    outside.focus();
    view.rerender(content(false));
    expect(resolveOpener).toHaveBeenCalledOnce();
    expect(document.activeElement).toBe(outside);
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
          onCopy={(event) => copyWholeReader(event, "presented text")}
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
              copyWholeReader(event, rows.join("\n"), content.current!)
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
            copyWholeReader(
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
    expect(region.tagName).toBe("PRE");
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
    render(
      <CodeBlock label="Go source" language="go">
        {source}
      </CodeBlock>,
    );
    expect(screen.getByRole("region", { name: "Go source" }).textContent).toBe(
      source,
    );

    fireEvent.click(screen.getByRole("button", { name: "Copy code" }));
    expect(writeText).toHaveBeenCalledWith(source);
    expect(screen.queryByRole("button", { name: "Copied" })).toBeNull();
    await act(async () => accepted.resolve());
    expect(screen.getByRole("button", { name: "Copied" })).toBeTruthy();
    vi.unstubAllGlobals();
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
    ).toEqual(["true", "true"]);
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

describe("copy feedback", () => {
  function CopyHarness({
    material,
    copyMaterial,
  }: {
    material: string;
    copyMaterial: (text: string) => Promise<boolean>;
  }) {
    const feedback = useCopyFeedback(material, 1500, copyMaterial);
    return (
      <button
        type="button"
        aria-label={feedback.copied ? "copied" : "copy"}
        onClick={feedback.copy}
      >
        {material}
      </button>
    );
  }

  afterEach(() => {
    vi.clearAllTimers();
    vi.useRealTimers();
  });

  it("lets only the latest intent publish feedback for the same material", async () => {
    const older = Promise.withResolvers<boolean>();
    const latest = Promise.withResolvers<boolean>();
    const copyMaterial = vi
      .fn<(text: string) => Promise<boolean>>()
      .mockReturnValueOnce(older.promise)
      .mockReturnValueOnce(latest.promise);
    render(<CopyHarness material="same" copyMaterial={copyMaterial} />);

    fireEvent.click(screen.getByRole("button", { name: "copy" }));
    fireEvent.click(screen.getByRole("button", { name: "copy" }));
    await act(async () => latest.resolve(false));
    await act(async () => older.resolve(true));

    expect(screen.getByRole("button", { name: "copy" })).toBeTruthy();
  });

  it("retires the feedback timer with its mounted owner", async () => {
    vi.useFakeTimers();
    const copyMaterial = vi
      .fn<(text: string) => Promise<boolean>>()
      .mockResolvedValueOnce(true);
    const view = render(
      <CopyHarness material="same" copyMaterial={copyMaterial} />,
    );

    fireEvent.click(screen.getByRole("button", { name: "copy" }));
    await act(async () => {});
    expect(screen.getByRole("button", { name: "copied" })).toBeTruthy();
    expect(vi.getTimerCount()).toBe(1);

    view.unmount();

    expect(vi.getTimerCount()).toBe(0);
  });
});

describe("inert media and callouts", () => {
  it("names an image notice by its label and describes it with the visible detail", () => {
    render(
      <>
        <ImageNotice label="Inline architecture" detail="Embedded image data" />
        <ImageNotice detail="Image unavailable" />
      </>,
    );
    const labelled = screen.getByRole("img", {
      name: "Inline architecture",
      description: "Embedded image data",
    });
    expect(labelled.textContent).toBe(
      "Inline architecture Embedded image data",
    );
    expect(
      screen.getByRole("img", { name: "Image unavailable" }).textContent,
    ).toBe("Image unavailable");
  });

  it("reads an alert marker only when it stands alone or before a space", () => {
    expect(readCalloutMarker("[!TIP] Same-line guidance.")).toEqual({
      kind: "tip",
      rest: "Same-line guidance.",
    });
    expect(readCalloutMarker("[!NOTE]\nBody")).toEqual({
      kind: "note",
      rest: "Body",
    });
    expect(readCalloutMarker("[!NOTE]book")).toBeNull();
    expect(readCalloutMarker("\\[!NOTE]")).toBeNull();
  });
});

describe("modal form ownership", () => {
  it("resolves return focus after local Escape retires the task", async () => {
    const target = createRef<HTMLButtonElement>();
    const initial = createRef<HTMLInputElement>();
    const resolve = vi.fn(() => target.current);
    function Task() {
      const [open, setOpen] = useState(true);
      return (
        <>
          <button ref={target}>Return target</button>
          <ModalDialog
            open={open}
            onOpenChange={setOpen}
            title="Edit record"
            initialFocus={initial}
            finalFocus={resolve}
          >
            <input ref={initial} aria-label="Record title" />
          </ModalDialog>
        </>
      );
    }
    render(<Task />);
    await screen.findByRole("dialog", { name: "Edit record" });
    await waitFor(() => expect(document.activeElement).toBe(initial.current));
    expect(resolve).not.toHaveBeenCalled();
    fireEvent.keyDown(initial.current!, { key: "Escape" });
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    await waitFor(() => expect(document.activeElement).toBe(target.current));
    expect(resolve).toHaveBeenCalledOnce();
  });

  it("blocks dismissal while pending and leaves composition Escape to the input", async () => {
    const change = vi.fn();
    const content = (dismissible: boolean) => (
      <ModalDialog
        open
        onOpenChange={change}
        title="Save record"
        closeLabel="Close record"
        dismissible={dismissible}
      >
        <input aria-label="Draft title" />
      </ModalDialog>
    );
    const view = render(content(false));
    await screen.findByRole("dialog", { name: "Save record" });
    const field = screen.getByRole("textbox", { name: "Draft title" });
    const close = screen.getByRole<HTMLButtonElement>("button", {
      name: "Close record",
    });
    expect(close.disabled).toBe(true);
    fireEvent.keyDown(field, { key: "Escape" });
    fireEvent.click(close);
    expect(change).not.toHaveBeenCalled();

    view.rerender(content(true));
    expect(close.disabled).toBe(false);
    expect(fireEvent.keyDown(field, { key: "Escape", isComposing: true })).toBe(
      true,
    );
    expect(fireEvent.keyDown(field, { key: "Escape", keyCode: 229 })).toBe(
      true,
    );
    expect(change).not.toHaveBeenCalled();
    fireEvent.keyDown(field, { key: "Escape" });
    expect(change).toHaveBeenCalledExactlyOnceWith(false);
  });

  it("keeps body fields and footer submission in the same native form", async () => {
    const submit = vi.fn((event: FormEvent<HTMLFormElement>) =>
      event.preventDefault(),
    );
    const close = vi.fn();
    render(
      <ModalDialog
        open
        onOpenChange={close}
        title="Configure package"
        closeLabel="Close editor"
        description="Review the captured release."
        onSubmit={submit}
        footer={<Button type="submit">Save package</Button>}
      >
        <TextField aria-label="Package input" defaultValue="captured" />
      </ModalDialog>,
    );
    const dialog = await screen.findByRole("dialog", {
      name: "Configure package",
    });
    const field = screen.getByRole("textbox", {
      name: "Package input",
    }) as HTMLInputElement;
    const save = screen.getByRole("button", {
      name: "Save package",
    }) as HTMLButtonElement;
    expect(field.form).toBe(dialog);
    expect(save.form).toBe(field.form);
    expect(dialog.getAttribute("aria-describedby")).toBe(
      screen.getByText("Review the captured release.").id,
    );
    fireEvent.click(save);
    expect(submit).toHaveBeenCalledOnce();
    fireEvent.click(screen.getByRole("button", { name: "Close editor" }));
    expect(close.mock.calls[0]?.[0]).toBe(false);
    expect(submit).toHaveBeenCalledOnce();
  });

  it("leaves an installed component's form as the only submit owner", async () => {
    const submit = vi.fn((event: FormEvent<HTMLFormElement>) =>
      event.preventDefault(),
    );
    render(
      <ModalDialog open onOpenChange={() => {}} title="Session action">
        <Form aria-label="Session details" onSubmit={submit}>
          <FormSection title="Session details">
            <FormField label="Session title">
              <TextField />
            </FormField>
          </FormSection>
          <FormActions label="Save session title">
            <Button type="submit">Save title</Button>
          </FormActions>
        </Form>
      </ModalDialog>,
    );
    const dialog = await screen.findByRole("dialog", {
      name: "Session action",
    });
    expect(dialog.tagName).toBe("DIV");
    expect(dialog.querySelectorAll("form").length).toBe(1);
    const form = screen.getByRole("form", { name: "Session details" });
    const field = screen.getByRole("textbox", {
      name: "Session title",
    }) as HTMLInputElement;
    const save = screen.getByRole("button", {
      name: "Save title",
    }) as HTMLButtonElement;
    expect(field.form).toBe(form);
    expect(save.form).toBe(form);
    fireEvent.click(save);
    expect(submit).toHaveBeenCalledOnce();
  });
});
describe("shared controls", () => {
  it("labels composite form controls without replacing their option names", () => {
    render(
      <FormGroup label="Transport">
        <Segmented
          value="stdio"
          ariaLabel="Transport options"
          onChange={() => {}}
          options={[
            { value: "stdio", label: "stdio" },
            { value: "http", label: "Streamable HTTP" },
          ]}
        />
      </FormGroup>,
    );
    const group = screen.getByRole("group", { name: "Transport" });
    expect(group.contains(screen.getByRole("tab", { name: "stdio" }))).toBe(
      true,
    );
    expect(
      group.contains(screen.getByRole("tab", { name: "Streamable HTTP" })),
    ).toBe(true);
  });
  it("distinguishes an unselected control from a selected empty-string option", () => {
    const change = vi.fn();
    const view = render(
      <Select
        label="Choose provider"
        value={null}
        onValueChange={change}
        options={[{ value: "", label: "All providers" }]}
      />,
    );
    expect(screen.getByRole("combobox").textContent).toContain(
      "Choose provider",
    );
    view.rerender(
      <Select
        label="Choose provider"
        value=""
        onValueChange={change}
        options={[{ value: "", label: "All providers" }]}
      />,
    );
    expect(screen.getByRole("combobox").textContent).toContain("All providers");
    expect(change).not.toHaveBeenCalled();
  });
  it("preserves visibility utilities on status metadata", () => {
    render(
      <Metadata role="status" className="sr-only">
        Read complete.
      </Metadata>,
    );
    const status = screen.getByRole("status");
    expect(status.classList.contains("sr-only")).toBe(true);
    expect(status.textContent).toBe("Read complete.");
  });
  it("applies the caller's className on structural primitives", () => {
    render(
      <PanelSection title="Danger" className="danger-zone">
        <ListRow className="row-pinned">
          <Stats values={[["Total", 1]]} className="totals-wide" />
        </ListRow>
      </PanelSection>,
    );
    expect(document.querySelector("section.danger-zone")).not.toBeNull();
    expect(document.querySelector("article.row-pinned")).not.toBeNull();
    expect(document.querySelector("dl.totals-wide")).not.toBeNull();
  });
  it("drops nullish facts and tolerates duplicate labels", () => {
    render(
      <Facts
        values={[
          ["Owner", "a"],
          ["Owner", "b"],
          ["Due", null],
          ["Note", undefined],
        ]}
      />,
    );
    expect(
      [...document.querySelectorAll("dt")].map((n) => n.textContent),
    ).toEqual(["Owner", "Owner"]);
  });
  it("derives heading levels from nesting without skips", () => {
    render(
      <Panel label="Reports" header={<PanelHeader title="Reports" />}>
        <RecordGroup>
          <ListRow>
            <RecordHeading title="Row A" />
          </ListRow>
        </RecordGroup>
        <PanelSection title="Archive">
          <RecordHeading title="Row B" />
        </PanelSection>
      </Panel>,
    );
    expect(screen.getByRole("heading", { name: "Reports" }).tagName).toBe("H2");
    expect(screen.getByRole("heading", { name: "Row A" }).tagName).toBe("H3");
    expect(screen.getByRole("heading", { name: "Archive" }).tagName).toBe("H3");
    expect(screen.getByRole("heading", { name: "Row B" }).tagName).toBe("H4");
  });
  it("lets a region reset the base heading level", () => {
    render(
      <HeadingLevel value={1}>
        <Panel label="Settings" header={<PanelHeader title="Providers" />}>
          <RecordGroup>
            <ListRow>
              <RecordHeading title="OpenAI" />
            </ListRow>
          </RecordGroup>
        </Panel>
      </HeadingLevel>,
    );
    expect(screen.getByRole("heading", { name: "Providers" }).tagName).toBe(
      "H1",
    );
    expect(screen.getByRole("heading", { name: "OpenAI" }).tagName).toBe("H2");
  });
  it("projects the field's invalid state to its accessible state", () => {
    const view = render(<TextField invalid aria-label="Endpoint" />);
    expect(
      screen
        .getByRole("textbox", { name: "Endpoint" })
        .getAttribute("aria-invalid"),
    ).toBe("true");
    view.rerender(<TextField invalid={false} aria-label="Endpoint" />);
    expect(
      screen
        .getByRole("textbox", { name: "Endpoint" })
        .getAttribute("aria-invalid"),
    ).toBeNull();
    view.rerender(<TextArea invalid aria-label="Instructions" />);
    expect(
      screen
        .getByRole("textbox", { name: "Instructions" })
        .getAttribute("aria-invalid"),
    ).toBe("true");
  });
  it("keeps pending buttons keyboard focusable and blocks submission", () => {
    const click = vi.fn();
    render(
      <Button pending onClick={click}>
        Save
      </Button>,
    );
    const button = screen.getByRole("button", { name: "Save" });
    fireEvent.click(button);
    expect(click).not.toHaveBeenCalled();
    expect(button.getAttribute("aria-disabled")).toBe("true");
    expect(button.getAttribute("disabled")).toBeNull();
  });
  it("uses one StyleX merge for control overrides", () => {
    render(<Button styles={vocab.negative}>Delete</Button>);
    expect(screen.getByRole("button").className).toContain(
      Object.values(vocab.negative).find((value) => typeof value === "string"),
    );
  });
  it.each([TextField, TextArea, SearchField])(
    "merges field overrides without forwarding them to the DOM",
    (Field) => {
      const { container } = render(
        <Field
          aria-label="Draft"
          value="draft"
          readOnly
          styles={vocab.negative}
        />,
      );
      const ink = Object.values(vocab.negative).find(
        (value) => typeof value === "string",
      );
      expect(container.firstElementChild?.className).toContain(ink);
      expect(container.querySelector("[styles]")).toBeNull();
    },
  );
  it("keeps disclosure content lazy and exposes its expanded state", async () => {
    const displayed = vi.fn();
    function Evidence() {
      displayed();
      return <p>Recorded evidence</p>;
    }
    render(
      <Disclosure title="Inspect observation">
        <Evidence />
      </Disclosure>,
    );
    const trigger = screen.getByRole("button", { name: "Inspect observation" });
    expect(trigger.getAttribute("aria-expanded")).toBe("false");
    expect(displayed).not.toHaveBeenCalled();
    fireEvent.click(trigger);
    await screen.findByText("Recorded evidence");
    expect(trigger.getAttribute("aria-expanded")).toBe("true");
  });
  it("retains an opted-in disclosure reader outside the closed accessible scope", async () => {
    render(
      <Disclosure title="Read reason" defaultOpen keepMounted>
        <ScrollRegion label="Recorded reason">
          Complete recorded requirements
        </ScrollRegion>
      </Disclosure>,
    );
    const reader = screen.getByRole("region", { name: "Recorded reason" });
    const trigger = screen.getByRole("button", { name: "Read reason" });

    fireEvent.click(trigger);
    await waitFor(() => {
      expect(
        screen.queryByRole("region", { name: "Recorded reason" }),
      ).toBeNull();
    });
    expect(reader.isConnected).toBe(true);
    expect(trigger.getAttribute("aria-expanded")).toBe("false");

    fireEvent.click(trigger);
    expect(screen.getByRole("region", { name: "Recorded reason" })).toBe(
      reader,
    );
  });
  it("keeps checkbox changes at the controlled consumer", () => {
    const change = vi.fn();
    render(
      <Checkbox checked={false} onCheckedChange={change} label="Pinned only" />,
    );
    fireEvent.click(screen.getByRole("checkbox", { name: "Pinned only" }));
    expect(change).toHaveBeenCalledWith(true, expect.anything());
  });
  it("uses the consumer's font scale with the icon library's optical master", () => {
    const view = render(
      <FontScaleProvider fontSize={18}>
        <TooltipProvider>
          <IconButton icon={Brain} aria-label="Memory" />
        </TooltipProvider>
      </FontScaleProvider>,
    );
    expect(
      screen
        .getByRole("button", { name: "Memory" })
        .querySelector("svg")
        ?.getAttribute("width"),
    ).toBe("20");
    view.rerender(
      <FontScaleProvider fontSize={11}>
        <TooltipProvider>
          <IconButton icon={Brain} aria-label="Memory" />
        </TooltipProvider>
      </FontScaleProvider>,
    );
    expect(
      screen
        .getByRole("button", { name: "Memory" })
        .querySelector("svg")
        ?.getAttribute("width"),
    ).toBe("13");
  });
  it("preserves readonly fields while a write is pending", () => {
    render(
      <TextArea
        pending
        value="Recorded"
        aria-label="Fact"
        onChange={() => {}}
      />,
    );
    expect(
      (screen.getByRole("textbox", { name: "Fact" }) as HTMLTextAreaElement)
        .readOnly,
    ).toBe(true);
  });
  it("clears search through an explicitly named control", () => {
    const clear = vi.fn();
    render(
      <SearchField
        value="fact"
        aria-label="Search"
        onChange={() => {}}
        onClear={clear}
        clearLabel="Clear search"
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "Clear search" }));
    expect(clear).toHaveBeenCalledOnce();
  });
});

describe("hosted panel identity", () => {
  function page(label: string, header: ReactNode) {
    return (
      <Panel label={label} header={header}>
        <input aria-label={`${label} draft`} />
      </Panel>
    );
  }
  function richHeader(title: string) {
    return (
      <PanelHeader
        title={title}
        description="Review the workspace changes."
        context={<a href="#files">All files</a>}
      />
    );
  }
  function presentation(label: string) {
    const region = screen.getByRole("region", { name: label });
    const panel = within(region);
    const link = panel.getByRole("link", { name: "All files" });
    return {
      content: region.firstElementChild!.className,
      description: panel.getByText("Review the workspace changes.").className,
      context: link.parentElement!.className,
    };
  }

  it("keeps an independent page's display heading visible", () => {
    render(page("Changes", <PanelHeader title="Changes" />));
    const heading = screen.getByRole("heading", { name: "Changes" });
    expect(heading.classList.contains("sr-only")).toBe(false);
    expect(heading.closest(".sr-only")).toBeNull();
  });

  it.each(["compact", "page"] as const)(
    "keeps the %s hosted heading accessible without repeating its display title",
    (density) => {
      render(
        <PanelHost density={density}>
          {page("Title only", <PanelHeader title="Title only" />)}
          {page("Rich", richHeader("Rich"))}
        </PanelHost>,
      );
      const titleOnly = screen.getByRole("heading", { name: "Title only" });
      expect(titleOnly.closest("header")!.classList.contains("sr-only")).toBe(
        true,
      );

      const rich = within(screen.getByRole("region", { name: "Rich" }));
      const heading = rich.getByRole("heading", { name: "Rich" });
      expect(heading.classList.contains("sr-only")).toBe(true);
      expect(heading.closest("header")!.classList.contains("sr-only")).toBe(
        false,
      );
      expect(
        rich.getByText("Review the workspace changes.").closest(".sr-only"),
      ).toBeNull();
      expect(
        rich.getByRole("link", { name: "All files" }).closest(".sr-only"),
      ).toBeNull();
    },
  );

  it("keeps page spacing and supporting text without a hidden title's context gap", () => {
    render(
      <>
        {page("Independent", richHeader("Independent"))}
        <PanelHost density="page">
          {page("Page host", richHeader("Page host"))}
        </PanelHost>
        <PanelHost>
          {page("Compact host", richHeader("Compact host"))}
        </PanelHost>
      </>,
    );
    const independent = presentation("Independent");
    const compact = presentation("Compact host");
    const paged = presentation("Page host");
    expect(paged.content).toBe(independent.content);
    expect(paged.description).toBe(independent.description);
    expect(paged.context).toBe(compact.context);
    expect(compact.content).not.toBe(independent.content);
    expect(compact.description).not.toBe(independent.description);
    expect(compact.context).not.toBe(independent.context);
  });

  it("changes hosted density without remounting content or losing a draft", () => {
    const hosted = (density: "compact" | "page") => (
      <PanelHost density={density}>
        {page("Changes", richHeader("Changes"))}
      </PanelHost>
    );
    const view = render(hosted("compact"));
    const field = screen.getByRole<HTMLInputElement>("textbox", {
      name: "Changes draft",
    });
    const heading = screen.getByRole("heading", { name: "Changes" });
    fireEvent.change(field, { target: { value: "unsaved draft" } });
    field.focus();
    const compact = presentation("Changes");

    view.rerender(hosted("page"));
    expect(screen.getByRole("textbox", { name: "Changes draft" })).toBe(field);
    expect(screen.getByRole("heading", { name: "Changes" })).toBe(heading);
    expect(field.value).toBe("unsaved draft");
    expect(document.activeElement).toBe(field);
    expect(heading.classList.contains("sr-only")).toBe(true);
    const paged = presentation("Changes");
    expect(paged.content).not.toBe(compact.content);
    expect(paged.description).not.toBe(compact.description);

    view.rerender(hosted("compact"));
    expect(screen.getByRole("textbox", { name: "Changes draft" })).toBe(field);
    expect(field.value).toBe("unsaved draft");
    expect(document.activeElement).toBe(field);
    expect(presentation("Changes")).toEqual(compact);
  });
});

describe("search field mutation contract", () => {
  it("returns focus to its controlled input when clearing removes the action", () => {
    const ref = createRef<HTMLInputElement>();
    const clear = vi.fn();
    function Search() {
      const [query, setQuery] = useState("found");
      return (
        <SearchField
          ref={ref}
          value={query}
          onValueChange={setQuery}
          aria-label="Search"
          onClear={() => {
            clear();
            setQuery("");
          }}
          clearLabel="Clear search"
        />
      );
    }
    const view = render(<Search />);
    const input = screen.getByRole("searchbox", { name: "Search" });
    expect(ref.current).toBe(input);
    const button = screen.getByRole("button", { name: "Clear search" });
    button.focus();
    fireEvent.click(button);
    expect(clear).toHaveBeenCalledOnce();
    expect(ref.current?.value).toBe("");
    expect(document.activeElement).toBe(input);
    expect(screen.queryByRole("button", { name: "Clear search" })).toBeNull();
    view.unmount();
    expect(ref.current).toBeNull();
  });

  it("preserves callback ref cleanup for its input", () => {
    const release = vi.fn();
    const ref = vi.fn(() => release);
    const view = render(
      <SearchField ref={ref} value="" aria-label="Search" readOnly />,
    );
    expect(ref).toHaveBeenCalledWith(
      screen.getByRole("searchbox", { name: "Search" }),
    );
    view.rerender(
      <SearchField ref={ref} value="found" aria-label="Search" readOnly />,
    );
    expect(ref).toHaveBeenCalledOnce();
    expect(release).not.toHaveBeenCalled();
    view.unmount();
    expect(release).toHaveBeenCalledOnce();
  });

  it.each(["disabled", "readOnly", "pending"] as const)(
    "blocks clearing while %s and allows it after recovery",
    (state) => {
      const clear = vi.fn();
      const props = {
        value: "fixed",
        "aria-label": "Search",
        onClear: clear,
        clearLabel: "Clear search",
        onChange: () => {},
      };
      const view = render(<SearchField {...props} {...{ [state]: true }} />);
      const button = screen.getByRole("button", { name: "Clear search" });
      fireEvent.click(button);
      fireEvent.keyDown(button, { key: "Enter" });
      expect(clear).not.toHaveBeenCalled();
      view.rerender(<SearchField {...props} />);
      fireEvent.click(screen.getByRole("button", { name: "Clear search" }));
      expect(clear).toHaveBeenCalledOnce();
    },
  );

  it("derives the clear action from the controlled value", () => {
    const props = {
      "aria-label": "Search",
      onClear: () => {},
      clearLabel: "Clear search",
      onChange: () => {},
    };
    const view = render(<SearchField {...props} value="" />);
    expect(screen.queryByRole("button", { name: "Clear search" })).toBeNull();
    view.rerender(<SearchField {...props} value="found" />);
    expect(screen.getByRole("button", { name: "Clear search" })).toBeTruthy();
    view.rerender(<SearchField {...props} value="" />);
    expect(screen.queryByRole("button", { name: "Clear search" })).toBeNull();
  });
});

describe("search field keyboard contract", () => {
  const host = vi.fn();
  beforeEach(() => {
    host.mockClear();
    document.addEventListener("keydown", host);
  });
  afterEach(() => document.removeEventListener("keydown", host));

  function Search({
    onClear,
    onKeyDown,
  }: {
    onClear?: () => void;
    onKeyDown?: SearchFieldProps["onKeyDown"];
  }) {
    const [query, setQuery] = useState("found");
    return (
      <SearchField
        value={query}
        onValueChange={setQuery}
        onKeyDown={onKeyDown}
        onClear={() => {
          onClear?.();
          setQuery("");
        }}
        clearLabel="Clear search"
        aria-label="Search"
      />
    );
  }
  const field = () =>
    screen.getByRole<HTMLInputElement>("searchbox", { name: "Search" });

  it("treats the query as literal text unless the caller opts in", () => {
    const view = render(<SearchField value="" aria-label="Search" readOnly />);
    expect(field().getAttribute("autocapitalize")).toBe("none");
    expect(field().getAttribute("autocorrect")).toBe("off");
    expect(field().getAttribute("spellcheck")).toBe("false");
    view.rerender(
      <SearchField value="" aria-label="Search" readOnly spellCheck />,
    );
    expect(field().getAttribute("spellcheck")).toBe("true");
  });

  it("clears a query in place and lets the next Escape reach its host", () => {
    const clear = vi.fn();
    render(<Search onClear={clear} />);
    const input = field();
    input.focus();
    expect(fireEvent.keyDown(input, { key: "Escape" })).toBe(false);
    expect(clear).toHaveBeenCalledOnce();
    expect(input.value).toBe("");
    expect(document.activeElement).toBe(input);
    expect(host).not.toHaveBeenCalled();
    expect(fireEvent.keyDown(input, { key: "Escape" })).toBe(true);
    expect(clear).toHaveBeenCalledOnce();
    expect(host).toHaveBeenCalledOnce();
  });

  it("restores the input when Escape clears from the clear control", () => {
    const clear = vi.fn();
    render(<Search onClear={clear} />);
    const button = screen.getByRole("button", { name: "Clear search" });
    button.focus();
    fireEvent.keyDown(button, { key: "Escape" });
    expect(clear).toHaveBeenCalledOnce();
    expect(field().value).toBe("");
    expect(document.activeElement).toBe(field());
    expect(screen.queryByRole("button", { name: "Clear search" })).toBeNull();
    expect(host).not.toHaveBeenCalled();
  });

  it("leaves an Escape the caller handled to the caller", () => {
    const clear = vi.fn();
    const keyDown = vi.fn<NonNullable<SearchFieldProps["onKeyDown"]>>((event) =>
      event.preventDefault(),
    );
    render(<Search onClear={clear} onKeyDown={keyDown} />);
    fireEvent.keyDown(field(), { key: "Escape" });
    expect(keyDown).toHaveBeenCalledOnce();
    expect(clear).not.toHaveBeenCalled();
    expect(field().value).toBe("found");
  });

  it.each([{ isComposing: true }, { keyCode: 229 }])(
    "leaves IME cancellation to the composition %o",
    (composition) => {
      const clear = vi.fn();
      render(<Search onClear={clear} />);
      expect(
        fireEvent.keyDown(field(), { key: "Escape", ...composition }),
      ).toBe(true);
      expect(clear).not.toHaveBeenCalled();
      expect(field().value).toBe("found");
    },
  );

  it.each(["disabled", "readOnly", "pending"] as const)(
    "does not consume Escape while %s",
    (state) => {
      const clear = vi.fn();
      render(
        <SearchField
          value="fixed"
          aria-label="Search"
          onChange={() => {}}
          onClear={clear}
          clearLabel="Clear search"
          {...{ [state]: true }}
        />,
      );
      for (const target of [
        field(),
        screen.getByRole("button", { name: "Clear search" }),
      ])
        expect(fireEvent.keyDown(target, { key: "Escape" })).toBe(true);
      expect(clear).not.toHaveBeenCalled();
      expect(host).toHaveBeenCalledTimes(2);
    },
  );

  it("keeps a native-only query's Escape local without replacing the browser clear", () => {
    const view = render(
      <SearchField value="found" aria-label="Search" onChange={() => {}} />,
    );
    expect(fireEvent.keyDown(field(), { key: "Escape" })).toBe(true);
    expect(host).not.toHaveBeenCalled();
    view.rerender(
      <SearchField value="" aria-label="Search" onChange={() => {}} />,
    );
    fireEvent.keyDown(field(), { key: "Escape" });
    expect(host).toHaveBeenCalledOnce();
  });
});

describe("diff semantics", () => {
  const labels = {
    region: "文件差异，可横向滚动",
    beforeLine: "原行号",
    beforeCode: "原代码",
    afterLine: "新行号",
    afterCode: "新代码",
    changedCode: "变更代码",
  };
  it.each(["unified", "split"] as const)(
    "takes all %s accessibility labels from its consumer",
    (layout) => {
      render(
        <DiffTable
          rows={[{ type: "added", code: "false", rightLine: 1 }]}
          layout={layout}
          label="示例.ts"
          labels={labels}
        />,
      );
      expect(screen.getByRole("region", { name: labels.region })).toBeTruthy();
      const headings = screen
        .getAllByRole("columnheader")
        .map((element) => element.textContent);
      expect(headings).toEqual(
        layout === "split"
          ? [
              labels.beforeLine,
              labels.beforeCode,
              labels.afterLine,
              labels.afterCode,
            ]
          : [labels.beforeLine, labels.afterLine, labels.changedCode],
      );
      expect(screen.getByText("false")).toBeTruthy();
      expect(
        screen.getByText("+", { exact: true }).getAttribute("aria-hidden"),
      ).toBe("true");
    },
  );
});

describe("selection motion projection", () => {
  it("reads current theme timing and scale without keeping another preference", async () => {
    const { selectionTransition } = await import("../dist/index.js");
    const root = document.documentElement;
    const previous = root.style.cssText;
    try {
      root.style.setProperty("--dur-slow-base", "300ms");
      root.style.setProperty("--motion-scale", "1");
      expect(selectionTransition.type).toBe("spring");
      expect(selectionTransition.bounce).toBe(0);
      expect(selectionTransition.duration).toBe(0.3);
      root.style.setProperty("--motion-scale", "2");
      expect(selectionTransition.duration).toBe(0.6);
      root.style.setProperty("--dur-slow-base", "450ms");
      expect(selectionTransition.duration).toBe(0.9);
      root.style.setProperty("--motion-scale", "0");
      expect(selectionTransition.duration).toBe(0);
    } finally {
      root.style.cssText = previous;
    }
  });
});
