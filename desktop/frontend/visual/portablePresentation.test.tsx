import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { TrajectoryReport } from "../../../plugins/trajectory/src/component";
import { RUNTIME_AGENT_SESSION_SNAPSHOTS } from "./agentSessionSnapshots";
import { visualTrajectory } from "./trajectorySnapshot";

afterEach(cleanup);

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
