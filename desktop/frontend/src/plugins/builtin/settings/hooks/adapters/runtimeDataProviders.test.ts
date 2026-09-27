import { describe, expect, it, vi } from "vitest";
import { lookupDataProvider } from "@/plugins/sdk/selectors";
import { contributeForTest } from "@/plugins/sdk/testKernel";
import { createFlameClient } from "@flame/runtime-contract/client";
import { createMemoryTransport } from "@flame/runtime-contract/client/transports/memory";
import {
  respondSuccess,
  waitForRequest,
} from "@flame/runtime-contract/client/transports/memory.testkit";
import { HOOKS_KEY } from "../application/hookQueries";
import { registerHookDataProviders } from "./runtimeDataProviders";

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
