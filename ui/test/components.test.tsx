import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { FormEvent } from "react";
import { createRef, useState } from "react";
import { Brain } from "@flame/icons/react";
import {
  Button,
  CodeBlock,
  ImageNotice,
  readCalloutMarker,
  setCodeWrap,
  useCopyFeedback,
  Disclosure,
  DiffTable,
  Checkbox,
  Form,
  FormActions,
  FormField,
  FormGroup,
  FormSection,
  FontScaleProvider,
  IconButton,
  Metadata,
  ModalDialog,
  RecordForm,
  SearchField,
  Segmented,
  Select,
  TextArea,
  TextField,
  TooltipProvider,
} from "../dist/index.js";
import { vocab } from "../dist/vocabulary.js";

afterEach(cleanup);
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
  afterEach(() => setCodeWrap(false));

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
