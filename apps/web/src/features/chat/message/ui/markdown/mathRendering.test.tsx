import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});

import { fireEvent, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { MarkdownRenderer } from "@/features/chat/message/ui/markdown/MarkdownRenderer";
// The app pins one KaTeX (0.19.0) for both the stylesheet and every renderer
// through `rehype-katex`/`micromark-extension-math`. These cases keep that pinned
// renderer honest: the version jump past the integrations' declared `^0.16.0`
// range must still produce correct markup, and GHSA-238p-pmpm-9mq7 (a prototype
// pollution gadget that bypassed `trust: false` before 0.18.2) must stay closed.
describe("markdown math rendering", () => {
  const literal = String.raw`printf '%s\n' "$1" "$2"; \(item\); \[x\]; [/inline]x[/inline]; $19.99`;

  it.each([
    ["shell fence", `\`\`\`sh\n${literal}\n\`\`\``],
    ["PowerShell fence", `\`\`\`powershell\n${literal}\n\`\`\``],
    ["indented code", `    ${literal}`],
    ["list code", `- Example\n\n  \`\`\`text\n  ${literal}\n  \`\`\``],
    ["unfinished fence", `\`\`\`sh\n${literal}`],
  ])("preserves displayed and copied %s source", async (_name, text) => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    const previous = Object.getOwnPropertyDescriptor(navigator, "clipboard");
    Object.defineProperty(navigator, "clipboard", { configurable: true, value: { writeText } });
    finishTestClient(() => {
      if (previous) Object.defineProperty(navigator, "clipboard", previous);
      else Reflect.deleteProperty(navigator, "clipboard");
    });
    const { container } = testClient.render(<MarkdownRenderer text={text} reveal="instant" />);
    expect(container.querySelector("pre")?.textContent).toBe(literal);
    expect(
      container.querySelector("[data-markdown-copy-text]")?.getAttribute("data-markdown-copy-text"),
    ).toBe(literal);
    fireEvent.click(container.querySelector<HTMLButtonElement>('button[aria-label="Copy code"]')!);
    await waitFor(() => expect(writeText).toHaveBeenCalledWith(literal));
  });

  it("preserves inline code and link destinations while parsing prose math", () => {
    const destination = "https://example.test/$1/\\(item\\)";
    const { container } = testClient.render(
      <MarkdownRenderer text={`\`${literal}\` [source](<${destination}>)`} reveal="instant" />,
    );
    expect(container.querySelector("code")?.textContent).toBe(literal);
    expect(container.querySelector("a")?.getAttribute("href")).toBe(
      "https://example.test/$1/(item)",
    );
    expect(container.querySelector(".katex")).toBeNull();
  });

  it("retains exact code material across streamed fence updates", async () => {
    const { container, rerender } = testClient.render(
      <MarkdownRenderer text={`\`\`\`sh\n${literal}`} streaming reveal="smooth" />,
    );
    await waitFor(() => expect(container.querySelector("pre")?.textContent).toBe(literal));
    const next = `${literal}\nprintf '%s' "$3"`;
    rerender(<MarkdownRenderer text={`\`\`\`sh\n${next}\n\`\`\``} streaming reveal="smooth" />);
    await waitFor(() => {
      expect(container.querySelector("pre")?.textContent).toBe(next);
      expect(
        container
          .querySelector("[data-markdown-copy-text]")
          ?.getAttribute("data-markdown-copy-text"),
      ).toBe(next);
    });
  });

  it("preserves generated SVG material containing mathematical punctuation", () => {
    const source = String.raw`<svg xmlns="http://www.w3.org/2000/svg"><text>$1 \(x\) \[x\]</text></svg>`;
    const { container } = testClient.render(
      <MarkdownRenderer text={`\`\`\`svg\n${source}\n\`\`\``} reveal="instant" />,
    );
    expect(
      container.querySelector("[data-markdown-copy-text]")?.getAttribute("data-markdown-copy-text"),
    ).toBe(source);
    expect(container.querySelector(".katex")).toBeNull();
  });

  it.each([
    [String.raw`\(x^2\)`, false],
    [String.raw`\\(x^2\\)`, false],
    ["[/inline]x^2[/inline]", false],
    ["\\[\nx^2\n\n+ y^2\n\\]", true],
    ["[/math]\nx^2\n\n+ y^2\n[/math]", true],
  ] as const)("renders parser-owned prose formula %s", (text, display) => {
    const { container } = testClient.render(<MarkdownRenderer text={text} reveal="instant" />);
    expect(container.querySelector(".katex")).toBeTruthy();
    expect(container.querySelector(".katex-error")).toBeNull();
    expect(Boolean(container.querySelector(".katex-display"))).toBe(display);
  });

  it("renders currency as authored beside a formula", () => {
    const { container } = testClient.render(
      <MarkdownRenderer text="Pay $5 for $x$, then $19.99 or $1,299." reveal="instant" />,
    );
    expect(container.textContent).toContain("Pay $5 for");
    expect(container.textContent).toContain("then $19.99 or $1,299.");
    expect(container.querySelectorAll(".katex")).toHaveLength(1);
  });

  it("renders inline math through the pinned KaTeX", () => {
    const { container } = testClient.render(
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
    const { container } = testClient.render(
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
    const { container } = testClient.render(
      <MarkdownRenderer text={"$\\frac{a}{$"} reveal="instant" />,
    );
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
    const { container } = testClient.render(
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
