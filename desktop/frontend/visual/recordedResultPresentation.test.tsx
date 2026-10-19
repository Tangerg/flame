import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { CapturedToolResult, ComponentContext } from "@flame/plugin-sdk";
import { RecordedResult } from "../../../plugins/tool-results/src/component";

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
