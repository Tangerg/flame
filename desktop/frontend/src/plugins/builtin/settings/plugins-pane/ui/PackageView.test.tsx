import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { PluginCarrier } from "@/foundation/pluginCarrier";
import type { ContributionLifetime } from "@/plugins/sdk/definePlugin";
import { contributeForTest, resetKernelForTest } from "@/plugins/sdk/testKernel";
import { PackageView } from "./PackageView";

afterEach(async () => {
  cleanup();
  await resetKernelForTest();
});

it("retries initial reads only after the predecessor has physically retired", async () => {
  let lifetime!: ContributionLifetime;
  await contributeForTest((ctx) => {
    lifetime = ctx.lifetime("package-test");
  });
  const closed = Promise.withResolvers<void>();
  const pages: {
    signal: AbortSignal;
    send: ReturnType<typeof vi.fn>;
    close: ReturnType<typeof vi.fn>;
  }[] = [];
  const carrier: PluginCarrier = {
    async open(_container, signal, receive) {
      const page = {
        signal,
        send: vi.fn(async () => receive({ type: "connected" })),
        close: vi.fn(() => (pages.length === 1 ? closed.promise : Promise.resolve())),
      };
      pages.push(page);
      receive({ type: "ready" });
      return page;
    },
  };
  const reads = {
    binding: { installationId: "fixture", digest: "release", viewId: "view" },
    resource: vi.fn(async () => "<!doctype html>"),
    initial: vi.fn(async () => ({ data: [] })),
    read: vi.fn(async () => ({ data: [] })),
  };
  reads.initial.mockRejectedValueOnce(new Error("temporary failure"));
  render(<PackageView title="Package" reads={reads} lifetime={lifetime} carrier={carrier} />);
  await screen.findByRole("alert");
  expect(reads.initial).toHaveBeenCalledTimes(1);
  expect(pages[0]!.send).not.toHaveBeenCalled();
  expect(pages[0]!.signal.aborted).toBe(true);
  fireEvent.click(screen.getByRole("button", { name: "Retry" }));
  await act(async () => {
    await Promise.resolve();
  });
  expect(pages).toHaveLength(1);
  expect(reads.initial).toHaveBeenCalledTimes(1);
  closed.resolve();
  await waitFor(() => expect(pages).toHaveLength(2));
  await waitFor(() => expect(pages[1]!.send).toHaveBeenCalledOnce());
  expect(pages[0]!.signal.aborted).toBe(true);
  expect(pages[0]!.close).toHaveBeenCalledOnce();
  expect(screen.queryByRole("alert")).toBeNull();
});
