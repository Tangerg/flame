import { render } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { MarkdownRenderer } from "./MarkdownRenderer";

// The app pins one KaTeX (0.19.0) for both the stylesheet and every renderer
// through `rehype-katex`/`micromark-extension-math`. These cases keep that pinned
// renderer honest: the version jump past the integrations' declared `^0.16.0`
// range must still produce correct markup, and GHSA-238p-pmpm-9mq7 (a prototype
// pollution gadget that bypassed `trust: false` before 0.18.2) must stay closed.
describe("markdown math rendering", () => {
  it("renders inline math through the pinned KaTeX", () => {
    const { container } = render(
      <MarkdownRenderer text="Energy is $E = mc^2$." reveal="instant" />,
    );
    const katex = container.querySelector(".katex");
    expect(katex).toBeTruthy();
    expect(container.querySelector(".katex-error")).toBeNull();
    expect(
      container.querySelector('.katex annotation[encoding="application/x-tex"]')?.textContent,
    ).toBe("E = mc^2");
  });

  it("renders display math as a centered KaTeX block", () => {
    const { container } = render(
      <MarkdownRenderer text={"$$\n\\frac{a}{b}\n$$"} reveal="instant" />,
    );
    expect(container.querySelector(".katex-display")).toBeTruthy();
    expect(container.querySelector(".katex-display")?.tagName).toBe("SPAN");
    expect(container.querySelector(".katex-display")?.getAttribute("dir")).toBe("ltr");
    expect(container.querySelector(".katex-display .katex-mathml math")).toBeTruthy();
    expect(container.querySelector('.katex-display .katex-html[aria-hidden="true"]')).toBeTruthy();
    expect(container.querySelector(".katex-error")).toBeNull();
    expect(
      container.querySelector('.katex annotation[encoding="application/x-tex"]')?.textContent,
    ).toBe("\\frac{a}{b}");
  });

  it("surfaces malformed math as a KaTeX error rather than throwing", () => {
    const { container } = render(<MarkdownRenderer text={"$\\frac{a}{$"} reveal="instant" />);
    expect(container.querySelector(".katex-error")).toBeTruthy();
  });
});

describe("markdown math trust boundary (GHSA-238p-pmpm-9mq7)", () => {
  afterEach(() => {
    delete (Object.prototype as Record<string, unknown>).trust;
  });

  it("refuses a javascript: \\href even when Object.prototype.trust is polluted", () => {
    // Before 0.18.2 KaTeX read `trust` through inheritance and would emit the
    // unsafe link once Object.prototype was polluted. The pinned 0.19.0 uses
    // own-property checks, so the inherited flag must not re-enable trust.
    Object.defineProperty(Object.prototype, "trust", {
      value: true,
      configurable: true,
      enumerable: false,
      writable: true,
    });
    const { container } = render(
      <MarkdownRenderer text={"$\\href{javascript:alert(1)}{click}$"} reveal="instant" />,
    );
    // No element may carry an executable javascript: URL. The raw TeX survives
    // only as inert annotation text, which is not an href, so assert the attribute.
    expect(container.querySelector('[href^="javascript:" i]')).toBeNull();
    expect(container.innerHTML.toLowerCase()).not.toContain('href="javascript:');
    // The refused \href renders as KaTeX's error color rather than a trusted link.
    expect(container.querySelector('.katex [style*="#cc0000"]')).toBeTruthy();
  });
});
