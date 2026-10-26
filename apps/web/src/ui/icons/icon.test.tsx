import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { act, cleanup } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { createRef } from "react";
import { BellOff } from "@flame/icons/react";
import { UI_FONT_SIZE_MAX_PX, UI_FONT_SIZE_MIN_PX } from "@flame/ui/typography";
import { Icon, ICON_NAMES, knownIconName } from "@/ui/icons/icon";
import { productIconSizePx } from "@/lib/productIconScale";
afterEach(() => {
  cleanup();
  testClient.theme.appearance.publishUiFontSize(null);
});

describe("Flame icon integration", () => {
  it("renders every application name with the authored optical geometry", () => {
    for (const name of ICON_NAMES) {
      expect(knownIconName(name)).toBe(name);
      const { container, unmount } = testClient.render(<Icon name={name} size="md" />);
      const svg = container.querySelector("svg")!;
      expect(svg.getAttribute("viewBox")).toBe("0 0 16 16");
      expect(svg.getAttribute("stroke-width")).toBe("1.25");
      expect(svg.getAttribute("aria-hidden")).toBe("true");
      expect(svg.getAttribute("data-icon-name")).toBe(name);
      unmount();
    }
  });

  it("changes the actual size and optical master together when UI fonts change", () => {
    const { container } = testClient.render(<Icon name="bell-off" size="lg" />);
    const svg = () => container.querySelector("svg")!;
    expect(svg().style.width).toBe("20px");
    expect(svg().getAttribute("viewBox")).toBe("0 0 16 16");
    act(() => testClient.theme.appearance.publishUiFontSize(16));
    expect(svg().style.width).toBe("22px");
    expect(svg().getAttribute("viewBox")).toBe("0 0 24 24");
    act(() => testClient.theme.appearance.publishUiFontSize(15));
    expect(svg().style.width).toBe("21px");
    expect(svg().getAttribute("viewBox")).toBe("0 0 16 16");
  });

  it("keeps the explicit composer size independent of the UI font", () => {
    const { container } = testClient.render(<Icon name="brain" size="composer" />);
    act(() => testClient.theme.appearance.publishUiFontSize(18));
    const svg = container.querySelector("svg")!;
    expect(svg.style.width).toBe("16px");
    expect(svg.getAttribute("viewBox")).toBe("0 0 16 16");
  });

  it("selects the right master for every application name, font and size token", () => {
    const names = [...ICON_NAMES];
    const { container, rerender } = testClient.render(<Icon name="plus" />);
    for (let font = UI_FONT_SIZE_MIN_PX; font <= UI_FONT_SIZE_MAX_PX; font++) {
      act(() => testClient.theme.appearance.publishUiFontSize(font));
      for (const size of ["xs", "sm", "md", "lg", "xl", "composer"] as const) {
        rerender(
          <>
            {names.map((name) => (
              <Icon key={name} name={name} size={size} />
            ))}
          </>,
        );
        const pixels = productIconSizePx(size, font);
        const grid = pixels < 22 ? 16 : 24;
        const svgs = [...container.querySelectorAll("svg")];
        expect(svgs).toHaveLength(names.length);
        expect(new Set(svgs.map((svg) => svg.getAttribute("viewBox")))).toEqual(
          new Set([`0 0 ${grid} ${grid}`]),
        );
        expect(new Set(svgs.map((svg) => svg.getAttribute("stroke-width")))).toEqual(
          new Set([grid === 16 ? "1.25" : "1.5"]),
        );
        expect(new Set(svgs.map((svg) => svg.style.width))).toEqual(new Set([`${pixels}px`]));
      }
    }
  });

  it("passes an SVG ref through the public React API", () => {
    const ref = createRef<SVGSVGElement>();
    const { container } = testClient.render(<BellOff size={16} ref={ref} />);
    expect(ref.current).toBe(container.querySelector("svg"));
  });
});
