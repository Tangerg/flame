import { act, cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { beforeEach, afterEach, expect, it, onTestFinished } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
import { SelectionQuote } from "./SelectionQuote";
let client: TestClient;
let restore: () => void;
beforeEach(() => {
  client = createTestClient();
  onTestFinished(() => client.dispose());
  const selection = document.getSelection()!;
  const descriptor = Object.getOwnPropertyDescriptor(selection, "getComposedRanges");
  Object.defineProperty(selection, "getComposedRanges", {
    configurable: true,
    value: () =>
      Array.from({ length: selection.rangeCount }, (_, index) => {
        const range = selection.getRangeAt(index);
        return {
          startContainer: range.startContainer,
          endContainer: range.endContainer,
          startOffset: range.startOffset,
          endOffset: range.endOffset,
          collapsed: range.collapsed,
        };
      }),
  });
  restore = () => {
    selection.removeAllRanges();
    if (descriptor) Object.defineProperty(selection, "getComposedRanges", descriptor);
    else Reflect.deleteProperty(selection, "getComposedRanges");
  };
});
afterEach(() => {
  cleanup();
  restore();
});
function open() {
  client.render(
    <>
      <div data-quote-source="message">
        <p>Captured original text</p>
      </div>
      <SelectionQuote />
    </>,
  );
  const source = screen.getByText("Captured original text");
  const selection = document.getSelection()!;
  const range = document.createRange();
  range.selectNodeContents(source);
  selection.removeAllRanges();
  selection.addRange(range);
  return source;
}
it("opens only for an explicit context menu and quotes the immutable captured text", async () => {
  const source = open();
  fireEvent(document, new Event("selectionchange"));
  expect(screen.queryByRole("menuitem")).toBeNull();
  fireEvent.contextMenu(source, { clientX: 10, clientY: 10 });
  const action = await screen.findByRole("menuitem", { name: "Quote in message" });
  source.textContent = "Replacement after admission";
  document.getSelection()!.removeAllRanges();
  fireEvent.click(action);
  await waitFor(() =>
    expect(client.composer.access.captureSubmission().value).toContain("Captured original text"),
  );
  expect(client.composer.access.captureSubmission().value).not.toContain("Replacement");
});
it("refuses a captured quote after its original conversation scope retires", async () => {
  const source = open();
  fireEvent.contextMenu(source, { clientX: 10, clientY: 10 });
  const action = await screen.findByRole("menuitem", { name: "Quote in message" });
  act(() => client.agent.access.selectAgentSession("successor-session"));
  fireEvent.click(action);
  expect(client.composer.access.captureSubmission().value).toBe("");
});
it("does not admit a native selection spanning independent sources", () => {
  const source = open();
  const other = document.createElement("div");
  other.dataset.quoteSource = "message";
  other.textContent = "Other source";
  document.body.append(other);
  const range = document.createRange();
  range.setStart(source.firstChild!, 0);
  range.setEnd(other.firstChild!, 5);
  const selection = document.getSelection()!;
  selection.removeAllRanges();
  selection.addRange(range);
  fireEvent.contextMenu(source);
  expect(screen.queryByRole("menuitem")).toBeNull();
  other.remove();
});
