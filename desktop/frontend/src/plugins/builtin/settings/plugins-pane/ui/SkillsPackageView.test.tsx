import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { ContributionLifetime } from "@/plugins/sdk/definePlugin";
import type { PluginCarrier } from "@/foundation/pluginCarrier";
import { contributeForTest, resetKernelForTest } from "@/plugins/sdk/testKernel";
import type { SkillViewSnapshot } from "../application/pluginView";
import { SkillsPackageView } from "./SkillsPackageView";
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
vi.mock("@/plugins/builtin/workspace/public/skills", () => ({
  SkillInspectionControl: ({ onInspect }: { onInspect(name: string): void }) => (
    <button onClick={() => onInspect("verify")}>Inspect verify</button>
  ),
  SkillLibraryControls: ({ onSaved }: { onSaved(): void }) => (
    <button onClick={onSaved}>Runtime receipt</button>
  ),
  SkillProposalControls: ({ onSaved }: { onSaved(): void }) => (
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
    lifetime = ctx.lifetime("skills-view");
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
  const initial: SkillViewSnapshot = { type: "available", data: { skills: [], diagnostics: [] } };
  const reads = vi.fn(() => ({
    load: async () => ({ html: "<!doctype html>", initial }),
    read: async () => initial,
  }));
  const props = { title: "Skills", reads, lifetime, carrier };
  const view = render(<SkillsPackageView {...props} />);
  await waitFor(() => expect(pages).toHaveLength(1));
  return { props, view, reads, pages };
}
it("retires captured documents and workspaces while keeping the user library independent", async () => {
  const f = await fixture();
  expect(f.reads).toHaveBeenLastCalledWith({
    type: "available",
    workspace: { path: "/project-a" },
  });
  fireEvent.click(screen.getByRole("button", { name: "Inspect verify" }));
  await waitFor(() => expect(f.pages).toHaveLength(2));
  expect(f.reads).toHaveBeenLastCalledWith({
    type: "detail",
    workspace: { path: "/project-a" },
    name: "verify",
  });
  await waitFor(() => expect(f.pages[0]!.close).toHaveBeenCalledOnce());
  model.workspace = { status: "ready", cwd: "/project-b" };
  f.view.rerender(<SkillsPackageView {...f.props} />);
  await waitFor(() => expect(f.pages).toHaveLength(3));
  expect(f.reads).toHaveBeenLastCalledWith({
    type: "available",
    workspace: { path: "/project-b" },
  });
  expect(f.pages[1]!.signal.aborted).toBe(true);
  fireEvent.click(screen.getByRole("tab", { name: "Personal" }));
  await waitFor(() => expect(f.pages).toHaveLength(4));
  expect(f.reads).toHaveBeenLastCalledWith({ type: "library" });
  model.workspace = { status: "resolving" };
  f.view.rerender(<SkillsPackageView {...f.props} />);
  expect(f.pages).toHaveLength(4);
  expect(f.pages[3]!.signal.aborted).toBe(false);
  fireEvent.click(screen.getByRole("tab", { name: "Review" }));
  await waitFor(() => expect(f.pages[3]!.close).toHaveBeenCalledOnce());
  expect(f.pages).toHaveLength(4);
});
it("refreshes a proposal projection after a trusted decision and retires on capability loss", async () => {
  const f = await fixture();
  fireEvent.click(screen.getByRole("tab", { name: "Review" }));
  await waitFor(() => expect(f.pages).toHaveLength(2));
  fireEvent.click(screen.getByRole("button", { name: "Runtime receipt" }));
  await waitFor(() => expect(f.pages).toHaveLength(3));
  await waitFor(() => expect(f.pages[1]!.close).toHaveBeenCalledOnce());
  model.available = false;
  f.view.rerender(<SkillsPackageView {...f.props} />);
  await waitFor(() => expect(f.pages[2]!.close).toHaveBeenCalledOnce());
  expect(screen.getByText("Skills are off")).toBeDefined();
});
