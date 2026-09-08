import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ReasoningBlock } from "./ReasoningBlock";

function renderReasoning(status: "running" | "complete", text: string) {
  return render(<ReasoningBlock text={text} status={status} />);
}

afterEach(() => {
  vi.useRealTimers();
});

describe("ReasoningBlock disclosure policy", () => {
  it("turns the first user toggle into an explicit override of the automatic state", () => {
    renderReasoning("complete", "Hidden rationale");
    const trigger = screen.getByRole("button", { name: /Thought/ });

    expect(trigger.getAttribute("aria-expanded")).toBe("false");
    expect(screen.queryByRole("region")).toBeNull();

    fireEvent.click(trigger);
    expect(trigger.getAttribute("aria-expanded")).toBe("true");
    expect(screen.getByRole("region").textContent).toContain("Hidden rationale");

    fireEvent.click(trigger);
    expect(trigger.getAttribute("aria-expanded")).toBe("false");
  });

  it("keeps settled reasoning prose inside the disclosure body", () => {
    renderReasoning("complete", "Hidden rationale that should not compete with the answer");

    expect(screen.getByRole("button", { name: /Thought/ })).toBeTruthy();
    expect(
      screen.queryByText("Hidden rationale that should not compete with the answer"),
    ).toBeNull();
  });

  it("sets expanded reasoning apart as an indented aside instead of a card", () => {
    renderReasoning("running", "Inspect the protocol boundary");

    const activity = screen.getByRole("button", { name: /Thinking/ }).closest("[data-shell]");
    const body = screen.getByRole("region");

    // `data-shell` is the decision: a line, not a card. That it is INDENTED and carries a rule
    // marking its extent is geometry, and geometry is measured where CSS exists — see the
    // closure suite, which reads the body's own border and inset back.
    expect(activity?.getAttribute("data-shell")).toBe("line");
    expect(body).toBeTruthy();
  });

  it("carries live state on the Thinking label instead of a trailing status dot", () => {
    renderReasoning("running", "Inspect the protocol boundary");

    const trigger = screen.getByRole("button", { name: "Thinking" });
    // The slot, not the class: the shimmer is StyleX now and its class name is generated.
    expect(trigger.querySelector('[data-slot="loader"]')).not.toBeNull();
    expect(trigger.querySelector(".animate-pulse-dot")).toBeNull();
  });

  it("keeps the bounded streaming rationale keyboard-scrollable", () => {
    const { container } = renderReasoning("running", "Inspect the protocol boundary");

    // The slot, not the class that happened to produce the scrolling: what this is named for
    // is that the window can be reached by keyboard, and `overflow-y-auto` was only ever the
    // spelling. Whether it fades its clipped edges is measured in the closure suite.
    const scrollport = container.querySelector<HTMLElement>('[data-slot="reasoning-scroller"]');
    expect(scrollport).not.toBeNull();
    expect(scrollport!.tabIndex).toBe(0);
  });

  it("does not disguise Run cancellation as an Answer now activity action", () => {
    renderReasoning("running", "A predecessor renderer is still settling.");

    expect(screen.queryByRole("button", { name: /Answer now/ })).toBeNull();
  });

  it("does not invent reasoning duration from renderer mount time", () => {
    vi.useFakeTimers();
    renderReasoning("running", "A restored reasoning item is still streaming.");

    act(() => {
      vi.advanceTimersByTime(3_000);
    });

    expect(screen.queryByText("3s", { exact: true })).toBeNull();
  });
});
