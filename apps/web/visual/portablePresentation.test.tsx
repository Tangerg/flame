import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { TrajectoryReport } from "../../../plugins/trajectory/component/component";
import { Markdown } from "../../../plugins/file-renderers/component/component";
import { createTranslator, type ComponentContext } from "@flame/component-sdk";
import { type ContentBlock, type TrajectoryEntry } from "@flame/runtime-client/wire";
import { RUNTIME_AGENT_SESSION_SNAPSHOTS } from "./agentSessionSnapshots";
import { visualTrajectory } from "./trajectorySnapshot";
import { en as trajectoryEn } from "../../../plugins/trajectory/component/locales/en";
const reportT = createTranslator({ en: trajectoryEn }, "en");
afterEach(cleanup);

function renderCapturedMarkdown(content: string) {
  const context: ComponentContext = {
    component: { installationId: "captured-file", digest: "a".repeat(64), componentId: "markdown" },
    locale: "en",
    signal: new AbortController().signal,
    file: {
      path: "review.md",
      content,
      startLine: 1,
      totalLines: content.split("\n").length,
      truncated: false,
    },
    get runtime(): never {
      throw new Error("Captured Markdown must not request live data");
    },
    get telemetry(): never {
      throw new Error("Captured Markdown must not request telemetry");
    },
    track() {
      throw new Error("Captured Markdown must not start I/O");
    },
    compose() {
      throw new Error("Captured Markdown must not compose another component");
    },
  };
  return render(<Markdown context={context} />);
}

it("keeps captured links complete, non-repeating and inert", () => {
  const url = "https://example.test/review";
  const annotation = "Accepted evidence and follow-up";
  const { container } = renderCapturedMarkdown(
    [
      url,
      `<${url}>`,
      `[${url}](${url})`,
      `[**Review** \`guide\`](https://example.test/guide "${annotation}")`,
      "[Another source](https://example.test/other)",
      "[Empty destination]()",
      "[Recorded command](javascript:alert%281%29)",
    ].join("\n\n"),
  );
  const document = container.querySelector('[data-slot="document-body"]')!;
  const paragraphs = Array.from(document.querySelectorAll(":scope > p"));
  expect(paragraphs.map((paragraph) => paragraph.textContent)).toEqual([
    url,
    url,
    url,
    "Review guide (https://example.test/guide)",
    "Another source (https://example.test/other)",
    "Empty destination",
    "Recorded command (javascript:alert%281%29)",
  ]);
  const annotated = screen.getByTitle(annotation);
  expect(annotated.tagName).toBe("SPAN");
  expect(annotated.querySelector("strong")?.textContent).toBe("Review");
  expect(annotated.querySelector("code")?.textContent).toBe("guide");
  expect(document.querySelectorAll("[title]")).toHaveLength(1);
  expect(
    document.querySelector("a, button, input, img, iframe, script, [href], [onclick]"),
  ).toBeNull();
});

it("preserves authored column alignment in captured Markdown tables", () => {
  renderCapturedMarkdown(
    [
      "| Change | Checks | Owner |",
      "| :--- | ---: | :---: |",
      "| `safe()` | 18 | **Runtime** |",
    ].join("\n"),
  );
  const table = screen.getByRole("table") as HTMLTableElement;
  for (const row of table.rows) {
    expect(Array.from(row.cells, (cell) => cell.style.textAlign)).toEqual([
      "left",
      "right",
      "center",
    ]);
  }
  expect(table.querySelector("code")?.textContent).toBe("safe()");
  expect(table.querySelector("strong")?.textContent).toBe("Runtime");
});

it("renders each tight captured task once with its primary name and immutable state", () => {
  const { container } = renderCapturedMarkdown(
    "- [x] Keep the **durable** fact\n- [ ] Verify `runtime.Commit`",
  );
  expect(screen.getAllByRole("checkbox")).toHaveLength(2);
  const complete = screen.getByRole("checkbox", { name: "Keep the durable fact", checked: true });
  const pending = screen.getByRole("checkbox", { name: "Verify runtime.Commit", checked: false });
  fireEvent.click(complete);
  fireEvent.click(pending);
  expect(screen.getByRole("checkbox", { name: "Keep the durable fact", checked: true })).toBe(
    complete,
  );
  expect(screen.getByRole("checkbox", { name: "Verify runtime.Commit", checked: false })).toBe(
    pending,
  );
  expect(container.textContent).not.toContain("[x]");
  expect(container.textContent).not.toContain("[ ]");
  expect(container.querySelector("strong")?.textContent).toBe("durable");
});

it("keeps loose task descriptions, nested lists, ordinary items and literal checkbox code", () => {
  const { container } = renderCapturedMarkdown(
    [
      "3. [x] Keep the **durable** fact",
      "",
      "   Explain `[x]` as literal code in this follow-up.",
      "",
      "   - Nested detail",
      "4. Ordinary item",
      "5. [ ] Verify `runtime.Commit`",
    ].join("\n"),
  );
  expect(screen.getAllByRole("checkbox")).toHaveLength(2);
  expect(
    screen.getByRole("checkbox", { name: "Keep the durable fact", checked: true }),
  ).toBeTruthy();
  expect(
    screen.getByRole("checkbox", { name: "Verify runtime.Commit", checked: false }),
  ).toBeTruthy();
  const list = container.querySelector("ol")!;
  expect(list.start).toBe(3);
  expect(list.querySelectorAll(":scope > li")).toHaveLength(3);
  expect(screen.getByText("Nested detail", { exact: true }).closest("ul")).not.toBeNull();
  expect(screen.getByText("Ordinary item", { exact: true }).closest("li")).not.toBeNull();
  expect(screen.getByText("[x]", { exact: true }).tagName).toBe("CODE");
  expect(container.textContent).not.toContain("[x] Keep");
  expect(container.textContent).not.toContain("[ ] Verify");
});

it("reads captured alert markers as the shared callout and keeps other quotes and markers literal", () => {
  const view = renderCapturedMarkdown(
    [
      "> [!WARNING]",
      "> Keep `runtime.Commit` scoped.",
      ">",
      "> - Confirm the result.",
      "",
      "> [!TIP] Same-line guidance.",
      "",
      "> An ordinary [!NOTE] quotation.",
      "",
      "> \\[!NOTE]",
      "> keep literal",
      "",
      "> [!NOTE]book stays prose",
      "",
      "```md",
      "> [!NOTE]",
      "```",
    ].join("\n"),
  );

  const callouts = view.container.querySelectorAll('[data-slot="callout"][role="note"]');
  expect(Array.from(callouts, (callout) => callout.getAttribute("data-callout"))).toEqual([
    "warning",
    "tip",
  ]);
  expect(callouts[0]?.textContent).toBe("WarningKeep runtime.Commit scoped.Confirm the result.");
  expect(callouts[1]?.textContent).toBe("TipSame-line guidance.");
  expect(callouts[0]?.querySelector("code")?.textContent).toBe("runtime.Commit");
  expect(
    Array.from(view.container.querySelectorAll("blockquote"), (quote) =>
      quote.textContent?.replace(/\s+/g, " ").trim(),
    ),
  ).toEqual(["An ordinary [!NOTE] quotation.", "[!NOTE] keep literal", "[!NOTE]book stays prose"]);
  expect(view.container.querySelector("pre code")?.textContent).toBe("> [!NOTE]");
});

it("gives captured code fences the shared code header while keeping the source literal", () => {
  const source = '<script>alert("x")</script>\n<img src="https://tracker.example/a.png">';
  const view = renderCapturedMarkdown(
    ["```html", source, "```", "", "```", "plain", "```"].join("\n"),
  );

  const regions = view.getAllByRole("region", { name: /Code block/ });
  expect(regions.map((region) => region.textContent)).toEqual([source, "plain"]);
  expect(view.container.querySelector("script, img")).toBeNull();
  expect(
    Array.from(
      view.container.querySelectorAll('[data-slot="code-caption"]'),
      (caption) => caption.firstElementChild?.textContent,
    ),
  ).toEqual(["html", "text"]);
  expect(view.getAllByRole("button", { name: "Copy code" })).toHaveLength(2);
  expect(view.getAllByRole("button", { name: "Enable word wrap" })).toHaveLength(2);
});

it("shows tool execution status independently of its approval decision", () => {
  const row = visualTrajectory(RUNTIME_AGENT_SESSION_SNAPSHOTS.waiting).find(
    (entry) => entry.type === "item" && entry.item.type === "toolCall",
  );
  if (row?.type !== "item" || row.item.type !== "toolCall") {
    throw new Error("waiting snapshot must contain a tool call");
  }
  render(
    <TrajectoryReport
      pageNumber={1}
      onClearFilters={vi.fn()}
      t={reportT}
      locale="en"
      kind="all"
      run="all"
      search=""
      page={{
        data: [
          { ...row, item: { ...row.item, status: "incomplete", approvalDecision: "approve" } },
        ],
      }}
    />,
  );
  expect(screen.getByText("Page 1 · 1 record")).toBeTruthy();
  expect(screen.getByText("1 tool call")).toBeTruthy();
  const summary = screen.getByRole("button", { name: /incomplete/ });
  expect(summary.textContent).toContain("Approval: approve");
  expect(screen.queryByText(row.item.id, { exact: true })).toBeNull();
  fireEvent.click(summary);
  expect(summary.getAttribute("aria-expanded")).toBe("true");
  expect(screen.getByText(row.item.id, { exact: true }).textContent).toBe(row.item.id);
});

it("keeps timed-out Runs and refused or canceled tools in attention without rewriting their state", () => {
  const original = visualTrajectory(RUNTIME_AGENT_SESSION_SNAPSHOTS.idle).find(
    (entry) => entry.type === "run",
  );
  if (!original || original.type !== "run") throw new Error("idle snapshot must contain a Run");
  const timeout: TrajectoryEntry = {
    ...original,
    run: {
      ...original.run,
      id: "run_timeout",
      status: "finished",
      outcome: { type: "timedOut", error: { type: "timeout" } },
    },
  };
  const tool = {
    type: "toolCall" as const,
    status: "incomplete" as const,
    runId: original.run.id,
    startedAt: original.occurredAt,
    tool: { name: "shell", arguments: { command: "go test ./..." } },
  };
  const refused: TrajectoryEntry = {
    type: "item",
    occurredAt: original.occurredAt,
    item: {
      ...tool,
      id: "item_refused",
      approvalDecision: "deny",
      error: { type: "denied_by_user" },
    },
  };
  const canceledTool: TrajectoryEntry = {
    type: "item",
    occurredAt: original.occurredAt,
    item: { ...tool, id: "item_canceled", error: { type: "tool_canceled" } },
  };
  const canceledRun: TrajectoryEntry = {
    ...original,
    run: {
      ...original.run,
      id: "run_canceled",
      status: "finished",
      outcome: { type: "canceled" },
    },
  };
  const completed: TrajectoryEntry = {
    ...original,
    run: {
      ...original.run,
      id: "run_completed",
      status: "finished",
      outcome: { type: "completed" },
    },
  };
  const { container } = render(
    <TrajectoryReport
      pageNumber={1}
      onClearFilters={vi.fn()}
      t={reportT}
      locale="en"
      page={{ data: [timeout, refused, canceledTool, canceledRun, completed] }}
      kind="attention"
      run="all"
      search=""
    />,
  );

  expect(
    Array.from(container.querySelectorAll("[data-trajectory-record]"), (record) =>
      record.getAttribute("data-trajectory-record"),
    ),
  ).toEqual(["run:run_timeout", "item:item_refused", "item:item_canceled"]);
  expect(screen.getByText("3 need attention")).toBeTruthy();
  const timeoutSummary = screen.getByRole("button", { name: /run_timeout/ });
  expect(timeoutSummary.textContent).toContain("timedOut");
  const refusedRecord = within(
    container.querySelector<HTMLElement>('[data-trajectory-record="item:item_refused"]')!,
  );
  const refusedSummary = refusedRecord.getByRole("button", { expanded: false });
  expect(refusedSummary.textContent).toContain("incomplete");
  expect(refusedSummary.textContent).toContain("Approval: deny");
  expect(
    within(
      container.querySelector<HTMLElement>('[data-trajectory-record="item:item_canceled"]')!,
    ).getByRole("button", { expanded: false }).textContent,
  ).toContain("incomplete");
  fireEvent.click(timeoutSummary);
  expect(
    within(timeoutSummary.closest("section")!).getByRole("region", { name: "Recorded evidence" })
      .textContent,
  ).toBe(JSON.stringify(timeout, null, 2));
});

it("reads settled neutral states as quiet metadata and keeps attention states beside the title", () => {
  const original = visualTrajectory(RUNTIME_AGENT_SESSION_SNAPSHOTS.idle).find(
    (entry) => entry.type === "run",
  );
  if (!original || original.type !== "run") throw new Error("idle snapshot must contain a Run");
  const run = (id: string, outcome: "completed" | "failed"): TrajectoryEntry => ({
    ...original,
    run: {
      ...original.run,
      id,
      status: "finished",
      outcome:
        outcome === "failed"
          ? { type: "failed", error: { type: "agent_stuck" } }
          : { type: outcome },
    },
  });
  render(
    <TrajectoryReport
      pageNumber={1}
      onClearFilters={vi.fn()}
      t={reportT}
      locale="en"
      page={{ data: [run("run_settled", "completed"), run("run_broken", "failed")] }}
      kind="all"
      run="all"
      search=""
    />,
  );
  const meta = (id: string) =>
    screen.getByRole("button", { name: new RegExp(id) }).querySelector("time")!.parentElement!
      .textContent;
  expect(meta("run_settled")).toContain("completed");
  expect(meta("run_settled")).toContain("run_settled");
  expect(meta("run_broken")).not.toContain("failed");
  expect(screen.getByRole("button", { name: /run_broken/ }).textContent).toContain("failed");
  expect(screen.getByText("Page 1 · 2 records")).toBeTruthy();
  expect(screen.getByText("1 needs attention")).toBeTruthy();
  expect(screen.getByText("0 tool calls")).toBeTruthy();
});

it("distinguishes same-time Runs before expansion and keeps one Run identity fact", () => {
  const original = visualTrajectory(RUNTIME_AGENT_SESSION_SNAPSHOTS.idle).find(
    (entry) => entry.type === "run",
  );
  if (!original || original.type !== "run") throw new Error("idle snapshot must contain a Run");
  const root = { ...original, run: { ...original.run, rootRunId: original.run.id } };
  const child = {
    ...root,
    run: {
      ...root.run,
      id: "run_same_second_child",
      parentRunId: root.run.id,
      spawnedByItemId: "item_delegate",
    },
  };
  render(
    <TrajectoryReport
      pageNumber={1}
      onClearFilters={vi.fn()}
      t={reportT}
      locale="en"
      page={{ data: [root, child] }}
      kind="all"
      run="all"
      search=""
    />,
  );

  for (const entry of [root, child]) {
    const trigger = screen.getByRole("button", { name: new RegExp(entry.run.id), expanded: false });
    expect(trigger.textContent).toContain(`${entry.run.provider}/${entry.run.model}`);
    expect(trigger.textContent).toContain(entry.run.outcome?.type ?? entry.run.status);
    expect(trigger.querySelector("code")?.textContent).toBe(entry.run.id);
    expect(trigger.querySelector("time")?.getAttribute("datetime")).toBe(entry.occurredAt);
    fireEvent.click(trigger);
    const record = trigger.closest("section")!;
    const facts = within(record.querySelector("dl")!);
    expect(facts.queryByText("Record ID")).toBeNull();
    expect(facts.getAllByText("Run ID")).toHaveLength(1);
    expect(facts.getByText("Run ID").nextElementSibling?.textContent).toBe(entry.run.id);
    expect(facts.getByText("Root Run ID").nextElementSibling?.textContent).toBe(root.run.id);
    expect(within(record).getByRole("region", { name: "Recorded evidence" })).toBeTruthy();
  }
  const childFacts = within(
    screen
      .getByRole("button", { name: new RegExp(child.run.id), expanded: true })
      .closest("section")!
      .querySelector("dl")!,
  );
  expect(childFacts.getByText("Parent Run ID").nextElementSibling?.textContent).toBe(root.run.id);
  expect(childFacts.getByText("Delegating Item ID").nextElementSibling?.textContent).toBe(
    child.run.spawnedByItemId,
  );
});

it("reads recorded text and images in order while keeping full evidence and reasoning redaction", () => {
  const occurredAt = "2026-07-31T08:00:00.000Z";
  const first = "First observation.\n\nKeep **literal** Markdown and <script> text.";
  const last = "After the recorded image.\nPreserve the final line.";
  const image = { type: "image", mime: "image/png", data: "aW1hZ2U=" } as const;
  const common = { createdAt: occurredAt, runId: "run_root", status: "completed" as const };
  const message = {
    ...common,
    type: "userMessage" as const,
    id: "item_body_user",
    content: [
      { type: "text", text: first },
      image,
      { type: "text", text: last },
    ] satisfies ContentBlock[],
    capturedExtension: { observation: "Keep this unknown field in the raw evidence." },
  };
  const reasoning = {
    ...common,
    type: "reasoning" as const,
    id: "item_body_reasoning",
    text: "Accepted reasoning.\n\nThe second paragraph remains complete.",
  };
  const secret = "REDACTED_REASONING_MUST_NEVER_APPEAR";
  const redacted = { ...reasoning, id: "item_redacted", redacted: true, text: secret };
  const cases = [
    { item: message, reading: [first, "image", last] },
    {
      item: { ...message, type: "agentMessage" as const, id: "item_body_agent" },
      reading: [first, "image", last],
    },
    { item: reasoning, reading: [reasoning.text] },
    {
      item: {
        ...common,
        type: "compaction" as const,
        id: "item_body_compaction",
        summary: "Recorded compaction.\n\nKeep its complete summary.",
      },
      reading: ["Recorded compaction.\n\nKeep its complete summary."],
    },
  ];
  const entries: TrajectoryEntry[] = cases.map(({ item }) => ({ type: "item", occurredAt, item }));
  const { container } = render(
    <TrajectoryReport
      pageNumber={1}
      onClearFilters={vi.fn()}
      t={reportT}
      locale="en"
      page={{ data: [...entries, { type: "item", occurredAt, item: redacted }] }}
      kind="all"
      run="all"
      search=""
    />,
  );

  for (const sample of cases) {
    const record = container.querySelector<HTMLElement>(
      `[data-trajectory-record="item:${sample.item.id}"]`,
    )!;
    const scope = within(record);
    fireEvent.click(scope.getByRole("button", { expanded: false }));
    const reading = Array.from(record.querySelectorAll("p, img"))
      .filter((node) => node.tagName === "IMG" || sample.reading.includes(node.textContent ?? ""))
      .map((node) => (node.tagName === "IMG" ? "image" : node.textContent));
    expect(reading).toEqual(sample.reading);
    expect(record.querySelector("strong, script, a")).toBeNull();
    const images = sample.reading.includes("image")
      ? [`data:${image.mime};base64,${image.data}`]
      : [];
    expect(scope.queryAllByRole("img").map((node) => node.getAttribute("src"))).toEqual(images);
    expect(scope.queryByRole("region", { name: "Recorded evidence" })).toBeNull();
    fireEvent.click(scope.getByRole("button", { name: "Recorded evidence", expanded: false }));
    expect(scope.getByRole("region", { name: "Recorded evidence" }).textContent).toBe(
      JSON.stringify({ type: "item", occurredAt, item: sample.item }, null, 2),
    );
  }
  const hidden = within(container.querySelector('[data-trajectory-record="item:item_redacted"]')!);
  fireEvent.click(hidden.getByRole("button", { expanded: false }));
  expect(hidden.getByText("Reasoning was redacted.")).toBeTruthy();
  expect(hidden.queryByRole("button", { name: "Recorded evidence" })).toBeNull();
  expect(hidden.getByRole("region", { name: "Recorded evidence" }).textContent).toContain(
    '"redacted": true',
  );
  expect(container.textContent).not.toContain(secret);
});

it("searches recorded tool data and message metadata without image bytes or redacted reasoning", () => {
  const occurredAt = "2026-07-31T08:00:00.000Z";
  const common = { createdAt: occurredAt, runId: "run_root", status: "completed" as const };
  const tool = {
    type: "toolCall" as const,
    id: "item_search_tool",
    runId: "run_root",
    status: "completed" as const,
    startedAt: occurredAt,
    finishedAt: "2026-07-31T08:00:01.000Z",
    durationMillis: 1000,
    tool: {
      name: "lookup",
      arguments: {
        data: "ARGUMENT_DIRECT_DATA",
        filter: { data: "ARGUMENT_NESTED_DATA" },
        rows: [{ data: "ARGUMENT_ARRAY_DATA" }],
      },
      result: {
        data: "RESULT_DATA_VISIBLE_902",
        page: { data: "RESULT_NESTED_DATA" },
        rows: [{ data: "RESULT_ARRAY_DATA" }],
        preview: { type: "image", mime: "image/png", data: "searchable business data" },
      },
    },
    capturedExtension: { data: "EXTENSION_DATA_VALUE" },
  };
  const userImage = {
    type: "image",
    mime: "image/webp",
    caption: "USER_IMAGE_METADATA",
    data: "VVNFUl9JTUFHRV9CWVRFUw==",
  } as const;
  const agentImage = {
    type: "image",
    mime: "image/gif",
    data: "QUdFTlRfSU1BR0VfQllURVM=",
  } as const;
  const user = {
    ...common,
    type: "userMessage" as const,
    id: "item_search_user",
    content: [{ type: "text", text: "USER_VISIBLE_TEXT" }, userImage] satisfies ContentBlock[],
  };
  const agent = {
    ...common,
    type: "agentMessage" as const,
    id: "item_search_agent_image",
    content: [agentImage, { type: "text", text: "AGENT_VISIBLE_TEXT" }] satisfies ContentBlock[],
  };
  const omitted = { ...common, type: "agentMessage" as const, id: "item_search_agent_omitted" };
  const empty = { ...omitted, id: "item_search_agent_empty", content: [] };
  const secret = "REDACTED_SEARCH_SECRET";
  const redacted = {
    ...common,
    type: "reasoning" as const,
    id: "item_search_redacted",
    redacted: true,
    text: secret,
  };
  const items = [tool, user, agent, omitted, empty, redacted];
  const entries: TrajectoryEntry[] = items.map((item) => ({ type: "item", occurredAt, item }));
  const recorded = structuredClone(entries);
  const report = (search: string, kind = "all") => (
    <TrajectoryReport
      pageNumber={1}
      onClearFilters={vi.fn()}
      t={reportT}
      locale="en"
      page={{ data: entries }}
      kind={kind}
      run="all"
      search={search}
    />
  );
  const { container, rerender } = render(report(""));
  const matches = (search: string, kind?: string) => {
    rerender(report(search, kind));
    return Array.from(container.querySelectorAll("[data-trajectory-record]"), (record) =>
      record.getAttribute("data-trajectory-record")?.replace("item:", ""),
    );
  };

  expect(matches("item_search_")).toEqual(items.map((item) => item.id));
  for (const value of [
    "ARGUMENT_DIRECT_DATA",
    "ARGUMENT_NESTED_DATA",
    "ARGUMENT_ARRAY_DATA",
    "RESULT_DATA_VISIBLE_902",
    "result_data_visible_902",
    "RESULT_NESTED_DATA",
    "RESULT_ARRAY_DATA",
    "searchable business data",
    "EXTENSION_DATA_VALUE",
  ])
    expect(matches(value)).toEqual([tool.id]);
  expect(matches(userImage.data)).toEqual([]);
  expect(matches(agentImage.data)).toEqual([]);
  expect(matches(userImage.mime)).toEqual([user.id]);
  expect(matches(userImage.caption)).toEqual([user.id]);
  expect(matches(agentImage.mime)).toEqual([agent.id]);
  expect(matches('"type":"image"', "message")).toEqual([user.id, agent.id]);
  expect(matches("USER_VISIBLE_TEXT")).toEqual([user.id]);
  expect(matches("AGENT_VISIBLE_TEXT")).toEqual([agent.id]);
  expect(matches("item_search_agent_")).toEqual([agent.id, omitted.id, empty.id]);
  expect(matches(secret)).toEqual([]);
  expect(matches(redacted.id)).toEqual([redacted.id]);

  matches("");
  const { text: _hidden, ...shown } = redacted;
  for (const item of items) {
    const scope = within(
      container.querySelector<HTMLElement>(`[data-trajectory-record="item:${item.id}"]`)!,
    );
    fireEvent.click(scope.getByRole("button", { expanded: false }));
    const nested = scope.queryByRole("button", { name: "Recorded evidence", expanded: false });
    if (nested) fireEvent.click(nested);
    expect(scope.getByRole("region", { name: "Recorded evidence" }).textContent).toBe(
      JSON.stringify({ type: "item", occurredAt, item: item === redacted ? shown : item }, null, 2),
    );
  }
  expect(container.textContent).not.toContain(secret);
  expect(entries).toEqual(recorded);
});
