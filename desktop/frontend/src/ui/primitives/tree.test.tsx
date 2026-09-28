import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { TreePrimitive } from "./tree";

describe("tree keyboard navigation", () => {
  it("does not enter the next sibling when an expanded branch has no visible children", () => {
    render(
      <TreePrimitive.Root>
        <TreePrimitive.Item level={1} expanded selected focusable onActivate={vi.fn()}>
          Empty directory
        </TreePrimitive.Item>
        <TreePrimitive.Item level={1} selected={false} focusable={false} onActivate={vi.fn()}>
          Sibling file
        </TreePrimitive.Item>
      </TreePrimitive.Root>,
    );
    const directory = screen.getByRole("treeitem", { name: "Empty directory" });
    directory.focus();
    fireEvent.keyDown(directory, { key: "ArrowRight" });
    expect(document.activeElement).toBe(directory);
  });

  it("enters a visible child and returns to its parent with the opposite arrow", () => {
    render(
      <TreePrimitive.Root>
        <TreePrimitive.Item level={1} expanded selected focusable onActivate={vi.fn()}>
          Directory
        </TreePrimitive.Item>
        <TreePrimitive.Group>
          <TreePrimitive.Item level={2} selected={false} focusable={false} onActivate={vi.fn()}>
            Child
          </TreePrimitive.Item>
        </TreePrimitive.Group>
      </TreePrimitive.Root>,
    );
    const directory = screen.getByRole("treeitem", { name: "Directory" });
    const child = screen.getByRole("treeitem", { name: "Child" });
    directory.focus();
    fireEvent.keyDown(directory, { key: "ArrowRight" });
    expect(document.activeElement).toBe(child);
    fireEvent.keyDown(child, { key: "ArrowLeft" });
    expect(document.activeElement).toBe(directory);
  });
});
