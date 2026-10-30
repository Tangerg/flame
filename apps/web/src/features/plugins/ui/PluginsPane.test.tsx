import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { onlineManager, QueryClientProvider } from "@tanstack/react-query";
import { queryClient } from "@/lib/queryClient";
import { DATA_PROVIDER } from "@/extensions/dataProvider";
import {
  type PackageOperations,
  PACKAGES_KEY,
  PROVIDERS_KEY,
} from "@/features/plugins/adapters/packages";
import { act, cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { type PluginInstallation } from "@flame/runtime-client/wire";
import { type Host } from "dougong";
import { afterEach, describe, expect, it, vi } from "vitest";
import { reportPluginError, usePluginErrorStore } from "@/extensions/errors";
import { startKernel, stopKernel } from "@/extensions/bootstrap";
import { definePlugin } from "@/extensions/definePlugin";
import { PluginsPane } from "@/features/plugins/ui/PluginsPane";
import { toast } from "sonner";

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
let host: Host | undefined;

function pauseQueries() {
  const wasOnline = onlineManager.isOnline();
  onlineManager.setOnline(false);
  finishTestClient(() => onlineManager.setOnline(wasOnline));
}

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
  testClient.installedPlugins.access.setFailure(null);
  vi.unstubAllGlobals();
  vi.clearAllMocks();
  if (!host) return;
  const owned = host;
  host = undefined;
  await stopKernel(owned);
});

describe("PluginsPane client error reading", () => {
  const message =
    "Could not load contribution /workspace/LongConfigurationReaderWithoutSoftBreaks.ts.\n\nRequired input: explicit selection.";
  const detail = "Error: missing input\n    at load (reader.ts:4)\n\nLast recorded frame.";

  async function openError(plugin = "flame.builtin.example") {
    host = await startKernel([example]);
    reportPluginError(plugin, "events", message, detail);
    testClient.render(
      <QueryClientProvider client={queryClient}>
        <PluginsPane />
      </QueryClientProvider>,
    );
    fireEvent.click(screen.getByTitle("Show error detail"));
  }

  it("names a keyboard-readable stack and relates Copy to its complete error", async () => {
    await openError();

    const reader = screen.getByRole<HTMLElement>("region", {
      name: /Could not load contribution/,
    });
    expect(reader.tabIndex).toBe(0);
    expect(reader.textContent).toBe(detail);
    const plugin = screen.getByRole("group", { name: "flame.builtin.example" });
    const copy = within(plugin).getByRole("button", { name: "Copy error" });
    const description = document.getElementById(copy.getAttribute("aria-describedby") ?? "");
    expect(description?.textContent).toBe(message);
    expect(description?.getAttribute("title")).toBeNull();
  });

  it.each(["flame.builtin.example", "kernel"])(
    "returns to the loaded-plugin list after clearing %s",
    async (plugin) => {
      await openError(plugin);
      const clear = screen.getByRole("button", { name: "Clear" });
      act(() => clear.focus());
      fireEvent.click(clear);

      const list = screen.getByRole("region", { name: "Loaded client plugins" });
      expect(document.activeElement).toBe(within(list).getByText("Loaded client plugins"));
      expect(usePluginErrorStore.getState().log).toEqual([]);
      expect(screen.queryByRole("button", { name: "Copy error" })).toBeNull();
      expect(screen.getByText("flame.builtin.example")).toBeTruthy();
    },
  );

  it.each([
    {
      outcome: "accepted",
      write: () => Promise.resolve(),
      success: [["Error copied"]],
      failure: [],
    },
    {
      outcome: "denied",
      write: () => Promise.reject(new Error("NotAllowedError: private detail")),
      success: [],
      failure: [["Couldn't copy. Select the error text to copy it."]],
    },
  ])("reports an $outcome copy without changing its payload or focus", async (result) => {
    const writeText = vi.fn(result.write);
    vi.stubGlobal("navigator", { clipboard: { writeText } });
    await openError();
    const copy = screen.getByRole("button", { name: "Copy error" });
    act(() => copy.focus());
    fireEvent.click(copy);

    await waitFor(() => {
      expect(writeText).toHaveBeenCalledExactlyOnceWith(`[event handler] ${message}\n\n${detail}`);
      expect(vi.mocked(toast.success).mock.calls).toEqual(result.success);
      expect(vi.mocked(toast.error).mock.calls).toEqual(result.failure);
    });
    expect(document.activeElement).toBe(copy);
  });
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
      locales: [],
      themes: [],

      diagnostics: [],
    },
  };
}

describe("PluginsPane installation facts", () => {
  async function renderPackages(
    operations: Partial<
      Record<
        "install" | "configure" | "stage" | "setEnablement",
        (request: unknown) => Promise<PluginInstallation>
      >
    >,
    read: () => Promise<PluginInstallation[]> = async () => [reviewedInstallation()],
  ) {
    host = await startKernel([
      definePlugin({
        name: "test.package-feedback",
        setup(ctx) {
          ctx.contribute(DATA_PROVIDER, { key: PROVIDERS_KEY, fetcher: async () => [] });
          ctx.contribute(DATA_PROVIDER, {
            key: PACKAGES_KEY,
            fetcher: read,
          });
          ctx.cleanup(
            testClient.installedPlugins.access.publishOperations({
              signal: ctx.signal,
              pendingCommands: () => [],
              ...operations,
            } as unknown as PackageOperations),
          );
        },
      }),
    ]);
    testClient.render(
      <QueryClientProvider client={queryClient}>
        <PluginsPane />
      </QueryClientProvider>,
    );
  }

  const catalogCases = [
    {
      name: "empty",
      installations: [] as PluginInstallation[],
      visibleText: "No Runtime packages installed",
    },
    { name: "nonempty", installations: [reviewedInstallation()], visibleText: "Reviewed package" },
  ];

  it.each(catalogCases)(
    "waits for the first paused $name package result",
    async ({ installations, visibleText }) => {
      pauseQueries();
      const read = vi.fn(async () => installations);
      const install = vi.fn(async () => reviewedInstallation());
      await renderPackages({ install }, read);
      await waitFor(() =>
        expect(queryClient.getQueryState([PACKAGES_KEY])).toMatchObject({
          status: "pending",
          fetchStatus: "paused",
          data: undefined,
        }),
      );
      expect(read).not.toHaveBeenCalled();
      expect(screen.getAllByRole("status").map((status) => status.textContent)).toContain(
        "Loading",
      );
      expect(screen.queryByText("No Runtime packages installed")).toBeNull();
      const source = screen.getByRole("textbox", {
        name: "Absolute package directory or ZIP path on the Runtime",
      });
      fireEvent.change(source, { target: { value: "/captured/保留这个输入.zip" } });
      act(() => source.focus());

      await act(async () => onlineManager.setOnline(true));
      expect(await screen.findByText(visibleText)).toBeTruthy();
      expect(document.activeElement).toBe(source);
      expect(source).toHaveProperty("value", "/captured/保留这个输入.zip");
      expect(install).not.toHaveBeenCalled();
    },
  );

  it.each(catalogCases)(
    "keeps accepted $name packages during a paused reread",
    async ({ installations, visibleText }) => {
      const read = vi.fn(async () => installations);
      await renderPackages({}, read);
      await screen.findByText(visibleText);
      read.mockClear();
      pauseQueries();
      act(() => void queryClient.refetchQueries({ queryKey: [PACKAGES_KEY] }));
      await waitFor(() =>
        expect(queryClient.getQueryState([PACKAGES_KEY])).toMatchObject({
          status: "success",
          fetchStatus: "paused",
          data: installations,
        }),
      );
      expect(read).not.toHaveBeenCalled();
      expect(screen.getByText(visibleText)).toBeTruthy();
    },
  );

  it.each(["configure", "stage"] as const)(
    "retains an independent %s draft through a failed catalog refresh and retry",
    async (kind) => {
      const first = reviewedInstallation();
      const second: PluginInstallation = {
        ...first,
        id: "940ac827-b431-455b-af4b-e3a170bcfda1",
        selected: { ...first.selected, name: "Other package", digest: "2".repeat(64) },
      };
      const enablement = Promise.withResolvers<PluginInstallation>();
      const recovery = Promise.withResolvers<PluginInstallation[]>();
      let read: () => Promise<PluginInstallation[]> = async () => [first, second];
      const submit = vi.fn(async (_request: unknown) => second);
      await renderPackages({ setEnablement: () => enablement.promise, [kind]: submit }, () =>
        read(),
      );
      const firstRow = (await screen.findByText(first.selected.name)).closest("article")!;
      fireEvent.click(within(firstRow).getByRole("button", { name: "Package actions" }));
      fireEvent.click(await screen.findByRole("menuitem", { name: "Enable" }));

      const secondRow = screen.getByText(second.selected.name).closest("article")!;
      if (kind === "configure") {
        fireEvent.click(within(secondRow).getByRole("button", { name: "Configure inputs" }));
      } else {
        fireEvent.click(within(secondRow).getByRole("button", { name: "Package actions" }));
        fireEvent.click(await screen.findByRole("menuitem", { name: "Stage update" }));
      }
      const title = `${kind === "configure" ? "Configure inputs" : "Stage update"}: Other package`;
      const dialog = screen.getByRole("dialog", { name: title });
      const field = within(dialog).getByRole("textbox") as HTMLTextAreaElement;
      const changes = {
        valueChanges: { note: { type: "set", value: "保留这个配置草稿" } },
        serverChanges: {},
        skillChanges: {},
      };
      const value =
        kind === "configure" ? JSON.stringify(changes) : "/runtime/保留这个更新草稿.zip";
      fireEvent.change(field, { target: { value } });
      act(() => {
        field.focus();
        field.setSelectionRange(4, 12);
      });
      read = async () => {
        throw new Error("Catalog refresh rejected");
      };
      await act(async () => {
        enablement.resolve({ ...first, enabled: true });
      });
      await waitFor(() => expect(queryClient.getQueryState([PACKAGES_KEY])?.status).toBe("error"), {
        timeout: 2_000,
      });

      expect(queryClient.getQueryData([PACKAGES_KEY])).toEqual([first, second]);
      expect(screen.getByRole("dialog", { name: title })).toBe(dialog);
      expect(field.value).toBe(value);
      expect(document.activeElement).toBe(field);
      expect([field.selectionStart, field.selectionEnd]).toEqual([4, 12]);
      expect(screen.getByText("Showing the last loaded packages.")).toBeTruthy();
      expect(submit).not.toHaveBeenCalled();

      read = () => recovery.promise;
      act(() => {
        void queryClient.invalidateQueries({ queryKey: [PACKAGES_KEY] });
      });
      await waitFor(() =>
        expect(queryClient.getQueryState([PACKAGES_KEY])?.fetchStatus).toBe("fetching"),
      );
      expect(screen.getByRole("dialog", { name: title })).toBe(dialog);
      expect(field.value).toBe(value);
      expect(document.activeElement).toBe(field);
      await act(async () => {
        recovery.resolve([{ ...first, enabled: true }, second]);
      });
      await waitFor(() =>
        expect(queryClient.getQueryState([PACKAGES_KEY])?.status).toBe("success"),
      );
      expect(screen.getByRole("dialog", { name: title })).toBe(dialog);
      expect(field.value).toBe(value);
      expect(document.activeElement).toBe(field);

      fireEvent.click(
        within(dialog).getByRole("button", {
          name: kind === "configure" ? "Save" : "Stage update",
        }),
      );
      await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
      expect(submit).toHaveBeenCalledExactlyOnceWith(
        kind === "configure"
          ? { installationId: second.id, digest: second.selected.digest, ...changes }
          : { installationId: second.id, source: value },
      );
    },
  );

  it("retires a reviewed installation when an accepted catalog no longer contains it", async () => {
    let rows = [reviewedInstallation()];
    const configure = vi.fn(async () => reviewedInstallation());
    await renderPackages({ configure }, async () => rows);
    fireEvent.click(await screen.findByRole("button", { name: "Configure inputs" }));
    expect(screen.getByRole("dialog")).toBeTruthy();

    rows = [];
    await act(async () => {
      await queryClient.invalidateQueries({ queryKey: [PACKAGES_KEY] });
    });

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(await screen.findByText("No Runtime packages installed")).toBeTruthy();
    expect(screen.queryByText("Showing the last loaded packages.")).toBeNull();
    expect(configure).not.toHaveBeenCalled();
  });

  it("returns retry focus to the retained catalog while its recovery is pending", async () => {
    const installation = reviewedInstallation();
    const recovery = Promise.withResolvers<PluginInstallation[]>();
    let read: () => Promise<PluginInstallation[]> = async () => [installation];
    await renderPackages({}, () => read());
    await screen.findByText(installation.selected.name);
    read = async () => {
      throw new Error("Catalog refresh rejected");
    };
    await act(async () => {
      await queryClient.invalidateQueries({ queryKey: [PACKAGES_KEY] });
    });
    const retry = await screen.findByRole("button", { name: "Retry" });
    act(() => retry.focus());
    read = () => recovery.promise;

    fireEvent.click(retry);

    expect(document.activeElement).toBe(screen.getByText("Installed packages"));
    expect(screen.getByText(installation.selected.name)).toBeTruthy();
    await waitFor(() => expect(screen.getByRole("status").getAttribute("aria-busy")).toBe("true"));
    expect(screen.getByRole("status").textContent).toBe("Loading");
    await act(async () => recovery.resolve([installation]));
    await waitFor(() => expect(queryClient.getQueryState([PACKAGES_KEY])?.status).toBe("success"));
    expect(document.activeElement).toBe(screen.getByText("Installed packages"));
    expect(screen.queryByText("Showing the last loaded packages.")).toBeNull();
  });

  it("clears installation feedback on editing without clearing a realization failure", async () => {
    const install = vi
      .fn(async (_request: unknown) => reviewedInstallation())
      .mockRejectedValueOnce(new Error("Source unreadable"));
    await renderPackages({ install });
    const retry = vi.fn();
    act(() => testClient.installedPlugins.access.setFailure({ reason: "stream closed", retry }));
    const source = screen.getByRole("textbox", {
      name: "Absolute package directory or ZIP path on the Runtime",
    });
    fireEvent.change(source, { target: { value: "/runtime/Missing.Package.zip" } });
    fireEvent.click(screen.getByRole("button", { name: "Install" }));
    await screen.findByText("Source unreadable");
    expect(source).toHaveProperty("value", "/runtime/Missing.Package.zip");

    const nextSource = "/runtime/我的 Package/Review.ZIP";
    fireEvent.change(source, { target: { value: nextSource } });
    expect(screen.queryByText("Source unreadable")).toBeNull();
    expect(screen.getByText(/stream closed/)).toBeTruthy();
    expect(source.getAttribute("autocapitalize")).toBe("none");
    expect(source.getAttribute("autocorrect")).toBe("off");
    expect(source.getAttribute("spellcheck")).toBe("false");
    fireEvent.click(screen.getByRole("button", { name: "Install" }));
    await screen.findByText(/Installed Reviewed package\./);
    expect(install.mock.calls).toEqual([
      [{ source: "/runtime/Missing.Package.zip" }],
      [{ source: nextSource }],
    ]);
    expect(source).toHaveProperty("value", "");
    expect(screen.getByText(/stream closed/)).toBeTruthy();
    expect(retry).not.toHaveBeenCalled();
  });

  it("retires an installed notice when another package source is entered", async () => {
    const install = vi
      .fn(async (_request: unknown) => reviewedInstallation())
      .mockResolvedValueOnce(reviewedInstallation())
      .mockRejectedValueOnce(new Error("Next source unreadable"));
    await renderPackages({ install });
    const source = screen.getByRole("textbox", {
      name: "Absolute package directory or ZIP path on the Runtime",
    });
    fireEvent.change(source, { target: { value: "/first.zip" } });
    fireEvent.click(screen.getByRole("button", { name: "Install" }));
    await screen.findByText(/Installed Reviewed package\./);

    fireEvent.change(source, { target: { value: "/next.zip" } });
    expect(screen.queryByText(/Installed Reviewed package\./)).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Install" }));
    await screen.findByText("Next source unreadable");
    expect(screen.queryByText(/Installed Reviewed package\./)).toBeNull();
    expect(source).toHaveProperty("value", "/next.zip");
    expect(install.mock.calls).toEqual([[{ source: "/first.zip" }], [{ source: "/next.zip" }]]);
  });

  it("keeps valid configuration editable after a command failure and retries its revised draft", async () => {
    const configure = vi
      .fn(async (_request: unknown) => reviewedInstallation())
      .mockRejectedValueOnce(new Error("Connection lost"));
    await renderPackages({ configure });
    fireEvent.click(await screen.findByRole("button", { name: "Configure inputs" }));
    const dialog = screen.getByRole("dialog", { name: "Configure inputs: Reviewed package" });
    const field = within(dialog).getByRole("textbox", { name: "Input changes (JSON)" });
    const initial = (field as HTMLTextAreaElement).value;
    fireEvent.click(within(dialog).getByRole("button", { name: "Save" }));
    const alert = await within(dialog).findByRole("alert");
    expect(alert.textContent).toBe("Connection lost");
    expect(field.getAttribute("aria-invalid")).not.toBe("true");
    expect(field.getAttribute("aria-describedby")).toBe(alert.id);
    expect(field).toHaveProperty("value", initial);

    const changes = {
      valueChanges: { root: { type: "set", value: "/Workspace/我的 Project" } },
      serverChanges: {},
      skillChanges: {},
    };
    const revised = JSON.stringify(changes, null, 2);
    fireEvent.change(field, { target: { value: revised } });
    expect(within(dialog).queryByRole("alert")).toBeNull();
    expect(field.getAttribute("aria-describedby")).toBeNull();
    expect(field).toHaveProperty("value", revised);
    expect(field.getAttribute("autocapitalize")).toBe("none");
    expect(field.getAttribute("autocorrect")).toBe("off");
    expect(field.getAttribute("spellcheck")).toBe("false");
    fireEvent.click(within(dialog).getByRole("button", { name: "Save" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(configure.mock.calls).toEqual([
      [
        {
          installationId: reviewedInstallation().id,
          digest: "1".repeat(64),
          valueChanges: {},
          serverChanges: {},
          skillChanges: {},
        },
      ],
      [{ installationId: reviewedInstallation().id, digest: "1".repeat(64), ...changes }],
    ]);
  });

  it.each([
    { draft: "{", message: "Enter valid JSON with unique property names." },
    { draft: "[]", message: "Use a JSON object for input changes." },
    {
      draft:
        '{"valueChanges":{"root":{"type":"set","value":5}},"serverChanges":{},"skillChanges":{}}',
      message: 'Correct valueChanges["root"].value: expected a string.',
    },
    {
      draft:
        '{"valueChanges":{"root":{"type":"set","value":5},"region":{"type":"set","value":false}},"serverChanges":{},"skillChanges":{}}',
      message:
        'Correct valueChanges["root"].value: expected a string.\nCorrect valueChanges["region"].value: expected a string.',
    },
  ])(
    "marks invalid configuration and clears its feedback on correction: $draft",
    async ({ draft, message }) => {
      const configure = vi.fn(async (_request: unknown) => reviewedInstallation());
      await renderPackages({ configure });
      fireEvent.click(await screen.findByRole("button", { name: "Configure inputs" }));
      const dialog = screen.getByRole("dialog");
      const field = within(dialog).getByRole("textbox");
      const valid = (field as HTMLTextAreaElement).value;
      fireEvent.change(field, { target: { value: draft } });
      fireEvent.click(within(dialog).getByRole("button", { name: "Save" }));
      const alert = await within(dialog).findByRole("alert");
      expect(alert.textContent).toBe(message);
      expect(field.getAttribute("aria-invalid")).toBe("true");
      expect(field.getAttribute("aria-describedby")).toBe(alert.id);
      expect(field).toHaveProperty("value", draft);
      expect(configure).not.toHaveBeenCalled();

      fireEvent.change(field, { target: { value: valid } });
      expect(within(dialog).queryByRole("alert")).toBeNull();
      expect(field.getAttribute("aria-invalid")).not.toBe("true");
      fireEvent.click(within(dialog).getByRole("button", { name: "Save" }));
      await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
      expect(configure).toHaveBeenCalledOnce();
    },
  );

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
            testClient.installedPlugins.access.publishOperations({
              signal: ctx.signal,
              pendingCommands: () => [],
              configure: submit,
            } as unknown as PackageOperations),
          );
        },
      }),
    ]);
    testClient.render(
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
            testClient.installedPlugins.access.publishOperations({
              signal: ctx.signal,
              pendingCommands: () => [],
              configure: submit,
            } as unknown as PackageOperations),
          );
        },
      }),
    ]);
    testClient.render(
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
    fireEvent.change(field, { target: { value: "{}" } });
    expect(within(dialog).queryByRole("alert")).toBeNull();
    expect(field.getAttribute("aria-invalid")).not.toBe("true");
    expect(submit).not.toHaveBeenCalled();
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
            testClient.installedPlugins.access.publishOperations({
              signal: ctx.signal,
              pendingCommands: () => [],
              stage,
            } as unknown as PackageOperations),
          );
        },
      }),
    ]);
    testClient.render(
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
    expect(field.getAttribute("aria-invalid")).not.toBe("true");
    expect(field).toHaveProperty("value", "/next.zip");
    fireEvent.click(within(dialog).getByRole("button", { name: "Stage update" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    const request = { installationId: installation.id, source: "/next.zip" };
    expect(stage.mock.calls).toEqual([[request], [request]]);
  });

  it("retires validation feedback with a cancelled configuration edit", async () => {
    const configure = vi.fn(async () => reviewedInstallation());
    await renderPackages({ configure });
    fireEvent.click(await screen.findByRole("button", { name: "Configure inputs" }));
    const dialog = screen.getByRole("dialog");
    const field = within(dialog).getByRole<HTMLTextAreaElement>("textbox");
    const initial = field.value;
    fireEvent.change(field, { target: { value: "[]" } });
    fireEvent.click(within(dialog).getByRole("button", { name: "Save" }));
    expect((await within(dialog).findByRole("alert")).textContent).toBe(
      "Use a JSON object for input changes.",
    );

    fireEvent.keyDown(field, { key: "Escape" });
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(screen.queryByRole("alert")).toBeNull();
    expect(configure).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Configure inputs" }));
    expect(within(screen.getByRole("dialog")).getByRole("textbox")).toHaveProperty(
      "value",
      initial,
    );
    expect(within(screen.getByRole("dialog")).queryByRole("alert")).toBeNull();
  });

  it("retains command feedback after cancelling configuration review", async () => {
    const configure = vi.fn(async () => {
      throw new Error("The reviewed release is no longer selected.");
    });
    await renderPackages({ configure });
    fireEvent.click(await screen.findByRole("button", { name: "Configure inputs" }));
    const dialog = screen.getByRole("dialog");
    fireEvent.click(within(dialog).getByRole("button", { name: "Save" }));
    await within(dialog).findByText("The reviewed release is no longer selected.");
    fireEvent.click(within(dialog).getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());

    expect(screen.getByText("The reviewed release is no longer selected.")).toBeTruthy();
    expect(configure).toHaveBeenCalledOnce();
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
            testClient.installedPlugins.access.publishOperations({
              signal: ctx.signal,
              pendingCommands: () => [],
              configure: submit,
            } as unknown as PackageOperations),
          );
        },
      }),
    ]);
    testClient.render(
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
    const view = testClient.render(
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

    const view = testClient.render(
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
            testClient.installedPlugins.access.publishOperations({
              signal: ctx.signal,
              pendingCommands: () => [],
            } as unknown as PackageOperations),
          );
        },
      }),
    ]);
    testClient.render(
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
            testClient.installedPlugins.access.publishOperations({
              signal: ctx.signal,
              pendingCommands: () => [],
            } as unknown as PackageOperations),
          );
        },
      }),
    ]);
    testClient.render(
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
    testClient.installedPlugins.access.setFailure({ reason: "stream closed", retry });
    testClient.render(
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
