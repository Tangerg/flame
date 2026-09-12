import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { drainBrowserTasks } from "@/test/browserTasks";
import { ActivePlan } from "./ActivePlan";

const model = vi.hoisted(() => ({
  sessionId: "ses-a",
  generation: 1n,
  revision: 1,
  running: true,
  steps: [
    {
      id: "step-1",
      text: "Inspect",
      status: "active" as "active" | "done" | "pending",
    },
  ],
}));

vi.mock("@/plugins/builtin/agent/public/run", () => ({
  useIsCurrentRootRunning: () => model.running,
}));

vi.mock("@/plugins/builtin/agent/public/plan", async (importOriginal) => {
  const original = await importOriginal<typeof import("@/plugins/builtin/agent/public/plan")>();
  return {
    ...original,
    useSessionPlan: () =>
      original.SessionPlan.fromSnapshot(model.sessionId, model.generation, {
        revision: model.revision,
        steps: model.steps,
      }),
  };
});

describe("ActivePlan", () => {
  beforeEach(() => {
    model.sessionId = "ses-a";
    model.generation = 1n;
    model.revision = 1;
    model.running = true;
    model.steps = [{ id: "step-1", text: "Inspect", status: "active" }];
  });

  afterEach(async () => {
    cleanup();
    await drainBrowserTasks();
  });

  it("updates the compact progress with the authoritative replacement", () => {
    const { rerender } = render(<ActivePlan />);
    expect(screen.getByRole("button", { name: "Step 1 / 1" })).toBeTruthy();

    model.revision = 2;
    model.steps = [
      { id: "step-1", text: "Inspect", status: "done" as const },
      { id: "step-2", text: "Fix", status: "active" as const },
    ];
    rerender(<ActivePlan />);
    expect(screen.getByRole("button", { name: "Step 2 / 2" })).toBeTruthy();
  });

  it("keeps the focused checklist open while the accepted Plan advances", async () => {
    const { rerender } = render(<ActivePlan />);
    act(() => screen.getByRole("button", { name: "Step 1 / 1" }).focus());
    await waitFor(() => expect(screen.getByRole("tooltip").textContent).toContain("Inspect"));

    model.revision = 2;
    model.steps = [
      { id: "step-1", text: "Inspect", status: "done" },
      { id: "step-2", text: "Fix", status: "active" },
    ];
    rerender(<ActivePlan />);

    await waitFor(() => {
      expect(document.activeElement).toBe(screen.getByRole("button", { name: "Step 2 / 2" }));
      expect(screen.getByRole("tooltip").textContent).toContain("Fix");
    });
  });

  it.each(["session", "projection"] as const)(
    "closes the previous checklist when the %s changes",
    async (boundary) => {
      const { rerender } = render(<ActivePlan />);
      act(() => screen.getByRole("button", { name: "Step 1 / 1" }).focus());
      await waitFor(() => expect(screen.getByRole("tooltip").textContent).toContain("Inspect"));

      if (boundary === "session") model.sessionId = "ses-b";
      else model.generation = 2n;
      model.steps = [{ id: "step-1", text: "Other work", status: "active" }];
      rerender(<ActivePlan />);

      await waitFor(() => {
        expect(screen.queryByRole("tooltip")).toBeNull();
        expect(document.activeElement).not.toBe(screen.getByRole("button", { name: "Step 1 / 1" }));
      });
    },
  );

  it("shows a non-dismissible Plan only while its current Run is active", async () => {
    const { rerender } = render(<ActivePlan />);

    expect(screen.queryByRole("button", { name: "Dismiss plan banner" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Step 1 / 1" })).not.toBeNull();

    model.running = false;
    rerender(<ActivePlan />);

    await waitFor(() => {
      expect(screen.queryByRole("button", { name: "Step 1 / 1" })).toBeNull();
    });
  });

  it("uses Codex's compact step pill instead of a relocated Plan card", () => {
    model.steps = [
      { id: "step-1", text: "Inspect", status: "done" },
      { id: "step-2", text: "Fix", status: "active" },
      { id: "step-3", text: "Verify", status: "pending" },
    ];

    const { container } = render(<ActivePlan />);

    expect(screen.getByText("Step 2 / 3")).toBeTruthy();
    expect(screen.queryByRole("progressbar")).toBeNull();
    expect(screen.queryByRole("button", { name: /Expand plan/ })).toBeNull();
    expect(container.querySelector('[data-slot="active-plan-surface"]')).not.toBeNull();
    expect(container.querySelector('[data-slot="active-plan-pill"]')).not.toBeNull();
  });
});
