import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { RunAnnouncer } from "./RunAnnouncer";

const material = vi.hoisted(() => ({
  current: { status: "idle", outcome: null } as {
    status: "idle" | "running" | "waiting" | "finished";
    outcome: { type: string } | null;
  },
}));
vi.mock("@/plugins/builtin/agent/public/run", () => ({
  useCurrentRootMaterial: () => material.current,
}));

afterEach(cleanup);

const region = () => screen.getByRole("status") as HTMLElement;

describe("run announcer", () => {
  it("is present and silent while nothing is running", () => {
    material.current = { status: "idle", outcome: null };
    render(<RunAnnouncer />);

    expect(region().textContent).toBe("");
    expect(region().getAttribute("aria-live")).toBe("polite");
  });

  it("says nothing about the state it was mounted in", () => {
    material.current = { status: "finished", outcome: null };
    render(<RunAnnouncer />);

    expect(region().textContent).toBe("");
  });

  it("says the state, and only the state", () => {
    material.current = { status: "running", outcome: null };
    const view = render(<RunAnnouncer />);
    expect(region().textContent).toBe("");

    material.current = { status: "finished", outcome: null };
    view.rerender(<RunAnnouncer />);
    expect(region().textContent).toBe("Response complete");

    material.current = { status: "finished", outcome: { type: "canceled" } };
    view.rerender(<RunAnnouncer />);
    expect(region().textContent).toBe("The turn was stopped");
  });
});
