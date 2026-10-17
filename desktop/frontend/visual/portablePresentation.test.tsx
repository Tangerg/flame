import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { MemoryReport } from "../../../plugins/memory/src/component";
import { TrajectoryReport } from "../../../plugins/trajectory/src/component";
import { RUNTIME_AGENT_SESSION_SNAPSHOTS } from "./agentSessionSnapshots";
import { VISUAL_MEMORY_PAGE } from "./componentSnapshots";
import { visualTrajectory } from "./trajectorySnapshot";

afterEach(cleanup);

it("requires an explicit confirmation before deleting the captured memory", async () => {
  const command = vi.fn();
  render(
    <MemoryReport
      page={VISUAL_MEMORY_PAGE}
      disabled={false}
      edit={vi.fn()}
      command={command}
      search=""
      status="all"
      origin="all"
      pinned={false}
    />,
  );
  const trigger = screen.getByRole("button", {
    name: `More actions for ${VISUAL_MEMORY_PAGE.Rows[1]!.ID}`,
  });
  fireEvent.click(trigger);
  fireEvent.click(await screen.findByRole("menuitem", { name: "Delete" }));
  expect(command).not.toHaveBeenCalled();
  expect(screen.getByRole("region", { name: "Confirm memory deletion" }).textContent).toContain(
    VISUAL_MEMORY_PAGE.Rows[1]!.Content,
  );
  const cancel = screen.getByRole("button", { name: "Cancel" });
  cancel.focus();
  fireEvent.click(cancel);
  expect(command).not.toHaveBeenCalled();
  expect(screen.queryByRole("region", { name: "Confirm memory deletion" })).toBeNull();
  await waitFor(() => expect(document.activeElement).toBe(trigger));
  await waitFor(() => expect(screen.queryByRole("menu")).toBeNull());
  fireEvent.click(trigger);
  fireEvent.click(await screen.findByRole("menuitem", { name: "Delete" }));
  fireEvent.click(screen.getByRole("button", { name: "Delete memory" }));
  expect(command).toHaveBeenCalledExactlyOnceWith("flame.memory/delete.v1", {
    ID: VISUAL_MEMORY_PAGE.Rows[1]!.ID,
  });
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
