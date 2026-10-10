import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { ContributionLifetime } from "@/plugins/sdk/definePlugin";
import type { PluginCarrier } from "@/foundation/pluginCarrier";
import { contributeForTest, resetKernelForTest } from "@/plugins/sdk/testKernel";
import { UsagePackageView } from "./UsagePackageView";

afterEach(async () => {
  cleanup();
  await resetKernelForTest();
});

it("retires the prior report before rendering a different trusted period", async () => {
  let lifetime!: ContributionLifetime;
  await contributeForTest((ctx) => {
    lifetime = ctx.lifetime("usage-view");
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
    load: async () => ({ html: "<!doctype html>", initial: { total: {}, runs: 1 } }),
    read: async () => ({ total: {}, runs: 1 }),
  }));
  const view = render(
    <UsagePackageView title="Usage" reads={reads} lifetime={lifetime} carrier={carrier} />,
  );
  await waitFor(() => expect(pages).toHaveLength(1));
  expect(reads).toHaveBeenCalledExactlyOnceWith({});
  fireEvent.click(screen.getByRole("tab", { name: "7d" }));
  await waitFor(() => expect(reads).toHaveBeenLastCalledWith({ sinceDays: 7 }));
  await waitFor(() => expect(pages[0]!.close).toHaveBeenCalledOnce());
  expect(pages[0]!.signal.aborted).toBe(true);
  view.unmount();
  await waitFor(() => expect(pages[1]!.close).toHaveBeenCalledOnce());
});
