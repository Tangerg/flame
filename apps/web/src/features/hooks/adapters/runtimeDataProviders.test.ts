import { describe, expect, it, vi } from "vitest";
import { lookupDataProvider } from "@/extensions/dataProvider";
import { contributeForTest } from "@/test/kernel";
import { createFlameClient } from "@flame/runtime-client";
import { createMemoryTransport } from "@flame/runtime-client/testing";
import { respondSuccess, waitForRequest } from "@flame/runtime-client/testing";
import { HOOKS_KEY } from "@/features/hooks/application/hookQueries";
import { registerHookDataProviders } from "@/features/hooks/adapters/runtimeDataProviders";
describe("Hooks Runtime data provider", () => {
  it("binds inspection to the selected workspace and read cancellation", async () => {
    const transport = createMemoryTransport();
    const client = createFlameClient(transport);
    const send = vi.spyOn(transport, "send");
    const lifetime = new AbortController();
    try {
      await contributeForTest((ctx) => registerHookDataProviders(ctx, () => client));
      const fetcher = lookupDataProvider(HOOKS_KEY);
      if (!fetcher) throw new Error("Hooks data provider is not installed");
      const pending = fetcher({ cwd: "/work/selected" }, lifetime.signal);
      const request = await waitForRequest(transport, "hooks.list");
      expect(request.params).toEqual({ workspace: { path: "/work/selected" } });
      expect(send.mock.calls[0]?.[1]).toBe(lifetime.signal);
      respondSuccess(transport, request.id, { hooks: [], projectTrusted: false });
      await expect(pending).resolves.toEqual({ hooks: [], projectTrusted: false });
    } finally {
      await client.close();
    }
  });
});
