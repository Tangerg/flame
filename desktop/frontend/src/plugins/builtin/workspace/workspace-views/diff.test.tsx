import type { ReactNode } from "react";
import { act, fireEvent, render } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { WorkspaceFileDiff } from "../application/diffViewModel";

const projection = vi.hoisted(() => ({
  fileFocus: { path: "src/a.ts", revision: 1n },
  files: [] as WorkspaceFileDiff[],
}));

const initialFiles: WorkspaceFileDiff[] = [
  { path: "src/a.ts", status: "modified", rows: [] },
  { path: "src/b.ts", status: "modified", rows: [] },
];

vi.mock("../application/diffViewModel", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../application/diffViewModel")>()),
  useWorkspaceDiffView: () => ({
    fileFocus: projection.fileFocus,
    files: projection.files,
    gitEnabled: true,
    isError: false,
    isLoading: false,
    notARepo: false,
    view: {
      baseline: { type: "head", commit: "1234567890abcdef1234567890abcdef12345678" },
      files: projection.files,
      truncated: false,
    },
  }),
}));

vi.mock("@/ui", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/ui")>()),
  Segmented: () => <div />,
}));

vi.mock("@/ui/agent", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/ui/agent")>()),
  AgentViewNavigatorToggle: () => <button type="button" aria-label="toggle files" />,
  AgentViewSplit: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  AgentWorkspaceView: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}));

vi.mock("./views/DiffView", () => ({
  DiffView: () => <div />,
}));

vi.mock("./views/ReviewFileTree", () => ({
  ReviewFileTree: () => <div />,
}));

import { DiffWorkspaceSurface } from "./diff";
import { useContextDockStore } from "../adapters/contextDockStore";

let nativeScrollIntoView: typeof HTMLElement.prototype.scrollIntoView | undefined;
const scrolledPaths: string[] = [];

beforeEach(() => {
  projection.fileFocus = { path: "src/a.ts", revision: 1n };
  projection.files = initialFiles;
  scrolledPaths.length = 0;
  nativeScrollIntoView = HTMLElement.prototype.scrollIntoView;
  HTMLElement.prototype.scrollIntoView = function scrollIntoView() {
    scrolledPaths.push(this.getAttribute("data-diff-file") ?? "");
  };
});

afterEach(() => {
  if (nativeScrollIntoView) HTMLElement.prototype.scrollIntoView = nativeScrollIntoView;
  else Reflect.deleteProperty(HTMLElement.prototype, "scrollIntoView");
});

describe("DiffWorkspaceSurface", () => {
  it("shows the resolved baseline and preserves the full object identity", () => {
    const view = render(<DiffWorkspaceSurface />);
    expect(
      view
        .getByText("HEAD 1234567890ab → working tree, including untracked files")
        .getAttribute("title"),
    ).toBe("1234567890abcdef1234567890abcdef12345678");
  });
  it("locates every file navigation intent while the Diff view stays mounted", () => {
    const view = render(<DiffWorkspaceSurface />);
    expect(scrolledPaths).toEqual(["src/a.ts"]);

    act(() => {
      projection.fileFocus = { path: "src/b.ts", revision: 2n };
      view.rerender(<DiffWorkspaceSurface />);
    });

    expect(scrolledPaths).toEqual(["src/a.ts", "src/b.ts"]);
  });

  it("does not reinterpret a query replacement as a file navigation", () => {
    const view = render(<DiffWorkspaceSurface />);
    expect(scrolledPaths).toEqual(["src/a.ts"]);

    act(() => {
      projection.files = [...initialFiles];
      view.rerender(<DiffWorkspaceSurface />);
    });

    expect(scrolledPaths).toEqual(["src/a.ts"]);
  });

  it("keeps a focus intent pending until its file material arrives", () => {
    projection.files = [];
    const view = render(<DiffWorkspaceSurface />);
    expect(scrolledPaths).toEqual([]);

    act(() => {
      projection.files = initialFiles;
      view.rerender(<DiffWorkspaceSurface />);
    });

    expect(scrolledPaths).toEqual(["src/a.ts"]);
  });

  it("honours a repeated intent for the same file", () => {
    const view = render(<DiffWorkspaceSurface />);

    act(() => {
      projection.fileFocus = { path: "src/a.ts", revision: 2n };
      view.rerender(<DiffWorkspaceSurface />);
    });

    expect(scrolledPaths).toEqual(["src/a.ts", "src/a.ts"]);
  });

  it("opens a collapsed file when it is asked for, not only scrolls to its title", () => {
    projection.fileFocus = { path: "", revision: 1n };
    const view = render(<DiffWorkspaceSurface />);
    const header = () =>
      view.container.querySelector<HTMLButtonElement>('[data-diff-file="src/a.ts"] button')!;
    fireEvent.click(header());
    expect(header().getAttribute("aria-expanded")).toBe("false");

    act(() => useContextDockStore.getState().focusFile("src/a.ts"));
    projection.fileFocus = { path: "src/a.ts", revision: 2n };
    view.rerender(<DiffWorkspaceSurface />);
    expect(header().getAttribute("aria-expanded")).toBe("true");
    expect(scrolledPaths).toContain("src/a.ts");
  });
});
