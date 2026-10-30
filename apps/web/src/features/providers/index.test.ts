import { afterEach, expect, it } from "vitest";
import { type FlameClient } from "@flame/runtime-client";
import { lookupDataProvider } from "@/extensions/dataProvider";
import { loadPluginsForTest, resetKernelForTest } from "@/test/kernel";
import { createProviderCatalogPlugin } from "./index";

afterEach(resetKernelForTest);

it("publishes credential readiness and withdraws the read capability on disposal", async () => {
  const providers = [{ id: "endpoint", configured: true, credentialRequirement: "apiKeyOptional" }];
  const client = {
    providers: { list: async () => ({ data: providers }) },
  } as unknown as FlameClient;
  await loadPluginsForTest(createProviderCatalogPlugin(() => client));
  const read = lookupDataProvider("providers");
  expect(read).toBeDefined();
  expect(await read!()).toEqual([{ configured: true }]);
  await resetKernelForTest();
  expect(lookupDataProvider("providers")).toBeUndefined();
});
