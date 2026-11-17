import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { act, fireEvent, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";
import { CodeBlock } from "@flame/ui";
import { CodeArtifact } from "@/ui/atoms/code-artifact";
const LONG_CODE = Array.from({ length: 30 }, (_, index) => `line ${index + 1}`).join("\n");

describe("CodeArtifact", () => {
  beforeEach(() => {
    const reset = testClient.render(<CodeBlock label="Wrap preference">fixture</CodeBlock>);
    const enabled = screen.queryByRole("button", { name: "Disable word wrap" });
    if (enabled) fireEvent.click(enabled);
    reset.unmount();
  });

  it("retains the reading surface and focus while code updates before highlighting settles", async () => {
    const block = (code: string) => <CodeArtifact lang="javascript" code={code} />;
    const view = testClient.render(block("const accepted = true;"));
    await waitFor(() =>
      expect(screen.getByRole("region", { name: "Code content" }).textContent).toBe(
        "const accepted = true;",
      ),
    );
    const region = screen.getByRole("region", { name: "Code content" });
    region.focus();
    expect(document.activeElement).toBe(region);

    const next = "const accepted = true;\nconst owner = 'Runtime';";
    view.rerender(block(next));
    expect(screen.getByRole("region", { name: "Code content" })).toBe(region);
    expect(region.textContent).toBe(next);
    expect(document.activeElement).toBe(region);
    await waitFor(() => expect(region.querySelector("code")).not.toBeNull());
    expect(document.activeElement).toBe(region);
  });

  it("keeps long code readable in its scrolling surface instead of replacing it with a fold row", async () => {
    testClient.render(<CodeArtifact lang="text" code={LONG_CODE} />);
    await act(async () => Promise.resolve());

    expect(screen.getByRole("region", { name: "Code content" }).textContent).toContain("line 30");
    expect(screen.queryByText("Show 30 lines")).toBeNull();
  });

  it("offers the next code-wrap action without an inverted pressed state", async () => {
    testClient.render(<CodeArtifact lang="text" code="a very long line" />);
    await act(async () => Promise.resolve());
    const toggle = screen.getByRole("button", { name: "Enable word wrap" });

    expect(toggle.getAttribute("aria-pressed")).toBeNull();
    fireEvent.click(toggle);

    expect(
      screen.getByRole("button", { name: "Disable word wrap" }).getAttribute("aria-pressed"),
    ).toBeNull();
    expect(screen.getByRole("region", { name: "Code content" }).getAttribute("data-wrap")).toBe(
      "true",
    );
  });

  it("shares one wrap preference with captured code blocks rendered by the shared owner", async () => {
    testClient.render(
      <>
        <CodeArtifact lang="text" code="transcript" />
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

  it("keeps captured markup literal at the host source boundary", async () => {
    const source =
      '<script>parent.postMessage("ran")</script>\n<img src="https://tracker.example/a.png">';
    testClient.render(
      <CodeBlock label="Captured html" language="html">
        {source}
      </CodeBlock>,
    );
    const region = screen.getByRole("region", { name: "Captured html" });
    expect(region.textContent).toBe(source);

    expect(region.querySelector("code")).toBeTruthy();
    expect(region.querySelector("script, img")).toBeNull();
  });

  it("shares the word-wrap preference across code blocks", async () => {
    testClient.render(
      <>
        <CodeArtifact lang="text" code="first long line" />
        <CodeArtifact lang="text" code="second long line" />
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
