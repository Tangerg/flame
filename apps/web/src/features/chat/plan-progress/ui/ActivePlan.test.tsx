import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { drainBrowserTasks } from "@/test/browserTasks";
import { ActivePlan } from "@/features/chat/plan-progress/ui/ActivePlan";
import { ContentVisibility } from "@/ui/content-visibility";
const model = vi.hoisted(() => ({
  sessionId: "ses-a",
  generation: 1n,
  revision: 1,
  steps: [
    {
      id: "step-1",
      text: "Inspect",
      status: "active" as "active" | "done" | "pending",
    },
  ],
}));

vi.mock("@/features/agent/public/plan", async (importOriginal) => {
  const original = await importOriginal<typeof import("@/features/agent/public/plan")>();
  return {
    ...original,
    useSessionPlan: () =>
      original.SessionPlan.fromSnapshot(model.sessionId, model.generation, {
        revision: model.revision,
        steps: model.steps,
      }),
  };
});

async function openPlan(label = "Step 1 / 1") {
  const trigger = screen.getByRole("button", { name: label });
  act(() => trigger.focus());
  fireEvent.click(trigger);
  const popup = await screen.findByRole("dialog", { name: "Plan" });
  const reader = within(popup).getByRole("region", { name: /complete/ });
  await waitFor(() => expect(document.activeElement).toBe(reader));
  return { trigger, popup, reader };
}

describe("ActivePlan", () => {
  beforeEach(() => {
    model.sessionId = "ses-a";
    model.generation = 1n;
    model.revision = 1;
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
    const { popup, reader } = await openPlan();
    reader.scrollTop = 96;

    model.revision = 2;
    model.steps = [
      { id: "step-1", text: "Inspect", status: "done" },
      { id: "step-2", text: "Fix", status: "active" },
    ];
    rerender(<ActivePlan />);

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Step 2 / 2" })).toBeTruthy();
      expect(screen.getByRole("dialog", { name: "Plan" })).toBe(popup);
      expect(document.activeElement).toBe(reader);
      expect(reader.scrollTop).toBe(96);
      expect(reader.textContent).toContain("Fix");
      expect(within(reader).getByText("Fix").closest("li")?.getAttribute("aria-current")).toBe(
        "step",
      );
    });
  });

  it.each(["session", "projection"] as const)(
    "closes the previous checklist when the %s changes",
    async (boundary) => {
      const { rerender } = render(<ActivePlan />);
      await openPlan();

      if (boundary === "session") model.sessionId = "ses-b";
      else model.generation = 2n;
      model.steps = [{ id: "step-1", text: "Other work", status: "active" }];
      rerender(<ActivePlan />);

      await waitFor(() => {
        expect(screen.queryByRole("dialog", { name: "Plan" })).toBeNull();
        expect(document.activeElement).not.toBe(screen.getByRole("button", { name: "Step 1 / 1" }));
      });
    },
  );

  it("keeps a non-dismissible finished Plan reviewable between turns", async () => {
    const { rerender } = render(<ActivePlan />);

    expect(screen.queryByRole("button", { name: "Dismiss plan banner" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Step 1 / 1" })).not.toBeNull();

    model.revision = 2;
    model.steps = [{ id: "step-1", text: "Inspect", status: "done" }];
    rerender(<ActivePlan />);

    // No step is in flight, so the pill states the completion count instead of a
    // position, and stays put rather than taking the plan away with the Run.
    await waitFor(() => {
      expect(screen.queryByRole("button", { name: "1 of 1 complete" })).not.toBeNull();
    });
    const { reader } = await openPlan("1 of 1 complete");
    expect(reader.textContent).toContain("Inspect");
    expect(reader.querySelector("[aria-current]")).toBeNull();
  });

  it("stays down when the session has no plan", async () => {
    model.steps = [];
    render(<ActivePlan />);

    await waitFor(() => {
      expect(screen.queryByRole("button", { name: /complete|Step/ })).toBeNull();
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

  it("opens the complete named reader only after explicit activation", async () => {
    model.steps = Array.from({ length: 12 }, (_, index) => ({
      id: `step-${index + 1}`,
      text: index === 11 ? "Final acceptance — 末尾验收" : `Review ${index + 1}`,
      status: index === 0 ? "active" : "pending",
    }));
    render(<ActivePlan />);
    const trigger = screen.getByRole("button", { name: "Step 1 / 12" });
    act(() => trigger.focus());
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(screen.queryByRole("tooltip")).toBeNull();

    const { reader } = await openPlan("Step 1 / 12");
    expect(trigger.getAttribute("aria-expanded")).toBe("true");
    expect(within(reader).getAllByRole("listitem")).toHaveLength(12);
    expect(within(reader).getByText("Final acceptance — 末尾验收")).toBeTruthy();
  });

  it("returns to its original pill on Escape and opens a fresh reading task", async () => {
    render(<ActivePlan />);
    const { trigger, reader } = await openPlan();

    fireEvent.keyDown(reader, { key: "Escape" });

    await waitFor(() => {
      expect(screen.queryByRole("dialog", { name: "Plan" })).toBeNull();
      expect(document.activeElement).toBe(trigger);
      expect(trigger.getAttribute("aria-expanded")).toBe("false");
    });
    const reopened = await openPlan();
    expect(reopened.trigger).toBe(trigger);
    expect(reopened.reader).not.toBe(reader);
  });

  it("preserves later outside focus when a reader closes", async () => {
    render(
      <>
        <button>Later control</button>
        <ActivePlan />
      </>,
    );
    const { reader } = await openPlan();
    const later = screen.getByRole("button", { name: "Later control" });

    fireEvent.keyDown(reader, { key: "Escape" });
    act(() => later.focus());

    await waitFor(() => expect(screen.queryByRole("dialog", { name: "Plan" })).toBeNull());
    await drainBrowserTasks();
    expect(document.activeElement).toBe(later);
  });

  it("retires an open reader when the accepted Plan is removed", async () => {
    const { rerender } = render(<ActivePlan />);
    const { reader } = await openPlan();

    model.steps = [];
    rerender(<ActivePlan />);

    await waitFor(() => {
      expect(screen.queryByRole("dialog", { name: "Plan" })).toBeNull();
      expect(screen.queryByRole("button", { name: /complete|Step/ })).toBeNull();
      expect(document.activeElement).not.toBe(reader);
    });
  });

  it("retires an unmounted reader without reclaiming later focus", async () => {
    const scene = (mounted: boolean) => (
      <>
        <button>Later control</button>
        {mounted && <ActivePlan />}
      </>
    );
    const { rerender } = render(scene(true));
    await openPlan();
    const later = screen.getByRole("button", { name: "Later control" });

    act(() => later.focus());
    rerender(scene(false));

    await waitFor(() => expect(screen.queryByRole("dialog", { name: "Plan" })).toBeNull());
    await drainBrowserTasks();
    expect(document.activeElement).toBe(later);
  });

  it("retires a hidden conversation's reader and requires an explicit reopen on return", async () => {
    const scene = (visible: boolean) => (
      <div hidden={!visible}>
        <ContentVisibility visible={visible}>
          <ActivePlan />
        </ContentVisibility>
      </div>
    );
    const { rerender } = render(scene(true));
    await openPlan();

    rerender(scene(false));

    await waitFor(() => expect(screen.queryByRole("dialog", { name: "Plan" })).toBeNull());
    rerender(scene(true));
    expect(screen.getByRole("button", { name: "Step 1 / 1" })).toBeTruthy();
    expect(screen.queryByRole("dialog", { name: "Plan" })).toBeNull();
    await openPlan();
  });
});
