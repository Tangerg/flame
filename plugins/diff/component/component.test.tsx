// @vitest-environment happy-dom
import type { ComponentContext } from "@flame/component-sdk";
import type { Diff, GetDiffRequest } from "@flame/runtime-client/wire";
import { act } from "react";
import { createRoot } from "react-dom/client";
import { expect, it, vi } from "vitest";
import { WorkspaceDiff } from "./component";

it.each([
  [true, "No complete file fits this read’s budget."],
  [false, "No changes in this comparison."],
])(
  "returns from a scoped read to Refresh with an empty manifest (%s)",
  async (truncated, message) => {
    const get = vi.fn(async (_request: GetDiffRequest): Promise<Diff> => ({
      type: "visual",
      baseline: { type: "emptyTree" },
      visual: {
        patch: "",
        manifest: [],
        budget: { files: 1, rows: 1, bytes: 100 },
        truncated,
      },
    }));
    const signal = new AbortController().signal;
    const context = {
      locale: "en",
      signal,
      runtime: { workspace: () => ({ diff: { get } }) },
      workspace: {
        ref: { path: "/workspace" },
        focus: { path: "src/target.ts", revision: 1n },
      },
      track: <P extends Promise<unknown>>(start: (signal: AbortSignal) => P) =>
        start(signal),
    } as unknown as ComponentContext;
    const host = document.body.appendChild(document.createElement("div"));
    const root = createRoot(host);
    vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
    try {
      await act(async () => root.render(<WorkspaceDiff context={context} />));
      expect(host.textContent).toContain(message);
      expect(host.textContent).toContain("src/target.ts");
      expect(host.querySelector('[aria-label="Filter files"]')).toBeNull();
      expect(host.textContent).not.toMatch(/0 (included|changed) files/);

      const showFiles = host.querySelector<HTMLButtonElement>(
        '[aria-label="Show changed files"]',
      )!;
      await act(async () => {
        showFiles.focus();
        showFiles.click();
      });
      expect(get.mock.calls.map(([request]) => request.path)).toEqual([
        "src/target.ts",
        undefined,
      ]);
      expect(document.activeElement).toBe(
        host.querySelector('[aria-label="Refresh"]'),
      );
    } finally {
      act(() => root.unmount());
      host.remove();
      vi.unstubAllGlobals();
    }
  },
);
