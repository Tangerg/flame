import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { TrajectoryReport } from "../../../plugins/trajectory/src/component";
import { Markdown } from "../../../plugins/file-renderers/src/component";
import type { ComponentContext } from "@flame/plugin-sdk";
import type { ContentBlock, TrajectoryEntry } from "@flame/runtime-contract/wire";
import { RUNTIME_AGENT_SESSION_SNAPSHOTS } from "./agentSessionSnapshots";
import { visualTrajectory } from "./trajectorySnapshot";

afterEach(cleanup);

function renderCapturedMarkdown(content: string) {
  const context: ComponentContext = {
    component: { installationId: "captured-file", digest: "a".repeat(64), componentId: "markdown" },
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

it("shows tool execution status independently of its approval decision", () => {
  const row = visualTrajectory(RUNTIME_AGENT_SESSION_SNAPSHOTS.waiting).find(
    (entry) => entry.type === "item" && entry.item.type === "toolCall",
  );
  if (row?.type !== "item" || row.item.type !== "toolCall") {
    throw new Error("waiting snapshot must contain a tool call");
  }
  render(
    <TrajectoryReport
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
  const summary = screen.getByRole("button", { name: /incomplete/ });
  expect(summary.textContent).toContain("Approval: approve");
  expect(screen.queryByText(row.item.id, { exact: true })).toBeNull();
  fireEvent.click(summary);
  expect(summary.getAttribute("aria-expanded")).toBe("true");
  expect(screen.getByText(row.item.id, { exact: true }).textContent).toBe(row.item.id);
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
  render(<TrajectoryReport page={{ data: [root, child] }} kind="all" run="all" search="" />);

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
