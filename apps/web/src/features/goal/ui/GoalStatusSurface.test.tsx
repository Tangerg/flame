import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { RpcError } from "@flame/runtime-client";
import { drainBrowserTasks } from "@/test/browserTasks";
import { type GoalReadModel } from "@/features/goal/application/goalReadModel";
import { GoalStatusSurface } from "@/features/goal/ui/GoalStatusSurface";
const model = vi.hoisted(() => ({
  clearGoal: vi.fn(async () => {}),
  updateGoal: vi.fn(async () => {}),
  stopGoal: vi.fn(async () => {}),
  resumeGoal: vi.fn(async () => {}),
  runtimeAvailable: true,
  generation: 1,
  goal: {
    sessionId: "session-a",
    objective: "Ship alpha",
    status: "active",
    used: { runs: 1, costUsd: 0, steps: 2 },
    createdAt: "2026-08-12T08:00:00Z",
  } as GoalReadModel,
}));

vi.mock("@/features/goal/application/goalCommands", () => ({
  clearGoal: model.clearGoal,
  updateGoal: model.updateGoal,
  stopGoal: model.stopGoal,
  resumeGoal: model.resumeGoal,
}));

vi.mock("@/features/connection/public/serviceStatus", () => ({
  runtimeCommandsAvailable: () => model.runtimeAvailable,
  useRuntimeCommandsAvailable: () => model.runtimeAvailable,
}));

vi.mock("@/features/goal/application/goalReadModel", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/features/goal/application/goalReadModel")>()),
  useGoalMaterial: () => ({
    generation: model.generation,
    value: { available: true, goal: model.goal },
  }),
}));

describe("Goal status surface", () => {
  afterEach(async () => {
    cleanup();
    vi.useRealTimers();
    await drainBrowserTasks();
  });

  beforeEach(() => {
    model.goal = {
      sessionId: "session-a",
      objective: "Ship alpha",
      status: "active",
      used: { runs: 1, costUsd: 0, steps: 2 },
      createdAt: "2026-08-12T08:00:00Z",
    };
    model.stopGoal.mockClear();
    model.resumeGoal.mockClear();
    model.clearGoal.mockClear();
    model.updateGoal.mockClear();
    model.runtimeAvailable = true;
    model.generation = 1;
  });

  it("pauses an active goal from the standing surface", async () => {
    render(<GoalStatusSurface />);
    fireEvent.click(screen.getByRole("button", { name: "Pause goal" }));
    await vi.waitFor(() => expect(model.stopGoal).toHaveBeenCalledWith("session-a"));
    expect(model.resumeGoal).not.toHaveBeenCalled();
  });

  it("offers Flame Goal management actions in clear, lifecycle, edit order", () => {
    const { container } = render(<GoalStatusSurface />);

    expect(
      Array.from(container.querySelectorAll('[data-slot="goal-actions"] button')).map((button) =>
        button.getAttribute("aria-label"),
      ),
    ).toEqual(["Clear goal", "Pause goal", "Edit goal"]);
  });

  it("opens the same objective editor from the Goal summary", () => {
    render(<GoalStatusSurface />);

    fireEvent.click(screen.getByRole("button", { name: "Pursuing goal Ship alpha" }));

    expect(screen.getByRole("dialog", { name: "Edit goal" })).toBeTruthy();
  });

  it("clears the current goal through the Goal command owner", async () => {
    render(<GoalStatusSurface />);

    fireEvent.click(screen.getByRole("button", { name: "Clear goal" }));

    await vi.waitFor(() => expect(model.clearGoal).toHaveBeenCalledWith("session-a"));
  });

  it("edits only the objective in a Codex-style Goal dialog", async () => {
    render(<GoalStatusSurface />);

    fireEvent.click(screen.getByRole("button", { name: "Edit goal" }));
    const objective = screen.getByRole("textbox", { name: "Goal" });
    expect((objective as HTMLTextAreaElement).value).toBe("Ship alpha");

    fireEvent.change(objective, { target: { value: "  Ship beta  " } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await vi.waitFor(() =>
      expect(model.updateGoal).toHaveBeenCalledWith({
        sessionId: "session-a",
        objective: "Ship beta",
      }),
    );
  });

  it("keeps an open editor aligned with the current Goal lifecycle", async () => {
    const { rerender } = render(<GoalStatusSurface />);
    fireEvent.click(screen.getByRole("button", { name: "Edit goal" }));
    fireEvent.change(screen.getByRole("textbox", { name: "Goal" }), {
      target: { value: "Ship beta" },
    });

    model.goal = { ...model.goal, status: "completing" };
    rerender(<GoalStatusSurface />);
    const save = screen.getByRole("button", { name: "Save" });
    expect(save.hasAttribute("disabled")).toBe(true);
    fireEvent.click(save);
    expect(model.updateGoal).not.toHaveBeenCalled();

    model.goal = { ...model.goal, status: "paused" };
    rerender(<GoalStatusSurface />);
    expect((screen.getByRole("textbox", { name: "Goal" }) as HTMLTextAreaElement).value).toBe(
      "Ship beta",
    );
    expect(screen.getByRole("button", { name: "Save" }).hasAttribute("disabled")).toBe(false);
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await vi.waitFor(() =>
      expect(model.updateGoal).toHaveBeenCalledWith({
        sessionId: "session-a",
        objective: "Ship beta",
      }),
    );
  });

  const rejected = () =>
    new RpcError({ message: "objective rejected", data: { type: "invalid_request" } });

  async function saveSettled() {
    await vi.waitFor(() =>
      expect(screen.getByRole("button", { name: "Save" }).hasAttribute("disabled")).toBe(false),
    );
  }

  it("keeps a rejected objective edit in its editor and saves it on retry", async () => {
    model.updateGoal.mockRejectedValueOnce(rejected());
    render(<GoalStatusSurface />);
    fireEvent.click(screen.getByRole("button", { name: "Edit goal" }));
    const objective = screen.getByRole("textbox", { name: "Goal" }) as HTMLTextAreaElement;
    fireEvent.change(objective, { target: { value: "Ship beta" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    const dialog = screen.getByRole("dialog", { name: "Edit goal" });
    const alert = await within(dialog).findByRole("alert");
    expect(alert.textContent).toContain("objective rejected");
    expect(objective.getAttribute("aria-describedby")).toBe(alert.id);
    expect(objective.value).toBe("Ship beta");
    await saveSettled();

    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(within(dialog).queryByRole("alert")).toBeNull();
    await vi.waitFor(() => expect(screen.queryByRole("dialog", { name: "Edit goal" })).toBeNull());
    expect(model.updateGoal).toHaveBeenCalledTimes(2);
    expect(model.updateGoal).toHaveBeenLastCalledWith({
      sessionId: "session-a",
      objective: "Ship beta",
    });
  });

  it("clears an editor failure when the draft changes or the editor reopens", async () => {
    model.updateGoal.mockRejectedValueOnce(rejected()).mockRejectedValueOnce(rejected());
    render(<GoalStatusSurface />);
    fireEvent.click(screen.getByRole("button", { name: "Edit goal" }));
    const objective = screen.getByRole("textbox", { name: "Goal" });
    fireEvent.change(objective, { target: { value: "Ship beta" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await screen.findByRole("alert");
    await saveSettled();

    fireEvent.change(objective, { target: { value: "Ship gamma" } });
    expect(screen.queryByRole("alert")).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await screen.findByRole("alert");
    await saveSettled();
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    await vi.waitFor(() => expect(screen.queryByRole("dialog", { name: "Edit goal" })).toBeNull());

    fireEvent.click(screen.getByRole("button", { name: "Edit goal" }));
    expect(screen.getByRole("dialog", { name: "Edit goal" })).toBeTruthy();
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("keeps an offline objective save in the editor without invoking the command", async () => {
    model.runtimeAvailable = false;
    const { rerender } = render(<GoalStatusSurface />);
    fireEvent.click(screen.getByRole("button", { name: "Pursuing goal Ship alpha" }));
    const objective = screen.getByRole("textbox", { name: "Goal" });
    fireEvent.change(objective, { target: { value: "Ship beta" } });

    const save = screen.getByRole("button", { name: "Save" });
    expect(save.hasAttribute("disabled")).toBe(true);
    fireEvent.click(save);
    fireEvent.keyDown(objective, { key: "Enter", code: "Enter", ctrlKey: true });
    expect(model.updateGoal).not.toHaveBeenCalled();

    model.runtimeAvailable = true;
    rerender(<GoalStatusSurface />);
    model.runtimeAvailable = false;
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    const dialog = screen.getByRole("dialog", { name: "Edit goal" });
    expect(within(dialog).getByRole("alert")).toBeTruthy();
    expect((objective as HTMLTextAreaElement).value).toBe("Ship beta");
    expect(model.updateGoal).not.toHaveBeenCalled();
  });

  it("refuses to save an objective that is only whitespace", () => {
    render(<GoalStatusSurface />);

    fireEvent.click(screen.getByRole("button", { name: "Edit goal" }));
    fireEvent.change(screen.getByRole("textbox", { name: "Goal" }), { target: { value: "   " } });
    const save = screen.getByRole("button", { name: "Save" });

    expect(save.getAttribute("disabled")).not.toBeNull();
    fireEvent.click(save);
    expect(model.updateGoal).not.toHaveBeenCalled();
  });

  it("does not submit the Goal editor while an IME composition is active", () => {
    render(<GoalStatusSurface />);

    fireEvent.click(screen.getByRole("button", { name: "Edit goal" }));
    const objective = screen.getByRole("textbox", { name: "Goal" });
    fireEvent.change(objective, { target: { value: "Ship beta" } });
    fireEvent.keyDown(objective, {
      key: "Enter",
      code: "Enter",
      ctrlKey: true,
      isComposing: true,
    });

    expect(model.updateGoal).not.toHaveBeenCalled();
  });

  it("resumes a paused goal from the standing surface", async () => {
    model.goal = { ...model.goal, status: "paused" };
    render(<GoalStatusSurface />);
    fireEvent.click(screen.getByRole("button", { name: "Resume goal" }));
    await vi.waitFor(() => expect(model.resumeGoal).toHaveBeenCalledWith("session-a"));
    expect(model.stopGoal).not.toHaveBeenCalled();
  });

  it("keeps lifecycle commands visible but inert while Runtime is offline", () => {
    model.runtimeAvailable = false;
    render(<GoalStatusSurface />);

    const pause = screen.getByRole("button", { name: "Pause goal" });
    expect(pause.hasAttribute("disabled")).toBe(true);
    fireEvent.click(pause);
    expect(model.stopGoal).not.toHaveBeenCalled();
  });

  it("keeps a completing goal visible without exposing a lifecycle command", () => {
    model.goal = { ...model.goal, status: "completing" };
    render(<GoalStatusSurface />);

    expect(screen.getByText("Finishing goal")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Stop goal" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Resume goal" })).toBeNull();
    expect(model.stopGoal).not.toHaveBeenCalled();
    expect(model.resumeGoal).not.toHaveBeenCalled();
  });

  it("keeps the plain status word when the Runtime would still resume", () => {
    model.goal = {
      ...model.goal,
      status: "paused",
    };
    render(<GoalStatusSurface />);

    expect(screen.getByRole("button", { name: "Resume goal" })).toBeTruthy();
    expect(screen.getByText("Paused goal")).toBeTruthy();
  });

  it("shows the Runtime stop reason and safe model detail without starting work", () => {
    model.goal = {
      ...model.goal,
      status: "blocked",
      reason: {
        code: "blockedByModel",
        detail: "Provide the release target.\nThen I can continue.",
      },
    };
    render(<GoalStatusSurface />);

    const explanation = screen.getByRole("button", {
      name: "The agent reported that it cannot continue.",
    });
    expect(explanation.getAttribute("aria-expanded")).toBe("false");
    expect(screen.queryByText(/Provide the release target/)).toBeNull();
    fireEvent.click(explanation);
    expect(explanation.getAttribute("aria-expanded")).toBe("true");
    expect(screen.getByText(/Provide the release target/)).toBeTruthy();
    expect(model.resumeGoal).not.toHaveBeenCalled();
    expect(screen.getByRole("button", { name: "Resume goal" })).toBeTruthy();
  });

  it("replaces the explanation from current material and removes it when the owner resumes", () => {
    model.goal = { ...model.goal, status: "paused", reason: { code: "runtimeRestarted" } };
    const { rerender } = render(<GoalStatusSurface />);
    expect(screen.getByText("Paused after the execution service restarted.")).toBeTruthy();

    model.goal = { ...model.goal, reason: { code: "awaitingInput" } };
    rerender(<GoalStatusSurface />);
    expect(screen.queryByText("Paused after the execution service restarted.")).toBeNull();
    expect(screen.getByText("Waiting for your answer or approval.")).toBeTruthy();

    const { reason: _reason, ...active } = model.goal;
    model.goal = { ...active, status: "active" };
    rerender(<GoalStatusSurface />);
    expect(screen.queryByText("Waiting for your answer or approval.")).toBeNull();
    expect(screen.getByText("Pursuing goal")).toBeTruthy();
    expect(model.resumeGoal).not.toHaveBeenCalled();
  });

  it("keeps an old Goal explanation out of a replacement Session", () => {
    model.goal = {
      ...model.goal,
      status: "paused",
      reason: { code: "runNotCompleted", detail: "canceled" },
    };
    const { rerender } = render(<GoalStatusSurface />);
    expect(screen.getByText("The previous run did not finish successfully.")).toBeTruthy();
    model.goal = {
      sessionId: "session-b",
      objective: "Ship beta",
      status: "active",
      used: { runs: 0, steps: 0 },
      createdAt: "2026-10-14T00:00:00Z",
    };
    rerender(<GoalStatusSurface />);
    expect(screen.queryByText("The previous run did not finish successfully.")).toBeNull();
    expect(screen.getByText("Ship beta")).toBeTruthy();
  });

  it("presents Goal identity and creation age without usage counters", () => {
    render(<GoalStatusSurface />);

    expect(screen.getByText("Pursuing goal")).toBeTruthy();
    expect(screen.getByText("Ship alpha")).toBeTruthy();
    expect(screen.queryByText("Turns")).toBeNull();
    expect(screen.queryByText("Steps")).toBeNull();
  });

  it("uses the top-tray surface and the shared target icon", () => {
    const { container } = render(<GoalStatusSurface />);

    const surface = container.querySelector<HTMLElement>('[data-slot="composer-top-tray-surface"]');
    expect(surface).not.toBeNull();
    const glyph = surface?.querySelector('svg[data-icon-name="target"]');
    expect(glyph?.getAttribute("viewBox")).toBe("0 0 16 16");
    expect(glyph?.getAttribute("stroke-width")).toBe("1.25");
  });

  function controlClock() {
    vi.useFakeTimers({ toFake: ["Date", "setInterval", "clearInterval"] });
    vi.setSystemTime(new Date("2026-10-21T00:00:00Z"));
    model.goal = { ...model.goal, createdAt: new Date(Date.now() - 18_000).toISOString() };
  }

  it("reads creation age as time advances without changing the Goal", () => {
    controlClock();
    render(<GoalStatusSurface />);
    expect(screen.getByText("Time since goal creation: 18s")).toBeTruthy();

    act(() => vi.advanceTimersByTime(2_000));

    expect(screen.getByText("Time since goal creation: 20s")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Pursuing goal Ship alpha" })).toBeTruthy();
    expect(model.stopGoal).not.toHaveBeenCalled();
    expect(model.resumeGoal).not.toHaveBeenCalled();
    expect(model.updateGoal).not.toHaveBeenCalled();
    expect(model.clearGoal).not.toHaveBeenCalled();
  });

  it("keeps counting creation age after the Runtime pauses the Goal", () => {
    controlClock();
    const { rerender } = render(<GoalStatusSurface />);
    model.goal = { ...model.goal, status: "paused" };
    rerender(<GoalStatusSurface />);

    act(() => vi.advanceTimersByTime(10_000));

    expect(screen.getByText("Time since goal creation: 28s")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Resume goal" })).toBeTruthy();
    expect(model.resumeGoal).not.toHaveBeenCalled();
  });

  it("leaves the current objective draft and focus alone while time advances", async () => {
    controlClock();
    const { rerender } = render(<GoalStatusSurface />);
    const opener = screen.getByRole("button", { name: "Edit goal" });
    act(() => opener.focus());
    fireEvent.click(opener);
    const objective = screen.getByRole("textbox", { name: "Goal" });
    fireEvent.change(objective, { target: { value: "Keep my exact draft 草稿" } });
    act(() => objective.focus());
    model.goal = { ...model.goal, objective: "Accepted update" };
    rerender(<GoalStatusSurface />);

    act(() => vi.advanceTimersByTime(2_000));

    expect((objective as HTMLTextAreaElement).value).toBe("Keep my exact draft 草稿");
    expect(document.activeElement).toBe(objective);
    expect(screen.getByText("Time since goal creation: 20s")).toBeTruthy();
    fireEvent.keyDown(objective, { key: "Escape" });
    await vi.waitFor(() => {
      expect(screen.queryByRole("dialog", { name: "Edit goal" })).toBeNull();
      expect(document.activeElement).toBe(screen.getByRole("button", { name: "Edit goal" }));
    });
    expect(model.updateGoal).not.toHaveBeenCalled();
  });

  it("reads a replacement Goal's own creation time and retires the former editor", () => {
    controlClock();
    const { rerender } = render(<GoalStatusSurface />);
    fireEvent.click(screen.getByRole("button", { name: "Edit goal" }));
    model.goal = {
      ...model.goal,
      sessionId: "session-b",
      objective: "Ship beta",
      createdAt: new Date(Date.now() - 5_000).toISOString(),
    };

    rerender(<GoalStatusSurface />);

    expect(screen.getByText("Time since goal creation: 5s")).toBeTruthy();
    expect(screen.queryByText("Time since goal creation: 18s")).toBeNull();
    expect(screen.queryByRole("dialog", { name: "Edit goal" })).toBeNull();
    expect(model.updateGoal).not.toHaveBeenCalled();
  });

  it("omits an unreadable creation time", () => {
    controlClock();
    model.goal = { ...model.goal, createdAt: "invalid" };
    render(<GoalStatusSurface />);

    expect(screen.queryByText(/^Time since goal creation:/)).toBeNull();
    expect(screen.getByRole("button", { name: "Pursuing goal Ship alpha" })).toBeTruthy();
  });

  it("clamps a future creation timestamp to zero until the clock reaches it", () => {
    controlClock();
    model.goal = { ...model.goal, createdAt: new Date(Date.now() + 2_000).toISOString() };
    render(<GoalStatusSurface />);
    expect(screen.getByText("Time since goal creation: 0s")).toBeTruthy();

    act(() => vi.advanceTimersByTime(3_000));

    expect(screen.getByText("Time since goal creation: 1s")).toBeTruthy();
  });
});
