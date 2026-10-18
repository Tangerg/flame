import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { TrajectoryReport } from "../../../plugins/trajectory/src/component";
import { Markdown } from "../../../plugins/file-renderers/src/component";
import type { ComponentContext } from "@flame/plugin-sdk";
import { RUNTIME_AGENT_SESSION_SNAPSHOTS } from "./agentSessionSnapshots";
import { visualTrajectory } from "./trajectorySnapshot";

afterEach(cleanup);

it("preserves authored column alignment in captured Markdown tables", () => {
  const content = [
    "| Change | Checks | Owner |",
    "| :--- | ---: | :---: |",
    "| `safe()` | 18 | **Runtime** |",
  ].join("\n");
  const context: ComponentContext = {
    component: { installationId: "captured-file", digest: "a".repeat(64), componentId: "markdown" },
    signal: new AbortController().signal,
    file: {
      path: "review.md",
      content,
      startLine: 1,
      totalLines: 3,
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
  render(<Markdown context={context} />);
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
