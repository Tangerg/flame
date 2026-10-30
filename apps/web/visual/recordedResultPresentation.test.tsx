import { unavailableComponentNotifications } from "@/test/unavailableNotifications";
import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { act, cleanup, fireEvent, screen, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { type CapturedToolResult, type ComponentContext } from "@flame/component-sdk";
import { RecordedResult } from "../../../plugins/tool-results/component/component";
import declaration from "../../../plugins/tool-results/package/io.github.tangerg.flame/declaration.json";
import { selectToolResultRenderer } from "@/features/chat/tools/application/toolResultRenderer";
import { type ToolResultRendererSpec } from "@/features/chat/public/tools/types";
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
    saveFile(): never {
      throw new Error("Captured results must not save files");
    },
    notifications: unavailableComponentNotifications,
    subscribeRootRunSettlements() {
      throw new Error("This fixture must not observe run settlements");
    },
    registerCommand() {
      throw new Error("This fixture must not register client commands");
    },
    compose(): never {
      throw new Error("Captured results must not compose another component");
    },
  });
}

it.each([
  { name: "codeintel_lsp", id: "lsp", result: "src/main.go:3:1" },
  { name: "web_http_request", id: "http-request", result: '{"status":200}' },
])("selects the installed renderer for $name", ({ name, id, result }) => {
  const renderers = declaration.components.map((entry): ToolResultRendererSpec => ({
    id: entry.id,
    title: entry.title,
    selector: entry.selector as ToolResultRendererSpec["selector"],
    component: () => null,
  }));
  const tool = {
    id: "item",
    runId: "run",
    name,
    fn: name,
    args: "{}",
    status: "ok" as const,
    result,
  };
  expect(selectToolResultRenderer(renderers, tool)?.id).toBe(id);
});

it.each([
  { encoding: "base64", data: "AAECAw==", expected: "base64: AAECAw==" },
  {
    encoding: "utf8",
    data: "<img src=x onerror=alert(1)>",
    expected: "<img src=x onerror=alert(1)>",
  },
])(
  "preserves $encoding HTTP response bytes in the installed renderer",
  ({ encoding, data, expected }) => {
    const response = {
      status: 200,
      duration: "12ms",
      headers: {
        "content-type": [{ encoding: "utf8", data: "application/octet-stream" }],
        "x-raw": [{ encoding: "base64", data: "/wA=" }],
      },
      body: { encoding, data },
      truncated: true,
    };
    testClient.render(
      <RecordedResult context={capturedContext("web_http_request", JSON.stringify(response))} />,
    );
    const reader = screen.getByRole("region", { name: "Result" });
    expect(screen.getByText("Response", { exact: true })).toBeTruthy();
    expect(reader.textContent).toContain("200 12ms");
    expect(reader.textContent).toContain("content-type: application/octet-stream");
    expect(reader.textContent).toContain("x-raw: base64: /wA=");
    expect(reader.textContent).toContain(expected);
    expect(reader.textContent).not.toContain("[object Object]");
    expectInert(reader);
  },
);

it.each([
  { body: { encoding: "utf8", data: "body", unrecognized: "keep" } },
  { body: { encoding: "utf8", data: "body" }, unrecognized: "keep" },
  { body: { encoding: "future", data: "keep" } },
  { body: { encoding: "utf8", data: "body" }, headers: { "x-raw": [] } },
])("retains complete source for an unrecognized HTTP response %#", (value) => {
  const source = JSON.stringify(
    {
      status: 200,
      duration: "12ms",
      ...value,
    },
    null,
    2,
  );
  testClient.render(<RecordedResult context={capturedContext("web_http_request", source)} />);
  const reader = screen.getByRole("region", { name: "Result" });
  expect(Array.from(reader.querySelectorAll("pre"), (line) => line.textContent)).toEqual(
    source.split("\n"),
  );
});

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
  testClient.render(
    <RecordedResult context={capturedContext("web_search", JSON.stringify({ results }))} />,
  );
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
  testClient.render(
    <RecordedResult context={capturedContext("web_search", JSON.stringify({ results }))} />,
  );
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
    const rendered = testClient.render(<RecordedResult context={capturedContext(name, source)} />);
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
    "codeintel_lsp",
    "stop_shell",
    "propose_skill",
    "delegate_task",
  ]) {
    const rendered = testClient.render(
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
  testClient.render(<RecordedResult context={capturedContext("load_skill", source)} />);
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
    testClient.render(<RecordedResult context={capturedContext("read", "alpha\nbeta")} />);
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
  testClient.render(<RecordedResult context={capturedContext("shell", output)} />);
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
  testClient.render(<RecordedResult context={capturedContext("list_skills", source)} />);
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

it("presents the current Skill list page as complete summaries with its recorded continuation", () => {
  const page = {
    skills: [
      { name: "review-diff", description: "Keep &amp; and <img src=x onerror=alert(1)> literal." },
      { name: "trace-flaky-test", description: "Find the shared state.\nKeep the second line." },
    ],
    next_cursor: { offset: 2, snapshot: "a".repeat(64) },
  };
  testClient.render(
    <RecordedResult context={capturedContext("list_skills", JSON.stringify(page))} />,
  );
  const result = screen.getByRole("region", { name: "Result" });
  for (const skill of page.skills) {
    expect(within(result).getByText(skill.name, { exact: true })).toBeTruthy();
    expect(
      within(result).getByText(skill.description, { exact: true, normalizer: (value) => value })
        .tagName,
    ).toBe("P");
  }
  expect(result.querySelector("pre")?.textContent).toBe(JSON.stringify(page.next_cursor, null, 2));
  expect(
    screen.getByText("More pages", { exact: true }).closest("[data-slot='code-caption']"),
  ).toBeTruthy();
  expectInert(result);

  fireEvent.click(screen.getByRole("button", { name: "Find" }));
  const field = screen.getByRole("searchbox", { name: "Filter result" });
  fireEvent.change(field, { target: { value: "trace-flaky" } });
  expect(result.textContent).not.toContain("review-diff");
  expect(result.textContent).toContain(page.skills[1]!.description);
  expect(screen.getByText("More pages", { exact: true })).toBeTruthy();
  fireEvent.change(field, { target: { value: page.next_cursor.snapshot } });
  expect(result.querySelector("pre")?.textContent).toBe(JSON.stringify(page.next_cursor, null, 2));
  fireEvent.keyDown(field, { key: "Escape" });
  expect(document.activeElement).toBe(result);
  expect(result.textContent).toContain("review-diff");
});

it("keeps a complete empty Skill page distinct from locally hidden summaries", () => {
  testClient.render(<RecordedResult context={capturedContext("list_skills", '{"skills":[]}')} />);
  const result = screen.getByRole("region", { name: "Result" });
  expect(result.textContent).toBe("No skills were returned.");
  expect(screen.queryByText("More pages", { exact: true })).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Find" }));
  const field = screen.getByRole("searchbox", { name: "Filter result" });
  fireEvent.change(field, { target: { value: "anything" } });
  expect(result.textContent).toBe("No skills were returned.");
  expect(result.textContent).not.toContain("Close find");
  fireEvent.keyDown(field, { key: "Escape" });
  expect(result.textContent).toBe("No skills were returned.");
});

it.each(["load_skill", "read_skill_resource"])(
  "%s reads current page content without turning continuation into instruction text",
  (name) => {
    const content = "Keep <script>markup</script> literal.\n\n\tRead the next paragraph.\n";
    const page = { content, next_cursor: { offset: 81, snapshot: "b".repeat(64) } };
    testClient.render(<RecordedResult context={capturedContext(name, JSON.stringify(page))} />);
    const result = screen.getByRole("region", { name: "Result" });
    expect(Array.from(result.querySelectorAll("p"), (paragraph) => paragraph.textContent)).toEqual(
      name === "load_skill" ? [content, "next_cursor"] : ["next_cursor"],
    );
    expect(Array.from(result.querySelectorAll("pre"), (line) => line.textContent)).toEqual([
      ...(name === "read_skill_resource" ? content.split("\n").map((line) => line || " ") : []),
      JSON.stringify(page.next_cursor, null, 2),
    ]);
    expect(within(result).getByText("next_cursor", { exact: true })).toBeTruthy();
    expect(screen.getByText("More pages", { exact: true })).toBeTruthy();
    expectInert(result);
  },
);

it.each(["load_skill", "read_skill_resource"])(
  "%s leaves complete content and empty content literal without a continuation marker",
  (name) => {
    for (const content of ["First line.\n\nLast line.\n", ""]) {
      const rendered = testClient.render(
        <RecordedResult context={capturedContext(name, JSON.stringify({ content }))} />,
      );
      const result = screen.getByRole("region", { name: "Result" });
      expect(
        Array.from(result.querySelectorAll("p"), (paragraph) => paragraph.textContent),
      ).toEqual(name === "load_skill" ? [content || " "] : []);
      expect(Array.from(result.querySelectorAll("pre"), (line) => line.textContent)).toEqual(
        name === "read_skill_resource" ? content.split("\n").map((line) => line || " ") : [],
      );
      expect(screen.queryByText("More pages", { exact: true })).toBeNull();
      rendered.unmount();
    }
  },
);

it.each(["list_skills", "load_skill", "read_skill_resource"])(
  "%s copies the exact recorded page rather than reconstructing its JSON",
  async (name) => {
    const page =
      name === "list_skills"
        ? { skills: [{ name: "review", description: "Read the diff." }] }
        : { content: "Keep this body.\n" };
    const source =
      JSON.stringify({ ...page, next_cursor: { offset: 20, snapshot: "c".repeat(64) } }, null, 2) +
      "\n";
    const writeText = vi.fn(async () => {});
    vi.stubGlobal("navigator", { clipboard: { writeText } });
    try {
      testClient.render(<RecordedResult context={capturedContext(name, source)} />);
      fireEvent.click(screen.getByRole("button", { name: "Copy result" }));
      await act(async () => {});
      expect(writeText).toHaveBeenCalledExactlyOnceWith(source);
    } finally {
      vi.unstubAllGlobals();
    }
  },
);

it.each(["list_skills", "load_skill", "read_skill_resource"])(
  "%s preserves every byte of unknown or malformed page shapes",
  (name) => {
    const body =
      name === "list_skills"
        ? { skills: [{ name: "review", description: "Keep this summary." }] }
        : { content: "Keep every byte.\n" };
    const values = [
      { ...body, extra: "Do not discard this field." },
      { ...body, next_cursor: null },
      { ...body, next_cursor: { offset: 1, snapshot: "invalid" } },
      { ...body, next_cursor: { offset: 1, snapshot: "d".repeat(64), extra: "Keep me." } },
      { ...body, next_cursor: { offset: Number.MAX_SAFE_INTEGER + 1, snapshot: "d".repeat(64) } },
      name === "list_skills"
        ? { skills: [{ name: "review", description: "Keep this.", unknown: "Keep that too." }] }
        : { content: { text: "Keep this object complete." } },
    ];
    for (const value of values) {
      const source = JSON.stringify(value, null, 2);
      const rendered = testClient.render(
        <RecordedResult context={capturedContext(name, source)} />,
      );
      const result = screen.getByRole("region", { name: "Result" });
      expect(
        Array.from(result.querySelectorAll("p"), (paragraph) => paragraph.textContent),
      ).toEqual(name === "load_skill" ? [source] : []);
      expect(Array.from(result.querySelectorAll("pre"), (line) => line.textContent)).toEqual(
        name === "load_skill" ? [] : source.split("\n"),
      );
      expect(screen.queryByText("More pages", { exact: true })).toBeNull();
      expectInert(result);
      rendered.unmount();
    }
  },
);

it("distinguishes an empty skill list from a result where no summary fit", () => {
  const results = [
    [
      "<available_skills>\n  <truncated>true</truncated>\n</available_skills>",
      "More skills were omitted from this result.",
    ],
    ["<available_skills>\n</available_skills>", "No skills were returned."],
  ] as const;
  for (const [source, notice] of results) {
    const rendered = testClient.render(
      <RecordedResult context={capturedContext("list_skills", source)} />,
    );
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
      testClient.render(<RecordedResult context={capturedContext(name, result)} />);
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
  testClient.render(<RecordedResult context={capturedContext("grep", JSON.stringify({ hits }))} />);
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
    const rendered = testClient.render(<RecordedResult context={capturedContext(name, "")} />);
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
  const empty = testClient.render(
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
  testClient.render(
    <RecordedResult context={capturedContext("apply_patch", JSON.stringify({ changes }))} />,
  );
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
    const rendered = testClient.render(
      <RecordedResult context={capturedContext("list_skills", source)} />,
    );
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
    const shell = testClient.render(<RecordedResult context={capturedContext("shell", output)} />);
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

    const content = "# Title\n\nBody";
    const page = JSON.stringify({
      snapshotId: "a".repeat(64),
      url: "https://example.test/page",
      format: "markdown",
      content,
      offset: 0,
      totalRunes: Array.from(content).length,
    });
    testClient.render(<RecordedResult context={capturedContext("web_fetch", page)} />);
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

it.each(["web_fetch", "web_browse"])(
  "keeps a captured %s page continuation visible without fetching more content",
  (name) => {
    const page = {
      snapshotId: "b".repeat(64),
      url: "https://example.test/captured",
      format: "text",
      content: "First 🐱 page",
      offset: 0,
      totalRunes: 30,
      nextOffset: 12,
    };
    testClient.render(<RecordedResult context={capturedContext(name, JSON.stringify(page))} />);
    const result = screen.getByRole("region", { name: "Result" });
    expect(result.textContent).toContain(page.content);
    expect(result.textContent).toContain(page.url);
    expect(result.textContent).toContain(page.snapshotId);
    expect(result.textContent).toContain('"nextOffset": 12');
    expect(screen.getByText("More pages", { exact: true })).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Find" }));
    fireEvent.change(screen.getByRole("searchbox"), { target: { value: "First" } });
    expect(screen.getByText("More pages", { exact: true })).toBeTruthy();
    expectInert(result);
  },
);

it("preserves complete web source when metadata is malformed or unrecognised", () => {
  const page = {
    snapshotId: "c".repeat(64),
    url: "https://example.test/captured",
    format: "text",
    content: "Body",
    offset: 0,
    totalRunes: 4,
  };
  for (const value of [
    { ...page, nextOffset: 4 },
    { ...page, totalRunes: 10 },
    { ...page, future: "retained metadata" },
    { ...page, results: [] },
    { format: "text", content: "historical body" },
  ]) {
    const source = JSON.stringify(value);
    const rendered = testClient.render(
      <RecordedResult context={capturedContext("web_fetch", source)} />,
    );
    const result = screen.getByRole("region", { name: "Result" });
    expect(Array.from(result.querySelectorAll("pre"), (line) => line.textContent).join("\n")).toBe(
      source,
    );
    expectInert(result);
    rendered.unmount();
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
  testClient.render(<RecordedResult context={capturedContext("shell", output)} />);
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
  testClient.render(
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
  testClient.render(<RecordedResult context={capturedContext("read", source)} />);
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

it.each(["read", "glob", "web_search"])(
  "%s starts a changed filter at its first matching row without moving input focus",
  (name) => {
    const rows = Array.from(
      { length: 82 },
      (_, index) => `${index % 2 === 0 ? "keep" : "other"}/candidate-${index}`,
    );
    const source =
      name === "read"
        ? rows.join("\n")
        : JSON.stringify(
            name === "glob"
              ? { hits: rows.map((path) => ({ path })) }
              : { results: rows.map((title) => ({ title, url: `https://example.test/${title}` })) },
          );
    testClient.render(<RecordedResult context={capturedContext(name, source)} />);
    const reader = screen.getByRole("region", { name: "Result" });
    reader.scrollTop = 900;
    fireEvent.click(screen.getByRole("button", { name: "Find" }));
    const field = screen.getByRole("searchbox", { name: "Filter result" });
    expect(reader.scrollTop).toBe(900);

    fireEvent.change(field, { target: { value: "keep" } });
    expect(reader.scrollTop).toBe(0);
    expect(document.activeElement).toBe(field);
    expect(within(reader).getByText("keep/candidate-0", { exact: true })).toBeTruthy();
    expect(screen.getByText("40 of 41 matches")).toBeTruthy();

    reader.scrollTop = 400;
    fireEvent.change(field, { target: { value: "keep/candidate" } });
    expect(reader.scrollTop).toBe(0);
    expect(document.activeElement).toBe(field);
    expect(screen.getByText("40 of 41 matches")).toBeTruthy();
  },
);

it("preserves manual result reading across rerenders and an unchanged Find task", () => {
  const context = capturedContext(
    "read",
    Array.from({ length: 82 }, (_, index) => `captured ${index}`).join("\n"),
  );
  const rendered = testClient.render(<RecordedResult context={context} />);
  const reader = screen.getByRole("region", { name: "Result" });
  reader.scrollTop = 600;
  fireEvent.scroll(reader);
  rendered.rerender(<RecordedResult context={context} />);
  expect(reader.scrollTop).toBe(600);
  fireEvent.click(screen.getByRole("button", { name: "Find" }));
  expect(reader.scrollTop).toBe(600);
  fireEvent.click(screen.getByRole("button", { name: "Close find" }));
  expect(reader.scrollTop).toBe(600);
  expect(document.activeElement).toBe(reader);
});

it.each(["clear", "close"])("%s starts the restored result list at its first row", (action) => {
  const source = Array.from(
    { length: 82 },
    (_, index) => `${index % 2 === 0 ? "keep" : "other"}/candidate-${index}`,
  ).join("\n");
  testClient.render(<RecordedResult context={capturedContext("read", source)} />);
  const reader = screen.getByRole("region", { name: "Result" });
  fireEvent.click(screen.getByRole("button", { name: "Find" }));
  const field = screen.getByRole("searchbox", { name: "Filter result" });
  fireEvent.change(field, { target: { value: "keep" } });
  reader.scrollTop = 700;
  fireEvent.click(
    screen.getByRole("button", { name: action === "clear" ? "Clear search" : "Close find" }),
  );
  expect(reader.scrollTop).toBe(0);
  expect(document.activeElement).toBe(action === "clear" ? field : reader);
  expect(screen.getByText("40 of 82 rows")).toBeTruthy();
  expect(within(reader).getByText("other/candidate-1", { exact: true })).toBeTruthy();
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
  testClient.render(
    <>
      <p>Surrounding narrative</p>
      <RecordedResult context={capturedContext("shell", output)} />
    </>,
  );
  fireEvent.click(screen.getByRole("button", { name: /earlier lines/ }));
  return screen.getByRole("region", { name: "Tool output" });
}

function findUniqueOutputLine() {
  const output = Array.from({ length: 240 }, (_, index) =>
    index === 42 ? "Boundary needle alpha" : `line ${index}`,
  ).join("\n");
  const reader = expandOutput(output);
  Object.defineProperty(reader, "clientHeight", { configurable: true, value: 400 });
  fireEvent.click(screen.getByRole("button", { name: "Find" }));
  const field = screen.getByRole<HTMLInputElement>("searchbox", { name: "Find in output" });
  fireEvent.change(field, { target: { value: "Boundary needle" } });
  const status = () =>
    within(screen.getByRole("search", { name: "Output search" })).queryByRole("status")
      ?.textContent;
  const expectRevealed = () => {
    expect(reader.scrollTop).toBe(42 * 20 - 200);
    expect(reader.querySelector('[data-current-match] [data-output-line="42"]')?.textContent).toBe(
      "Boundary needle alpha",
    );
    expect(document.activeElement).toBe(field);
    expect(status()).toBe("1 of 1");
  };
  const expectScrolledAway = () => {
    expect(reader.scrollTop).toBe(2000);
    expect(reader.querySelector('[data-output-line="100"]')).toBeTruthy();
    expect(reader.querySelector('[data-output-line="42"]')).toBeNull();
  };
  const scrollAway = () => {
    reader.scrollTop = 2000;
    fireEvent.scroll(reader);
    expectScrolledAway();
  };
  return { reader, field, status, expectRevealed, scrollAway, expectScrolledAway };
}

it.each([
  {
    action: "Enter",
    request: (field: HTMLElement) => fireEvent.keyDown(field, { key: "Enter" }),
  },
  {
    action: "Shift+Enter",
    request: (field: HTMLElement) => fireEvent.keyDown(field, { key: "Enter", shiftKey: true }),
  },
  {
    action: "a refined query",
    request: (field: HTMLElement) =>
      fireEvent.change(field, { target: { value: "Boundary needle alpha" } }),
  },
])(
  "reveals the same unique output match again on $action after manual scrolling",
  ({ request }) => {
    const { field, expectRevealed, scrollAway } = findUniqueOutputLine();
    expectRevealed();
    scrollAway();
    request(field);
    expectRevealed();
  },
);

it("keeps a manual output scroll until an explicit Find action", () => {
  const { reader, field, status, expectRevealed, scrollAway, expectScrolledAway } =
    findUniqueOutputLine();
  expectRevealed();
  scrollAway();

  fireEvent.scroll(reader);
  expectScrolledAway();

  fireEvent.keyDown(field, { key: "Enter", isComposing: true });
  fireEvent.keyDown(field, { key: "Enter", keyCode: 229 });
  expectScrolledAway();
  expect(document.activeElement).toBe(field);
  expect(status()).toBe("1 of 1");

  fireEvent.change(field, { target: { value: "absent needle" } });
  expectScrolledAway();
  expect(status()).toBe("No matches");
  fireEvent.keyDown(field, { key: "Enter" });
  expectScrolledAway();

  fireEvent.click(screen.getByRole("button", { name: "Clear search" }));
  expectScrolledAway();
  expect(field.value).toBe("");
  expect(document.activeElement).toBe(field);
  expect(status()).toBeUndefined();

  fireEvent.change(field, { target: { value: "Boundary needle" } });
  expectRevealed();
  scrollAway();

  fireEvent.click(screen.getByRole("button", { name: "Close find" }));
  expect(screen.queryByRole("search", { name: "Output search" })).toBeNull();
  expect(document.activeElement).toBe(reader);
  expectScrolledAway();
  expect(reader.querySelector("[data-current-match]")).toBeNull();
});

it("scopes Select All to the presented result rows, outside find and caption controls", () => {
  const source = Array.from({ length: 82 }, (_, index) => `captured ${index}`).join("\n");
  testClient.render(
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
    testClient.render(<RecordedResult context={capturedContext("read", "alpha\nbeta")} />);
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
  testClient.render(
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
  testClient.render(<RecordedResult context={capturedContext("read", source)} />);
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
  {
    name: "list_skills",
    result: JSON.stringify({
      skills: [{ name: "review", description: "Read the diff.\nKeep its context." }],
    }),
    copied: "review\nRead the diff.\nKeep its context.",
  },
  {
    name: "load_skill",
    result: JSON.stringify({
      content: "Keep this body.\n",
      next_cursor: { offset: 16, snapshot: "e".repeat(64) },
    }),
    copied:
      "Keep this body.\n\nnext_cursor\n" +
      JSON.stringify({ offset: 16, snapshot: "e".repeat(64) }, null, 2),
  },
  {
    name: "read_skill_resource",
    result: JSON.stringify({ content: "First line.\n\nLast line.\n" }),
    copied: "First line.\n\nLast line.\n",
  },
])("copies whole $name result rows as their recorded text", ({ name, result, copied }) => {
  testClient.render(<RecordedResult context={capturedContext(name, result)} />);
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
  testClient.render(<RecordedResult context={capturedContext("read", plain.join("\n"))} />);
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
