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

  it("copies the complete output, including lines beyond the preview", () => {
    const output = Array.from({ length: 3_000 }, (_, index) => `line ${index}`).join("\n");
    render(<ToolOutputPanel output={output} status="ok" />);
    fireEvent.click(screen.getByRole("button", { name: "Copy output" }));
    expect(copyText).toHaveBeenCalledWith(output);
  });
});

describe("ToolOutputPanel height rule", () => {
  const output = (count: number) =>
    Array.from({ length: count }, (_, index) => `line ${index}`).join("\n");

  it("shows the newest lines while collapsed, so progress stays visible", () => {
    render(<ToolOutputPanel output={output(40)} status="running" />);
    expect(screen.getByText("line 39")).toBeTruthy();
    expect(screen.queryByText("line 0")).toBeNull();
    expect(screen.getByRole("button", { name: /31 earlier lines/ })).toBeTruthy();
  });

  it.each([10, 100, 999, 1_001])("expands %i lines into the same bounded region", (count) => {
    render(<ToolOutputPanel output={output(count)} status="ok" />);
    fireEvent.click(screen.getByRole("button", { name: /earlier line/ }));
    expect(screen.getByRole("region", { name: "Tool output" })).toBeTruthy();
    cleanup();
  });
});
