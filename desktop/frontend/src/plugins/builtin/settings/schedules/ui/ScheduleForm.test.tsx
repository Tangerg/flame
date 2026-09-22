import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { ScheduleForm } from "./ScheduleForm";

const { updateSchedule } = vi.hoisted(() => ({
  updateSchedule: vi.fn().mockResolvedValue(undefined),
}));

vi.mock("../application/scheduleCommands", () => ({
  createSchedule: vi.fn(),
  updateSchedule,
}));

describe("ScheduleForm", () => {
  it("submits the version the draft was opened from after a background refresh", async () => {
    const schedule = {
      id: "sch_1",
      title: "Review",
      instructions: "Original instructions",
      cron: "0 9 * * 1",
      enabled: true,
      revision: 7,
    };
    const onDone = vi.fn();
    const onCancel = vi.fn();
    const { rerender } = render(
      <ScheduleForm schedule={schedule} onDone={onDone} onCancel={onCancel} />,
    );
    fireEvent.change(screen.getByDisplayValue("Review"), {
      target: { value: "Weekly review" },
    });
    rerender(
      <ScheduleForm
        schedule={{ ...schedule, instructions: "Updated elsewhere", revision: 8 }}
        onDone={onDone}
        onCancel={onCancel}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(onDone).toHaveBeenCalledOnce());
    expect(updateSchedule).toHaveBeenCalledWith({
      id: "sch_1",
      title: "Weekly review",
      instructions: "Original instructions",
      cron: "0 9 * * 1",
      cwd: "",
      revision: 7,
    });
  });
});
