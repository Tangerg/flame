import { unavailableComponentNotifications } from "@/test/unavailableNotifications";
import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { cleanup, fireEvent, screen, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { TrajectoryReport } from "../../../plugins/trajectory/component/report";
import { Markdown } from "../../../plugins/file-renderers/component/component";
import { createTranslator, type ComponentContext } from "@flame/component-sdk";
import { type ContentBlock, type TrajectoryEntry } from "@flame/runtime-client/wire";
import { validateMethodResult } from "@flame/runtime-client/validation";
import { RUNTIME_AGENT_SESSION_SNAPSHOTS } from "./agentSessionSnapshots";
import { visualTrajectory } from "./trajectorySnapshot";
import { en as trajectoryEn } from "../../../plugins/trajectory/component/locales/en";
import { zh as trajectoryZh } from "../../../plugins/trajectory/component/locales/zh";
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
    saveFile() {
      throw new Error("This captured read must not save files");
    },
    notifications: unavailableComponentNotifications,
    subscribeRootRunSettlements() {
      throw new Error("This fixture must not observe run settlements");
    },
    registerCommand() {
      throw new Error("This fixture must not register client commands");
    },
    compose() {
      throw new Error("Captured Markdown must not compose another component");
    },
  };
  return testClient.render(<Markdown context={context} />);
}

it("decodes captured Markdown entities once across semantic text positions", () => {
  const { container } = renderCapturedMarkdown(
    [
      "# Tom &amp; Jerry &copy; &#x1F600;",
      "&amp;copy; &#38;copy; &copy &unknown; \\&copy;",
      '[**&copy;**](https://example.test/?a=1&amp;b=2 "&copy; &amp;copy;")',
      '![&copy; &#38;copy;](image&amp;name.png "&#169;")',
      "| &copy; |\n| --- |\n| &#169; &amp;copy; |",
      "<https://example.test/?a=1&amp;b=2>",
      "> [!NOTE] &#38;copy; &copy;",
    ].join("\n\n"),
  );
  expect(screen.getByRole("heading", { name: "Tom & Jerry © 😀" })).toBeTruthy();
  expect(screen.getByText("&copy; &copy; &copy &unknown; &copy;")).toBeTruthy();
  const link = screen.getByTitle("© &copy;");
  expect(link.querySelector("strong")?.textContent).toBe("©");
  expect(link.textContent).toBe("© (https://example.test/?a=1&b=2)");
  expect(screen.getByRole("columnheader").textContent).toBe("©");
  expect(screen.getByRole("cell").textContent).toBe("© &copy;");
  expect(screen.getByRole("img", { name: "© &copy;" })).toBeTruthy();
  expect(screen.getByText("&copy; ©")).toBeTruthy();
  expect(container.textContent).toContain("image&name.png");
  expect(container.textContent).toContain("https://example.test/?a=1&amp;b=2");
});

it("keeps code entities and authored HTML literal without executable elements", () => {
  const { container } = renderCapturedMarkdown(
    [
      "`&copy; &#169; &amp;copy;`",
      "```text\n&copy; &#169; &amp;copy;\n```",
      '<script title="&copy;">&amp;</script>',
      '<img src="x" onerror="alert(1)">',
      "&lt;img src=x onerror=alert(1)&gt;",
    ].join("\n\n"),
  );
  expect(Array.from(container.querySelectorAll("code"), (node) => node.textContent)).toEqual([
    "&copy; &#169; &amp;copy;",
    "&copy; &#169; &amp;copy;",
  ]);
  expect(container.textContent).toContain('<script title="&copy;">&amp;</script>');
  expect(container.textContent).toContain('<img src="x" onerror="alert(1)">');
  expect(container.textContent).toContain("<img src=x onerror=alert(1)>");
  expect(container.querySelector("script, img, [onerror]")).toBeNull();
});

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

it("retains code-fence metadata in its caption without changing the source", () => {
  const view = renderCapturedMarkdown('```ts title="sample.ts"\nconst sample = 1;\n```');
  expect(
    view.container.querySelector('[data-slot="code-caption"]')?.firstElementChild?.textContent,
  ).toBe('ts title="sample.ts"');
  expect(view.container.querySelector("pre code")?.textContent).toBe("const sample = 1;");
});

it("shows tool execution status independently of its approval decision", () => {
  const row = visualTrajectory(RUNTIME_AGENT_SESSION_SNAPSHOTS.waiting).find(
    (entry) => entry.type === "item" && entry.item.type === "toolCall",
  );
  if (row?.type !== "item" || row.item.type !== "toolCall") {
    throw new Error("waiting snapshot must contain a tool call");
  }
  testClient.render(
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
  const summary = screen.getByRole("button", { name: /Incomplete/ });
  expect(summary.textContent).toContain("Approval: Approved");
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
  const { container } = testClient.render(
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
  expect(timeoutSummary.textContent).toContain("Timed out");
  const refusedRecord = within(
    container.querySelector<HTMLElement>('[data-trajectory-record="item:item_refused"]')!,
  );
  const refusedSummary = refusedRecord.getByRole("button", { expanded: false });
  expect(refusedSummary.textContent).toContain("Incomplete");
  expect(refusedSummary.textContent).toContain("Approval: Denied");
  expect(
    within(
      container.querySelector<HTMLElement>('[data-trajectory-record="item:item_canceled"]')!,
    ).getByRole("button", { expanded: false }).textContent,
  ).toContain("Incomplete");
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
  testClient.render(
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
  expect(meta("run_settled")).toContain("Completed");
  expect(meta("run_settled")).toContain("run_settled");
  expect(meta("run_broken")).not.toContain("Failed");
  expect(screen.getByRole("button", { name: /run_broken/ }).textContent).toContain("Failed");
  expect(screen.getByText("Page 1 · 2 records")).toBeTruthy();
  expect(screen.getByText("1 needs attention")).toBeTruthy();
  expect(screen.getByText("0 tool calls")).toBeTruthy();
});

type RecordedRun = Extract<TrajectoryEntry, { type: "run" }>["run"];
type RecordedModel = Extract<TrajectoryEntry, { type: "model" }>["model"];

function stateRecords(): TrajectoryEntry[] {
  const original = visualTrajectory(RUNTIME_AGENT_SESSION_SNAPSHOTS.idle).find(
    (entry) => entry.type === "run",
  );
  if (!original || original.type !== "run") throw new Error("idle snapshot must contain a Run");
  const { outcome: _outcome, finishedAt: _finishedAt, ...open } = original.run;
  const occurredAt = original.occurredAt;
  const run = (
    id: string,
    status: RecordedRun["status"],
    outcome?: RecordedRun["outcome"],
  ): TrajectoryEntry => ({
    type: "run",
    occurredAt,
    run: outcome
      ? { ...open, id, status, finishedAt: occurredAt, outcome }
      : {
          ...open,
          id,
          status,
          ...(status === "running" ? { activeSegmentId: "segment_states" } : {}),
        },
  });
  const model = (callId: string, state: RecordedModel["state"]): TrajectoryEntry => ({
    type: "model",
    occurredAt,
    model: {
      callId,
      runId: open.id,
      segmentId: "segment_states",
      state,
      startedAt: occurredAt,
      ...(state === "unknown" ? { settledAt: occurredAt } : {}),
    },
  });
  const endpoint = (id: string, state: "completed" | "failed"): TrajectoryEntry => ({
    type: "endpoint",
    occurredAt,
    endpoint: {
      id,
      runId: open.id,
      sessionId: open.sessionId,
      caller: {
        id: `invocation_${id}`,
        origin: "automatic",
        target: { scope: "session", reference: open.sessionId },
      },
      kind: "chat",
      provider: open.provider,
      model: open.model,
      request: "{}",
      response: "{}",
      state,
      startedAt: occurredAt,
      settledAt: occurredAt,
      ...(state === "failed" ? { failure: "provider returned 503" } : {}),
    },
  });
  const tool = {
    type: "toolCall" as const,
    runId: open.id,
    startedAt: occurredAt,
    tool: { name: "shell", arguments: { command: "go test ./..." } },
  };
  return [
    run("run_running", "running"),
    run("run_waiting", "waiting"),
    run("run_completed", "finished", { type: "completed" }),
    run("run_failed", "finished", { type: "failed", error: { type: "agent_stuck" } }),
    run("run_timed_out", "finished", { type: "timedOut", error: { type: "timeout" } }),
    run("run_canceled", "finished", { type: "canceled" }),
    run("run_lost", "finished", { type: "lost", loss: { reason: "restart" } }),
    model("call_started", "started"),
    model("call_unknown", "unknown"),
    endpoint("endpoint_completed", "completed"),
    endpoint("endpoint_failed", "failed"),
    {
      type: "item",
      occurredAt,
      item: { ...tool, id: "item_permitted", status: "running", approvalDecision: "approve" },
    },
    {
      type: "item",
      occurredAt,
      item: {
        ...tool,
        id: "item_refused",
        status: "incomplete",
        approvalDecision: "deny",
        finishedAt: occurredAt,
      },
    },
    {
      type: "item",
      occurredAt,
      item: {
        type: "userMessage",
        id: "item_message",
        runId: open.id,
        status: "completed",
        createdAt: occurredAt,
        content: [{ type: "text", text: "Keep this recorded message unchanged." }],
      },
    },
  ];
}

const recordKey = (entry: TrajectoryEntry) =>
  entry.type === "run"
    ? `run:${entry.run.id}`
    : entry.type === "model"
      ? `model:${entry.model.callId}`
      : entry.type === "endpoint"
        ? `endpoint:${entry.endpoint.id}`
        : `item:${entry.item.id}`;

it.each([
  {
    locale: "en",
    evidence: "Recorded evidence",
    approvalFact: "Approval",
    approved: "Approved",
    denied: "Denied",
    inline: (decision: string) => `Approval: ${decision}`,
    labels: {
      "run:run_running": "Running",
      "run:run_waiting": "Waiting",
      "run:run_completed": "Completed",
      "run:run_failed": "Failed",
      "run:run_timed_out": "Timed out",
      "run:run_canceled": "Canceled",
      "run:run_lost": "Lost",
      "model:call_started": "Started",
      "model:call_unknown": "Outcome unknown",
      "endpoint:endpoint_completed": "Completed",
      "endpoint:endpoint_failed": "Failed",
      "item:item_permitted": "Running",
      "item:item_refused": "Incomplete",
      "item:item_message": "Completed",
    } as Record<string, string>,
  },
  {
    locale: "zh",
    evidence: "已记录的证据",
    approvalFact: "审批",
    approved: "已批准",
    denied: "已拒绝",
    inline: (decision: string) => `审批：${decision}`,
    labels: {
      "run:run_running": "运行中",
      "run:run_waiting": "等待中",
      "run:run_completed": "已完成",
      "run:run_failed": "失败",
      "run:run_timed_out": "超时",
      "run:run_canceled": "已取消",
      "run:run_lost": "无法恢复",
      "model:call_started": "已开始",
      "model:call_unknown": "结果未知",
      "endpoint:endpoint_completed": "已完成",
      "endpoint:endpoint_failed": "失败",
      "item:item_permitted": "运行中",
      "item:item_refused": "未完成",
      "item:item_message": "已完成",
    } as Record<string, string>,
  },
])(
  "localizes recorded states and approval decisions in $locale without rewriting evidence",
  ({ locale, evidence, approvalFact, approved, denied, inline, labels }) => {
    const entries = stateRecords();
    expect(validateMethodResult("sessions.trajectory", { data: entries })).toEqual([]);
    const recorded = structuredClone(entries);
    const { container } = testClient.render(
      <TrajectoryReport
        pageNumber={1}
        onClearFilters={vi.fn()}
        t={createTranslator({ en: trajectoryEn, zh: trajectoryZh }, locale)}
        locale={locale}
        page={{ data: entries }}
        kind="all"
        run="all"
        search=""
      />,
    );
    const wireValue =
      locale === "zh"
        ? /\b(running|waiting|finished|started|completed|failed|timedOut|canceled|lost|unknown|incomplete|approve|deny)\b/
        : /\b(timedOut|approve|deny)\b/;

    for (const entry of entries) {
      const key = recordKey(entry);
      const record = within(
        container.querySelector<HTMLElement>(`[data-trajectory-record="${key}"]`)!,
      );
      const summary = record.getByRole("button", { expanded: false });
      expect(summary.textContent).toContain(labels[key]);
      expect(summary.textContent).not.toMatch(wireValue);
      fireEvent.click(summary);
      const raw = record.queryByRole("button", { name: evidence, expanded: false });
      if (raw) fireEvent.click(raw);
      expect(record.getByRole("region", { name: evidence }).textContent).toBe(
        JSON.stringify(entry, null, 2),
      );
    }
    const summaryOf = (key: string) =>
      within(container.querySelector<HTMLElement>(`[data-trajectory-record="${key}"]`)!).getByRole(
        "button",
        { expanded: true },
      ).textContent;

    for (const [key, decision, status] of [
      ["item:item_permitted", approved, labels["item:item_permitted"]],
      ["item:item_refused", denied, labels["item:item_refused"]],
    ] as const) {
      const record = container.querySelector<HTMLElement>(`[data-trajectory-record="${key}"]`)!;
      expect(summaryOf(key)).toContain(inline(decision));
      expect(summaryOf(key)).toContain(status);
      expect(
        within(record.querySelector("dl")!).getByText(approvalFact).nextElementSibling?.textContent,
      ).toBe(decision);
    }
    expect(entries).toEqual(recorded);
  },
);

it("keeps attention on recorded wire states and searches both raw values and readable labels", () => {
  const entries = stateRecords();
  const report = (kind: string, search: string) => (
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
  const { container, rerender } = testClient.render(report("all", ""));
  const shown = (kind: string, search: string) => {
    rerender(report(kind, search));
    return Array.from(container.querySelectorAll("[data-trajectory-record]"), (record) =>
      record.getAttribute("data-trajectory-record"),
    );
  };

  expect(screen.getByText("7 need attention")).toBeTruthy();
  expect(shown("attention", "")).toEqual([
    "run:run_waiting",
    "run:run_failed",
    "run:run_timed_out",
    "run:run_lost",
    "model:call_unknown",
    "endpoint:endpoint_failed",
    "item:item_refused",
  ]);
  expect(shown("all", "timedOut")).toEqual(["run:run_timed_out"]);
  expect(shown("all", '"state":"unknown"')).toEqual(["model:call_unknown"]);
  expect(shown("all", '"approvalDecision":"deny"')).toEqual(["item:item_refused"]);
  expect(shown("all", "Timed out")).toEqual(["run:run_timed_out"]);
  expect(shown("all", "Outcome unknown")).toEqual(["model:call_unknown"]);
  expect(shown("all", "Denied")).toEqual(["item:item_refused"]);
  expect(shown("attention", "Outcome unknown")).toEqual(["model:call_unknown"]);
});

it.each([
  {
    locale: "en",
    lost: "Lost",
    unknown: "OUTCOME UNKNOWN",
    denied: "Denied",
    inline: "Approval: Denied",
    approved: "approval: approved",
    otherLocale: "Approval: 已拒绝",
    pageCounts: ["Page 1 · 1 of 14 records", "7 need attention"],
  },
  {
    locale: "zh",
    lost: "无法恢复",
    unknown: "结果未知",
    denied: "已拒绝",
    inline: "审批：已拒绝",
    approved: "审批：已批准",
    otherLocale: "Approval: Denied",
    pageCounts: ["第 1 页 · 1 / 14 条记录", "7 条需要关注"],
  },
])(
  "finds the readable $locale state and approval labels beside recorded evidence",
  ({ locale, lost, unknown, denied, inline, approved, otherLocale, pageCounts }) => {
    const entries = stateRecords();
    const recorded = structuredClone(entries);
    const t = createTranslator({ en: trajectoryEn, zh: trajectoryZh }, locale);
    const report = (search: string, kind = "all", run = "all") => (
      <TrajectoryReport
        pageNumber={1}
        onClearFilters={vi.fn()}
        t={t}
        locale={locale}
        page={{ data: entries }}
        kind={kind}
        run={run}
        search={search}
      />
    );
    const { container, rerender } = testClient.render(report(""));
    const shown = (search: string, kind?: string, run?: string) => {
      rerender(report(search, kind, run));
      return Array.from(container.querySelectorAll("[data-trajectory-record]"), (record) =>
        record.getAttribute("data-trajectory-record"),
      );
    };
    const call = entries.find((entry) => entry.type === "model" && entry.model.state === "unknown");
    if (call?.type !== "model") throw new Error("state records must contain an unknown call");

    expect(shown(lost)).toEqual(["run:run_lost"]);
    expect(shown(denied)).toEqual(["item:item_refused"]);
    expect(shown(inline)).toEqual(["item:item_refused"]);
    expect(shown(approved)).toEqual(["item:item_permitted"]);
    expect(shown(otherLocale)).toEqual([]);
    expect(shown(`${denied} `)).toEqual([]);
    expect(shown("lost")).toEqual(["run:run_lost"]);
    expect(shown('"approvalDecision":"deny"')).toEqual(["item:item_refused"]);
    expect(shown("item_refused")).toEqual(["item:item_refused"]);
    expect(shown(unknown, "model")).toEqual(["model:call_unknown"]);
    expect(shown(unknown, "run")).toEqual([]);
    expect(shown(denied, "toolCall")).toEqual(["item:item_refused"]);
    expect(shown(unknown, "all", "run_lost")).toEqual([]);
    expect(shown(unknown, "attention", call.model.runId)).toEqual(["model:call_unknown"]);
    const status = container.querySelector<HTMLElement>('[role="status"]')!;
    for (const text of pageCounts) expect(status.textContent).toContain(text);
    expect(entries).toEqual(recorded);
  },
);

it("derives readable search labels from the current locale without rewriting the query", () => {
  const entries = stateRecords();
  const recorded = structuredClone(entries);
  const catalog = { en: trajectoryEn, zh: trajectoryZh };
  const report = (locale: string, search: string) => (
    <TrajectoryReport
      pageNumber={1}
      onClearFilters={vi.fn()}
      t={createTranslator(catalog, locale)}
      locale={locale}
      page={{ data: entries }}
      kind="all"
      run="all"
      search={search}
    />
  );
  const { container, rerender } = testClient.render(report("en", "Lost"));
  const shown = () =>
    Array.from(container.querySelectorAll("[data-trajectory-record]"), (record) =>
      record.getAttribute("data-trajectory-record"),
    );

  expect(shown()).toEqual(["run:run_lost"]);
  rerender(report("zh", "Lost"));
  expect(shown()).toEqual(["run:run_lost"]);
  rerender(report("zh", "无法恢复"));
  expect(shown()).toEqual(["run:run_lost"]);
  rerender(report("en", "无法恢复"));
  expect(shown()).toEqual([]);
  rerender(report("zh", "审批：已拒绝"));
  expect(shown()).toEqual(["item:item_refused"]);
  rerender(report("en", "审批：已拒绝"));
  expect(shown()).toEqual([]);
  rerender(report("en", "Approval: Denied"));
  expect(shown()).toEqual(["item:item_refused"]);
  expect(entries).toEqual(recorded);
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
  testClient.render(
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
    expect(trigger.textContent).toContain("Completed");
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
  const { container } = testClient.render(
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
  const { container, rerender } = testClient.render(report(""));
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

const validRecordedImage =
  "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=";
const unreadableRecordedImage = btoa("not a PNG image");

function imageRecord(
  data: string,
  type: "userMessage" | "agentMessage" = "userMessage",
): TrajectoryEntry {
  return {
    type: "item",
    occurredAt: "2026-07-31T08:00:00.000Z",
    item: {
      type,
      id: "item_image_decode",
      runId: "run_root",
      status: "completed",
      createdAt: "2026-07-31T08:00:00.000Z",
      content: [
        { type: "text", text: "Before the recorded image." },
        { type: "image", mime: "image/png", data },
        { type: "text", text: "The next image remains readable." },
        { type: "image", mime: "image/png", data: validRecordedImage },
      ],
    },
  };
}

function imageReport(record: TrajectoryEntry, locale = "en") {
  return (
    <TrajectoryReport
      pageNumber={1}
      onClearFilters={vi.fn()}
      t={createTranslator({ en: trajectoryEn, zh: trajectoryZh }, locale)}
      locale={locale}
      page={{ data: [record] }}
      kind="all"
      run="all"
      search=""
    />
  );
}

it.each(["userMessage", "agentMessage"] as const)(
  "names a failed %s image while retaining its status, neighboring image and exact evidence",
  (type) => {
    const record = imageRecord(unreadableRecordedImage, type);
    const { container } = testClient.render(imageReport(record));
    const scope = within(
      container.querySelector('[data-trajectory-record="item:item_image_decode"]')!,
    );
    const summary = scope.getByRole("button", { expanded: false });
    fireEvent.click(summary);
    const [failed, neighbor] = scope.getAllByRole("img", { name: "Recorded image/png image" });
    fireEvent.error(failed!);
    expect(scope.getByText("This recorded image could not be displayed.")).toBeTruthy();
    expect(
      scope.getAllByRole("button", { name: "View image: Recorded image/png image" }),
    ).toHaveLength(1);
    expect(scope.getAllByRole("img", { name: "Recorded image/png image" })).toContain(neighbor);
    expect(neighbor).toHaveProperty("src", "data:image/png;base64," + validRecordedImage);
    expect(summary.textContent).toContain("Completed");
    expect(scope.getByText("Before the recorded image.")).toBeTruthy();
    expect(scope.getByText("The next image remains readable.")).toBeTruthy();
    fireEvent.click(scope.getByRole("button", { name: "Recorded evidence", expanded: false }));
    expect(scope.getByRole("region", { name: "Recorded evidence" }).textContent).toBe(
      JSON.stringify(record, null, 2),
    );
    expect(scope.queryByRole("alert")).toBeNull();
  },
);

it("ends an image's failure witness when its recorded source is replaced", () => {
  const view = testClient.render(imageReport(imageRecord(unreadableRecordedImage)));
  const record = within(
    view.container.querySelector('[data-trajectory-record="item:item_image_decode"]')!,
  );
  fireEvent.click(record.getByRole("button", { expanded: false }));
  fireEvent.error(screen.getAllByRole("img", { name: "Recorded image/png image" })[0]!);
  expect(screen.getByText("This recorded image could not be displayed.")).toBeTruthy();
  view.rerender(imageReport(imageRecord(validRecordedImage)));
  expect(screen.queryByText("This recorded image could not be displayed.")).toBeNull();
  expect(
    screen.getAllByRole("button", { name: "View image: Recorded image/png image" }),
  ).toHaveLength(2);
  expect(
    screen
      .getAllByRole("img", { name: "Recorded image/png image" })
      .every(
        (image) => image.getAttribute("src") === "data:image/png;base64," + validRecordedImage,
      ),
  ).toBe(true);
});

it("keeps a recorded image failure and its identity readable in Chinese", () => {
  const { container } = testClient.render(imageReport(imageRecord(unreadableRecordedImage), "zh"));
  const record = within(
    container.querySelector('[data-trajectory-record="item:item_image_decode"]')!,
  );
  fireEvent.click(record.getByRole("button", { expanded: false }));
  fireEvent.error(screen.getAllByRole("img", { name: "已记录的 image/png 图像" })[0]!);
  expect(screen.getByText("无法显示这张已记录的图片。")).toBeTruthy();
  expect(screen.getAllByRole("button", { name: "查看图片: 已记录的 image/png 图像" })).toHaveLength(
    1,
  );
});
