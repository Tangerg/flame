import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { type ComponentProps } from "react";
import { act, fireEvent, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { TreePrimitive } from "@/ui/primitives/tree";
describe("tree item selection semantics", () => {
  it("declares selection for selectable branches and omits it for nonselectable items", () => {
    testClient.render(
      <TreePrimitive.Root>
        <TreePrimitive.Item level={1} expanded selected focusable onActivate={vi.fn()}>
          Selectable branch
        </TreePrimitive.Item>
        <TreePrimitive.Group>
          <TreePrimitive.Item level={2} selected={false} focusable={false} onActivate={vi.fn()}>
            Selectable leaf
          </TreePrimitive.Item>
        </TreePrimitive.Group>
        <TreePrimitive.Item level={1} expanded={false} focusable={false} onActivate={vi.fn()}>
          Nonselectable branch
        </TreePrimitive.Item>
        <TreePrimitive.Item level={1} focusable={false} onActivate={vi.fn()}>
          Nonselectable leaf
        </TreePrimitive.Item>
      </TreePrimitive.Root>,
    );
    const selection = (name: string) =>
      screen.getByRole("treeitem", { name }).getAttribute("aria-selected");
    expect(selection("Selectable branch")).toBe("true");
    expect(selection("Selectable leaf")).toBe("false");
    expect(selection("Nonselectable branch")).toBeNull();
    expect(selection("Nonselectable leaf")).toBeNull();
    expect(
      screen.getByRole("treeitem", { name: "Nonselectable branch" }).getAttribute("aria-expanded"),
    ).toBe("false");
  });
});

describe("tree keyboard navigation", () => {
  it("does not enter the next sibling when an expanded branch has no visible children", () => {
    testClient.render(
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
    testClient.render(
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

describe("tree focus entry", () => {
  function renderTree(rootProps: ComponentProps<typeof TreePrimitive.Root> = {}) {
    const onActivate = vi.fn();
    testClient.render(
      <>
        <button>Outside</button>
        <TreePrimitive.Root aria-label="Files" {...rootProps}>
          <TreePrimitive.Item level={1} selected focusable={false} onActivate={onActivate}>
            Selected
          </TreePrimitive.Item>
          <TreePrimitive.Item level={1} selected={false} focusable onActivate={onActivate}>
            Cursor
          </TreePrimitive.Item>
          <TreePrimitive.Item level={1} selected={false} focusable={false} onActivate={onActivate}>
            Next
          </TreePrimitive.Item>
          <TreePrimitive.Group>
            <button>Retry</button>
          </TreePrimitive.Group>
        </TreePrimitive.Root>
      </>,
    );
    return { onActivate, tree: screen.getByRole("tree", { name: "Files" }) };
  }

  it("lets the browser reveal the existing cursor row without changing selection", () => {
    const { onActivate, tree } = renderTree();
    const cursor = screen.getByRole("treeitem", { name: "Cursor" });
    const focusCursor = vi.spyOn(cursor, "focus");

    act(() => tree.focus());
    expect(document.activeElement).toBe(cursor);
    expect(focusCursor).toHaveBeenCalledExactlyOnceWith();
    expect(onActivate).not.toHaveBeenCalled();
    expect(screen.getByRole("treeitem", { name: "Selected" }).getAttribute("aria-selected")).toBe(
      "true",
    );
    fireEvent.keyDown(cursor, { key: "ArrowDown" });
    expect(document.activeElement).toBe(screen.getByRole("treeitem", { name: "Next" }));
  });

  it("leaves focus on a descendant the tree did not hand it", () => {
    renderTree();
    for (const descendant of [
      screen.getByRole("treeitem", { name: "Next" }),
      screen.getByRole("button", { name: "Retry" }),
    ]) {
      act(() => descendant.focus());
      expect(document.activeElement).toBe(descendant);
    }
  });

  it("keeps focus on the tree when its caller prevents the entry", () => {
    const { tree } = renderTree({ onFocus: (event) => event.preventDefault() });
    act(() => tree.focus());
    expect(document.activeElement).toBe(tree);
  });

  it("keeps focus where its caller moved it", () => {
    const { tree } = renderTree({
      onFocus: () => screen.getByRole("button", { name: "Outside" }).focus(),
    });
    act(() => tree.focus());
    expect(document.activeElement).toBe(screen.getByRole("button", { name: "Outside" }));
  });

  it("keeps focus on a tree without a cursor row", () => {
    testClient.render(
      <TreePrimitive.Root aria-label="Files">
        <TreePrimitive.Item level={1} selected focusable={false} onActivate={vi.fn()}>
          Only
        </TreePrimitive.Item>
      </TreePrimitive.Root>,
    );
    const tree = screen.getByRole("tree", { name: "Files" });
    act(() => tree.focus());
    expect(document.activeElement).toBe(tree);
  });
});
