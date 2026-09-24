import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { MermaidViewer } from "./MermaidViewer";

const SVG = '<svg viewBox="0 0 200 100" xmlns="http://www.w3.org/2000/svg"></svg>';

describe("MermaidViewer", () => {
  it("opens fitted, then reads at true size and zooms from there", () => {
    const { container } = render(<MermaidViewer svg={SVG} />);
    expect(container.querySelector("[data-mermaid-fit]")).not.toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "100%" }));
    const sized = () => container.querySelector<HTMLElement>("[data-mermaid-scale]");
    expect(sized()?.style.width).toBe("200px");

    fireEvent.click(screen.getByRole("button", { name: /zoom in/i }));
    expect(sized()?.style.width).toBe("300px");

    fireEvent.click(screen.getByRole("button", { name: /fit/i }));
    expect(container.querySelector("[data-mermaid-fit]")).not.toBeNull();
  });
});
