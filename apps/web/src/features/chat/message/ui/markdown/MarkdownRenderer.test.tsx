import { beforeEach, expect, it, onTestFinished, vi } from "vitest";
import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import { createTestClient, type TestClient } from "@/test/renderClient";
import { MarkdownRenderer } from "./MarkdownRenderer";
let client: TestClient;
beforeEach(() => {
  client = createTestClient();
  onTestFinished(() => client.dispose());
});
it("renders standard Markdown as one current document with literal unfinished inline delimiters", () => {
  const view = client.render(
    <MarkdownRenderer text="Read `unfinished and **unfinished" streaming />,
  );
  expect(view.container.querySelector("code,strong,.fade-in")).toBeNull();
  expect(view.container.textContent).toContain("`unfinished and **unfinished");
  view.rerender(<MarkdownRenderer text="Read `complete` and **complete**." streaming />);
  expect(view.container.querySelector("code")?.textContent).toBe("complete");
  expect(view.container.querySelector("strong")?.textContent).toBe("complete");
});
it("resolves later definitions against the complete streamed document", () => {
  const view = client.render(
    <MarkdownRenderer text={"[A reference][source]\n\nA note[^note]."} streaming />,
  );
  expect(view.container.querySelector('a[href="https://example.test/source"]')).toBeNull();
  view.rerender(
    <MarkdownRenderer
      text={
        "[A reference][source]\n\nA note[^note].\n\n[source]: https://example.test/source\n\n[^note]: The complete note."
      }
      streaming
    />,
  );
  expect(view.container.innerHTML).toContain("https://example.test/source");
  expect(view.container.innerHTML).toContain("data-footnotes");
});
it("keeps normal GFM lists, checked tasks and complete literal fenced source", () => {
  const code = 'const exact = "9007199254740993";\n// $1 <script>';
  const { container } = client.render(
    <MarkdownRenderer text={`- [x] Reviewed\n- [ ] Pending\n\n\`\`\`ts\n${code}`} streaming />,
  );
  expect(container.querySelectorAll('[role="checkbox"]')).toHaveLength(2);
  expect(container.querySelector('[role="checkbox"]')?.getAttribute("aria-checked")).toBe("true");
  expect(
    container.querySelector("[data-markdown-copy-text]")?.getAttribute("data-markdown-copy-text"),
  ).toBe(code);
  expect(container.querySelector("script")).toBeNull();
});
it("does not execute raw HTML or unsafe destinations", () => {
  const { container } = client.render(
    <MarkdownRenderer
      text={
        '<script>alert(1)</script>\n\n<img src=x onerror="alert(1)">\n\n[unsafe](javascript:alert%281%29)'
      }
    />,
  );
  expect(container.querySelector("script,img[onerror],iframe")).toBeNull();
  expect(container.querySelector('a[href^="javascript:"]')).toBeNull();
});
it("keeps currency and removed proprietary math markers literal", () => {
  const { container } = client.render(
    <MarkdownRenderer text={String.raw`Pay $5 or $x$. \(x\) [/inline]x[/inline].`} />,
  );
  expect(container.querySelector(".katex")).toBeNull();
  expect(container.textContent).toContain("Pay $5 or $x$");
});
it("keeps an open table presentation current when the document appends rows", async () => {
  const table = "| Name | Status |\n| --- | --- |\n| Original | Ready |";
  const view = client.render(<MarkdownRenderer text={table} streaming />);
  fireEvent.click(screen.getByRole("button", { name: "Expand table" }));
  const dialog = screen.getByRole("dialog");
  expect(within(dialog).getByText("Original")).toBeTruthy();
  view.rerender(<MarkdownRenderer text={table + "\n| Added | Pending |"} streaming />);
  await waitFor(() => expect(within(dialog).getByText("Added")).toBeTruthy());
  expect(screen.getByRole("dialog")).toBe(dialog);
});
it("copies captured code bytes without code framing", async () => {
  const writeText = vi.fn(async () => {});
  const descriptor = Object.getOwnPropertyDescriptor(navigator, "clipboard");
  Object.defineProperty(navigator, "clipboard", { configurable: true, value: { writeText } });
  onTestFinished(() => {
    if (descriptor) Object.defineProperty(navigator, "clipboard", descriptor);
    else Reflect.deleteProperty(navigator, "clipboard");
  });
  const code = 'const exact = "9007199254740993";';
  client.render(<MarkdownRenderer text={`\`\`\`ts\n${code}\n\`\`\``} />);
  fireEvent.click(screen.getByRole("button", { name: "Copy code" }));
  await waitFor(() => expect(writeText).toHaveBeenCalledExactlyOnceWith(code));
});
