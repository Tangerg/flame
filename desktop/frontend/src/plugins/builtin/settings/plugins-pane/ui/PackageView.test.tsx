import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { PluginCarrier } from "@/foundation/pluginCarrier";
import { configureNavigator } from "@/lib/navigation";
import { createMemoryNavigator } from "@/lib/navigation.testkit";
import type { ContributionLifetime } from "@/plugins/sdk/definePlugin";
import { contributeForTest, resetKernelForTest } from "@/plugins/sdk/testKernel";
import { PackageView } from "./PackageView";

afterEach(async () => {
  cleanup();
  await resetKernelForTest();
});

async function fixture(carrier: PluginCarrier) {
  let lifetime!: ContributionLifetime;
  await contributeForTest((ctx) => {
    lifetime = ctx.lifetime("package-view");
  });
  const navigation = createMemoryNavigator({ session: "session-a" });
  configureNavigator(navigation);
  const reads = vi.fn((_session: string) => ({
    load: async (_signal: AbortSignal) => ({ html: "<!doctype html>", initial: { data: [] } }),
    read: async () => ({ data: [] }),
  }));
  const view = render(
    <PackageView title="Recorded trajectory" reads={reads} carrier={carrier} lifetime={lifetime} />,
  );
  return { view, reads, navigation };
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
