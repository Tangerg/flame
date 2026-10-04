import { afterEach, expect, it, vi } from "vitest";
import type { FlameClient } from "@flame/runtime-contract/client";
import type { PluginInstallation } from "@flame/runtime-contract/wire";
import { queryClient } from "@/lib/queryClient";
import { definePlugin, contributionsTo } from "@/plugins/sdk";
import { COLOR_THEME } from "@/plugins/sdk/kernelPoints";
import { loadPluginsForTest, resetKernelForTest } from "@/plugins/sdk/testKernel";
import {
  RuntimeConnectionGeneration,
  RUNTIME_STREAM,
} from "@/plugins/builtin/runtime/public/services";
import { packageOperations, PACKAGES_KEY } from "./application/packages";
import { createPluginsPane } from "./index";

vi.mock("@/plugins/builtin/theme/public/appearance", () => ({ retainThemeSelection: vi.fn() }));

afterEach(async () => {
  await resetKernelForTest();
  queryClient.removeQueries({ queryKey: [PACKAGES_KEY] });
});

function connectionEvents(signal: AbortSignal): AsyncIterable<never> {
  return {
    [Symbol.asyncIterator]() {
      return {
        async next() {
          if (!signal.aborted)
            await new Promise<void>((resolve) =>
              signal.addEventListener("abort", () => resolve(), { once: true }),
            );
          return { done: true, value: undefined } as const;
        },
        async return() {
          return { done: true, value: undefined } as const;
        },
      };
    },
  };
}

function client(
  list: (signal?: AbortSignal) => Promise<{ data: PluginInstallation[] }>,
): FlameClient {
  return {
    plugins: { list },
    runtimeEvents: {
      subscribe: async (_params: unknown, signal: AbortSignal) => ({
        events: connectionEvents(signal),
      }),
    },
  } as unknown as FlameClient;
}

function installation(id: string, name: string): PluginInstallation {
  const digest = "a".repeat(64);
  return {
    id,
    source: "/package",
    enabled: true,
    approvedDigest: digest,
    availability: [],
    grants: [],
    values: {},
    disabledServers: [],
    disabledSkills: [],
    selected: {
      digest,
      name,
      requests: [],
      servers: [],
      inputs: [],
      skills: [],
      diagnostics: [],
      themes: [{ id: "theme", title: name, scheme: "dark", colors: { background: "#181a1e" } }],
    },
  };
}

it("joins the retired Runtime before publishing the latest package generation", async () => {
  const late = Promise.withResolvers<{ data: PluginInstallation[] }>();
  const original = vi.fn(() => late.promise);
  const successor = vi.fn(async () => ({
    data: [installation("d3cbafab-ef30-4e20-9583-42f5316dc865", "latest")],
  }));
  let selected = client(original);
  let generation = RuntimeConnectionGeneration.forProcess("first");
  const subscribers = new Set<() => void>();
  const runtime = definePlugin({
    name: "test.package-runtime",
    provides: { stream: RUNTIME_STREAM },
    setup: () => ({
      stream: {
        connectionGeneration: () => generation,
        subscribeConnection(listener: () => void) {
          subscribers.add(listener);
          return () => subscribers.delete(listener);
        },
        reportConnectionLoss: vi.fn(),
      },
    }),
  });
  await loadPluginsForTest(
    runtime,
    createPluginsPane(() => selected),
  );
  await vi.waitFor(() => expect(original).toHaveBeenCalledOnce());
  const retired = packageOperations.get();
  generation = RuntimeConnectionGeneration.forProcess("intermediate");
  for (const listener of subscribers) listener();
  expect(retired.signal.aborted).toBe(true);
  generation = RuntimeConnectionGeneration.forProcess("latest");
  selected = client(successor);
  for (const listener of subscribers) listener();
  late.resolve({ data: [installation("0c31c796-224c-40fa-a721-6696971bb697", "retired")] });
  await vi.waitFor(() => expect(successor).toHaveBeenCalledOnce());
  await vi.waitFor(() =>
    expect(contributionsTo(COLOR_THEME).map((entry) => entry.item.label)).toEqual([
      "latest · latest",
    ]),
  );
  expect(queryClient.getQueryData([PACKAGES_KEY])).toEqual([
    installation("d3cbafab-ef30-4e20-9583-42f5316dc865", "latest"),
  ]);
  expect(packageOperations.get().signal.aborted).toBe(false);
  await resetKernelForTest();
  expect(packageOperations.peek()).toBeNull();
  expect(subscribers.size).toBe(0);
  expect(contributionsTo(COLOR_THEME)).toEqual([]);
});
