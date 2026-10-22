import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { type CapturedToolResult, type ComponentContext } from "@flame/component-sdk";
import { RecordedResult } from "../../../plugins/tool-results/component/component";
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  document.documentElement.style.removeProperty("--leading-relaxed");
});

function capturedContext(
  name: string,
  result: string,
  range?: CapturedToolResult["range"],
): ComponentContext {
  return Object.freeze({
    component: Object.freeze({
      installationId: "captured-tool-results",
      digest: "a".repeat(64),
      componentId: "recorded-result",
    }),
    locale: "en",
    signal: new AbortController().signal,
    toolResult: Object.freeze({
      id: "item_recorded_result",
      runId: "run_recorded_result",
      name,
      fn: name,
      args: "{}",
      status: "ok" as const,
      result,
      range: range && Object.freeze(range),
    }),
    get runtime(): never {
      throw new Error("Captured results must not request live data");
    },
    get telemetry(): never {
      throw new Error("Captured results must not request telemetry");
    },
    track(): never {
      throw new Error("Captured results must not start I/O");
    },
    compose(): never {
      throw new Error("Captured results must not compose another component");
    },
  });
}

function expectInert(result: HTMLElement) {
  expect(result.querySelector("a, img, script, iframe, [onclick]")).toBeNull();
  expect(result.getElementsByTagName("available_skills")).toHaveLength(0);
  expect(result.getElementsByTagName("skill")).toHaveLength(0);
}

it("keeps web titles, URLs and excerpts distinct, complete and inert while filtering", () => {
  const title = 'Review <img src=x onerror="alert(1)">';
  const snippet = "Recorded excerpt <script>alert(1)</script> & literal <b>markup</b>.";
  const results = [
    { title, url: "https://example.test/review", snippet },
    { url: "https://example.test/untitled", snippet: "No recorded title." },
    { title: "   ", url: "https://example.test/blank", snippet: "Blank recorded title." },
    {
      title: "https://example.test/same",
      url: "https://example.test/same",
      snippet: "The title is already the URL.",
    },
  ];
  render(<RecordedResult context={capturedContext("web_search", JSON.stringify({ results }))} />);
  const result = screen.getByRole("region", { name: "Result" });
  expect(within(result).getByText(title, { exact: true })).toBeTruthy();
  expect(within(result).getByText(snippet, { exact: true }).tagName).toBe("P");
  for (const entry of results) {
    expect(result.textContent?.split(entry.url)).toHaveLength(2);
  }
  expectInert(result);

  fireEvent.click(screen.getByRole("button", { name: "Find" }));
  fireEvent.change(screen.getByRole("searchbox", { name: "Filter result" }), {
    target: { value: "Recorded excerpt" },
  });
  expect(within(result).getByText(title, { exact: true })).toBeTruthy();
  expect(within(result).getByText(results[0]!.url, { exact: true })).toBeTruthy();
  expect(within(result).getByText(snippet, { exact: true }).textContent).toBe(snippet);
  expect(result.textContent).not.toContain("No recorded title.");
  expectInert(result);
});

it("distinguishes an absent excerpt from a recorded empty paragraph", () => {
  const results = [
    { title: "Without a body", url: "https://example.test/absent" },
    { title: "Recorded blank", url: "https://example.test/empty", snippet: "" },
  ];
  render(<RecordedResult context={capturedContext("web_search", JSON.stringify({ results }))} />);
  const result = screen.getByRole("region", { name: "Result" });
  expect(within(result).getByText("Without a body", { exact: true })).toBeTruthy();
  expect(within(result).getByText("Recorded blank", { exact: true })).toBeTruthy();
  expect(Array.from(result.querySelectorAll("p"), (paragraph) => paragraph.textContent)).toEqual([
    " ",
  ]);
});

it("preserves complete literal JSON prose despite keys that resemble other result shapes", () => {
  const source = JSON.stringify(
    {
      hits: [{ path: "keep/hits.ts", snippet: "Keep this nested value." }],
      results: [{ title: "Keep the nested title", url: "https://example.test/nested" }],
      output: "Keep output inside the original JSON.",
      changes: [{ path: "keep/changes.ts", status: "modified" }],
      answer:
        '<available_skills><skill><name>literal</name></skill></available_skills> <img src=x onerror="alert(1)">',
    },
    null,
    2,
  );
  for (const name of ["ask_user", "load_skill", "search_memory"]) {
    const rendered = render(<RecordedResult context={capturedContext(name, source)} />);
    const result = screen.getByRole("region", { name: "Result" });
    expect(Array.from(result.querySelectorAll("p"), (paragraph) => paragraph.textContent)).toEqual([
      source,
    ]);
    expectInert(result);
    rendered.unmount();
  }
});

it("keeps known raw results as exact numbered source lines even when JSON contains changes", () => {
  const source = [
    "{",
    '  "changes": [{ "path": "read-me.ts", "status": "modified" }],',
    "",
    '  "output": "<script>keep this literal</script>"',
    "}",
  ].join("\n");
  for (const name of [
    "read",
    "read_skill_resource",
    "read_tool_result",
    "search_tools",
    "lsp",
    "stop_shell",
    "propose_skill",
    "delegate_task",
  ]) {
    const rendered = render(
      <RecordedResult
        context={capturedContext(
          name,
          source,
          name === "read" ? { start: 21, end: 25 } : undefined,
        )}
      />,
    );
    const result = screen.getByRole("region", { name: "Result" });
    expect(Array.from(result.querySelectorAll("pre"), (line) => line.textContent)).toEqual(
      source.split("\n").map((line) => line || " "),
    );
    const start = name === "read" ? 21 : 1;
    expect(
      within(result)
        .getAllByText("Line", { exact: true })
        .map((label) => label.parentElement?.textContent),
    ).toEqual(source.split("\n").map((_, index) => `Line ${start + index}`));
    expectInert(result);
    rendered.unmount();
  }
});

it("finds visible literal control sequences in captured instructions", () => {
  const source = "Keep \u001b[31m as literal instruction text.";
  render(<RecordedResult context={capturedContext("load_skill", source)} />);
  fireEvent.click(screen.getByRole("button", { name: "Find" }));
  fireEvent.change(screen.getByRole("searchbox", { name: "Filter result" }), {
    target: { value: "[31m" },
  });
  const result = screen.getByRole("region", { name: "Result" });
  expect(result.querySelector("p")?.textContent).toBe(source);
});

it.each(["searchbox", "clear"] as const)(
  "closes bounded Find over a query on Escape from its %s and leaves IME cancellation alone",
  (control) => {
    render(<RecordedResult context={capturedContext("read", "alpha\nbeta")} />);
    const result = screen.getByRole("region", { name: "Result" });
    fireEvent.click(screen.getByRole("button", { name: "Find" }));
    const field = screen.getByRole<HTMLInputElement>("searchbox", { name: "Filter result" });
    fireEvent.change(field, { target: { value: "alpha" } });
    expect(result.textContent).not.toContain("beta");
    const target =
      control === "searchbox" ? field : screen.getByRole("button", { name: "Clear search" });
    fireEvent.keyDown(target, { key: "Escape", keyCode: 229 });
    expect(field.value).toBe("alpha");
    fireEvent.keyDown(target, { key: "Escape" });
    expect(screen.queryByRole("searchbox", { name: "Filter result" })).toBeNull();
    expect(document.activeElement).toBe(result);
    expect(result.textContent).toContain("beta");
  },
);

it("cycles output matches with Enter and leaves an IME Enter to the composition", () => {
  document.documentElement.style.setProperty("--leading-relaxed", "20px");
  vi.spyOn(globalThis, "requestAnimationFrame").mockReturnValue(0);
  const output = Array.from({ length: 12 }, (_, index) => `line ${index}`).join("\n");
  render(<RecordedResult context={capturedContext("shell", output)} />);
  fireEvent.click(screen.getByRole("button", { name: "Show 3 earlier lines · 12 total" }));
  fireEvent.click(screen.getByRole("button", { name: "Find" }));
  const field = screen.getByRole("searchbox", { name: "Find in output" });
  fireEvent.change(field, { target: { value: "line" } });
  expect(screen.getByText("1 of 12")).toBeTruthy();
  fireEvent.keyDown(field, { key: "Enter", isComposing: true });
  fireEvent.keyDown(field, { key: "Enter", keyCode: 229 });
  expect(screen.getByText("1 of 12")).toBeTruthy();
  fireEvent.keyDown(field, { key: "Enter" });
  expect(screen.getByText("2 of 12")).toBeTruthy();
  fireEvent.keyDown(field, { key: "Enter", shiftKey: true });
  expect(screen.getByText("1 of 12")).toBeTruthy();
  fireEvent.keyDown(field, { key: "Escape" });
  expect(screen.queryByRole("search", { name: "Output search" })).toBeNull();
  expect(document.activeElement).toBe(screen.getByRole("region", { name: "Tool output" }));
});

it("decodes skill summary entities once, keeps descriptions inert and reports truncation", () => {
  const source = [
    "<available_skills>",
    "  <skill>",
    "    <name>review &amp; &lt;trace&gt;</name>",
    "    <description>Keep &amp;amp; and &lt;img src=x onerror=alert(1)&gt; &gt; intact.</description>",
    "  </skill>",
    "  <truncated>true</truncated>",
    "</available_skills>",
  ].join("\n");
  render(<RecordedResult context={capturedContext("list_skills", source)} />);
  const result = screen.getByRole("region", { name: "Result" });
  expect(within(result).getByText("review & <trace>", { exact: true })).toBeTruthy();
  const description = "Keep &amp; and <img src=x onerror=alert(1)> > intact.";
  expect(within(result).getByText(description, { exact: true }).tagName).toBe("P");
  expect(
    within(result).getByText("More skills were omitted from this result.", { exact: true }),
  ).toBeTruthy();
  expect(result.textContent).not.toContain("<available_skills>");
  expectInert(result);
});

it("distinguishes an empty skill list from a result where no summary fit", () => {
  const results = [
    [
      "<available_skills>\n  <truncated>true</truncated>\n</available_skills>",
      "More skills were omitted from this result.",
    ],
    ["<available_skills>\n</available_skills>", "No skills were returned."],
  ] as const;
  for (const [source, notice] of results) {
    const rendered = render(<RecordedResult context={capturedContext("list_skills", source)} />);
    const result = screen.getByRole("region", { name: "Result" });
    expect(within(result).getByText(notice, { exact: true })).toBeTruthy();
    expect(result.textContent).not.toContain("<available_skills>");
    expect(result.textContent).not.toContain("<truncated>");
    expectInert(result);
    rendered.unmount();
  }
});

it.each([
  { name: "glob", result: JSON.stringify({ hits: [] }), label: "Matches" },
  { name: "web_search", result: JSON.stringify({ results: [] }), label: "Results" },
])(
  "states that $name returned nothing before, during and after Find",
  async ({ name, result, label }) => {
    const writeText = vi.fn(async () => {});
    vi.stubGlobal("navigator", { clipboard: { writeText } });
    try {
      render(<RecordedResult context={capturedContext(name, result)} />);
      const reader = screen.getByRole("region", { name: "Result" });
      const expectReturnedNothing = () => {
        expect(reader.textContent).toBe("No results were returned.");
        expect(reader.textContent).not.toContain("Close find");
      };
      expect(
        screen.getByText(label, { exact: true }).closest("[data-slot='code-caption']"),
      ).toBeTruthy();
      expectReturnedNothing();

      fireEvent.click(screen.getByRole("button", { name: "Find" }));
      const field = screen.getByRole("searchbox", { name: "Filter result" });
      expectReturnedNothing();
      fireEvent.change(field, { target: { value: "anything" } });
      expectReturnedNothing();
      fireEvent.click(screen.getByRole("button", { name: "Clear search" }));
      expect(document.activeElement).toBe(field);
      expectReturnedNothing();
      fireEvent.change(field, { target: { value: "anything" } });
      fireEvent.click(screen.getByRole("button", { name: "Close find" }));
      expect(screen.queryByRole("searchbox", { name: "Filter result" })).toBeNull();
      expect(document.activeElement).toBe(reader);
      expectReturnedNothing();
      fireEvent.click(screen.getByRole("button", { name: "Copy result" }));
      await vi.waitFor(() => expect(writeText).toHaveBeenCalledWith(result));
    } finally {
      vi.unstubAllGlobals();
    }
  },
);

it("recovers every recorded hit after a filter that matched none is cleared or closed", () => {
  const hits = [
    { path: "src/a.go", lineNumber: 3, snippet: "alpha" },
    { path: "src/b.go", lineNumber: 9, snippet: "beta" },
  ];
  render(<RecordedResult context={capturedContext("grep", JSON.stringify({ hits }))} />);
  const reader = screen.getByRole("region", { name: "Result" });
  const expectAllHits = () => {
    expect(within(reader).getByText("alpha", { exact: true })).toBeTruthy();
    expect(within(reader).getByText("beta", { exact: true })).toBeTruthy();
    expect(reader.textContent).not.toContain("No matching rows");
    expect(reader.textContent).not.toContain("No results were returned.");
  };
  expectAllHits();

  fireEvent.click(screen.getByRole("button", { name: "Find" }));
  const field = screen.getByRole("searchbox", { name: "Filter result" });
  fireEvent.change(field, { target: { value: "no such hit" } });
  expect(reader.textContent).toBe("No matching rows. Close find to see the whole result.");
  fireEvent.click(screen.getByRole("button", { name: "Clear search" }));
  expectAllHits();

  fireEvent.change(field, { target: { value: "no such hit" } });
  expect(reader.textContent).toBe("No matching rows. Close find to see the whole result.");
  fireEvent.click(screen.getByRole("button", { name: "Close find" }));
  expectAllHits();
});

it("keeps a recorded blank source line as a numbered row rather than an empty result", () => {
  for (const name of ["read", "search_tools"]) {
    const rendered = render(<RecordedResult context={capturedContext(name, "")} />);
    const reader = screen.getByRole("region", { name: "Result" });
    expect(Array.from(reader.querySelectorAll("pre"), (line) => line.textContent)).toEqual([" "]);
    expect(within(reader).getByText("Line", { exact: true }).parentElement?.textContent).toBe(
      "Line 1",
    );
    expect(reader.textContent).not.toContain("No results were returned.");
    rendered.unmount();
  }
});

it("states an empty applied-change receipt instead of an empty table", () => {
  const empty = render(
    <RecordedResult context={capturedContext("apply_patch", JSON.stringify({ changes: [] }))} />,
  );
  expect(screen.getByText("No changes were recorded.", { exact: true })).toBeTruthy();
  expect(screen.queryByRole("table")).toBeNull();
  expect(screen.queryAllByRole("button")).toHaveLength(0);
  empty.unmount();

  const changes = [
    { path: "src/new.ts", status: "added" },
    { path: "src/moved.ts", status: "moved", from: "src/old.ts" },
  ];
  render(<RecordedResult context={capturedContext("apply_patch", JSON.stringify({ changes }))} />);
  const table = screen.getByRole("table", { name: "Applied changes" });
  expect(within(table).getAllByRole("row")).toHaveLength(2);
  expect(within(table).getByText("Created", { exact: true })).toBeTruthy();
  expect(within(table).getByText("src/new.ts", { exact: true })).toBeTruthy();
  expect(within(table).getByText("From src/old.ts", { exact: true })).toBeTruthy();
  expect(screen.queryByText("No changes were recorded.")).toBeNull();
});

it("preserves the complete literal result for malformed or unknown skills XML", () => {
  const sources = [
    "<available_skills>\n<skill><name>Incomplete</name><description>Keep every byte.</skill>\n</available_skills>",
    "<available_skills>\n<skill><name>Known</name><description>Keep this too.</description><unknown>Never discard me.</unknown></skill>\n</available_skills>",
  ];
  for (const source of sources) {
    const rendered = render(<RecordedResult context={capturedContext("list_skills", source)} />);
    const result = screen.getByRole("region", { name: "Result" });
    expect(Array.from(result.querySelectorAll("pre"), (line) => line.textContent).join("\n")).toBe(
      source,
    );
    expectInert(result);
    rendered.unmount();
  }
});

it("leads each recognised result with its caption actions and copies the recorded text", async () => {
  const writeText = vi.fn(async () => {});
  vi.stubGlobal("navigator", { clipboard: { writeText } });
  try {
    const lines = Array.from({ length: 12 }, (_, i) => `line ${i}`).join("\n");
    const output = `\u001b[32mok\u001b[0m\n${lines}`;
    const shell = render(<RecordedResult context={capturedContext("shell", output)} />);
    const preview = screen.getByRole("region", { name: "Tool output preview" });
    const buttons = screen.getAllByRole("button");
    expect(
      buttons.map((button) => button.getAttribute("aria-label") ?? button.textContent),
    ).toEqual(["Show 4 earlier lines · 13 total", "Copy output"]);
    for (const button of buttons)
      expect(
        button.compareDocumentPosition(preview) & Node.DOCUMENT_POSITION_FOLLOWING,
      ).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Copy output" }));
    await vi.waitFor(() => expect(writeText).toHaveBeenCalledWith(`ok\n${lines}`));
    shell.unmount();

    const page = JSON.stringify({ format: "markdown", content: "# Title\n\nBody" });
    render(<RecordedResult context={capturedContext("web_fetch", page)} />);
    const result = screen.getByRole("region", { name: "Result" });
    expect(
      screen.getByText("markdown", { exact: true }).closest("[data-slot='code-caption']"),
    ).toBeTruthy();
    expect(result.textContent).not.toContain("markdown");
    const find = screen.getByRole("button", { name: "Find" });
    expect(find.compareDocumentPosition(result) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Copy result" }));
    await vi.waitFor(() => expect(writeText).toHaveBeenLastCalledWith(page));
  } finally {
    vi.unstubAllGlobals();
  }
});

it("keeps one output toggle and returns keyboard focus to it on collapse", () => {
  document.documentElement.style.setProperty("--leading-relaxed", "20px");
  const frames: FrameRequestCallback[] = [];
  vi.spyOn(globalThis, "requestAnimationFrame").mockImplementation((callback) => {
    frames.push(callback);
    return frames.length;
  });
  const output = Array.from({ length: 12 }, (_, index) => `line ${index}`).join("\n");
  render(<RecordedResult context={capturedContext("shell", output)} />);
  const toggle = screen.getByRole("button", { name: "Show 3 earlier lines · 12 total" });
  const preview = screen.getByRole("region", { name: "Tool output preview" });
  preview.focus();
  fireEvent.click(toggle);
  act(() => frames.splice(0).forEach((callback) => callback(0)));
  const reader = screen.getByRole("region", { name: "Tool output" });
  expect(document.activeElement).toBe(reader);
  const collapse = screen.getByRole("button", { name: "Collapse" });
  expect(collapse).toBe(toggle);
  expect(collapse.getAttribute("aria-expanded")).toBe("true");
  collapse.focus();
  fireEvent.click(collapse);
  expect(document.activeElement).toBe(toggle);
  expect(toggle.getAttribute("aria-expanded")).toBe("false");
  expect(screen.queryByRole("region", { name: "Tool output" })).toBeNull();
});

it("does not reclaim output focus after a later user choice", () => {
  document.documentElement.style.setProperty("--leading-relaxed", "20px");
  const frames: FrameRequestCallback[] = [];
  vi.spyOn(globalThis, "requestAnimationFrame").mockImplementation((callback) => {
    frames.push(callback);
    return frames.length;
  });
  const output = Array.from({ length: 12 }, (_, index) => `line ${index}`).join("\n");
  render(
    <>
      <RecordedResult context={capturedContext("shell", output)} />
      <button>Continue working</button>
    </>,
  );
  const toggle = screen.getByRole("button", { name: "Show 3 earlier lines · 12 total" });
  toggle.focus();
  fireEvent.click(toggle);
  const destination = screen.getByRole("button", { name: "Continue working" });
  destination.focus();
  act(() => frames.splice(0).forEach((callback) => callback(0)));
  expect(document.activeElement).toBe(destination);
});

it("continues paged result reading at the first newly revealed row through the final page", () => {
  const source = Array.from({ length: 82 }, (_, index) => `captured ${index}`).join("\n");
  render(<RecordedResult context={capturedContext("read", source)} />);
  const reader = screen.getByRole("region", { name: "Result" });
  const more = screen.getByRole("button", { name: "Show 40 more matching rows" });
  more.focus();
  fireEvent.click(more);
  expect(document.activeElement).toBe(reader);
  expect(within(reader).getByText("captured 40")).toBeTruthy();
  expect(screen.getByText("80 of 82 rows")).toBeTruthy();
  const last = screen.getByRole("button", { name: "Show 2 more matching rows" });
  last.focus();
  fireEvent.click(last);
  expect(document.activeElement).toBe(reader);
  expect(within(reader).getByText("captured 80")).toBeTruthy();
  expect(screen.getByText("82 of 82 rows")).toBeTruthy();
  expect(screen.queryByRole("button", { name: /more matching rows/ })).toBeNull();
});

function selectAll(reader: HTMLElement, modifier: "metaKey" | "ctrlKey" = "metaKey") {
  reader.focus();
  return fireEvent.keyDown(reader, { key: "a", [modifier]: true });
}

function selectedRange() {
  const selection = document.getSelection()!;
  expect(selection.rangeCount).toBe(1);
  return selection.getRangeAt(0);
}

function copyFrom(target: HTMLElement) {
  const setData = vi.fn();
  const proceeded = fireEvent.copy(target, { clipboardData: { setData } });
  return { proceeded, setData };
}

function expandOutput(output: string) {
  document.documentElement.style.setProperty("--leading-relaxed", "20px");
  vi.spyOn(globalThis, "requestAnimationFrame").mockReturnValue(0);
  render(
    <>
      <p>Surrounding narrative</p>
      <RecordedResult context={capturedContext("shell", output)} />
    </>,
  );
  fireEvent.click(screen.getByRole("button", { name: /earlier lines/ }));
  return screen.getByRole("region", { name: "Tool output" });
}

it("scopes Select All to the presented result rows, outside find and caption controls", () => {
  const source = Array.from({ length: 82 }, (_, index) => `captured ${index}`).join("\n");
  render(
    <>
      <p>Surrounding narrative</p>
      <RecordedResult context={capturedContext("read", source)} />
    </>,
  );
  const reader = screen.getByRole("region", { name: "Result" });
  expect(selectAll(reader, "ctrlKey")).toBe(false);
  let range = selectedRange();
  expect(range.startContainer).toBe(reader);
  expect(range.endContainer).toBe(reader);
  expect(range.toString()).toContain("captured 39");
  expect(range.toString()).not.toContain("captured 40");
  expect(range.toString()).not.toContain("Surrounding narrative");
  expect(range.intersectsNode(screen.getByRole("button", { name: "Find" }))).toBe(false);
  expect(document.activeElement).toBe(reader);

  fireEvent.click(screen.getByRole("button", { name: "Find" }));
  const field = screen.getByRole("searchbox", { name: "Filter result" });
  fireEvent.change(field, { target: { value: "captured 7" } });
  expect(selectAll(reader)).toBe(false);
  range = selectedRange();
  expect(range.toString()).toContain("captured 79");
  expect(range.toString()).not.toContain("captured 6");
  expect(range.intersectsNode(field)).toBe(false);
  expect(range.intersectsNode(screen.getByRole("button", { name: "Close find" }))).toBe(false);
  document.getSelection()?.removeAllRanges();
});

it.each([{ altKey: true }, { shiftKey: true }, { isComposing: true }, { keyCode: 229 }])(
  "leaves Select All with %o to the platform",
  (modifier) => {
    render(<RecordedResult context={capturedContext("read", "alpha\nbeta")} />);
    const reader = screen.getByRole("region", { name: "Result" });
    document.getSelection()?.removeAllRanges();
    reader.focus();
    expect(fireEvent.keyDown(reader, { key: "a", metaKey: true, ...modifier })).toBe(true);
    expect(document.getSelection()?.rangeCount).toBe(0);
  },
);

it("selects only the preview excerpt and copies its presented lines", () => {
  const plain = Array.from({ length: 12 }, (_, index) => (index === 6 ? "" : `line ${index}`));
  const output = plain
    .map((line, index) => (index === 8 ? `\u001b[32m${line}\u001b[0m` : line))
    .join("\n");
  render(
    <>
      <p>Surrounding narrative</p>
      <RecordedResult context={capturedContext("shell", output)} />
    </>,
  );
  const preview = screen.getByRole("region", { name: "Tool output preview" });
  const selection = document.getSelection()!;
  selection.selectAllChildren(preview.querySelector('[data-output-line="4"]')!);
  let copied = copyFrom(preview);
  expect(copied.proceeded).toBe(true);
  expect(copied.setData).not.toHaveBeenCalled();

  expect(selectAll(preview)).toBe(false);
  const range = selectedRange();
  expect(range.startContainer).toBe(preview);
  expect(range.toString()).not.toContain("Surrounding narrative");
  expect(
    range.intersectsNode(screen.getByRole("button", { name: "Show 3 earlier lines · 12 total" })),
  ).toBe(false);
  copied = copyFrom(preview);
  expect(copied.proceeded).toBe(false);
  expect(copied.setData).toHaveBeenCalledExactlyOnceWith("text/plain", plain.slice(3).join("\n"));
  selection.removeAllRanges();
});

it("copies the current numbered, filtered and paged result rows without reader framing", () => {
  const plain = Array.from({ length: 82 }, (_, index) =>
    index === 2 ? "" : `Recorded boundary observation ${index + 1} · 中文证据`,
  );
  const source = plain
    .map((line, index) => (index === 4 ? `\u001b[31m${line}\u001b[0m` : line))
    .join("\n");
  render(<RecordedResult context={capturedContext("read", source)} />);
  const reader = screen.getByRole("region", { name: "Result" });
  const copyAll = () => {
    expect(selectAll(reader)).toBe(false);
    const copied = copyFrom(reader);
    expect(copied.proceeded).toBe(false);
    return copied.setData;
  };

  expect(copyAll()).toHaveBeenCalledExactlyOnceWith("text/plain", plain.slice(0, 40).join("\n"));

  fireEvent.click(screen.getByRole("button", { name: "Show 40 more matching rows" }));
  expect(copyAll()).toHaveBeenCalledExactlyOnceWith("text/plain", plain.slice(0, 80).join("\n"));

  fireEvent.click(screen.getByRole("button", { name: "Find" }));
  fireEvent.change(screen.getByRole("searchbox", { name: "Filter result" }), {
    target: { value: "42" },
  });
  expect(copyAll()).toHaveBeenCalledExactlyOnceWith(
    "text/plain",
    "Recorded boundary observation 42 · 中文证据",
  );

  document.getSelection()!.selectAllChildren(screen.getByText(/observation 42/));
  const partial = copyFrom(reader);
  expect(partial.proceeded).toBe(true);
  expect(partial.setData).not.toHaveBeenCalled();
  document.getSelection()?.removeAllRanges();
});

it.each([
  {
    name: "grep",
    result: JSON.stringify({
      hits: [
        { path: "src/a.go", lineNumber: 3, snippet: "\u001b[31mfailed\u001b[0m here" },
        { path: "src/b.go" },
      ],
    }),
    copied: "src/a.go:3\nfailed here\nsrc/b.go",
  },
  {
    name: "web_search",
    result: JSON.stringify({
      results: [
        { title: "Review", url: "https://example.test/review", snippet: "Recorded excerpt." },
        { url: "https://example.test/untitled", snippet: "" },
      ],
    }),
    copied:
      "Review\nhttps://example.test/review\nRecorded excerpt.\nhttps://example.test/untitled\n",
  },
  {
    name: "load_skill",
    result: "Keep \u001b[1m literal\n\nin prose",
    copied: "Keep \u001b[1m literal\n\nin prose",
  },
])("copies whole $name result rows as their recorded text", ({ name, result, copied }) => {
  render(<RecordedResult context={capturedContext(name, result)} />);
  const reader = screen.getByRole("region", { name: "Result" });
  expect(selectAll(reader)).toBe(false);
  const { proceeded, setData } = copyFrom(reader);
  expect(proceeded).toBe(false);
  expect(setData).toHaveBeenCalledExactlyOnceWith("text/plain", copied);
  document.getSelection()?.removeAllRanges();
});

it("copies the complete captured output after Select All in the virtualized reader", () => {
  const plain = Array.from({ length: 50_000 }, (_, index) => `line ${index}`);
  const lines = plain.map((line, index) =>
    index % 1000 === 0 ? `\u001b[31m${line}\u001b[0m` : line,
  );
  const output = `${lines.join("\n")}\n\n`;
  const reader = expandOutput(output);
  expect(reader.querySelectorAll("[data-output-line]").length).toBeLessThan(100);
  expect(selectAll(reader)).toBe(false);
  expect(selectedRange().toString()).not.toContain("line 25000");
  const { proceeded, setData } = copyFrom(reader);
  expect(proceeded).toBe(false);
  expect(setData).toHaveBeenCalledExactlyOnceWith("text/plain", `${plain.join("\n")}\n\n`);
  document.getSelection()?.removeAllRanges();
});

it("leaves partial, outside, collapsed and handled output copies native", () => {
  const output = Array.from({ length: 200 }, (_, index) => `line ${index}`).join("\n");
  const reader = expandOutput(output);
  const selection = document.getSelection()!;
  const line = reader.querySelector("[data-output-line]")!;

  selection.selectAllChildren(line);
  let copied = copyFrom(reader);
  expect(copied.proceeded).toBe(true);
  expect(copied.setData).not.toHaveBeenCalled();

  selection.selectAllChildren(screen.getByText("Surrounding narrative"));
  copied = copyFrom(reader);
  expect(copied.proceeded).toBe(true);
  expect(copied.setData).not.toHaveBeenCalled();

  selection.setBaseAndExtent(reader, 0, reader, 0);
  copied = copyFrom(reader);
  expect(copied.proceeded).toBe(true);
  expect(copied.setData).not.toHaveBeenCalled();

  selection.setBaseAndExtent(
    screen.getByText("Surrounding narrative"),
    0,
    reader,
    reader.childNodes.length,
  );
  copied = copyFrom(reader);
  expect(copied.proceeded).toBe(true);
  expect(copied.setData).not.toHaveBeenCalled();

  selectAll(reader);
  const handled = (event: Event) => event.preventDefault();
  document.addEventListener("copy", handled, { capture: true });
  try {
    copied = copyFrom(reader);
    expect(copied.setData).not.toHaveBeenCalled();
  } finally {
    document.removeEventListener("copy", handled, { capture: true });
    selection.removeAllRanges();
  }
});

it("copies the complete captured output when Select All precedes a virtual-window scroll", () => {
  const plain = Array.from({ length: 50_000 }, (_, index) => `line ${index}`);
  const source = `${plain.join("\n")}\n`;
  const reader = expandOutput(source);
  Object.defineProperty(reader, "clientHeight", { configurable: true, value: 400 });
  const scrollTo = (top: number) => {
    reader.scrollTop = top;
    fireEvent.scroll(reader);
  };
  scrollTo(49_980 * 20);
  expect(reader.querySelector('[data-output-line="49999"]')).toBeTruthy();
  expect(selectAll(reader)).toBe(false);

  scrollTo(49_900 * 20);
  expect(reader.querySelector('[data-output-line="49999"]')).toBeNull();
  expect(reader.querySelector('[data-output-line="49894"]')).toBeTruthy();
  let copied = copyFrom(reader);
  expect(copied.proceeded).toBe(false);
  expect(copied.setData).toHaveBeenCalledExactlyOnceWith("text/plain", source);

  scrollTo(49_980 * 20);
  copied = copyFrom(reader);
  expect(copied.proceeded).toBe(false);
  expect(copied.setData).toHaveBeenCalledExactlyOnceWith("text/plain", source);

  document.getSelection()!.selectAllChildren(reader.querySelector('[data-output-line="49990"]')!);
  scrollTo(49_970 * 20);
  copied = copyFrom(reader);
  expect(copied.proceeded).toBe(true);
  expect(copied.setData).not.toHaveBeenCalled();
  document.getSelection()?.removeAllRanges();
});

it("copies the presented result rows when Select All precedes a filter or page change", () => {
  const plain = Array.from({ length: 82 }, (_, index) => `captured ${index}`);
  render(<RecordedResult context={capturedContext("read", plain.join("\n"))} />);
  const reader = screen.getByRole("region", { name: "Result" });
  const copyWhole = (expected: string) => {
    const copied = copyFrom(reader);
    expect(copied.proceeded).toBe(false);
    expect(copied.setData).toHaveBeenCalledExactlyOnceWith("text/plain", expected);
  };

  fireEvent.click(screen.getByRole("button", { name: "Find" }));
  const field = screen.getByRole("searchbox", { name: "Filter result" });
  fireEvent.change(field, { target: { value: "captured 42" } });
  expect(selectAll(reader)).toBe(false);
  fireEvent.change(field, { target: { value: "" } });
  copyWhole(plain.slice(0, 40).join("\n"));

  fireEvent.change(field, { target: { value: "no such row" } });
  expect(selectAll(reader)).toBe(false);
  fireEvent.change(field, { target: { value: "captured 4" } });
  copyWhole(["captured 4", ...plain.slice(40, 50)].join("\n"));

  fireEvent.change(field, { target: { value: "" } });
  expect(selectAll(reader)).toBe(false);
  fireEvent.click(screen.getByRole("button", { name: "Show 40 more matching rows" }));
  fireEvent.click(screen.getByRole("button", { name: "Show 2 more matching rows" }));
  copyWhole(plain.join("\n"));

  document.getSelection()!.selectAllChildren(within(reader).getByText("captured 81"));
  fireEvent.change(field, { target: { value: "captured 8" } });
  const partial = copyFrom(reader);
  expect(partial.proceeded).toBe(true);
  expect(partial.setData).not.toHaveBeenCalled();
  document.getSelection()?.removeAllRanges();
});
