import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { act, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { beforeAll, describe, expect, it, vi } from "vitest";
import { type Element, type Root } from "hast";
import { getHighlighter } from "@/lib/highlight/shiki";
import { MarkdownRenderer } from "@/features/chat/message/ui/markdown/MarkdownRenderer";
import { rehypeFadeIn } from "@/features/chat/message/ui/markdown/rehypeFadeIn";
import { mountLocaleBundle, setLocale } from "@/lib/i18n/index";
import zh from "../../../../../../../../plugins/locale-zh/package/io.github.tangerg.flame/messages.json";
describe("markdownMessage", () => {
  beforeAll(async () => {
    await getHighlighter();
  });

  it("renders an empty string without throwing", () => {
    const { container } = testClient.render(<MarkdownRenderer text="" reveal="smooth" />);
    expect(container.querySelector(".md")).toBeTruthy();
  });

  it("wraps plain words in .fade-in spans while they are streaming", async () => {
    const { container } = testClient.render(
      <MarkdownRenderer text="Hello world" streaming reveal="smooth" />,
    );
    await waitFor(() => {
      const spans = container.querySelectorAll("span.fade-in");
      expect(spans.length).toBeGreaterThanOrEqual(2);
      const text = Array.from(spans)
        .map((span) => span.textContent)
        .join("|");
      expect(text).toContain("Hello");
      expect(text).toContain("world");
    });
  });

  it("fades the whole inline code surface with the streaming prose", async () => {
    const { container } = testClient.render(
      <MarkdownRenderer text="Read `scope.Run` next." streaming reveal="smooth" />,
    );
    await waitFor(() => {
      const code = container.querySelector("p > code");
      expect(code?.textContent).toBe("scope.Run");
      expect(code?.classList.contains("fade-in")).toBe(true);
      expect(code?.children).toHaveLength(0);
    });
  });

  it.each(["", "js"])("keeps the streamed %s fence outside word fades", async (language) => {
    const { container } = testClient.render(
      <MarkdownRenderer
        text={`\`\`\`${language}\nconst x = 1;\n\`\`\``}
        streaming
        reveal="smooth"
      />,
    );
    await waitFor(() => {
      expect(container.querySelector(".shiki-block pre")?.textContent).toContain("const x = 1;");
    });
    expect(container.querySelector(".shiki-block .fade-in")).toBeNull();
  });

  it("preserves inline code properties and explicit fade opt-outs", () => {
    const code: Element = {
      type: "element",
      tagName: "code",
      properties: { className: ["existing-code"], title: "Scope run" },
      children: [{ type: "text", value: "scope.Run" }],
    };
    const optedOut: Element = {
      type: "element",
      tagName: "span",
      properties: { dataNoFade: true },
      children: [
        { type: "text", value: "Stable " },
        {
          type: "element",
          tagName: "code",
          properties: {},
          children: [{ type: "text", value: "scope.Run" }],
        },
      ],
    };
    const optedOutCode = structuredClone(code);
    optedOutCode.properties.dataNoFade = true;
    const originalOptOuts = structuredClone([optedOut, optedOutCode]);
    const tree: Root = { type: "root", children: [code, optedOut, optedOutCode] };
    rehypeFadeIn()(tree);

    expect(code.properties).toEqual({
      className: ["existing-code", "fade-in"],
      title: "Scope run",
    });
    expect(code.children).toEqual([{ type: "text", value: "scope.Run" }]);
    expect([optedOut, optedOutCode]).toEqual(originalOptOuts);
  });

  it("auto-closes an unmatched inline backtick so it renders as code", () => {
    const { container } = testClient.render(<MarkdownRenderer text="use `code" reveal="smooth" />);
    expect(container.querySelector("code")).toBeTruthy();
    expect(container.textContent ?? "").toContain("code");
  });

  it("auto-closes an unmatched `**` so it renders as strong", () => {
    const { container } = testClient.render(
      <MarkdownRenderer text="that **really" reveal="smooth" />,
    );
    expect(container.querySelector("strong")).toBeTruthy();
    expect(container.textContent ?? "").toContain("really");
  });

  it("renders a complete fenced code block as a Shiki block", () => {
    const src = "before\n```js\nconst x = 1;\n```\nafter";
    const { container } = testClient.render(<MarkdownRenderer text={src} reveal="smooth" />);
    expect(container.querySelector(".shiki-block")).toBeTruthy();
    expect(container.textContent ?? "").toContain("const x = 1;");
  });

  it("keeps a fenced block without a language in the code-block surface", async () => {
    const src = "before\n```\ncommand --flag\n```\nafter";
    const { container } = testClient.render(<MarkdownRenderer text={src} reveal="smooth" />);
    await act(async () => Promise.resolve());

    expect(container.querySelector(".shiki-block")).toBeTruthy();
    expect(container.querySelector(".shiki-block pre")?.textContent).toBe("command --flag");
    expect(container.querySelector('.shiki-block button[aria-label="Copy code"]')).toBeTruthy();
  });

  it("renders a streaming-partial code block (no closer yet)", () => {
    const src = "```js\nconst x";
    const { container } = testClient.render(<MarkdownRenderer text={src} reveal="smooth" />);
    expect(container.querySelector(".shiki-block")).toBeTruthy();
    expect(container.textContent ?? "").toContain("const x");
  });

  it("renders an SVG fence as an isolated image preview while keeping code copy", async () => {
    const src =
      '```svg\n<svg xmlns="http://www.w3.org/2000/svg"><circle cx="8" cy="8" r="6" /></svg>\n```';
    const { container } = testClient.render(<MarkdownRenderer text={src} reveal="instant" />);
    await act(async () => Promise.resolve());

    const image = container.querySelector('img[alt="Image generated by the assistant"]');
    expect(image).toBeTruthy();
    expect(image!.getAttribute("src")).toMatch(/^data:image\/svg\+xml/);
    expect(container.querySelector("script")).toBeNull();
    expect(container.querySelector('.shiki-block button[aria-label="Copy code"]')).toBeTruthy();
    expect(container.querySelector('.shiki-block [data-slot="code-body"]')).toBeNull();
    expect(container.querySelector('.shiki-block[data-variant="preview"]')).toBeTruthy();
    expect(container.querySelector('[data-slot="shiki-preview-body"][tabindex="0"]')).toBeTruthy();
  });

  it("keeps HTML fences as inert, copyable source code", async () => {
    const src =
      '```html\n<!doctype html><html><body><script>parent.postMessage("ran", "*")</script></body></html>\n```';
    const { container } = testClient.render(<MarkdownRenderer text={src} reveal="instant" />);
    await act(async () => Promise.resolve());

    expect(container.querySelector("iframe")).toBeNull();
    expect(container.querySelector(".shiki-block")?.textContent).toContain("parent.postMessage");
    expect(container.querySelector('.shiki-block button[aria-label="Copy code"]')).toBeTruthy();
  });

  it("does not fetch remote Markdown images in the restricted desktop surface", () => {
    const src = "![Tracking pixel](https://tracker.example/pixel.png)";
    const { container } = testClient.render(<MarkdownRenderer text={src} reveal="instant" />);

    expect(container.querySelector('img[src^="https://tracker.example/"]')).toBeNull();
    const unavailable = within(container).queryByRole("img", {
      name: "Tracking pixel",
      description: "Image unavailable",
    });
    expect(unavailable, "the reader is still told an image belongs here").toBeTruthy();
    expect(unavailable?.textContent).toContain("Tracking pixel");
    expect(unavailable?.textContent).toContain("Image unavailable");
    expect(
      container.querySelector('button[aria-label="Tracking pixel"]'),
      "and it is not offered as something to press",
    ).toBeNull();
  });

  it("opens safe inline Markdown images through an accessible preview trigger", () => {
    const tinyPng =
      "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=";
    const { container } = testClient.render(
      <MarkdownRenderer text={`![Build graph](${tinyPng})`} reveal="instant" />,
    );

    const trigger = container.querySelector<HTMLButtonElement>('button[aria-label="Build graph"]');
    expect(trigger).toBeTruthy();
    const img = trigger?.querySelector("img");
    expect(img?.getAttribute("src")).toMatch(/^data:image\//);
    expect(img?.getAttribute("loading"), "an inline image has nothing to defer").toBeNull();
  });

  it("hands an image inside a link over to the link", () => {
    const tinyPng =
      "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=";

    const linked = [
      `[![build](${tinyPng})](https://example.test)`,
      `[**![build](${tinyPng})**](https://example.test)`,
      `[![build](https://img.example/b.png)](https://example.test)`,
      `[![build](${tinyPng}) docs](https://example.test)`,
    ];
    for (const src of linked) {
      const { container, unmount } = testClient.render(
        <MarkdownRenderer text={src} reveal="instant" />,
      );
      expect(
        container.querySelectorAll("a button, a a, a input").length,
        `${src}: a control nested inside the link`,
      ).toBe(0);
      expect(container.querySelector("a")).toBeTruthy();
      unmount();
    }

    const { container } = testClient.render(
      <MarkdownRenderer text={`![build](${tinyPng})`} reveal="instant" />,
    );
    expect(container.querySelector('button[aria-label="build"]')).toBeTruthy();
  });

  it("promotes image-only paragraphs into Codex wide and gallery media blocks", () => {
    const first =
      "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=";
    const second = "data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///ywAAAAAAQABAAACAUwAOw==";
    const { container } = testClient.render(
      <MarkdownRenderer
        text={`![Architecture](${first})\n\n![Before](${first}) ![After](${second})`}
        reveal="instant"
      />,
    );

    const wide = container.querySelector<HTMLParagraphElement>(
      'p[data-wide-markdown-block="true"]',
    );
    expect(wide?.querySelector('button[aria-label="Architecture"]')).toBeTruthy();

    const gallery = container.querySelector<HTMLParagraphElement>(
      'p[data-markdown-image-grid="true"]',
    );
    expect(gallery).toBeTruthy();
    expect(gallery?.querySelectorAll(":scope > button")).toHaveLength(2);
  });

  it("navigates between Markdown images inside one Codex preview gallery", () => {
    const first =
      "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=";
    const second = "data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///ywAAAAAAQABAAACAUwAOw==";
    testClient.render(
      <MarkdownRenderer text={`![Before](${first}) ![After](${second})`} reveal="instant" />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Before" }));
    const dialog = screen.getByRole("dialog", { name: "Before" });
    fireEvent.keyDown(dialog, { key: "ArrowLeft" });
    expect(screen.getByRole("dialog", { name: "Before" })).toBeTruthy();
    fireEvent.keyDown(dialog, { key: "ArrowRight" });
    expect(screen.getByRole("dialog", { name: "After" })).toBeTruthy();
    fireEvent.keyDown(dialog, { key: "ArrowRight" });
    expect(screen.getByRole("dialog", { name: "After" })).toBeTruthy();
    fireEvent.keyDown(screen.getByRole("button", { name: "Close image preview" }), {
      key: "ArrowLeft",
    });
    expect(screen.getByRole("dialog", { name: "Before" })).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Next image" }));
    expect(screen.getByRole("dialog", { name: "After" })).toBeTruthy();
  });

  it("exposes Codex close and zoom controls in the image preview", () => {
    const image =
      "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=";
    testClient.render(<MarkdownRenderer text={`![Architecture](${image})`} reveal="instant" />);

    fireEvent.click(screen.getByRole("button", { name: "Architecture" }));
    const dialog = screen.getByRole("dialog", { name: "Architecture" });
    expect(screen.getByRole("button", { name: "Close image preview" })).toBeTruthy();
    expect(
      screen.getByRole("button", { name: "Zoom out image" }).getAttribute("aria-disabled"),
    ).toBe("true");

    fireEvent.click(screen.getByRole("button", { name: "Zoom in image" }));
    expect(dialog.querySelector('[data-image-zoom="125"]')).toBeTruthy();
  });

  it("keeps GFM tables semantic and exposes the Codex copy action", async () => {
    const src = "| Name | Count |\n|---|---:|\n| alpha | 12 |";
    const writeText = vi.fn().mockResolvedValue(undefined);
    const clipboardDescriptor = Object.getOwnPropertyDescriptor(navigator, "clipboard");
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: { writeText },
    });
    try {
      const { container } = testClient.render(<MarkdownRenderer text={src} reveal="smooth" />);
      const table = container.querySelector("table");
      expect(table).toBeTruthy();
      expect(container.querySelectorAll("td").length).toBe(2);

      const copy = container.querySelector<HTMLButtonElement>('button[aria-label="Copy table"]');
      expect(copy).toBeTruthy();
      fireEvent.click(copy!);
      await waitFor(() => expect(writeText).toHaveBeenCalledWith(src));
    } finally {
      if (clipboardDescriptor) {
        Object.defineProperty(navigator, "clipboard", clipboardDescriptor);
      } else {
        Reflect.deleteProperty(navigator, "clipboard");
      }
    }
  });

  it("opens a semantic Codex preview for a wide Markdown table", () => {
    const src = "| Name | Count |\n|---|---:|\n| alpha | 12 |";
    testClient.render(<MarkdownRenderer text={src} reveal="instant" />);

    fireEvent.click(screen.getByRole("button", { name: "Expand table" }));

    const dialog = screen.getByRole("dialog", { name: "Table preview" });
    expect(dialog.querySelector("table")?.textContent).toContain("alpha");
    expect(dialog.querySelector("table")?.getAttribute("dir")).toBe("auto");
  });

  it("marks directional and numeric table material like Codex", () => {
    const src = "| Name | Count |\n|---|---:|\n| alpha | 12 |";
    const { container } = testClient.render(<MarkdownRenderer text={src} reveal="smooth" />);

    expect(container.querySelector("table")?.getAttribute("dir")).toBe("auto");
    expect(container.querySelector("td.md-table-cell-numeric")?.textContent).toBe("12");
  });

  it("counts a quantity as numeric however the model spelled it", () => {
    const quantities = ["12", "91.2%", "4.2s", "1,234", "-3", "+7", "0.5", "128MB"];
    const prose = ["alpha", "n/a", "2024-01-15", "v1.2.3", "3 of 7", "see 12 rows"];

    const cells = (value: string) => {
      const { container, unmount } = testClient.render(
        <MarkdownRenderer
          text={`| Name | Value |\n|---|---:|\n| alpha | ${value} |`}
          reveal="instant"
        />,
      );
      const marked = [...container.querySelectorAll("td.md-table-cell-numeric")].map(
        (cell) => cell.textContent,
      );
      unmount();
      return marked;
    };

    expect(quantities.filter((value) => !cells(value).includes(value))).toEqual([]);
    expect(prose.filter((value) => cells(value).length > 0)).toEqual([]);
  });

  it("keeps unspaced-script paragraphs as distinct directional prose", () => {
    const japanese = [
      "実行は完了しました。",
      "それではつづきをおねがいします。",
      "コンパイルエラーガアリマス。",
    ];
    const { container, unmount } = testClient.render(
      <MarkdownRenderer text={japanese.join("\n\n")} reveal="instant" />,
    );
    expect(
      Array.from(
        container.querySelectorAll('.md > p[dir="auto"]'),
        (paragraph) => paragraph.textContent,
      ),
    ).toEqual(japanese);
    unmount();

    const spaced = ["실행이 완료되었습니다.", "The run finished."];
    const other = testClient.render(
      <MarkdownRenderer text={spaced.join("\n\n")} reveal="instant" />,
    );
    expect(
      Array.from(
        other.container.querySelectorAll('.md > p[dir="auto"]'),
        (paragraph) => paragraph.textContent,
      ),
    ).toEqual(spaced);
  });

  it("renders markdown lists", () => {
    const src = "- one\n- two\n- three";
    const { container } = testClient.render(<MarkdownRenderer text={src} reveal="smooth" />);
    expect(container.querySelector("ul")).toBeTruthy();
    expect(container.querySelectorAll("li").length).toBe(3);
  });

  it("gives loose task items the Codex checkbox and body columns", () => {
    const src = [
      "1. [x] Preserve the durable fact",
      "",
      "   Keep the follow-up aligned with the task body.",
      "2. [ ] Verify the projected view",
    ].join("\n");
    const { container } = testClient.render(<MarkdownRenderer text={src} reveal="instant" />);

    const task = container.querySelector("li.task-list-item");
    expect(task?.firstElementChild).toBe(
      screen.getByRole("checkbox", { name: "Preserve the durable fact", checked: true }),
    );
    expect(task?.querySelectorAll(":scope > p")).toHaveLength(2);
    expect(task?.querySelector(":scope > p")?.textContent?.trim()).toBe(
      "Preserve the durable fact",
    );
  });

  it("names checked and unchecked task markers from their visible task text", () => {
    const src = [
      "- [x] Preserve the durable fact",
      "- [ ] 验证 projected view",
      "",
      "  Keep this loose follow-up in the same task.",
    ].join("\n");
    testClient.render(<MarkdownRenderer text={src} reveal="instant" />);

    const complete = screen.getByRole("checkbox", {
      name: "Preserve the durable fact",
      checked: true,
    });
    const pending = screen.getByRole("checkbox", { name: "验证 projected view", checked: false });
    expect(complete.getAttribute("aria-label")).toBe("Preserve the durable fact");
    expect(complete.getAttribute("aria-disabled")).toBe("true");
    expect(pending.getAttribute("aria-label")).toBe("验证 projected view");
    expect(pending.getAttribute("aria-disabled")).toBe("true");
    fireEvent.click(complete);
    fireEvent.click(pending);
    expect(screen.getByRole("checkbox", { name: "Preserve the durable fact", checked: true })).toBe(
      complete,
    );
    expect(screen.getByRole("checkbox", { name: "验证 projected view", checked: false })).toBe(
      pending,
    );
  });

  it("preserves paragraph boundaries and automatic direction for multilingual prose", () => {
    const src = [
      "# عنوان",
      "",
      "中文第一段。",
      "",
      "中文第二段。",
      "",
      "- عنصر",
      "",
      "1. خطوة",
      "",
      "> اقتباس",
    ].join("\n");
    const { container } = testClient.render(<MarkdownRenderer text={src} reveal="instant" />);

    expect(container.querySelector('h3[dir="auto"][data-prose-level="1"]')).toBeTruthy();
    expect(
      Array.from(
        container.querySelectorAll('.md > p[dir="auto"]'),
        (paragraph) => paragraph.textContent,
      ),
    ).toEqual(["中文第一段。", "中文第二段。"]);
    expect(container.querySelector('ul[dir="auto"]')).toBeTruthy();
    expect(container.querySelector('ol[dir="auto"]')).toBeTruthy();
    expect(container.querySelector('blockquote[dir="auto"]')).toBeTruthy();
  });

  it("preserves generated alert titles and their decorative icons", () => {
    const { container } = testClient.render(
      <MarkdownRenderer
        text={
          "> [!NOTE]\n> Read the evidence.\n\n> [!WARNING]\n> Keep the original result.\n\n> A plain quotation."
        }
        reveal="instant"
      />,
    );
    const callouts = container.querySelectorAll('[data-slot="callout"][role="note"]');
    expect(Array.from(callouts, (callout) => callout.getAttribute("data-callout"))).toEqual([
      "note",
      "warning",
    ]);
    expect(callouts[0]?.textContent?.replace(/\s+/g, " ").trim()).toBe("Note Read the evidence.");
    const titles = container.querySelectorAll('[data-slot="callout-title"]');
    expect(Array.from(titles, (title) => title.textContent)).toEqual(["Note", "Warning"]);
    for (const title of titles) {
      expect(title.getAttribute("dir")).toBe("auto");
      expect(title.querySelector('svg[aria-hidden="true"] path')).toBeTruthy();
    }
    expect(container.querySelector("blockquote > p")?.textContent).toBe("A plain quotation.");
  });

  it("updates every Callout title when the host switches between Chinese and English", async () => {
    const unmount = mountLocaleBundle("zh", zh);
    try {
      const kinds = ["NOTE", "TIP", "IMPORTANT", "WARNING", "CAUTION"];
      const { container } = testClient.render(
        <MarkdownRenderer
          text={kinds.map((kind) => `> [!${kind}]\n> Original body.`).join("\n\n")}
          reveal="instant"
        />,
      );
      const titles = () =>
        Array.from(
          container.querySelectorAll('[data-slot="callout-title"]'),
          (node) => node.textContent,
        );
      expect(titles()).toEqual(["Note", "Tip", "Important", "Warning", "Caution"]);
      await act(async () => setLocale("zh"));
      expect(titles()).toEqual(["说明", "提示", "重要", "警告", "注意"]);
      expect(
        container.querySelectorAll('[data-slot="callout"] p:not([data-slot="callout-title"])'),
      ).toHaveLength(5);
      await act(async () => setLocale("en"));
      expect(titles()).toEqual(["Note", "Tip", "Important", "Warning", "Caution"]);
    } finally {
      await act(async () => setLocale("en"));
      unmount();
    }
  });

  it("claims only an authored alert marker and keeps escaped or attached markers literal", () => {
    const { container } = testClient.render(
      <MarkdownRenderer
        text={
          "> [!NOTE]\n> Keep **bold** and `code`.\n\n> \\[!NOTE]\n> keep literal\n\n> [!NOTE]book stays prose"
        }
        reveal="instant"
      />,
    );
    const callouts = container.querySelectorAll('[data-slot="callout"]');
    expect(callouts).toHaveLength(1);
    expect(callouts[0]?.querySelector("strong")?.textContent).toBe("bold");
    expect(callouts[0]?.querySelector("code")?.textContent).toBe("code");
    expect(
      Array.from(container.querySelectorAll("blockquote"), (quote) =>
        quote.textContent?.replace(/\s+/g, " ").trim(),
      ),
    ).toEqual(["[!NOTE] keep literal", "[!NOTE]book stays prose"]);
  });

  it("keeps unmatched numeric brackets as ordinary prose instead of citation chrome", () => {
    const { container } = testClient.render(
      <MarkdownRenderer text="Read values[1] before values[12]." reveal="instant" />,
    );

    expect(container.querySelector("sup[data-citation]")).toBeNull();
    expect(container.textContent).toContain("values[1]");
    expect(container.textContent).toContain("values[12]");
  });

  it("resolves full, collapsed and shortcut references when streamed definitions arrive", async () => {
    const body = "Read [Full guide][protocol].\n\nThen [Collapsed guide][] and [Shortcut guide].";
    const complete = [
      body,
      "",
      '[protocol]: https://example.test/protocol "Protocol guide"',
      '[Collapsed guide]: https://example.test/implementation "Implementation guide"',
      '[Shortcut guide]: https://example.test/reading "Reading guide"',
    ].join("\n");
    const previousScale = testClient.theme.appearance.read().motionScale;
    testClient.theme.appearance.publishMotionScale(0);
    const { container, rerender, unmount } = testClient.render(
      <MarkdownRenderer text={body} reveal="smooth" streaming />,
    );
    try {
      expect(within(container).queryAllByRole("link")).toHaveLength(0);
      expect(container.textContent).toContain("[Full guide][protocol]");
      expect(container.textContent).toContain("[Collapsed guide][]");
      expect(container.textContent).toContain("[Shortcut guide]");

      rerender(<MarkdownRenderer text={complete} reveal="smooth" streaming />);
      await waitFor(() => {
        expect(within(container).getAllByRole("link")).toHaveLength(3);
      });

      for (const streaming of [true, false]) {
        rerender(<MarkdownRenderer text={complete} reveal="smooth" streaming={streaming} />);
        const links = within(container).getAllByRole("link");
        expect(
          links.map((link) => [
            link.textContent,
            link.getAttribute("href"),
            link.getAttribute("title"),
          ]),
        ).toEqual([
          ["Full guide", "https://example.test/protocol", "Protocol guide"],
          ["Collapsed guide", "https://example.test/implementation", "Implementation guide"],
          ["Shortcut guide", "https://example.test/reading", "Reading guide"],
        ]);
        expect(
          [...container.querySelectorAll("p")].map((paragraph) => paragraph.textContent),
        ).toEqual(["Read Full guide.", "Then Collapsed guide and Shortcut guide."]);
      }
    } finally {
      unmount();
      testClient.theme.appearance.publishMotionScale(previousScale);
    }
  });

  it("isolates inline and fenced code as left-to-right inside bidirectional prose", async () => {
    const src =
      "تشغيل `npm run check -- --watch=false`\n\n```sh\nnpm run check -- --watch=false\n```";
    const { container } = testClient.render(<MarkdownRenderer text={src} reveal="instant" />);
    await act(async () => Promise.resolve());

    expect.soft(container.querySelector("p > code")?.getAttribute("dir")).toBe("ltr");
    expect.soft(container.querySelector(".shiki-block")?.getAttribute("dir")).toBe("ltr");
  });

  it("drops them again once the message has settled", () => {
    const { container } = testClient.render(
      <MarkdownRenderer text="Hello world" reveal="smooth" />,
    );
    expect(container.querySelectorAll("span.fade-in")).toHaveLength(0);
    expect(container.textContent ?? "").toContain("Hello world");
  });

  it("drops fade-in wrappers for the instant reveal mode", () => {
    const { container } = testClient.render(
      <MarkdownRenderer text="Hello world" reveal="instant" />,
    );
    expect(container.querySelectorAll("span.fade-in").length).toBe(0);
    expect(container.textContent ?? "").toContain("Hello world");
  });

  it("uses the typewriter pipeline, which is the one that carries a caret", async () => {
    const { container } = testClient.render(
      <MarkdownRenderer text="Hello world" reveal="typewriter" streaming />,
    );
    await waitFor(() => {
      expect(container.querySelectorAll("span.type-caret").length).toBeGreaterThan(0);
    });
    expect(container.querySelectorAll("span.fade-in")).toHaveLength(0);
    const shown = (container.textContent ?? "").replace(/\u200b/g, "").trim();
    expect("Hello world".startsWith(shown)).toBe(true);
    expect(shown.length).toBeGreaterThan(0);
  });

  it("gives the smooth reveal word fades and no caret", async () => {
    const { container } = testClient.render(
      <MarkdownRenderer text="Hello world" reveal="smooth" streaming />,
    );
    await waitFor(() => {
      expect(container.querySelectorAll("span.fade-in").length).toBeGreaterThan(0);
    });
    expect(container.querySelectorAll("span.type-caret")).toHaveLength(0);
  });

  it("renders model placeholders as literal text instead of unknown React elements", () => {
    const { container } = testClient.render(
      <MarkdownRenderer text="ANGLE=<chosen>" reveal="instant" />,
    );
    expect(container.textContent).toContain("ANGLE=<chosen>");
    expect(container.querySelector("chosen")).toBeNull();
  });

  it("keeps supported semantic raw HTML while rendering dangerous tags literally", () => {
    const { container } = testClient.render(
      <MarkdownRenderer text={'value<sup>2</sup> <script>alert("x")</script>'} reveal="instant" />,
    );
    expect(container.querySelector("sup")?.textContent).toBe("2");
    expect(container.querySelector("script")).toBeNull();
    expect(container.textContent).toContain('<script>alert("x")</script>');
  });

  it("keeps non-Codex raw HTML disclosures inert and literal", () => {
    const source = "<details><summary>More</summary><div>Hidden</div></details>";
    const { container } = testClient.render(<MarkdownRenderer text={source} reveal="instant" />);

    expect(container.querySelector("details")).toBeNull();
    expect(container.querySelector("summary")).toBeNull();
    expect(container.textContent).toContain("<details>");
    expect(container.textContent).toContain("<summary>More</summary>");
  });

  it("does not install attributes from model-authored raw HTML", () => {
    const source = '<strong class="model-tone">Loud</strong>';
    const { container } = testClient.render(<MarkdownRenderer text={source} reveal="instant" />);

    expect(container.querySelector("strong")).toBeNull();
    expect(container.textContent).toContain('<strong class="model-tone">');
  });

  it("keeps model-authored style blocks inert instead of installing a stylesheet", async () => {
    const source = '<style>@import url("https://tracker.example/agent.css");</style>Visible';
    const { container } = testClient.render(<MarkdownRenderer text={source} reveal="instant" />);
    await act(async () => Promise.resolve());

    const installedStyles = Array.from(container.querySelectorAll("*")).filter((element) =>
      element.shadowRoot?.querySelector("style"),
    );
    expect.soft(installedStyles).toHaveLength(0);
    expect
      .soft(container.textContent)
      .toContain('<style>@import url("https://tracker.example/agent.css");</style>');
  });

  it("nests every authored heading level below the turn that holds it", () => {
    const source = ["# One", "## Two", "### Three", "#### Four", "##### Five", "###### Six"].join(
      "\n\n",
    );
    const { container } = testClient.render(<MarkdownRenderer text={source} reveal="instant" />);

    const headings = [...container.querySelectorAll<HTMLElement>("[data-prose-level]")];
    expect(headings.map((h) => [h.dataset.proseLevel, h.tagName])).toEqual([
      ["1", "H3"],
      ["2", "H3"],
      ["3", "H4"],
      ["4", "H5"],
      ["5", "H6"],
      ["6", "H6"],
    ]);
    expect(container.querySelectorAll("h1, h2")).toHaveLength(0);
  });
});
