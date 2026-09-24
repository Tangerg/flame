import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { PlanList } from "./PlanList";

describe("PlanList", () => {
  afterEach(cleanup);

  it("names the step in progress and brings it into view", () => {
    const scrolled = vi.fn();
    HTMLElement.prototype.scrollIntoView = scrolled;
    render(
      <PlanList
        steps={[
          { id: "a", text: "Verify ownership", status: "done" },
          { id: "b", text: "Review evidence", status: "active" },
          { id: "c", text: "Run gates", status: "pending" },
        ]}
      />,
    );
    const current = screen.getByText("Review evidence").closest("[aria-current]");
    expect(current?.getAttribute("aria-current")).toBe("step");
    expect(document.querySelectorAll('[aria-current="step"]')).toHaveLength(1);
    expect(scrolled).toHaveBeenCalledWith({ block: "nearest" });
  });
});
