import { render, waitFor, within } from "@testing-library/react";
import { beforeAll, describe, expect, it } from "vitest";
import { motionScale, publishMotionScale } from "@/lib/appearance";
import { getHighlighter } from "@/lib/highlight/shiki";
import { t } from "@/lib/i18n";
import { MarkdownRenderer } from "./MarkdownRenderer";
import { markdownSelectionPayload } from "./markdownSelectionCopy";

const NOTES = [
  "# A supported answer",
  "",
  "A supported claim[^note]. Another reference[^note].",
  "",
  "[^note]: The accepted observation stays available.",
  "",
  "    Supporting context stays in a second paragraph.",
].join("\n");

describe("markdown footnote presentation", () => {
  beforeAll(async () => {
    await getHighlighter();
  });

  it("closes each message's repeated references, descriptions and return links", () => {
    const { container } = render(
      <>
        <MarkdownRenderer text={NOTES} reveal="instant" />
        <MarkdownRenderer text={NOTES} reveal="instant" />
      </>,
    );
    const roots = container.querySelectorAll<HTMLElement>(".md");
    expect(roots).toHaveLength(2);

    for (const root of roots) {
      const section = root.querySelector("section[data-footnotes]")!;
      const label = section.querySelector<HTMLElement>(".sr-only")!;
      expect(label).toBeTruthy();
      expect(label.id).not.toBe("");
      expect(label.tagName).toBe("H3");
      expect(label.getAttribute("data-prose-level")).toBe("2");
      expect(label.textContent).toBe(t("message.markdown.footnotes"));

      const definition = section.querySelector<HTMLElement>("li[id]")!;
      const references = root.querySelectorAll<HTMLAnchorElement>("a[data-footnote-ref]");
      const returns = section.querySelectorAll<HTMLAnchorElement>("a[data-footnote-backref]");
      expect(references).toHaveLength(2);
      expect(returns).toHaveLength(2);

      for (const [index, reference] of references.entries()) {
        expect(reference.id).not.toBe("");
        expect(reference.getAttribute("href")).toBe(`#${definition.id}`);
        expect(reference.getAttribute("aria-describedby")).toBe(label.id);
        expect(reference.ownerDocument.getElementById(label.id)).toBe(label);
        expect(reference.ownerDocument.getElementById(definition.id)).toBe(definition);
        expect(reference.getAttribute("target")).toBeNull();
        expect(returns[index]!.getAttribute("href")).toBe(`#${reference.id}`);
        expect(returns[index]!.getAttribute("target")).toBeNull();
        expect(returns[index]!.getAttribute("aria-label")).toBe(
          t("message.markdown.backReference", { reference: index === 0 ? "1" : "1-2" }),
        );
        expect(returns[index]!.getAttribute("data-markdown-copy")).toBe("exclude");
      }
      expect(new Set([...returns].map((link) => link.getAttribute("aria-label"))).size).toBe(2);
    }

    const ids = [...container.querySelectorAll("[id]")].map((element) => element.id);
    expect(new Set(ids).size).toBe(ids.length);
    expect(container.querySelector("[node]")).toBeNull();
  });

  it("copies the answer and note text without auxiliary labels or return controls", () => {
    const { container } = render(<MarkdownRenderer text={NOTES} reveal="instant" />);
    const root = container.querySelector<HTMLElement>(".md")!;
    const range = document.createRange();
    range.selectNodeContents(root);
    const selection = document.getSelection()!;
    selection.removeAllRanges();
    selection.addRange(range);
    try {
      const payload = markdownSelectionPayload(root, selection)!;
      expect(payload.plainText).toContain("A supported answer");
      expect(payload.plainText).toContain("A supported claim1. Another reference1.");
      expect(payload.plainText).toContain("The accepted observation stays available.");
      expect(payload.plainText).toContain("Supporting context stays in a second paragraph.");
      expect(payload.plainText).not.toContain(t("message.markdown.footnotes"));
      expect(payload.plainText).not.toContain("↩");
      expect(payload.htmlText).toContain("data-footnote-ref");
      expect(payload.htmlText).not.toContain("data-footnote-backref");
      expect(payload.htmlText).not.toContain('class="sr-only"');
    } finally {
      selection.removeAllRanges();
    }
  });

  it("resolves a footnote whose authored label uses the answer's language", () => {
    const { container } = render(
      <MarkdownRenderer
        text={"这是一条带来源的说明。[^来源]\n\n[^来源]: 保留完整的来源内容。"}
        reveal="instant"
      />,
    );
    const reference = container.querySelector<HTMLAnchorElement>("a[data-footnote-ref]");
    expect(reference).not.toBeNull();
    const target = container.querySelector("[data-footnotes] li[id]")!;
    expect(reference?.getAttribute("href")).toBe(`#${target.id}`);
    expect(target.textContent).toContain("保留完整的来源内容。");
    expect(container.textContent).not.toContain("[^来源]");
  });

  it("keeps fragment anchors local, web links external and authored HTML literal", () => {
    const text = [
      "Read [guide](https://example.test/protocol) or [jump](#existing-target).",
      "",
      '<a id="authored-target" href="#existing-target">literal anchor</a>',
      "",
      "[Blocked](javascript:alert%281%29)",
    ].join("\n");
    const { container } = render(<MarkdownRenderer text={text} reveal="instant" />);
    const guide = within(container).getByRole("link", { name: "guide" });
    expect(guide.getAttribute("href")).toBe("https://example.test/protocol");
    expect(guide.getAttribute("target")).toBe("_blank");
    expect(guide.getAttribute("rel")).toBe("noopener noreferrer");
    const fragment = within(container).getByRole("link", { name: "jump" });
    expect(fragment.getAttribute("href")).toBe("#existing-target");
    expect(fragment.getAttribute("target")).toBeNull();
    expect(container.querySelector("#authored-target")).toBeNull();
    expect(container.textContent).toContain('<a id="authored-target" href="#existing-target">');
    expect(container.querySelector('[href^="javascript:" i]')).toBeNull();
    expect(within(container).queryByRole("link", { name: "Blocked" })).toBeNull();
    expect(within(container).getByText("Blocked").closest("a")?.getAttribute("href")).toBeNull();
  });

  it("keeps generated identities stable as streaming grows and completes", async () => {
    const previousScale = motionScale();
    publishMotionScale(0);
    const { container, rerender, unmount } = render(
      <MarkdownRenderer text={NOTES} reveal="smooth" streaming />,
    );
    try {
      await waitFor(() => {
        expect(container.querySelectorAll("a[data-footnote-ref]")).toHaveLength(2);
      });
      const ids = [...container.querySelectorAll("[id]")].map((element) => element.id);
      const complete = `${NOTES}\n\nThe answer then continues.`;
      rerender(<MarkdownRenderer text={complete} reveal="smooth" streaming />);
      await waitFor(() => {
        expect(container.textContent).toContain("The answer then continues.");
      });
      expect([...container.querySelectorAll("[id]")].map((element) => element.id)).toEqual(ids);

      rerender(<MarkdownRenderer text={complete} reveal="smooth" streaming={false} />);
      await waitFor(() => {
        expect(container.querySelectorAll("a[data-footnote-backref]")).toHaveLength(2);
      });
      expect([...container.querySelectorAll("[id]")].map((element) => element.id)).toEqual(ids);
    } finally {
      unmount();
      publishMotionScale(previousScale);
    }
  });
});
