import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ReasoningBlock } from "./ReasoningBlock";

function renderReasoning(status: "running" | "complete", text: string) {
  return render(<ReasoningBlock text={text} status={status} />);
}

function scrollportOf(container: HTMLElement): HTMLElement {
  const scrollport = container.querySelector<HTMLElement>('[data-slot="reasoning-scroller"]');
  expect(scrollport).not.toBeNull();
  return scrollport!;
}

function stubScrollGeometry(scrollHeight: number, clientHeight: number): () => void {
  const scroll = vi
    .spyOn(HTMLElement.prototype, "scrollHeight", "get")
    .mockReturnValue(scrollHeight);
  const client = vi
    .spyOn(HTMLElement.prototype, "clientHeight", "get")
    .mockReturnValue(clientHeight);
  return () => {
    scroll.mockRestore();
    client.mockRestore();
  };
}

afterEach(() => {
  vi.useRealTimers();
});

describe("ReasoningBlock disclosure policy", () => {
  it("reports the wait it actually watched, and says nothing when it watched none", () => {
    vi.useFakeTimers();
    const now = vi.spyOn(performance, "now");
    now.mockReturnValue(0);
    const view = render(<ReasoningBlock text="Hidden rationale" status="running" />);
    expect(screen.getByRole("button", { name: /Thinking/ })).toBeTruthy();

    now.mockReturnValue(8_400);
    view.rerender(<ReasoningBlock text="Hidden rationale" status="complete" />);
    expect(screen.getByRole("button", { name: /Thought for 8.4s/ })).toBeTruthy();

    view.unmount();
    render(<ReasoningBlock text="Hidden rationale" status="complete" />);
    expect(screen.getByRole("button", { name: /^Thought$/ })).toBeTruthy();
    now.mockRestore();
  });

  it("remembers being collapsed while it thought apart from being opened once it had", () => {
    const view = render(<ReasoningBlock text="Hidden rationale" status="running" />);
    const live = () => screen.getByRole("button", { name: /Thinking|Thought/ });

    expect(live().getAttribute("aria-expanded")).toBe("true");

    fireEvent.click(live());
    expect(live().getAttribute("aria-expanded")).toBe("false");

    view.rerender(<ReasoningBlock text="Hidden rationale" status="complete" />);
    expect(live().getAttribute("aria-expanded")).toBe("false");

    fireEvent.click(live());
    expect(live().getAttribute("aria-expanded")).toBe("true");

    view.rerender(<ReasoningBlock text="Hidden rationale" status="running" />);
    expect(
      live().getAttribute("aria-expanded"),
      "the collapse it was given while running is the one it kept",
    ).toBe("false");
  });

  it("turns the first user toggle into an explicit override of the automatic state", () => {
    renderReasoning("complete", "Hidden rationale");
    const trigger = screen.getByRole("button", { name: /Thought/ });

    expect(trigger.getAttribute("aria-expanded")).toBe("false");
    const shut = document.getElementById(trigger.getAttribute("aria-controls") ?? "");
    expect(shut, "the region a shut trigger names has to exist").not.toBeNull();
    expect(shut!.textContent).toBe("");

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

    expect(activity?.getAttribute("data-shell")).toBe("line");
    expect(body).toBeTruthy();
  });

  it("carries live state on the Thinking label instead of a trailing status dot", () => {
    renderReasoning("running", "Inspect the protocol boundary");

    const trigger = screen.getByRole("button", { name: "Thinking" });
    expect(trigger.querySelector('[data-slot="loader"]')).not.toBeNull();
    expect(trigger.querySelector(".animate-pulse-dot")).toBeNull();
  });

  it("keeps an overflowing rationale keyboard-scrollable", () => {
    const overflow = stubScrollGeometry(400, 192);
    try {
      const { container } = renderReasoning("running", "Inspect the protocol boundary");
      expect(scrollportOf(container).tabIndex).toBe(0);
    } finally {
      overflow();
    }
  });

  it("leaves a rationale that fits out of the tab order", () => {
    const { container } = renderReasoning("running", "Inspect the protocol boundary");

    expect(scrollportOf(container).tabIndex).toBe(-1);
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
