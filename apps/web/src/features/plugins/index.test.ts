import {
  createInstalledPlugins,
  type InstalledPluginsOwner,
} from "@/features/plugins/adapters/installedPlugins";
import { type ThemeAccess } from "@/features/theme/public/theme";
import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { TOOL_RESULT_RENDERER } from "@/features/chat/public/tools/types";
import { WORKSPACE_FILE_RENDERER } from "@/features/workspace/public/views";
import { act, cleanup } from "@testing-library/react";
import { createElement, useEffect } from "react";
import { type ComponentContext } from "@flame/component-sdk";
import { ComponentModules } from "@/features/plugins/adapters/componentModules";
import { Slot } from "@/extensions/host/Slot";
import { COMMAND } from "@/features/command/public/extensionPoints";
import { afterEach, expect, it, onTestFinished, vi } from "vitest";
import { type FlameClient } from "@flame/runtime-client";
import { type PluginInstallation, type ReadPluginLocaleRequest } from "@flame/runtime-client/wire";
import { queryClient } from "@/lib/queryClient";
import { definePlugin } from "@/extensions/definePlugin";
import { contributionsTo } from "@/extensions/kernel";
import { COLOR_THEME } from "@/features/theme/public/extensionPoints";
import { WORKSPACE_VIEW } from "@/features/workspace/public/views";
import { SETTINGS_PANE } from "@/features/settings/public/panes";
import { LOCALE } from "@/features/i18n/public/extensionPoints";
import { activeLocale, setLocale, t } from "@/lib/i18n/index";
import { loadPluginsForTest, resetKernelForTest } from "@/test/kernel";
import { usePluginErrorStore } from "@/extensions/errors";
import { RuntimeConnectionGeneration, RUNTIME_STREAM } from "@/features/connection/public/services";
import { type ContributionLifetime } from "@/extensions/definePlugin";
import { PACKAGES_KEY } from "@/features/plugins/adapters/packages";
import { createPluginsPlugin } from "@/features/plugins/index";
import { contributePaletteTheme } from "@/features/theme/public/appearance";
import { retainThemeSelection } from "@/features/theme/public/appearance";
let installed: InstalledPluginsOwner;
const pluginsPane = (client: () => FlameClient) => {
  const owned = createInstalledPlugins(
    client,
    () => "http://runtime.test",
    testClient.theme.access,
    async () => "requested",
  );
  installed = owned;
  onTestFinished(() => owned.dispose());
  return createPluginsPlugin(owned);
};

vi.mock("@/features/theme/public/appearance", () => ({
  retainThemeSelection: vi.fn(),
  contributePaletteTheme: vi.fn(
    (
      _preference: ThemeAccess["preference"],
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
    enabled: true,
    realization: { type: "available", unavailableBackends: [] },
    presentation: "admitted",
    inputStates: {},
    disabledServers: [],
    disabledSkills: [],
    selected: {
      digest,
      name,
      contributions: [],
      points: [],
      pointValues: [],
      components: [],
      servers: [],
      inputs: [],
      skills: [],
      diagnostics: [],

      locales: [],

      themes: [{ id: "theme", title: name, scheme: "dark", colors: { background: "#181a1e" } }],
    },
  };
}

it("loads only an admitted locale's exact release and withdraws it with its Runtime generation", async () => {
  const admitted = installation("d3cbafab-ef30-4e20-9583-42f5316dc865", "japanese");
  admitted.selected.locales = [{ tag: "ja", title: "日本語" }];
  const withheld: PluginInstallation = {
    ...installation("0c31c796-224c-40fa-a721-6696971bb697", "withheld"),
    presentation: "withheld",
  };
  withheld.selected.locales = admitted.selected.locales;
  const runtime = client(async () => ({ data: [admitted, withheld] }));
  const read = vi.fn(async (_request: ReadPluginLocaleRequest, _signal?: AbortSignal) => ({
    messages: { "common.loading": "読込中" },
  }));
  runtime.plugins.readLocale = read;
  onTestFinished(() => setLocale("en"));
  await loadPluginsForTest(
    runtimePlugin(),
    pluginsPane(() => runtime),
  );
  await vi.waitFor(() => expect(contributionsTo(LOCALE)).toHaveLength(1));
  const digest = admitted.selected.digest;
  admitted.selected.digest = "b".repeat(64);
  setLocale("ja");
  await contributionsTo(LOCALE)[0]!.item.activate();
  expect(t("common.loading")).toBe("読込中");
  expect(read).toHaveBeenCalledOnce();
  expect(read.mock.calls[0]?.[0]).toEqual({
    installationId: admitted.id,
    digest,
    tag: "ja",
  });
  expect(contributionsTo(LOCALE).map((entry) => entry.item.id)).toEqual(["ja"]);
  await resetKernelForTest();
  expect(t("common.loading")).toBe("Loading");
  expect(activeLocale()).toBe("ja");
});

it("mounts an admitted client effect independently of navigation and withdraws its commands", async () => {
  const admitted = installation("d3cbafab-ef30-4e20-9583-42f5316dc865", "exports");
  admitted.selected.components = [
    {
      id: "exports",
      title: "Exports",
      scope: "client",
      placement: "clientEffect",
      entry: "io.github.tangerg.flame/components/component.js",
      export: "Exports",
    },
  ];
  const withheld = {
    ...installation("0c31c796-224c-40fa-a721-6696971bb697", "withheld"),
    presentation: "withheld" as const,
  };
  withheld.selected.components = admitted.selected.components;
  const run = vi.fn();
  const contexts: ComponentContext[] = [];
  function Effect({ context }: { context: ComponentContext }) {
    useEffect(() => {
      contexts.push(context);
      return context.registerCommand({ id: "export", label: "Export captured Session", run });
    }, [context]);
    return null;
  }
  const load = vi.spyOn(ComponentModules.prototype, "component").mockResolvedValue({
    component: Effect,
    signal: new AbortController().signal,
    reload: async () => {},
  });
  onTestFinished(() => load.mockRestore());
  await loadPluginsForTest(
    runtimePlugin(),
    pluginsPane(() => client(async () => ({ data: [admitted, withheld] }))),
  );
  testClient.navigation.go({ session: "source" });
  testClient.render(createElement(Slot, { name: "app.overlay" }));
  await vi.waitFor(() => expect(contributionsTo(COMMAND)).toHaveLength(1));
  const command = contributionsTo(COMMAND)[0]!.item;
  const mounted = contexts.length;
  expect(load).toHaveBeenCalledOnce();
  expect(contexts.at(-1)!.sessionId).toBeUndefined();
  await act(() => testClient.navigation.go({ session: "chosen" }));
  expect(contexts).toHaveLength(mounted);
  expect(contributionsTo(COMMAND)[0]!.item).toBe(command);
  await command.run();
  expect(run).toHaveBeenCalledExactlyOnceWith({
    sessionId: "chosen",
    signal: contexts.at(-1)!.signal,
  });
  await act(() => resetKernelForTest());
  expect(contexts.at(-1)!.signal.aborted).toBe(true);
  expect(contributionsTo(COMMAND)).toEqual([]);
  await expect(command.run()).rejects.toBe(contexts.at(-1)!.signal.reason);
  expect(run).toHaveBeenCalledOnce();
});

it("keeps package realization owned by its generation when the query cache is cancelled", async () => {
  const pending = Promise.withResolvers<{ data: PluginInstallation[] }>();
  const row = installation("d3cbafab-ef30-4e20-9583-42f5316dc865", "package");
  const list = vi.fn((_signal?: AbortSignal) => pending.promise);
  onTestFinished(() => pending.resolve({ data: [row] }));
  await loadPluginsForTest(
    runtimePlugin(),
    pluginsPane(() => client(list)),
  );
  await vi.waitFor(() => expect(list).toHaveBeenCalledOnce());

  await queryClient.cancelQueries({ queryKey: [PACKAGES_KEY] });

  expect(list.mock.calls[0]![0]!.aborted).toBe(false);
  pending.resolve({ data: [row] });
  await vi.waitFor(() =>
    expect(contributionsTo(COLOR_THEME).map((entry) => entry.item.label)).toEqual([
      "package · package",
    ]),
  );
  expect(queryClient.getQueryData([PACKAGES_KEY])).toEqual([row]);
  expect(installed.access.failure()).toBeNull();
});

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
    pluginsPane(() => selected),
  );
  await vi.waitFor(() => expect(original).toHaveBeenCalledOnce());
  const retired = installed.access.captureOperations()!;
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
  expect(installed.access.captureOperations()!.signal.aborted).toBe(false);
  await resetKernelForTest();
  expect(installed.access.captureOperations()).toBeNull();
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
    pluginsPane(() => client(list, subscribe)),
  );
  await vi.waitFor(() => expect(installed.access.failure()?.reason).toBe("event stream refused"));
  expect(contributionsTo(COLOR_THEME)).toEqual([]);
  expect(list).not.toHaveBeenCalled();

  installed.access.failure()!.retry();

  expect(installed.access.failure()).toBeNull();
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
    pluginsPane(() => client(async () => ({ data: [] }), subscribe)),
  );
  await vi.waitFor(() => expect(installed.access.failure()).not.toBeNull());
  await resetKernelForTest();
  expect(installed.access.failure()).toBeNull();
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
    pluginsPane(() => client(async () => ({ data: [] }), subscribe)),
  );
  await vi.waitFor(() => expect(installed.access.failure()).not.toBeNull());
  const failed = installed.access.failure()!;
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
  contribute.mockImplementation((preference, owner, theme) => {
    original(preference, owner, theme);
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
    pluginsPane(() =>
      client(
        async () => ({
          data: [
            installation("d3cbafab-ef30-4e20-9583-42f5316dc865", "first"),
            installation("0c31c796-224c-40fa-a721-6696971bb697", "second"),
          ],
        }),
        subscribe,
      ),
    ),
  );
  await vi.waitFor(() => expect(contributionsTo(COLOR_THEME)).toHaveLength(2));
  failure.resolve();
  await vi.waitFor(() => expect(contributionsTo(COLOR_THEME)).toEqual([]));
  expect(disposed.sort()).toEqual(["first · first", "second · second"]);
  await vi.waitFor(() => expect(installed.access.failure()).not.toBeNull());
  const reason = installed.access.failure()!.reason;
  expect(reason).toContain("event stream failed");
  expect(reason).toContain("page close failed");
});

it("preserves a snapshot failure when closing its subscription also fails", async () => {
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
    pluginsPane(() =>
      client(async () => {
        throw new Error("snapshot refused");
      }, subscribe),
    ),
  );
  await vi.waitFor(() => expect(installed.access.failure()).not.toBeNull());
  const reason = installed.access.failure()!.reason;
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
      pluginsPane(() => client(list, subscribe)),
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
    expect(installed.access.failure()).toBeNull();
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
      pluginsPane(() => client(list, subscribe)),
    );
    ready.resolve();
    await vi.waitFor(() => expect(installed.access.failure()?.reason).toBe(failure.message));
    expect(active.size).toBe(0);
    expect(contributionsTo(COLOR_THEME)).toEqual([]);

    failing = false;
    installed.access.failure()!.retry();

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

it("presents and retains only the contributions Runtime admits", async () => {
  const admitted = installation("d3cbafab-ef30-4e20-9583-42f5316dc865", "admitted");
  const withheld: PluginInstallation = {
    ...installation("0c31c796-224c-40fa-a721-6696971bb697", "withheld"),
    presentation: "withheld",
  };
  admitted.selected.components = [
    {
      id: "icons",
      title: "Icons",
      scope: "installation",
      placement: "workspaceView",
      entry: "io.github.tangerg.flame/components/component.js",
      export: "Gallery",
    },
    {
      id: "usage",
      title: "Usage",
      scope: "installation",
      placement: "workspaceView",
      entry: "io.github.tangerg.flame/components/component.js",
      export: "Usage",
    },
    {
      id: "trajectory",
      title: "Trajectory",
      scope: "session",
      placement: "workspaceView",
      entry: "io.github.tangerg.flame/components/component.js",
      export: "Trajectory",
    },
    {
      id: "diagnostics",
      title: "Diagnostics",
      scope: "installation",
      placement: "workspaceView",
      entry: "io.github.tangerg.flame/components/component.js",
      export: "Diagnostics",
      material: "diagnostics",
    },
  ];
  withheld.selected.components = admitted.selected.components;
  admitted.selected.components.push(
    {
      id: "settings",
      title: "Provider settings",
      scope: "client",
      placement: "settingsPane",
      settingsIntent: "modelProviders",
      material: "providers",
      entry: "io.github.tangerg.flame/components/component.js",
      export: "Settings",
    },
    {
      id: "search",
      title: "Search",
      scope: "toolResult",
      placement: "toolResultRenderer",
      selector: { type: "resultShape", shape: "search" },
      entry: "io.github.tangerg.flame/components/component.js",
      export: "Result",
    },
    {
      id: "markdown",
      title: "Markdown",
      scope: "file",
      placement: "fileRenderer",
      selector: { type: "fileExtension", name: "md" },
      entry: "io.github.tangerg.flame/components/component.js",
      export: "Markdown",
    },
  );
  await loadPluginsForTest(
    runtimePlugin(),
    pluginsPane(() => client(async () => ({ data: [admitted, withheld] }))),
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
    { id: "package:d3cbafab-ef30-4e20-9583-42f5316dc865:icons", title: "admitted · Icons" },
    { id: "package:d3cbafab-ef30-4e20-9583-42f5316dc865:usage", title: "admitted · Usage" },
    {
      id: "package:d3cbafab-ef30-4e20-9583-42f5316dc865:trajectory",
      title: "admitted · Trajectory",
    },
    {
      id: "package:d3cbafab-ef30-4e20-9583-42f5316dc865:diagnostics",
      title: "admitted · Diagnostics",
    },
  ]);
  expect(contributionsTo(WORKSPACE_FILE_RENDERER).map((entry) => entry.item.extension)).toEqual([
    "md",
  ]);
  expect(contributionsTo(TOOL_RESULT_RENDERER).map((entry) => entry.item.title)).toEqual([
    "admitted · Search",
  ]);
  expect(
    contributionsTo(SETTINGS_PANE)
      .filter((entry) => entry.item.material === "providers")
      .map((entry) => ({
        id: entry.item.id,
        label: entry.item.label,
        group: entry.item.group,
        intent: entry.item.intent,
      })),
  ).toEqual([
    {
      id: "package:d3cbafab-ef30-4e20-9583-42f5316dc865:settings",
      label: "Provider settings",
      group: "extensions",
      intent: "modelProviders",
    },
  ]);
  expect(retainThemeSelection).toHaveBeenLastCalledWith(
    testClient.theme.access.preference,
    ["package:d3cbafab-ef30-4e20-9583-42f5316dc865:theme"],
    "package:",
  );
  await resetKernelForTest();
  expect(contributionsTo(SETTINGS_PANE)).toEqual([]);
});
