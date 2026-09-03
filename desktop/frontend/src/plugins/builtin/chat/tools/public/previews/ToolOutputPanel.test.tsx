import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ToolOutputPanel } from "./ToolOutputPanel";

const copyText = vi.hoisted(() => vi.fn<(text: string) => Promise<boolean>>());

vi.mock("@/lib/clipboard", () => ({ copyText }));

afterEach(() => {
  cleanup();
  copyText.mockReset();
});

describe("ToolOutputPanel copy material ownership", () => {
  it("does not lend a retired streaming-output copy response to the replacement output", async () => {
    const retiredCopy = Promise.withResolvers<boolean>();
    copyText.mockReturnValueOnce(retiredCopy.promise);
    const view = render(<ToolOutputPanel output="old output" status="running" />);

    fireEvent.click(screen.getByRole("button", { name: "Copy output" }));
    expect(copyText).toHaveBeenCalledWith("old output");

    view.rerender(<ToolOutputPanel output="replacement output" status="running" />);
    await act(async () => retiredCopy.resolve(true));

    expect(screen.getByRole("button", { name: "Copy output" })).toBeTruthy();
  });

  // Expanding used to render every line, and the cost is superlinear — nine seconds at fifty
  // thousand. The whole output is a click away in the terminal view either way.
  it("stops expanding where the frame is still a frame", () => {
    const lines = 3_000;
    const output = Array.from({ length: lines }, (_, index) => `line ${index}`).join("\n");
    const { container } = render(<ToolOutputPanel output={output} status="ok" />);

    fireEvent.click(screen.getByRole("button", { name: /Show 1000 of 3000/ }));

    const rendered = container.querySelectorAll("div.whitespace-pre-wrap");
    expect(rendered).toHaveLength(1_000);
    expect(screen.getByText(/2000 more lines/)).toBeTruthy();
  });
});
