import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { Collapsible } from "./collapsible";

function Pair({
  first = true,
  second = true,
  showFirst = true,
}: {
  first?: boolean;
  second?: boolean;
  showFirst?: boolean;
}) {
  return (
    <div
      data-testid="scrollport"
      style={{
        overflowY: "auto",
        scrollbarWidth: "thin",
        paddingRight: "1px",
        border: "2px solid",
      }}
    >
      {showFirst && (
        <Collapsible open={first}>
          <p>First</p>
        </Collapsible>
      )}
      <Collapsible open={second}>
        <p>Second</p>
      </Collapsible>
    </div>
  );
}

function row(text: string): HTMLElement {
  return screen.getByText(text).parentElement!.parentElement!;
}

function geometry(element: HTMLElement) {
  Object.defineProperties(element, {
    offsetWidth: { configurable: true, value: 130 },
    clientWidth: { configurable: true, value: 100 },
  });
  element.scrollTop = 42;
}

function sharedPair() {
  const view = render(<Pair />);
  const scrollport = screen.getByTestId("scrollport");
  geometry(scrollport);
  row("First").style.transitionDuration = "100ms";
  row("Second").style.transitionDuration = "200ms";
  return { ...view, scrollport };
}

function expectLocked(scrollport: HTMLElement) {
  expect(scrollport.style.scrollbarWidth).toBe("none");
  expect(scrollport.style.paddingRight).toBe("27px");
  scrollport.scrollTop = 80;
  fireEvent.scroll(scrollport);
  expect(scrollport.scrollTop).toBe(42);
}

function expectReleased(scrollport: HTMLElement) {
  expect(scrollport.style.scrollbarWidth).toBe("thin");
  expect(scrollport.style.paddingRight).toBe("1px");
  scrollport.scrollTop = 80;
  fireEvent.scroll(scrollport);
  expect(scrollport.scrollTop).toBe(80);
}

beforeEach(() => vi.useFakeTimers());
afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

describe("Collapsible shared scroll lifetime", () => {
  it("restores the original styles after two simultaneous collapses finish", () => {
    const { rerender, scrollport } = sharedPair();
    rerender(<Pair first={false} second={false} />);
    expect(scrollport.style.scrollbarWidth).toBe("none");
    act(() => vi.advanceTimersByTime(200));
    expectReleased(scrollport);
  });

  it.each([
    ["First", "Second"],
    ["Second", "First"],
  ])("keeps the shared lock when %s finishes before %s", (first, last) => {
    const { rerender, scrollport } = sharedPair();
    rerender(<Pair first={false} second={false} />);
    fireEvent.transitionEnd(row(first));
    expectLocked(scrollport);
    fireEvent.transitionEnd(row(last));
    expectReleased(scrollport);
    act(() => vi.runAllTimers());
    expectReleased(scrollport);
  });

  it("retains the original baseline when a second collapse starts later", () => {
    const { rerender, scrollport } = sharedPair();
    rerender(<Pair first={false} />);
    act(() => vi.advanceTimersByTime(50));
    rerender(<Pair first={false} second={false} />);
    act(() => vi.advanceTimersByTime(50));
    expectLocked(scrollport);
    act(() => vi.advanceTimersByTime(150));
    expectReleased(scrollport);
  });

  it("releases each lease once through unmount and transition cancellation", () => {
    const { rerender, scrollport } = sharedPair();
    rerender(<Pair first={false} second={false} />);
    rerender(<Pair first={false} second={false} showFirst={false} />);
    expectLocked(scrollport);
    fireEvent(row("Second"), new Event("transitioncancel", { bubbles: true }));
    expectReleased(scrollport);
    fireEvent.transitionEnd(row("Second"));
    act(() => vi.runAllTimers());
    expectReleased(scrollport);
  });

  it("cancels a reopening element's lease without releasing its sibling", () => {
    const { rerender, scrollport } = sharedPair();
    rerender(<Pair first={false} second={false} />);
    rerender(<Pair first second={false} />);
    expectLocked(scrollport);
    fireEvent.transitionEnd(row("Second"));
    expectReleased(scrollport);
  });

  it("ignores descendant transition events", () => {
    const { rerender, scrollport } = sharedPair();
    rerender(<Pair first={false} />);
    fireEvent.transitionEnd(screen.getByText("First"));
    expectLocked(scrollport);
    fireEvent.transitionEnd(row("First"));
    expectReleased(scrollport);
  });

  it("owns independent containers and restores the correct padding side", () => {
    const Independent = ({ open }: { open: boolean }) => (
      <>
        <div
          data-testid="left"
          style={{
            overflowY: "auto",
            scrollbarWidth: "thin",
            paddingRight: "1px",
            border: "2px solid",
          }}
        >
          <Collapsible open={open}>
            <p>Left</p>
          </Collapsible>
        </div>
        <div
          data-testid="right"
          style={{ overflowY: "auto", direction: "rtl", paddingLeft: "1px", border: "2px solid" }}
        >
          <Collapsible open={open}>
            <p>Right</p>
          </Collapsible>
        </div>
      </>
    );
    const { rerender } = render(<Independent open />);
    const left = screen.getByTestId("left");
    const right = screen.getByTestId("right");
    geometry(left);
    geometry(right);
    row("Left").style.transitionDuration = "100ms";
    row("Right").style.transitionDuration = "200ms";
    rerender(<Independent open={false} />);
    expect(left.style.scrollbarWidth).toBe("none");
    expect(right.style.scrollbarWidth).toBe("none");
    expect(right.style.paddingLeft).toBe("27px");
    fireEvent.transitionEnd(row("Left"));
    expectReleased(left);
    expect(right.style.scrollbarWidth).toBe("none");
    fireEvent.transitionEnd(row("Right"));
    expect(right.style.scrollbarWidth).toBe("");
    expect(right.style.paddingLeft).toBe("1px");
  });
});
