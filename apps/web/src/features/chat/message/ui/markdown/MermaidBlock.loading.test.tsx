import { waitFor } from "@testing-library/react";
import { expect, it, onTestFinished, vi } from "vitest";
import { createTestClient } from "@/test/renderClient";
import { MermaidBlock } from "@/features/chat/message/ui/markdown/MermaidBlock";

vi.mock("beautiful-mermaid", () => {
  throw new Error("renderer chunk unavailable");
});

it("preserves the source when the diagram renderer cannot load", async () => {
  const client = createTestClient();
  onTestFinished(() => client.dispose());
  const code = "graph TD; A-->B";
  const { container, queryByRole } = client.render(<MermaidBlock code={code} />);

  await waitFor(() => expect(container.querySelector(".shiki-block")).toBeTruthy());
  expect(queryByRole("status", { name: "Loading Mermaid diagram" })).toBeNull();
  expect(container.textContent).toContain(code);
});
