import type { CSSProperties } from "react";
import { render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useScrollReach } from "./use-scroll-reach";

const geometry = { width: 100, height: 100, scrollWidth: 100, scrollHeight: 100 };

function Scrollport({
  overflowX,
  overflowY,
  content = "Recorded output",
}: {
  overflowX: CSSProperties["overflowX"];
  overflowY: CSSProperties["overflowY"];
  content?: string;
}) {
  const scroll = useScrollReach();
  return (
    <div {...scroll} aria-label="Recorded output" style={{ overflowX, overflowY }}>
      {content}
    </div>
  );
}

beforeEach(() => {
  Object.assign(geometry, { width: 100, height: 100, scrollWidth: 100, scrollHeight: 100 });
  for (const [property, value] of [
    ["clientWidth", "width"],
    ["clientHeight", "height"],
    ["scrollWidth", "scrollWidth"],
    ["scrollHeight", "scrollHeight"],
  ] as const) {
    vi.spyOn(HTMLElement.prototype, property, "get").mockImplementation(() => geometry[value]);
  }
});

afterEach(() => vi.restoreAllMocks());

describe("scroll reachability", () => {
  it.each(["auto", "scroll"] as const)(
    "makes horizontal %s overflow keyboard reachable",
    (mode) => {
      geometry.scrollWidth = 400;
      render(<Scrollport overflowX={mode} overflowY="hidden" />);
      expect(screen.getByLabelText("Recorded output").getAttribute("tabindex")).toBe("0");
    },
  );

  it.each(["auto", "scroll"] as const)("keeps vertical %s overflow keyboard reachable", (mode) => {
    geometry.scrollHeight = 400;
    render(<Scrollport overflowX="hidden" overflowY={mode} />);
    expect(screen.getByLabelText("Recorded output").getAttribute("tabindex")).toBe("0");
  });

  it.each(["hidden", "clip", "visible"] as const)(
    "does not add a keyboard stop for %s overflow",
    (mode) => {
      geometry.scrollWidth = 400;
      geometry.scrollHeight = 400;
      render(<Scrollport overflowX={mode} overflowY={mode} />);
      expect(screen.getByLabelText("Recorded output").getAttribute("tabindex")).toBeNull();
    },
  );

  it("does not count a clipped axis when the scrollable axis fits", () => {
    geometry.scrollWidth = 400;
    render(<Scrollport overflowX="hidden" overflowY="auto" />);
    expect(screen.getByLabelText("Recorded output").getAttribute("tabindex")).toBeNull();
  });

  it("removes the keyboard stop when the content fits or scrolling is clipped", () => {
    geometry.scrollWidth = 400;
    const view = render(<Scrollport overflowX="auto" overflowY="auto" />);
    const scrollport = screen.getByLabelText("Recorded output");
    expect(scrollport.getAttribute("tabindex")).toBe("0");

    geometry.scrollWidth = 100;
    view.rerender(<Scrollport overflowX="auto" overflowY="auto" content="Short output" />);
    expect(scrollport.getAttribute("tabindex")).toBeNull();

    geometry.scrollWidth = 400;
    view.rerender(<Scrollport overflowX="hidden" overflowY="auto" />);
    expect(scrollport.getAttribute("tabindex")).toBeNull();
  });
});
