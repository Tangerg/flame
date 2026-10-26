import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { act, fireEvent, screen, waitFor } from "@testing-library/react";
import { beforeAll, beforeEach, describe, expect, it } from "vitest";
import { getHighlighter } from "@/lib/highlight/shiki";
import { CodeBlock, CodeHighlightProvider, setCodeWrap } from "@flame/ui";
import { HighlightedCodeSource } from "@/lib/highlight/HighlightedCodeSource";
import { ShikiCodeBlock } from "@/ui/atoms/shiki-code-block";
const LONG_CODE = Array.from({ length: 30 }, (_, index) => `line ${index + 1}`).join("\n");

describe("ShikiCodeBlock", () => {
  beforeAll(async () => {
    await getHighlighter();
  });

  beforeEach(() => setCodeWrap(false));

  it("retains the reading surface and focus while code updates before highlighting settles", async () => {
    const block = (code: string) => (
      <CodeHighlightProvider component={HighlightedCodeSource}>
        <ShikiCodeBlock lang="javascript" code={code} />
      </CodeHighlightProvider>
    );
    const view = testClient.render(block("const accepted = true;"));
    await waitFor(() => expect(view.container.querySelector("code .line")).not.toBeNull());
    const region = view.container.querySelector("pre")!;
    region.focus();
    expect(document.activeElement).toBe(region);

    const next = "const accepted = true;\nconst owner = 'Runtime';";
    view.rerender(block(next));
    expect(view.container.querySelector("pre")).toBe(region);
    expect(region.textContent).toBe(next);
    expect(document.activeElement).toBe(region);
    await waitFor(() => expect(region.querySelector("code .line")).not.toBeNull());
    expect(document.activeElement).toBe(region);
  });

  it("keeps long code readable in its scrolling surface instead of replacing it with a fold row", async () => {
    testClient.render(<ShikiCodeBlock lang="text" code={LONG_CODE} />);
    await act(async () => Promise.resolve());

    expect(screen.getByRole("region", { name: "Code content" }).textContent).toContain("line 30");
    expect(screen.queryByText("Show 30 lines")).toBeNull();
  });

  it("offers the Codex code-wrap toggle with pressed-state feedback", async () => {
    testClient.render(<ShikiCodeBlock lang="text" code="a very long line" />);
    await act(async () => Promise.resolve());
    const toggle = screen.getByRole("button", { name: "Enable word wrap" });

    expect(toggle.getAttribute("aria-pressed")).toBe("false");
    fireEvent.click(toggle);

    expect(
      screen.getByRole("button", { name: "Disable word wrap" }).getAttribute("aria-pressed"),
    ).toBe("true");
    expect(screen.getByRole("region", { name: "Code content" }).getAttribute("data-wrap")).toBe(
      "true",
    );
  });

  it("shares one wrap preference with captured code blocks rendered by the shared owner", async () => {
    testClient.render(
      <>
        <ShikiCodeBlock lang="text" code="transcript" />
        <CodeBlock label="Captured">captured</CodeBlock>
      </>,
    );
    await act(async () => Promise.resolve());
    fireEvent.click(screen.getAllByRole("button", { name: "Enable word wrap" })[0]!);

    expect(screen.getByRole("region", { name: "Code content" }).getAttribute("data-wrap")).toBe(
      "true",
    );
    expect(screen.getByRole("region", { name: "Captured" }).getAttribute("data-wrap")).toBe("true");
  });

  it("colors captured code through the host highlighter without executing authored markup", async () => {
    const source =
      '<script>parent.postMessage("ran")</script>\n<img src="https://tracker.example/a.png">';
    testClient.render(
      <CodeHighlightProvider component={HighlightedCodeSource}>
        <CodeBlock label="Captured html" language="html">
          {source}
        </CodeBlock>
      </CodeHighlightProvider>,
    );
    const region = screen.getByRole("region", { name: "Captured html" });
    expect(region.textContent).toBe(source);

    await waitFor(() => expect(region.querySelector("span[style*='color']")).not.toBeNull());
    expect(
      Array.from(region.querySelectorAll(".line"), (line) => line.textContent).join("\n"),
    ).toBe(source);
    expect(region.querySelector("script, img")).toBeNull();
  });

  it("shares the Codex wrap preference across code blocks", async () => {
    testClient.render(
      <>
        <ShikiCodeBlock lang="text" code="first long line" />
        <ShikiCodeBlock lang="text" code="second long line" />
      </>,
    );
    await act(async () => Promise.resolve());

    fireEvent.click(screen.getAllByRole("button", { name: "Enable word wrap" })[0]!);

    expect(screen.getAllByRole("button", { name: "Disable word wrap" })).toHaveLength(2);
    for (const body of screen.getAllByRole("region", { name: "Code content" })) {
      expect(body.getAttribute("data-wrap")).toBe("true");
    }
  });
});
