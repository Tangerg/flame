import { QueryClientProvider } from "@tanstack/react-query";
import { queryClient } from "@/lib/queryClient";
import { DATA_PROVIDER } from "@/plugins/sdk/kernelPoints";
import { packageOperations, PACKAGES_KEY, usePackageRealization } from "../application/packages";
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
  usePackageRealization.setState({ failure: null });
  if (!host) return;
  const owned = host;
  host = undefined;
  await stopKernel(owned);
});

function reviewedInstallation(): PluginInstallation {
  return {
    id: "940ac827-b431-455b-af4b-e3a170bcfda0",
    source: "/package",
    state: "unapproved",
    inputStates: {},
    disabledServers: [],
    disabledSkills: [],
    realization: { type: "available", unavailableBackends: [] },
    presentation: "withheld",
    selected: {
      digest: "1".repeat(64),
      name: "Reviewed package",
      servers: [],
      inputs: [],
      skills: [],
      themes: [],
      views: [],
      actions: [],
      diagnostics: [],
    },
  };
}

describe("PluginsPane installation facts", () => {
  it("asks for approval of a selected release before it can run", async () => {
    let installation: PluginInstallation = {
      ...reviewedInstallation(),
      state: "enabled",
    };
    const approve = vi.fn(async () => reviewedInstallation());
    host = await startKernel([
      definePlugin({
        name: "test.package-approval",
        setup(ctx) {
          ctx.contribute(DATA_PROVIDER, {
            key: PACKAGES_KEY,
            fetcher: async () => [installation],
          });
          ctx.cleanup(
            packageOperations.configure({
              signal: ctx.signal,
              approve,
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
    expect(await screen.findByText("Enabled.")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Trust this release" })).toBeNull();
    const selectedDigest = "2".repeat(64);
    installation = {
      ...installation,
      state: "unapproved",
      selected: { ...installation.selected, digest: selectedDigest },
    };
    await act(async () => {
      await queryClient.invalidateQueries({ queryKey: [PACKAGES_KEY] });
    });
    expect(
      await screen.findByText("Not approved: review this release before enabling it."),
    ).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Trust this release" }));
    await waitFor(() =>
      expect(approve).toHaveBeenCalledWith({
        installationId: installation.id,
        digest: selectedDigest,
      }),
    );
  });

  it.each(["Configure"])("binds %s to the release opened for review", async () => {
    const reviewedDigest = "1".repeat(64);
    const replacementDigest = "2".repeat(64);
    let installation = reviewedInstallation();
    const submit = vi.fn(async () => reviewedInstallation());
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
        valueChanges: {},
        serverChanges: {},
        skillChanges: {},
      }),
    );
  });

  it.each([
    {
      name: "replacement digest",
      changes: {
        valueChanges: {},
        serverChanges: {},
        skillChanges: {},
        digest: "2".repeat(64),
      },
      error: "“digest” belongs to the reviewed release",
    },
    {
      name: "replacement installation",
      changes: {
        valueChanges: {},
        serverChanges: {},
        skillChanges: {},
        installationId: "d8bd51f0-0fc9-4acd-a7c4-1e962c9306e0",
      },
      error: "“installationId” belongs to the reviewed release",
    },
  ])("rejects $name without submitting or losing the draft", async ({ changes, error }) => {
    const installation = reviewedInstallation();
    const submit = vi.fn(async () => reviewedInstallation());
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

  it("renders observed realization and typed diagnostics without secret text", async () => {
    const installation: PluginInstallation = {
      ...reviewedInstallation(),
      realization: { type: "available", unavailableBackends: ["backend"] },
      inputStates: { token: { type: "configured" }, region: { type: "value", value: "eu-west" } },
      selected: {
        ...reviewedInstallation().selected,
        diagnostics: [
          { component: { type: "skill", name: "broken" }, code: "invalidDeclaration" },
          { component: { type: "mcp" }, code: "unavailableComponent" },
        ],
      },
    };
    host = await startKernel([
      definePlugin({
        name: "test.package-realization",
        setup(ctx) {
          ctx.contribute(DATA_PROVIDER, { key: PACKAGES_KEY, fetcher: async () => [installation] });
          ctx.cleanup(
            packageOperations.configure({ signal: ctx.signal } as unknown as ReturnType<
              typeof packageOperations.get
            >),
          );
        },
      }),
    ]);
    render(
      <QueryClientProvider client={queryClient}>
        <PluginsPane />
      </QueryClientProvider>,
    );
    expect(
      await screen.findByText("Server “backend” can't start: its backend couldn't be prepared."),
    ).toBeTruthy();
    expect(screen.getByText("Skill “broken” is invalid and was disabled.")).toBeTruthy();
    expect(
      screen.getByText("The MCP configuration couldn't be read and was disabled."),
    ).toBeTruthy();
    expect(screen.getByText(/"configured"/)).toBeTruthy();
    expect(screen.getByText(/eu-west/)).toBeTruthy();
  });

  it("reports an unavailable release instead of backend conditions", async () => {
    const installation: PluginInstallation = {
      ...reviewedInstallation(),
      realization: { type: "releaseUnavailable" },
    };
    host = await startKernel([
      definePlugin({
        name: "test.package-release",
        setup(ctx) {
          ctx.contribute(DATA_PROVIDER, { key: PACKAGES_KEY, fetcher: async () => [installation] });
          ctx.cleanup(
            packageOperations.configure({ signal: ctx.signal } as unknown as ReturnType<
              typeof packageOperations.get
            >),
          );
        },
      }),
    ]);
    render(
      <QueryClientProvider client={queryClient}>
        <PluginsPane />
      </QueryClientProvider>,
    );
    expect(
      await screen.findByText(
        "This release's files failed verification, so its components are unavailable.",
      ),
    ).toBeTruthy();
  });

  it("labels a failed row operation with the operation that ran", async () => {
    const installation = reviewedInstallation();
    host = await startKernel([
      definePlugin({
        name: "test.package-revoke",
        setup(ctx) {
          ctx.contribute(DATA_PROVIDER, { key: PACKAGES_KEY, fetcher: async () => [installation] });
          ctx.cleanup(
            packageOperations.configure({
              signal: ctx.signal,
              revoke: async () => {
                throw { code: "refused" };
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
    fireEvent.click(await screen.findByRole("button", { name: "Revoke trust" }));
    expect((await screen.findByRole("alert")).textContent).toContain("Revoke trust");
  });

  it("shows this window's package realization failure with a local retry", async () => {
    host = await startKernel([example]);
    const retry = vi.fn();
    usePackageRealization.setState({ failure: { reason: "stream closed", retry } });
    render(
      <QueryClientProvider client={queryClient}>
        <PluginsPane />
      </QueryClientProvider>,
    );
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("stream closed");
    fireEvent.click(within(alert).getByRole("button", { name: "Retry" }));
    expect(retry).toHaveBeenCalledOnce();
  });
});
