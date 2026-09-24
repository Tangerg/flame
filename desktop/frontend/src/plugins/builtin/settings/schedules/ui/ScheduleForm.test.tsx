import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { SelectableModel } from "@/plugins/builtin/settings/providers/public/queries";
import { ScheduleForm } from "./ScheduleForm";

const { updateSchedule, createSchedule, useModels } = vi.hoisted(() => ({
  updateSchedule: vi.fn().mockResolvedValue(undefined),
  createSchedule: vi.fn().mockResolvedValue(undefined),
  useModels: vi.fn(),
}));

vi.mock("../application/scheduleCommands", () => ({
  createSchedule,
  updateSchedule,
}));

vi.mock("@/plugins/builtin/settings/providers/public/queries", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/plugins/builtin/settings/providers/public/queries")>()),
  useModels,
}));

beforeEach(() => {
  updateSchedule.mockClear();
  createSchedule.mockClear();
  useModels.mockReturnValue({
    data: [
      new SelectableModel({
        provider: "openai",
        id: "gpt-5",
        label: "Shared model",
        reasoning: true,
        reasoningLevels: ["low", "high"],
      }),
      new SelectableModel({ provider: "deepseek", id: "deepseek-chat", label: "Shared model" }),
    ],
    isLoading: false,
    isError: false,
    refetch: vi.fn(),
  });
});

describe("ScheduleForm", () => {
  it("creates a schedule with the chosen provider, model and reasoning effort", async () => {
    render(<ScheduleForm onDone={vi.fn()} onCancel={vi.fn()} />);
    fireEvent.change(screen.getByRole("textbox", { name: "Instructions to run…" }), {
      target: { value: "Review changes" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Switch model" }));
    fireEvent.click(await screen.findByRole("menuitem", { name: "OpenAI / Shared model" }));
    fireEvent.click(screen.getByRole("button", { name: "Switch reasoning effort" }));
    fireEvent.click(await screen.findByRole("menuitem", { name: "High" }));
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() =>
      expect(createSchedule).toHaveBeenCalledWith(
        expect.objectContaining({
          modelSelection: { provider: "openai", model: "gpt-5", reasoningEffort: "high" },
        }),
      ),
    );
  });

  it("keeps an unavailable saved model and can explicitly restore the Runtime default", async () => {
    const schedule = {
      id: "sch_1",
      title: "Review",
      instructions: "Review changes",
      cron: "0 9 * * 1",
      enabled: true,
      revision: 7,
      provider: "unavailable",
      model: "saved-model",
      reasoningEffort: "high",
    };
    render(<ScheduleForm schedule={schedule} onDone={vi.fn()} onCancel={vi.fn()} />);
    expect(screen.getByRole("button", { name: "Switch model" }).textContent).toContain(
      "saved-model",
    );
    expect(screen.getByRole("button", { name: "Switch reasoning effort" }).textContent).toContain(
      "High",
    );
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(updateSchedule).toHaveBeenCalledOnce());
    expect(updateSchedule.mock.calls[0]?.[0]).not.toHaveProperty("modelSelection");
    fireEvent.click(screen.getByRole("button", { name: "Switch model" }));
    fireEvent.click(await screen.findByRole("menuitem", { name: "Runtime default" }));
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() =>
      expect(updateSchedule).toHaveBeenLastCalledWith(
        expect.objectContaining({ modelSelection: null }),
      ),
    );
    expect(screen.queryByRole("button", { name: "Switch reasoning effort" })).toBeNull();
  });

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
