import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { ContributionLifetime } from "@/plugins/sdk/definePlugin";
import type { PluginCarrier } from "@/foundation/pluginCarrier";
import { contributeForTest, resetKernelForTest } from "@/plugins/sdk/testKernel";
import { MemoryPackageView } from "./MemoryPackageView";
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
vi.mock("./MemoryActions", () => ({
  MemoryActions: ({ onSaved }: { onSaved(): void }) => (
    <button onClick={onSaved}>Runtime receipt</button>
  ),
}));
afterEach(async () => {
  cleanup();
  model.available = true;
  model.workspace = { status: "ready", cwd: "/project-a" };
  await resetKernelForTest();
});
async function fixture() {
  let lifetime!: ContributionLifetime;
  await contributeForTest((ctx) => {
    lifetime = ctx.lifetime("memory-view");
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
    load: async () => ({ html: "<!doctype html>", initial: { data: [] } }),
    read: async () => ({ data: [] }),
  }));
  const props = { title: "Memory", reads, lifetime, carrier };
  const view = render(<MemoryPackageView {...props} />);
  return { view, props, reads, pages };
}
it("retires the captured project before switching to a user target", async () => {
  const f = await fixture();
  await waitFor(() =>
    expect(f.reads).toHaveBeenCalledExactlyOnceWith({
      scope: "project",
      workspace: { path: "/project-a" },
    }),
  );
  fireEvent.click(screen.getByRole("button", { name: "User" }));
  await waitFor(() => expect(f.reads).toHaveBeenLastCalledWith({ scope: "user" }));
  await waitFor(() => expect(f.pages[0]!.close).toHaveBeenCalledOnce());
  expect(f.pages[0]!.signal.aborted).toBe(true);
  model.workspace = { status: "ready", cwd: "/project-b" };
  f.view.rerender(<MemoryPackageView {...f.props} />);
  expect(f.reads).toHaveBeenCalledTimes(2);
  fireEvent.click(screen.getByRole("button", { name: "Project" }));
  await waitFor(() =>
    expect(f.reads).toHaveBeenLastCalledWith({
      scope: "project",
      workspace: { path: "/project-b" },
    }),
  );
});
it("withdraws project reads while the selected workspace is resolving", async () => {
  const f = await fixture();
  await waitFor(() => expect(f.pages).toHaveLength(1));
  model.workspace = { status: "resolving" };
  f.view.rerender(<MemoryPackageView {...f.props} />);
  await waitFor(() => expect(f.pages[0]!.close).toHaveBeenCalledOnce());
  expect(f.reads).toHaveBeenCalledTimes(1);
  model.workspace = { status: "ready", cwd: "/project-b" };
  f.view.rerender(<MemoryPackageView {...f.props} />);
  await waitFor(() =>
    expect(f.reads).toHaveBeenLastCalledWith({
      scope: "project",
      workspace: { path: "/project-b" },
    }),
  );
});
it("refreshes the page after a trusted receipt and withdraws on capability loss", async () => {
  const f = await fixture();
  await waitFor(() => expect(f.pages).toHaveLength(1));
  fireEvent.click(screen.getByRole("button", { name: "Runtime receipt" }));
  await waitFor(() => expect(f.pages).toHaveLength(2));
  await waitFor(() => expect(f.pages[0]!.close).toHaveBeenCalledOnce());
  act(() => {
    model.available = false;
    f.view.rerender(<MemoryPackageView {...f.props} />);
  });
  await waitFor(() => expect(f.pages[1]!.close).toHaveBeenCalledOnce());
  expect(screen.getByText("Agent memory is unavailable")).toBeTruthy();
});
