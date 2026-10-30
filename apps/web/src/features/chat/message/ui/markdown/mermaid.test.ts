import { describe, expect, it } from "vitest";
import { copyMermaidSvg, renderMermaid } from "@/features/chat/message/ui/markdown/mermaid";

const diagrams = [
  "flowchart LR\n A[开始] --> B[完成]",
  "sequenceDiagram\n participant A as 客户端\n participant B as 服务端\n A->>B: 请求",
  "classDiagram\n Animal <|-- Duck\n Animal : +int age",
  "stateDiagram-v2\n [*] --> Ready\n Ready --> Done",
  "erDiagram\n CUSTOMER ||--o{ ORDER : places",
  'pie title 统计\n "通过" : 3\n "等待" : 1',
];

describe("Mermaid renderer", () => {
  it.each(diagrams)("renders supported source without an external font: %s", async (source) => {
    const svg = await renderMermaid(source, "fixture");
    const parsed = new DOMParser().parseFromString(svg, "image/svg+xml");
    expect(parsed.querySelector("parsererror")).toBeNull();
    expect(parsed.documentElement.tagName).toBe("svg");
    expect(Number(parsed.documentElement.getAttribute("width"))).toBeGreaterThan(0);
    expect(svg).not.toContain("@import");
    expect(svg).toContain("system-ui");
    expect(svg).toContain("var(--color-text)");
  });

  it("keeps authored HTML, callbacks, links and image requests inert", async () => {
    const svg = await renderMermaid(
      [
        "flowchart LR",
        ' A["<script>alert(1)</script><img src=https://tracker.test/a>"] --> B["$$x$$"]',
        ' click A "https://tracker.test/click"',
        " click B call run()",
      ].join("\n"),
      "untrusted",
    );
    const root = new DOMParser().parseFromString(svg, "image/svg+xml");
    expect(root.querySelector("script, foreignObject, image, a")).toBeNull();
    expect(root.documentElement.textContent).toContain("alert(1)");
    for (const node of root.querySelectorAll("*")) {
      for (const attribute of Array.from(node.attributes).filter(
        (value) => !value.name.startsWith("xmlns"),
      )) {
        expect(attribute.name).not.toMatch(/^on/i);
        expect(attribute.value).not.toMatch(/(?:javascript:|https?:|url\(https?)/i);
      }
    }
  });

  it("does not emit image resources from image-shaped nodes", async () => {
    const svg = await renderMermaid(
      'flowchart LR\n A@{ img: "https://tracker.test/image.png", label: "picture" }',
      "images",
    );
    const root = new DOMParser().parseFromString(svg, "image/svg+xml");
    expect(root.querySelector("image, foreignObject")).toBeNull();
    expect(svg).not.toContain("tracker.test");
  });

  it("gives the viewer independent IDs and preserves its references and literal text", () => {
    const source =
      '<svg xmlns="http://www.w3.org/2000/svg" id="diagram" aria-labelledby="title"><title id="title">marker</title><defs><marker id="arrow"/></defs><path marker-end="url(#arrow)"/><use href="#arrow"/><text>arrow</text></svg>';
    const copy = new DOMParser().parseFromString(copyMermaidSvg(source, "viewer"), "image/svg+xml");
    expect(copy.documentElement.id).toBe("viewer-diagram");
    expect(copy.documentElement.getAttribute("aria-labelledby")).toBe("viewer-title");
    expect(copy.querySelector("path")?.getAttribute("marker-end")).toBe("url(#viewer-arrow)");
    expect(copy.querySelector("use")?.getAttribute("href")).toBe("#viewer-arrow");
    expect(copy.querySelector("text")?.textContent).toBe("arrow");
  });
});
