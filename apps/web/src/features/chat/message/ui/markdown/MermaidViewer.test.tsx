import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { fireEvent, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { MermaidViewer } from "@/features/chat/message/ui/markdown/MermaidViewer";
const SVG = '<svg viewBox="0 0 200 100" xmlns="http://www.w3.org/2000/svg"></svg>';

describe("MermaidViewer", () => {
  it("preserves the captured SVG and its reading surface across camera commands", () => {
    testClient.render(<MermaidViewer svg={SVG} onClose={vi.fn()} />);
    const diagram = screen.getByRole("img", { name: "Diagram" });
    for (const name of ["100%", "Zoom in diagram", "Fit"]) {
      fireEvent.click(screen.getByRole("button", { name }));
      expect(screen.getByRole("img", { name: "Diagram" })).toBe(diagram);
      expect(diagram.querySelector("svg")?.getAttribute("viewBox")).toBe("0 0 200 100");
    }
  });

  it("provides a named keyboard reading region and an explicit close action", () => {
    const onClose = vi.fn();
    testClient.render(<MermaidViewer svg={SVG} onClose={onClose} />);

    const viewport = screen.getByRole("region", { name: "Diagram" });
    expect(viewport.tabIndex).toBe(0);
    expect(viewport.contains(screen.getByRole("img", { name: "Diagram" }))).toBe(true);
    viewport.focus();
    expect(document.activeElement).toBe(viewport);

    fireEvent.click(screen.getByRole("button", { name: "Close" }));
    expect(onClose).toHaveBeenCalledOnce();
  });
});
