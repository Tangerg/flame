import { browserPluginCarrier } from "@/platform/browserPluginCarrier";
import { cleanup, render, screen } from "@testing-library/react";
import { configureNavigator } from "@/lib/navigation";
import { createMemoryNavigator } from "@/lib/navigation.testkit";
import { createElement } from "react";
import { afterEach, expect, it, onTestFinished, vi } from "vitest";
import type { FlameClient } from "@flame/runtime-contract/client";
import type { PluginInstallation } from "@flame/runtime-contract/wire";
import { queryClient } from "@/lib/queryClient";
import { definePlugin, contributionsTo } from "@/plugins/sdk";
import { COLOR_THEME, WORKSPACE_VIEW } from "@/plugins/sdk/kernelPoints";
import { loadPluginsForTest, resetKernelForTest } from "@/plugins/sdk/testKernel";
import { usePluginErrorStore } from "@/plugins/sdk/errors";
import {
  RuntimeConnectionGeneration,
  RUNTIME_STREAM,
} from "@/plugins/builtin/runtime/public/services";
import type { ContributionLifetime } from "@/plugins/sdk/definePlugin";
import { packageOperations, PACKAGES_KEY, usePackageRealization } from "./application/packages";
import { createPluginsPane } from "./index";
import {
  contributePaletteTheme,
  retainThemeSelection,
} from "@/plugins/builtin/theme/public/appearance";

vi.mock("@/plugins/builtin/theme/public/appearance", () => ({
  retainThemeSelection: vi.fn(),
  contributePaletteTheme: vi.fn(
    (
      owner: Pick<ContributionLifetime, "contribute">,
      theme: { id: string; label: string; scheme: "dark" | "light" },
    ) => owner.contribute(COLOR_THEME, { id: theme.id, label: theme.label, scheme: theme.scheme }),
  ),
}));

afterEach(async () => {
  cleanup();
  vi.mocked(retainThemeSelection).mockClear();
  await resetKernelForTest();
  queryClient.removeQueries({ queryKey: [PACKAGES_KEY] });
  usePackageRealization.setState({ failure: null });
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
  subscribe: (
    params: unknown,
    signal: AbortSignal,
  ) => Promise<{ events: AsyncIterable<unknown> }> = async (
    _params: unknown,
    signal: AbortSignal,
  ) => ({
    events: connectionEvents(signal),
  }),
): FlameClient {
  return {
    plugins: { list },
    runtimeEvents: { subscribe },
  } as unknown as FlameClient;
}

function runtimePlugin() {
  const subscribers = new Set<() => void>();
  return definePlugin({
    name: "test.package-runtime",
    provides: { stream: RUNTIME_STREAM },
    setup: () => ({
      stream: {
        connectionGeneration: () => RuntimeConnectionGeneration.forProcess("only"),
        subscribeConnection(listener: () => void) {
          subscribers.add(listener);
          return () => subscribers.delete(listener);
        },
        reportConnectionLoss: vi.fn(),
      },
    }),
  });
}

function installation(id: string, name: string): PluginInstallation {
  const digest = "a".repeat(64);
  return {
    id,
    source: "/package",
    state: "enabled",
    realization: { type: "available", unavailableBackends: [] },
    presentation: "admitted",
    inputStates: {},
    disabledServers: [],
    disabledSkills: [],
    selected: {
      digest,
      name,
      servers: [],
      inputs: [],
      skills: [],
      diagnostics: [],
      views: [],
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
    createPluginsPane(() => selected, browserPluginCarrier),
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

it("keeps a failed realization local to this client and retries it on request", async () => {
  const list = vi.fn(async () => ({
    data: [installation("d3cbafab-ef30-4e20-9583-42f5316dc865", "package")],
  }));
  const subscribe = vi
    .fn()
    .mockRejectedValueOnce(new Error("event stream refused"))
    .mockImplementation(async (_params: unknown, signal: AbortSignal) => ({
      events: connectionEvents(signal),
    }));
  await loadPluginsForTest(
    runtimePlugin(),
    createPluginsPane(() => client(list, subscribe), browserPluginCarrier),
  );
  await vi.waitFor(() =>
    expect(usePackageRealization.getState().failure?.reason).toBe("event stream refused"),
  );
  expect(contributionsTo(COLOR_THEME)).toEqual([]);
  expect(list).not.toHaveBeenCalled();

  usePackageRealization.getState().failure!.retry();

  expect(usePackageRealization.getState().failure).toBeNull();
  await vi.waitFor(() =>
    expect(contributionsTo(COLOR_THEME).map((entry) => entry.item.label)).toEqual([
      "package · package",
    ]),
  );
  expect(subscribe).toHaveBeenCalledTimes(2);
});

it("withdraws its realization failure when the owning generation retires", async () => {
  const subscribe = vi.fn().mockRejectedValue(new Error("event stream refused"));
  await loadPluginsForTest(
    runtimePlugin(),
    createPluginsPane(() => client(async () => ({ data: [] }), subscribe), browserPluginCarrier),
  );
  await vi.waitFor(() => expect(usePackageRealization.getState().failure).not.toBeNull());
  await resetKernelForTest();
  expect(usePackageRealization.getState().failure).toBeNull();
});

it("consumes a realization retry once and refuses it after retirement", async () => {
  const subscribe = vi
    .fn()
    .mockRejectedValueOnce(new Error("event stream refused"))
    .mockImplementation(async (_params: unknown, signal: AbortSignal) => ({
      events: connectionEvents(signal),
    }));
  await loadPluginsForTest(
    runtimePlugin(),
    createPluginsPane(() => client(async () => ({ data: [] }), subscribe), browserPluginCarrier),
  );
  await vi.waitFor(() => expect(usePackageRealization.getState().failure).not.toBeNull());
  const failed = usePackageRealization.getState().failure!;
  failed.retry();
  failed.retry();
  await vi.waitFor(() => expect(subscribe).toHaveBeenCalledTimes(2));
  await resetKernelForTest();
  expect(() => failed.retry()).not.toThrow();
  expect(subscribe).toHaveBeenCalledTimes(2);
});

it("withdraws every contribution and reports failure when one cleanup rejects", async () => {
  const contribute = vi.mocked(contributePaletteTheme);
  const original = contribute.getMockImplementation()!;
  const disposed: string[] = [];
  contribute.mockImplementation((owner, theme) => {
    original(owner, theme);
    owner.cleanup(() => {
      disposed.push(theme.label);
      if (theme.label.startsWith("first")) throw new Error("page close failed");
    });
  });
  onTestFinished(() => {
    contribute.mockImplementation(original);
  });
  const failure = Promise.withResolvers<void>();
  const subscribe = async () => ({
    events: {
      [Symbol.asyncIterator]() {
        return {
          async next() {
            await failure.promise;
            throw new Error("event stream failed");
          },
          async return() {
            return { done: true, value: undefined } as const;
          },
        };
      },
    },
  });
  await loadPluginsForTest(
    runtimePlugin(),
    createPluginsPane(
      () =>
        client(
          async () => ({
            data: [
              installation("d3cbafab-ef30-4e20-9583-42f5316dc865", "first"),
              installation("0c31c796-224c-40fa-a721-6696971bb697", "second"),
            ],
          }),
          subscribe,
        ),
      browserPluginCarrier,
    ),
  );
  await vi.waitFor(() => expect(contributionsTo(COLOR_THEME)).toHaveLength(2));
  failure.resolve();
  await vi.waitFor(() => expect(contributionsTo(COLOR_THEME)).toEqual([]));
  expect(disposed.sort()).toEqual(["first · first", "second · second"]);
  await vi.waitFor(() => expect(usePackageRealization.getState().failure).not.toBeNull());
  const reason = usePackageRealization.getState().failure!.reason;
  expect(reason).toContain("event stream failed");
  expect(reason).toContain("page close failed");
});

it("preserves a snapshot failure when closing its subscription also fails", async () => {
  const defaults = queryClient.getQueryDefaults([PACKAGES_KEY]);
  queryClient.setQueryDefaults([PACKAGES_KEY], { retry: false });
  onTestFinished(() => queryClient.setQueryDefaults([PACKAGES_KEY], defaults));
  const close = vi.fn(async () => {
    throw new Error("stream close failed");
  });
  const subscribe = async () => ({
    events: {
      [Symbol.asyncIterator]() {
        return {
          async next() {
            return { done: true, value: undefined } as const;
          },
          return: close,
        };
      },
    },
  });
  await loadPluginsForTest(
    runtimePlugin(),
    createPluginsPane(
      () =>
        client(async () => {
          throw new Error("snapshot refused");
        }, subscribe),
      browserPluginCarrier,
    ),
  );
  await vi.waitFor(() => expect(usePackageRealization.getState().failure).not.toBeNull());
  const reason = usePackageRealization.getState().failure!.reason;
  expect(reason).toContain("snapshot refused");
  expect(reason).toContain("stream close failed");
  expect(close).toHaveBeenCalledOnce();
  await resetKernelForTest();
  expect(close).toHaveBeenCalledOnce();
});

it.each(["resolve", "reject"] as const)(
  "joins a late subscription whose close will %s",
  async (outcome) => {
    const late = Promise.withResolvers<{ events: AsyncIterable<never> }>();
    const stopping = Promise.withResolvers<void>();
    const close = vi.fn(() => stopping.promise);
    const acquired = {
      events: {
        [Symbol.asyncIterator]() {
          return {
            async next() {
              return { done: true, value: undefined } as const;
            },
            async return() {
              await close();
              return { done: true, value: undefined } as const;
            },
          };
        },
      },
    };
    const subscribe = vi.fn((_params: unknown, _signal: AbortSignal) => late.promise);
    const list = vi.fn(async () => ({ data: [] }));
    onTestFinished(() => {
      late.resolve(acquired);
      stopping.resolve();
    });
    await loadPluginsForTest(
      runtimePlugin(),
      createPluginsPane(() => client(list, subscribe), browserPluginCarrier),
    );
    await vi.waitFor(() => expect(subscribe).toHaveBeenCalledOnce());
    let stopped = false;
    const retirement = resetKernelForTest().then(() => {
      stopped = true;
    });
    await vi.waitFor(() => expect(subscribe.mock.calls[0]![1].aborted).toBe(true));
    late.resolve(acquired);
    await vi.waitFor(() => expect(close).toHaveBeenCalledOnce());
    expect(stopped).toBe(false);
    expect(list).not.toHaveBeenCalled();
    if (outcome === "reject") stopping.reject(new Error("late stream close failed"));
    else stopping.resolve();
    await retirement;
    expect(usePackageRealization.getState().failure).toBeNull();
    expect(
      usePluginErrorStore
        .getState()
        .log.filter((entry) => entry.message === "late stream close failed"),
    ).toHaveLength(outcome === "reject" ? 1 : 0);
  },
);

it.each(["initial snapshot", "event refresh", "event stream"] as const)(
  "releases the failed %s subscription before retry and retirement",
  async (stage) => {
    const defaults = queryClient.getQueryDefaults([PACKAGES_KEY]);
    queryClient.setQueryDefaults([PACKAGES_KEY], { retry: false });
    onTestFinished(() => queryClient.setQueryDefaults([PACKAGES_KEY], defaults));
    const ready = Promise.withResolvers<void>();
    const active = new Set<AbortSignal>();
    const row = installation("d3cbafab-ef30-4e20-9583-42f5316dc865", "package");
    const failure = new Error(`${stage} unavailable`);
    let failing = true;
    const list = vi.fn(async (): Promise<{ data: PluginInstallation[] }> => {
      if (failing && (stage === "initial snapshot" || list.mock.calls.length > 1)) throw failure;
      return { data: [row] };
    });
    const subscribe = vi.fn(async (_params: unknown, signal: AbortSignal) => {
      await ready.promise;
      active.add(signal);
      const close = () => {
        active.delete(signal);
        signal.removeEventListener("abort", close);
      };
      signal.addEventListener("abort", close, { once: true });
      return {
        events: {
          [Symbol.asyncIterator]() {
            const pending = connectionEvents(signal)[Symbol.asyncIterator]();
            return {
              async next() {
                if (failing && stage === "event stream") throw failure;
                if (failing && stage === "event refresh")
                  return { done: false, value: {} } as const;
                return pending.next();
              },
              async return() {
                close();
                return { done: true, value: undefined } as const;
              },
            };
          },
        },
      };
    });
    await loadPluginsForTest(
      runtimePlugin(),
      createPluginsPane(() => client(list, subscribe), browserPluginCarrier),
    );
    ready.resolve();
    await vi.waitFor(() =>
      expect(usePackageRealization.getState().failure?.reason).toBe(failure.message),
    );
    expect(active.size).toBe(0);
    expect(contributionsTo(COLOR_THEME)).toEqual([]);

    failing = false;
    usePackageRealization.getState().failure!.retry();

    await vi.waitFor(() => expect(subscribe).toHaveBeenCalledTimes(2));
    await vi.waitFor(() =>
      expect(contributionsTo(COLOR_THEME).map((entry) => entry.item.label)).toEqual([
        "package · package",
      ]),
    );
    expect(active.size).toBe(1);
    await resetKernelForTest();
    expect(active.size).toBe(0);
    expect(contributionsTo(COLOR_THEME)).toEqual([]);
  },
);

it("presents and retains only the themes Runtime admits", async () => {
  const admitted = installation("d3cbafab-ef30-4e20-9583-42f5316dc865", "admitted");
  const withheld: PluginInstallation = {
    ...installation("0c31c796-224c-40fa-a721-6696971bb697", "withheld"),
    presentation: "withheld",
  };
  admitted.selected.views = [{ id: "trajectory", title: "Trajectory", type: "sessionTrajectory" }];
  withheld.selected.views = admitted.selected.views;
  await loadPluginsForTest(
    runtimePlugin(),
    createPluginsPane(
      () => client(async () => ({ data: [admitted, withheld] })),
      browserPluginCarrier,
    ),
  );
  await vi.waitFor(() =>
    expect(contributionsTo(COLOR_THEME).map((entry) => entry.item.label)).toEqual([
      "admitted · admitted",
    ]),
  );
  expect(
    contributionsTo(WORKSPACE_VIEW).map((entry) => ({
      id: entry.item.id,
      title: entry.item.title,
    })),
  ).toEqual([
    {
      id: "package:d3cbafab-ef30-4e20-9583-42f5316dc865:trajectory",
      title: "admitted · Trajectory",
    },
  ]);
  expect(retainThemeSelection).toHaveBeenLastCalledWith(
    ["package:d3cbafab-ef30-4e20-9583-42f5316dc865:theme"],
    "package:",
  );
});

it("cancels and joins both initial view reads when either one fails", async () => {
  const row = installation("d3cbafab-ef30-4e20-9583-42f5316dc865", "package");
  row.selected.views = [{ id: "trajectory", title: "Trajectory", type: "sessionTrajectory" }];
  const pending = Promise.withResolvers<{ data: [] }>();
  const readView = vi.fn(async () => {
    throw new Error("package resource unavailable");
  });
  const readTrajectory = vi.fn((_params: unknown, _signal?: AbortSignal) => pending.promise);
  const selected = client(async () => ({ data: [row] }));
  selected.plugins.readView = readView;
  selected.plugins.readTrajectory = readTrajectory;
  const close = vi.fn(async () => {});
  const send = vi.fn(async () => {});
  try {
    await loadPluginsForTest(
      runtimePlugin(),
      createPluginsPane(() => selected, {
        async open(_container, _signal, receive) {
          receive({ type: "ready" });
          return { close, send };
        },
      }),
    );
    await vi.waitFor(() => expect(contributionsTo(WORKSPACE_VIEW)).toHaveLength(1));
    configureNavigator(createMemoryNavigator({ session: "session-a" }));
    const View = contributionsTo(WORKSPACE_VIEW)[0]!.item.component;
    render(createElement(View));
    await vi.waitFor(() => expect(readTrajectory).toHaveBeenCalledOnce());
    expect(readTrajectory.mock.calls[0]![1]!.aborted).toBe(true);
    expect(send).not.toHaveBeenCalled();
    let retired = false;
    const retirement = resetKernelForTest().then(() => (retired = true));
    await vi.waitFor(() => expect(close).toHaveBeenCalledOnce());
    expect(retired).toBe(false);
    pending.resolve({ data: [] });
    await retirement;
    expect(readTrajectory.mock.calls[0]![0]).toEqual({
      installationId: row.id,
      digest: row.selected.digest,
      viewId: "trajectory",
      sessionId: "session-a",
      cursor: undefined,
    });
    expect(screen.queryByRole("alert")).toBeNull();
  } finally {
    pending.resolve({ data: [] });
  }
});
