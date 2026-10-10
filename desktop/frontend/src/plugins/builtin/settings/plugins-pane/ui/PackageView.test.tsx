import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { PluginCarrier } from "@/foundation/pluginCarrier";
import { configureNavigator } from "@/lib/navigation";
import { publishScheme } from "@/lib/appearance";
import { createMemoryNavigator } from "@/lib/navigation.testkit";
import type { ContributionLifetime } from "@/plugins/sdk/definePlugin";
import { contributeForTest, resetKernelForTest } from "@/plugins/sdk/testKernel";
import { PackageView } from "./PackageView";

const capabilities = vi.hoisted(() => ({ subagents: false }));
vi.mock("@/plugins/builtin/runtime/public/capabilities", () => ({
  useRuntimeCapability: () => capabilities.subagents,
}));

afterEach(async () => {
  capabilities.subagents = false;
  cleanup();
  publishScheme("dark");
  await resetKernelForTest();
});

it("captures the host scheme and retires a page when that presentation changes", async () => {
  publishScheme("light");
  const pages: { close: ReturnType<typeof vi.fn>; send: ReturnType<typeof vi.fn> }[] = [];
  const carrier: PluginCarrier = {
    async open(_container, _signal, receive) {
      const page = {
        close: vi.fn(async () => {}),
        send: vi.fn(async () => receive({ type: "connected" })),
      };
      pages.push(page);
      receive({ type: "ready" });
      return page;
    },
  };
  await fixture(carrier);
  await waitFor(() =>
    expect(pages[0]?.send).toHaveBeenCalledWith(expect.objectContaining({ scheme: "light" })),
  );
  act(() => publishScheme("dark"));
  await waitFor(() =>
    expect(pages[1]?.send).toHaveBeenCalledWith(expect.objectContaining({ scheme: "dark" })),
  );
  await waitFor(() => expect(pages[0]!.close).toHaveBeenCalledOnce());
});

async function fixture(carrier: PluginCarrier) {
  let lifetime!: ContributionLifetime;
  await contributeForTest((ctx) => {
    lifetime = ctx.lifetime("package-view");
  });
  const navigation = createMemoryNavigator({ session: "session-a" });
  configureNavigator(navigation);
  const reads = vi.fn((_session: string, _includeDescendants: boolean) => ({
    load: async (_signal: AbortSignal) => ({ html: "<!doctype html>", initial: { data: [] } }),
    read: async () => ({ data: [] }),
  }));
  const view = render(
    <PackageView title="Recorded trajectory" reads={reads} carrier={carrier} lifetime={lifetime} />,
  );
  return { view, reads, navigation, lifetime };
}

it("renders terminal failure even when initialization connected in the same turn", async () => {
  const close = vi.fn(async () => {});
  await fixture({
    async open(_container, _signal, receive) {
      receive({ type: "ready" });
      return {
        close,
        async send() {
          receive({ type: "connected" });
          receive({ type: "failure", reason: "native page terminated" });
        },
      };
    },
  });
  expect((await screen.findByRole("alert")).textContent).toBe("native page terminated");
  await waitFor(() => expect(close).toHaveBeenCalledOnce());
  expect(screen.queryByRole("status")).toBeNull();
});

it("retirements and late messages cannot advance the newly selected Session view", async () => {
  const pages: {
    signal: AbortSignal;
    receive: (message: unknown) => void;
    close: ReturnType<typeof vi.fn>;
  }[] = [];
  const f = await fixture({
    async open(_container, signal, receive) {
      const page = { signal, receive, close: vi.fn(async () => {}) };
      pages.push(page);
      receive({ type: "ready" });
      return {
        close: page.close,
        async send() {
          receive({ type: "connected" });
        },
      };
    },
  });
  await waitFor(() => expect(screen.queryByRole("status")).toBeNull());
  act(() => f.navigation.go({ session: "session-b" }));
  await waitFor(() => expect(pages).toHaveLength(2));
  await waitFor(() => expect(pages[0]!.close).toHaveBeenCalledOnce());
  expect(pages[0]!.signal.aborted).toBe(true);
  act(() => pages[0]!.receive({ type: "failure", reason: "retired native process" }));
  await waitFor(() => expect(screen.queryByRole("status")).toBeNull());
  expect(screen.queryByRole("alert")).toBeNull();
  expect(f.reads.mock.calls.map(([session]) => session)).toEqual(["session-a", "session-b"]);
  f.view.unmount();
  await waitFor(() => expect(pages[1]!.close).toHaveBeenCalledOnce());
});

it("retires the bound cursor scope when descendant capability changes", async () => {
  const pages: { signal: AbortSignal; close: ReturnType<typeof vi.fn> }[] = [];
  const carrier: PluginCarrier = {
    async open(_container, signal, receive) {
      const page = { signal, close: vi.fn(async () => {}) };
      pages.push(page);
      receive({ type: "ready" });
      return {
        close: page.close,
        async send() {
          receive({ type: "connected" });
        },
      };
    },
  };
  const f = await fixture(carrier);
  await waitFor(() => expect(f.reads).toHaveBeenCalledExactlyOnceWith("session-a", false));
  capabilities.subagents = true;
  f.view.rerender(
    <PackageView
      title="Recorded trajectory"
      reads={f.reads}
      carrier={carrier}
      lifetime={f.lifetime}
    />,
  );
  await waitFor(() => expect(f.reads).toHaveBeenLastCalledWith("session-a", true));
  await waitFor(() => expect(pages[0]!.close).toHaveBeenCalledOnce());
  expect(pages[0]!.signal.aborted).toBe(true);
  expect(pages).toHaveLength(2);
});
