import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { type Host } from "dougong";
import { createFlameClient, RpcError, type FlameClient } from "@flame/runtime-client";
import { createMemoryTransport } from "@flame/runtime-client/testing";
import { type PluginProviderSelection } from "@flame/runtime-client/wire";
import { DATA_PROVIDER } from "@/extensions/dataProvider";
import { definePlugin } from "@/extensions/definePlugin";
import { startKernel, stopKernel } from "@/extensions/bootstrap";
import { queryClient } from "@/lib/queryClient";
import { PROVIDERS_KEY, type PackageOperations } from "@/features/plugins/adapters/packages";
import { ProviderSelections } from "./ProviderSelections";
import { VISUAL_PACKAGES, VISUAL_PLUGIN_PROVIDERS } from "../../../../visual/componentSnapshots";
import { VISUAL_CAPABILITIES } from "../../../../visual/agentFixtureFacts";

let host: Host | undefined;
let client: FlameClient | undefined;
let resources: AbortController;
let priorDefaults: ReturnType<typeof queryClient.getQueryDefaults>;
beforeEach(() => {
  resources = new AbortController();
  priorDefaults = queryClient.getQueryDefaults([PROVIDERS_KEY]);
  queryClient.setQueryDefaults([PROVIDERS_KEY], { retry: false });
});
afterEach(async () => {
  cleanup();
  resources.abort();
  client?.close();
  client = undefined;
  queryClient.clear();
  queryClient.setQueryDefaults([PROVIDERS_KEY], priorDefaults);
  if (host) await stopKernel(host);
  host = undefined;
});

async function mountSelections(
  read: (signal?: AbortSignal) => Promise<PluginProviderSelection[]>,
  select = vi.fn((value: PluginProviderSelection) => value),
  installations = VISUAL_PACKAGES,
) {
  const memory = createMemoryTransport();
  client = createFlameClient(
    {
      recv: memory.recv,
      close: memory.close,
      async send(request, signal, options) {
        if (request.method !== "plugins.selectProvider") throw new Error("Unexpected test command");
        const result = select(structuredClone(request.params) as PluginProviderSelection);
        await memory.send(request, signal, options);
        memory.inject({ jsonrpc: "2.0", id: request.id, result });
      },
    },
    { capabilities: () => VISUAL_CAPABILITIES },
  );
  const operations: PackageOperations = {
    ...client.plugins,
    signal: resources.signal,
    track: (operation) => operation,
  };
  host = await startKernel([
    definePlugin({
      name: "test.provider-selection-read",
      setup(ctx) {
        ctx.contribute(DATA_PROVIDER, {
          key: PROVIDERS_KEY,
          fetcher: (_params, signal) => read(signal),
        });
      },
    }),
  ]);
  return render(
    <QueryClientProvider client={queryClient}>
      <ProviderSelections installations={installations} operations={operations} />
      <textarea aria-label="Later draft" defaultValue="Keep this draft." />
    </QueryClientProvider>,
  );
}

it("keeps contributed candidates available without inventing a selection during the first read", async () => {
  const read = vi.fn(
    (signal?: AbortSignal) =>
      new Promise<PluginProviderSelection[]>((_resolve, reject) =>
        signal?.addEventListener("abort", () => reject(signal.reason), { once: true }),
      ),
  );
  await mountSelections(read);
  expect(screen.getAllByRole("combobox", { name: "Choose provider" })).toHaveLength(2);
  expect(screen.getAllByText("Selection not read.")).toHaveLength(2);
  expect(screen.queryByText("No provider selected for this role.")).toBeNull();
});

it("shows an initial read failure beside known roles and recovers through the persistent refresh action", async () => {
  const read = vi
    .fn<(signal?: AbortSignal) => Promise<PluginProviderSelection[]>>()
    .mockRejectedValueOnce(new Error("Private read unavailable"))
    .mockResolvedValue(VISUAL_PLUGIN_PROVIDERS);
  await mountSelections(read);
  expect(await screen.findByText("Couldn't load")).toBeTruthy();
  expect(screen.getAllByText("Selection not read.")).toHaveLength(2);
  expect(screen.queryByText("No provider selected for this role.")).toBeNull();
  const refresh = screen.getByRole("button", { name: "Refresh selections" });
  act(() => refresh.focus());
  fireEvent.click(refresh);
  await waitFor(() => expect(screen.queryByText("Couldn't load")).toBeNull());
  expect(screen.getByText(/Selected: flame.memory/)).toBeTruthy();
  expect(document.activeElement).toBe(refresh);
});

it("treats a successful empty read as known unselected roles", async () => {
  await mountSelections(async () => []);
  await waitFor(() =>
    expect(screen.getAllByText("No provider selected for this role.")).toHaveLength(2),
  );
  expect(screen.queryByText("Selection not read.")).toBeNull();
  expect(screen.queryByText("Couldn't load")).toBeNull();
});

it("retains selected identities, the candidate draft and later focus after a failed refresh", async () => {
  const read = vi
    .fn<(signal?: AbortSignal) => Promise<PluginProviderSelection[]>>()
    .mockResolvedValueOnce(VISUAL_PLUGIN_PROVIDERS)
    .mockRejectedValue(new Error("Private refresh unavailable"));
  const select = vi.fn((value: PluginProviderSelection) => value);
  await mountSelections(read, select);
  await screen.findByText(/Selected: flame.memory/);
  const chooser = screen.getAllByRole("combobox", { name: "Choose provider" })[0]!;
  fireEvent.click(chooser);
  fireEvent.click(await screen.findByRole("option", { name: /flame.memory/ }));
  const refresh = screen.getByRole("button", { name: "Refresh selections" });
  fireEvent.click(refresh);
  const draft = screen.getByRole("textbox", { name: "Later draft" });
  act(() => draft.focus());
  expect(await screen.findByText("Showing the last read selections.")).toBeTruthy();
  expect(screen.getByText(/Selected: flame.memory/)).toBeTruthy();
  expect(screen.getAllByRole("combobox", { name: "Choose provider" })[0]).toBe(chooser);
  expect(chooser.textContent).toContain("flame.memory");
  expect(document.activeElement).toBe(draft);
  expect(select).not.toHaveBeenCalled();
});

it("allows an explicit selection after an unavailable read without inferring a selected candidate", async () => {
  let selected: PluginProviderSelection[] = [];
  let available = false;
  const read = vi.fn(async () => {
    if (!available) throw new Error("Private read unavailable");
    return selected;
  });
  const select = vi.fn((value: PluginProviderSelection) => {
    selected = [value];
    available = true;
    return value;
  });
  await mountSelections(read, select);
  await screen.findByText("Couldn't load");
  const chooser = screen.getAllByRole("combobox", { name: "Choose provider" })[0]!;
  fireEvent.click(chooser);
  fireEvent.click(await screen.findByRole("option", { name: /flame.memory/ }));
  fireEvent.click(screen.getAllByRole("button", { name: "Apply selection" })[0]!);
  await screen.findByText(/Selected: flame.memory/);
  expect(select).toHaveBeenCalledExactlyOnceWith(VISUAL_PLUGIN_PROVIDERS[0]);
  expect(screen.queryByText("Couldn't load")).toBeNull();
});

it("retains the existing no-role read failure and recovery without a false empty state", async () => {
  await mountSelections(
    async () => {
      throw new Error("Private catalog unavailable");
    },
    undefined,
    [],
  );
  expect(await screen.findByText("Couldn't load")).toBeTruthy();
  expect(screen.getAllByText("Couldn't load")).toHaveLength(1);
  expect(screen.queryByText("No provider has been selected.")).toBeNull();
  expect(screen.getByRole("button", { name: "Refresh selections" })).toBeTruthy();
});

it("preserves unsupported-method feedback alongside contributed roles", async () => {
  await mountSelections(async () => {
    throw new RpcError({
      code: -32601,
      message: "method_not_found",
      data: { type: "method_not_found" },
    });
  });
  expect(await screen.findByText("Not supported by this runtime")).toBeTruthy();
  expect(screen.queryByText("Couldn't load")).toBeNull();
  expect(screen.getAllByText("Selection not read.")).toHaveLength(2);
});
