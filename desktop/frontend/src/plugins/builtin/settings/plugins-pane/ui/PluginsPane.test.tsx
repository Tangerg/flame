import { QueryClientProvider } from "@tanstack/react-query";
import { queryClient } from "@/lib/queryClient";
import { DATA_PROVIDER } from "@/plugins/sdk/kernelPoints";
import { packageOperations, PACKAGES_KEY } from "../application/packages";
import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { PluginInstallation } from "@flame/runtime-contract/wire";
import type { Host } from "dougong";
import { afterEach, describe, expect, it, vi } from "vitest";
import { reportPluginError, startKernel, stopKernel, usePluginErrorStore } from "@/plugins/sdk";
import { definePlugin } from "@/plugins/sdk";
import { PluginsPane } from "./PluginsPane";

let host: Host | undefined;
const example = definePlugin({
  name: "flame.builtin.example",
  setup(ctx) {
    ctx.contribute(DATA_PROVIDER, { key: PACKAGES_KEY, fetcher: async () => [] });
  },
});

afterEach(async () => {
  cleanup();
  queryClient.clear();
  usePluginErrorStore.getState().clearAll();
  if (!host) return;
  const owned = host;
  host = undefined;
  await stopKernel(owned);
});

function reviewedInstallation(): PluginInstallation {
  return {
    id: "940ac827-b431-455b-af4b-e3a170bcfda0",
    source: "/package",
    enabled: false,
    grants: [],
    values: {},
    disabledServers: [],
    disabledSkills: [],
    availability: [],
    selected: {
      digest: "1".repeat(64),
      name: "Reviewed package",
      servers: [],
      inputs: [],
      requests: [],
      skills: [],
      themes: [],
      diagnostics: [],
    },
  };
}

describe("PluginsPane installation facts", () => {
  it.each(["Approve", "Configure"])(
    "binds %s to the release opened for review",
    async (operation) => {
      const reviewedDigest = "1".repeat(64);
      const replacementDigest = "2".repeat(64);
      let installation = reviewedInstallation();
      const submit = vi.fn(async () => ({ availability: [] }));
      host = await startKernel([
        definePlugin({
          name: "test.package-review",
          setup(ctx) {
            ctx.contribute(DATA_PROVIDER, {
              key: PACKAGES_KEY,
              fetcher: async () => [installation],
            });
            ctx.cleanup(
              packageOperations.configure({
                signal: ctx.signal,
                approve: submit,
                configure: submit,
              } as unknown as ReturnType<typeof packageOperations.get>),
            );
          },
        }),
      ]);
      render(
        <QueryClientProvider client={queryClient}>
          <PluginsPane />
        </QueryClientProvider>,
      );
      fireEvent.click(
        await screen.findByRole("button", {
          name: operation === "Approve" ? "Trust release and grants" : "Configure inputs",
        }),
      );
      installation = {
        ...installation,
        selected: { ...installation.selected, digest: replacementDigest },
      };
      await act(async () => {
        await queryClient.invalidateQueries({ queryKey: [PACKAGES_KEY] });
      });
      expect(screen.getByText(replacementDigest)).toBeTruthy();
      fireEvent.click(screen.getByRole("button", { name: "Save" }));
      await waitFor(() =>
        expect(submit).toHaveBeenCalledWith({
          installationId: installation.id,
          digest: reviewedDigest,
          ...(operation === "Approve"
            ? { grants: [] }
            : { valueChanges: {}, disabledServers: [], disabledSkills: [] }),
        }),
      );
    },
  );

  it.each([
    {
      name: "overlong input",
      changes: {
        valueChanges: { token: { type: "set", value: "x".repeat(8193) } },
        disabledServers: [],
        disabledSkills: [],
      },
      error: "expected at most 8192 character(s)",
    },
    {
      name: "replacement digest",
      changes: {
        valueChanges: {},
        disabledServers: [],
        disabledSkills: [],
        digest: "2".repeat(64),
      },
      error: "digest belongs to the reviewed release",
    },
    {
      name: "replacement installation",
      changes: {
        valueChanges: {},
        disabledServers: [],
        disabledSkills: [],
        installationId: "d8bd51f0-0fc9-4acd-a7c4-1e962c9306e0",
      },
      error: "installationId belongs to the reviewed release",
    },
  ])("rejects $name without submitting or losing the draft", async ({ changes, error }) => {
    const installation = reviewedInstallation();
    const submit = vi.fn(async () => ({ availability: [] }));
    host = await startKernel([
      definePlugin({
        name: "test.package-input",
        setup(ctx) {
          ctx.contribute(DATA_PROVIDER, { key: PACKAGES_KEY, fetcher: async () => [installation] });
          ctx.cleanup(
            packageOperations.configure({
              signal: ctx.signal,
              configure: submit,
            } as unknown as ReturnType<typeof packageOperations.get>),
          );
        },
      }),
    ]);
    render(
      <QueryClientProvider client={queryClient}>
        <PluginsPane />
      </QueryClientProvider>,
    );
    fireEvent.click(await screen.findByRole("button", { name: "Configure inputs" }));
    const encoded = JSON.stringify(changes);
    fireEvent.change(within(screen.getByRole("dialog")).getByRole("textbox"), {
      target: { value: encoded },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await screen.findByText((text) => text.includes(error));
    expect(submit).not.toHaveBeenCalled();
    expect(within(screen.getByRole("dialog")).getByRole("textbox")).toHaveProperty(
      "value",
      encoded,
    );
  });

  it("shows and clears kernel errors without requiring an installed plugin", async () => {
    host = await startKernel([example]);
    reportPluginError("kernel", "setup", new Error("Lifecycle hook failed"), "hook stack");
    const view = render(
      <QueryClientProvider client={queryClient}>
        <PluginsPane />
      </QueryClientProvider>,
    );

    expect(screen.getByText("kernel")).toBeTruthy();
    fireEvent.click(screen.getByTitle("Show error detail"));
    expect(screen.getByText("Lifecycle hook failed")).toBeTruthy();
    expect(screen.getByText("hook stack")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Clear" }));
    expect(screen.queryByText("kernel")).toBeNull();
    expect(screen.getByText("flame.builtin.example")).toBeTruthy();
    expect(usePluginErrorStore.getState().log).toEqual([]);
    view.unmount();
  });

  it("renders installed plugins from the active Host read model", async () => {
    host = await startKernel([example]);

    const view = render(
      <QueryClientProvider client={queryClient}>
        <PluginsPane />
      </QueryClientProvider>,
    );

    expect(screen.getByText("flame.builtin.example")).toBeTruthy();
    view.unmount();
  });

  it("retains removal diagnostics after the removed row leaves the catalog", async () => {
    let installations: PluginInstallation[] = [
      {
        id: "940ac827-b431-455b-af4b-e3a170bcfda0",
        source: "/package",
        enabled: false,
        grants: [],
        values: {},
        disabledServers: [],
        disabledSkills: [],
        availability: [],
        selected: {
          digest: "1".repeat(64),
          name: "Removed package",
          servers: [],
          inputs: [],
          requests: [],
          skills: [],
          themes: [],
          diagnostics: [],
        },
      },
    ];
    host = await startKernel([
      definePlugin({
        name: "test.package-removal",
        setup(ctx) {
          ctx.contribute(DATA_PROVIDER, { key: PACKAGES_KEY, fetcher: async () => installations });
          ctx.cleanup(
            packageOperations.configure({
              signal: ctx.signal,
              uninstall: async () => {
                installations = [];
                return { availability: [{ component: "mcp", code: "reconciliation_failed" }] };
              },
            } as unknown as ReturnType<typeof packageOperations.get>),
          );
        },
      }),
    ]);
    render(
      <QueryClientProvider client={queryClient}>
        <PluginsPane />
      </QueryClientProvider>,
    );
    fireEvent.click(await screen.findByRole("button", { name: "Uninstall" }));
    await waitFor(() => expect(screen.queryByText(/Removed package/)).toBeNull());
    expect(screen.getByText("mcp: reconciliation_failed")).toBeTruthy();
  });
});
