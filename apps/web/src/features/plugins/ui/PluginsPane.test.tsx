import { QueryClientProvider } from "@tanstack/react-query";
import { queryClient } from "@/lib/queryClient";
import { DATA_PROVIDER } from "@/extensions/dataProvider";
import {
  packageOperations,
  PACKAGES_KEY,
  PROVIDERS_KEY,
  usePackageRealization,
} from "@/features/plugins/adapters/packages";
import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { type PluginInstallation } from "@flame/runtime-client/wire";
import { type Host } from "dougong";
import { afterEach, describe, expect, it, vi } from "vitest";
import { reportPluginError, usePluginErrorStore } from "@/extensions/errors";
import { startKernel, stopKernel } from "@/extensions/bootstrap";
import { definePlugin } from "@/extensions/definePlugin";
import { PluginsPane } from "@/features/plugins/ui/PluginsPane";
let host: Host | undefined;

const example = definePlugin({
  name: "flame.builtin.example",
  setup(ctx) {
    ctx.contribute(DATA_PROVIDER, { key: PROVIDERS_KEY, fetcher: async () => [] });
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
    enabled: false,
    inputStates: {},
    disabledServers: [],
    disabledSkills: [],
    realization: { type: "available", unavailableBackends: [] },
    presentation: "withheld",
    selected: {
      digest: "1".repeat(64),
      name: "Reviewed package",
      contributions: [],
      points: [],
      pointValues: [],
      components: [],
      servers: [],
      inputs: [],
      skills: [],
      themes: [],

      diagnostics: [],
    },
  };
}

describe("PluginsPane installation facts", () => {
  it.each(["Configure"])("binds %s to the release opened for review", async () => {
    const reviewedDigest = "1".repeat(64);
    const replacementDigest = "2".repeat(64);
    let installation: PluginInstallation = {
      ...reviewedInstallation(),
      selected: { ...reviewedInstallation().selected, version: "1.0.0" },
    };
    const submit = vi.fn(async () => reviewedInstallation());
    host = await startKernel([
      definePlugin({
        name: "test.package-review",
        setup(ctx) {
          ctx.contribute(DATA_PROVIDER, { key: PROVIDERS_KEY, fetcher: async () => [] });
          ctx.contribute(DATA_PROVIDER, {
            key: PACKAGES_KEY,
            fetcher: async () => [installation],
          });
          ctx.cleanup(
            packageOperations.configure({
              signal: ctx.signal,
              pendingCommands: () => [],
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
    fireEvent.click(await screen.findByRole("button", { name: "Release details" }));
    fireEvent.click(screen.getByRole("button", { name: "Configure inputs" }));
    installation = {
      ...installation,
      selected: {
        ...installation.selected,
        name: "Renamed package",
        version: "2.0.0",
        digest: replacementDigest,
      },
    };
    await act(async () => {
      await queryClient.invalidateQueries({ queryKey: [PACKAGES_KEY] });
    });
    expect(screen.getByText(replacementDigest)).toBeTruthy();
    expect(screen.getByText("Renamed package")).toBeTruthy();
    const dialog = screen.getByRole("dialog", {
      name: "Configure inputs: Reviewed package 1.0.0",
    });
    expect(within(dialog).getByRole("textbox", { name: "Input changes (JSON)" })).toBeTruthy();
    expect(dialog.textContent).toContain(`Reviewed release digest: ${reviewedDigest}.`);
    expect(dialog.textContent).not.toContain(replacementDigest);
    expect(dialog.textContent).not.toContain("Renamed package");
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
          ctx.contribute(DATA_PROVIDER, { key: PROVIDERS_KEY, fetcher: async () => [] });
          ctx.contribute(DATA_PROVIDER, { key: PACKAGES_KEY, fetcher: async () => [installation] });
          ctx.cleanup(
            packageOperations.configure({
              signal: ctx.signal,
              pendingCommands: () => [],
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
    const dialog = screen.getByRole("dialog");
    const field = within(dialog).getByRole("textbox");
    fireEvent.change(field, {
      target: { value: encoded },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    const message = await within(dialog).findByRole("alert");
    expect(message.textContent).toContain(error);
    expect(field.getAttribute("aria-invalid")).toBe("true");
    expect(message.id).not.toBe("");
    expect(field.getAttribute("aria-describedby")).toBe(message.id);
    expect(submit).not.toHaveBeenCalled();
    expect(field).toHaveProperty("value", encoded);
  });

  it("stages a path for the opened installation and keeps the draft after a failure", async () => {
    let installation = reviewedInstallation();
    const stage = vi
      .fn(async (_request: unknown) => reviewedInstallation())
      .mockRejectedValueOnce(new Error("Source unreadable"));
    host = await startKernel([
      definePlugin({
        name: "test.package-stage",
        setup(ctx) {
          ctx.contribute(DATA_PROVIDER, { key: PROVIDERS_KEY, fetcher: async () => [] });
          ctx.contribute(DATA_PROVIDER, {
            key: PACKAGES_KEY,
            fetcher: async () => [installation],
          });
          ctx.cleanup(
            packageOperations.configure({
              signal: ctx.signal,
              pendingCommands: () => [],
              stage,
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
    fireEvent.click(await screen.findByRole("button", { name: "Package actions" }));
    fireEvent.click(await screen.findByRole("menuitem", { name: "Stage update" }));
    const title = "Stage update: Reviewed package";
    const dialog = screen.getByRole("dialog", { name: title });
    expect(dialog.textContent).not.toContain("undefined");
    expect(dialog.textContent).toContain("Select the staged release separately afterward.");
    const field = within(dialog).getByRole("textbox", {
      name: "Absolute package directory or ZIP path on the Runtime",
    });
    expect(field).toHaveProperty("value", "/package");
    fireEvent.change(field, { target: { value: "/next.zip" } });
    installation = {
      ...installation,
      source: "/replaced",
      selected: { ...installation.selected, name: "Renamed package", version: "2.0.0" },
    };
    await act(async () => {
      await queryClient.invalidateQueries({ queryKey: [PACKAGES_KEY] });
    });
    expect(screen.getByRole("dialog", { name: title })).toBe(dialog);
    fireEvent.click(within(dialog).getByRole("button", { name: "Stage update" }));
    expect((await within(dialog).findByRole("alert")).textContent).toContain("Source unreadable");
    expect(field).toHaveProperty("value", "/next.zip");
    fireEvent.click(within(dialog).getByRole("button", { name: "Stage update" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    const request = { installationId: installation.id, source: "/next.zip" };
    expect(stage.mock.calls).toEqual([[request], [request]]);
  });

  it("discards a cancelled configure draft without submitting", async () => {
    const submit = vi.fn(async () => reviewedInstallation());
    host = await startKernel([
      definePlugin({
        name: "test.package-cancel",
        setup(ctx) {
          ctx.contribute(DATA_PROVIDER, { key: PROVIDERS_KEY, fetcher: async () => [] });
          ctx.contribute(DATA_PROVIDER, {
            key: PACKAGES_KEY,
            fetcher: async () => [reviewedInstallation()],
          });
          ctx.cleanup(
            packageOperations.configure({
              signal: ctx.signal,
              pendingCommands: () => [],
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
    const dialog = screen.getByRole("dialog", { name: "Configure inputs: Reviewed package" });
    const draft = within(dialog).getByRole("textbox");
    const initial = (draft as HTMLTextAreaElement).value;
    fireEvent.change(draft, { target: { value: "{}" } });
    fireEvent.click(within(dialog).getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(submit).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Configure inputs" }));
    expect(within(screen.getByRole("dialog")).getByRole("textbox")).toHaveProperty(
      "value",
      initial,
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
          ctx.contribute(DATA_PROVIDER, { key: PROVIDERS_KEY, fetcher: async () => [] });
          ctx.contribute(DATA_PROVIDER, { key: PACKAGES_KEY, fetcher: async () => [installation] });
          ctx.cleanup(
            packageOperations.configure({
              signal: ctx.signal,
              pendingCommands: () => [],
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
    expect(
      await screen.findByText("Server “backend” can't start: its backend couldn't be prepared."),
    ).toBeTruthy();
    expect(screen.getByText("Skill “broken” is invalid and was disabled.")).toBeTruthy();
    expect(
      screen.getByText("The MCP configuration couldn't be read and was disabled."),
    ).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Release details" }));
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
          ctx.contribute(DATA_PROVIDER, { key: PROVIDERS_KEY, fetcher: async () => [] });
          ctx.contribute(DATA_PROVIDER, { key: PACKAGES_KEY, fetcher: async () => [installation] });
          ctx.cleanup(
            packageOperations.configure({
              signal: ctx.signal,
              pendingCommands: () => [],
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
    expect(
      await screen.findByText(
        "This release's files failed verification, so its components are unavailable.",
      ),
    ).toBeTruthy();
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
