import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { ContributionLifetime } from "@/plugins/sdk/definePlugin";
import type { PluginCarrier } from "@/foundation/pluginCarrier";
import { contributeForTest, resetKernelForTest } from "@/plugins/sdk/testKernel";
import { selectWorkspaceDiffMode } from "@/plugins/builtin/workspace/public/navigation";
import {
  openDiffViewInDock,
  openFileInWorkingTreeDiff,
} from "@/plugins/builtin/workspace/public/deeplinks";
import { DiffPackageView } from "./DiffPackageView";
const model = vi.hoisted(() => ({
  available: true,
  workspace: { status: "ready", cwd: "/project-a" } as {
    status: "ready" | "resolving";
    cwd?: string;
  },
}));
vi.mock("@/plugins/builtin/agent/public/session", () => ({
  useActiveSessionWorkspace: () => model.workspace,
}));
vi.mock("@/plugins/builtin/runtime/public/capabilities", () => ({
  useRuntimeCapability: () => model.available,
}));
afterEach(async () => {
  cleanup();
  model.available = true;
  model.workspace = { status: "ready", cwd: "/project-a" };
  await resetKernelForTest();
});
it("retires reads when trusted mode, focus, workspace or Git authority changes", async () => {
  let lifetime!: ContributionLifetime;
  await contributeForTest((ctx) => {
    lifetime = ctx.lifetime("diff-view");
  });
  const pages: { signal: AbortSignal; close: ReturnType<typeof vi.fn> }[] = [];
  const carrier: PluginCarrier = {
    async open(_container, signal, receive) {
      const page = { signal, close: vi.fn(async () => {}) };
      pages.push(page);
      receive({ type: "ready" });
      return { close: page.close, send: async () => receive({ type: "connected" }) };
    },
  };
  const reads = vi.fn(() => ({
    binding: { installationId: "fixture", digest: "release", viewId: "view" },
    resource: async () => "<!doctype html>",
    initial: async () => ({ diff: { baseline: { type: "emptyTree" as const } }, focusPath: "" }),
    read: async () => ({ diff: { baseline: { type: "emptyTree" as const } }, focusPath: "" }),
  }));
  openDiffViewInDock();
  const props = { title: "Diff", reads, lifetime, carrier };
  const view = render(<DiffPackageView {...props} />);
  await waitFor(() => expect(pages).toHaveLength(1));
  expect(reads).toHaveBeenLastCalledWith({
    workspace: { path: "/project-a" },
    mode: "worktree",
    focusPath: "",
  });
  fireEvent.click(screen.getByRole("tab", { name: "Branch" }));
  await waitFor(() =>
    expect(reads).toHaveBeenLastCalledWith({
      workspace: { path: "/project-a" },
      mode: "base",
      focusPath: "",
    }),
  );
  await waitFor(() => expect(pages[0]!.close).toHaveBeenCalledOnce());
  act(() => {
    openFileInWorkingTreeDiff("src/a.ts");
    selectWorkspaceDiffMode("base");
  });
  await waitFor(() =>
    expect(reads).toHaveBeenLastCalledWith({
      workspace: { path: "/project-a" },
      mode: "base",
      focusPath: "src/a.ts",
    }),
  );
  await waitFor(() => expect(pages[1]!.close).toHaveBeenCalledOnce());
  model.workspace = { status: "ready", cwd: "/project-b" };
  view.rerender(<DiffPackageView {...props} />);
  await waitFor(() =>
    expect(reads).toHaveBeenLastCalledWith({
      workspace: { path: "/project-b" },
      mode: "base",
      focusPath: "src/a.ts",
    }),
  );
  await waitFor(() => expect(pages[2]!.close).toHaveBeenCalledOnce());
  model.available = false;
  view.rerender(<DiffPackageView {...props} />);
  await waitFor(() => expect(pages[3]!.close).toHaveBeenCalledOnce());
  expect(pages.every((page) => page.signal.aborted)).toBe(true);
  expect(reads).toHaveBeenCalledTimes(4);
});
