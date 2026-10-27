import { act, cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { Button, Select } from "@flame/ui";
import type { PluginInstallation } from "@flame/runtime-client/wire";
import { definePlugin } from "@/extensions/definePlugin";
import { DATA_PROVIDER } from "@/extensions/dataProvider";
import {
  PACKAGES_KEY,
  PROVIDERS_KEY,
  type PackageOperations,
} from "@/features/plugins/adapters/packages";
import { PackageManager } from "@/features/plugins/ui/PackageManager";
import { SETTINGS_PANE } from "@/features/settings/public/panes";
import { SettingsPage } from "@/features/settings/public/SettingsPage";
import { loadPluginsForTest } from "@/test/kernel";
import { createTestClient, type TestClient } from "@/test/renderClient";
import { drainBrowserTasks } from "@/test/browserTasks";

const sourceLabel = "Absolute package directory or ZIP path on the Runtime";

let client: TestClient;
beforeEach(() => {
  client = createTestClient();
});
afterEach(async () => {
  cleanup();
  await drainBrowserTasks();
  await client.dispose();
});

const installation: PluginInstallation = {
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
    name: "Draft package",
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

type Install = (
  request: Parameters<PackageOperations["install"]>[0],
) => Promise<PluginInstallation>;

async function openSettings(install = vi.fn<Install>(async () => installation)) {
  const controller = new AbortController();
  const operations = {
    signal: controller.signal,
    install,
    pendingCommands: () => [],
  } as unknown as PackageOperations;
  client.installedPlugins.access.publishOperations(operations);
  await loadPluginsForTest(
    definePlugin({
      name: "test.settings-drafts",
      setup(ctx) {
        ctx.contribute(DATA_PROVIDER, { key: PACKAGES_KEY, fetcher: async () => [] });
        ctx.contribute(DATA_PROVIDER, { key: PROVIDERS_KEY, fetcher: async () => [] });
        ctx.contribute(SETTINGS_PANE, {
          id: "plugins",
          label: "Plugins",
          order: 0,
          component: PackageManager,
        });
        ctx.contribute(SETTINGS_PANE, {
          id: "appearance",
          label: "Appearance",
          order: 1,
          component: () => <Button>Appearance action</Button>,
        });
        ctx.contribute(SETTINGS_PANE, {
          id: "reader",
          label: "Reader",
          order: 2,
          component: () => (
            <Select
              label="Reading size"
              value="14"
              onValueChange={() => undefined}
              options={[
                { value: "14", label: "14" },
                { value: "18", label: "18" },
              ]}
            />
          ),
        });
      },
    }),
  );
  const view = client.render(<SettingsPage />);
  const source = screen.getByRole<HTMLInputElement>("textbox", {
    name: sourceLabel,
  });
  await screen.findByText("No Runtime packages installed");
  return { view, source, operations, controller, install };
}

function choose(name: string) {
  const tab = screen.getByRole("tab", { name });
  act(() => tab.focus());
  fireEvent.click(tab);
  return tab;
}

describe("unfinished settings tasks", () => {
  it("keeps the exact installation input through category and search navigation", async () => {
    const { source, install } = await openSettings();
    const draft = "/workspace/插件 sources/Draft package";
    fireEvent.change(source, { target: { value: draft } });
    act(() => {
      source.focus();
      source.setSelectionRange(11, 17, "backward");
    });
    expect(screen.queryByRole("combobox", { name: "Reading size", hidden: true })).toBeNull();

    const appearance = choose("Appearance");
    await waitFor(() =>
      expect(
        screen.queryByRole("textbox", {
          name: sourceLabel,
        }),
      ).toBeNull(),
    );
    expect(document.activeElement).toBe(appearance);
    const search = screen.getByRole("searchbox");
    fireEvent.change(search, { target: { value: "plugins" } });
    fireEvent.keyDown(search, { key: "Enter" });

    expect(
      screen.getByRole("textbox", {
        name: sourceLabel,
      }),
    ).toBe(source);
    expect(source.value).toBe(draft);
    expect([source.selectionStart, source.selectionEnd, source.selectionDirection]).toEqual([
      11,
      17,
      "backward",
    ]);
    expect(install).not.toHaveBeenCalled();
  });

  it("retains pending admission and the accepted outcome when returning to an installation", async () => {
    const completion = Promise.withResolvers<PluginInstallation>();
    const install = vi.fn<Install>(() => completion.promise);
    const { source } = await openSettings(install);
    fireEvent.change(source, { target: { value: "/pending package" } });
    fireEvent.click(screen.getByRole("button", { name: "Install" }));
    choose("Appearance");
    await waitFor(() => expect(screen.queryByRole("textbox", { name: sourceLabel })).toBeNull());
    choose("Plugins");

    const submit = screen.getByRole("button", { name: "Install" });
    expect(submit.getAttribute("aria-disabled")).toBe("true");
    expect(submit.getAttribute("aria-busy")).toBe("true");
    expect(screen.getByRole("textbox", { name: sourceLabel })).toBe(source);
    expect(source.readOnly).toBe(true);
    fireEvent.click(submit);
    expect(install).toHaveBeenCalledExactlyOnceWith({ source: "/pending package" });
    act(() => submit.focus());
    await act(async () => completion.resolve(installation));

    expect(
      await screen.findByText(
        "Installed Draft package. Review its release below before enabling it.",
      ),
    ).toBeTruthy();
    expect(source.value).toBe("");
    expect(document.activeElement).toBe(submit);
    expect((submit as HTMLButtonElement).disabled).toBe(false);
    expect(submit.getAttribute("aria-disabled")).toBe("true");
    expect(submit.hasAttribute("aria-busy")).toBe(false);
    choose("Appearance");
    choose("Plugins");
    expect(
      screen.getByText("Installed Draft package. Review its release below before enabling it."),
    ).toBeTruthy();
    expect(install).toHaveBeenCalledTimes(1);
  });

  it("keeps a failed source editable after a hidden completion and retries its correction", async () => {
    const completion = Promise.withResolvers<PluginInstallation>();
    const install = vi.fn<Install>(() => completion.promise);
    const { source } = await openSettings(install);
    fireEvent.change(source, { target: { value: "/missing package" } });
    fireEvent.click(screen.getByRole("button", { name: "Install" }));
    const appearance = choose("Appearance");
    await waitFor(() => expect(screen.queryByRole("textbox", { name: sourceLabel })).toBeNull());
    await act(async () => completion.reject(new Error("Package source is unavailable")));
    expect(document.activeElement).toBe(appearance);
    choose("Plugins");

    expect(screen.getByRole("textbox", { name: sourceLabel })).toBe(source);
    expect(source.value).toBe("/missing package");
    expect(screen.getByText("Package source is unavailable")).toBeTruthy();
    expect(source.readOnly).toBe(false);
    fireEvent.change(source, { target: { value: "/corrected package" } });
    expect(screen.queryByText("Package source is unavailable")).toBeNull();
    install.mockResolvedValueOnce(installation);
    fireEvent.click(screen.getByRole("button", { name: "Install" }));
    expect(
      await screen.findByText(
        "Installed Draft package. Review its release below before enabling it.",
      ),
    ).toBeTruthy();
    expect(install.mock.calls.map(([request]) => request)).toEqual([
      { source: "/missing package" },
      { source: "/corrected package" },
    ]);
  });

  it("releases category drafts when the settings view closes", async () => {
    const { source, view, install } = await openSettings();
    fireEvent.change(source, { target: { value: "/closed package" } });
    choose("Appearance");
    view.unmount();
    client.navigation.go({ settings: "plugins" });
    client.render(<SettingsPage />);
    expect(
      screen.getByRole<HTMLInputElement>("textbox", {
        name: sourceLabel,
      }).value,
    ).toBe("");
    expect(install).not.toHaveBeenCalled();
  });

  it("retires an installation source when its Runtime operation generation is replaced", async () => {
    const { source, operations, controller, install } = await openSettings();
    fireEvent.change(source, { target: { value: "/previous runtime package" } });
    act(() => {
      controller.abort();
      client.installedPlugins.access.publishOperations({
        ...operations,
        signal: new AbortController().signal,
      });
    });
    expect(source.value).toBe("");
    expect(screen.getByRole<HTMLButtonElement>("button", { name: "Install" }).disabled).toBe(true);
    expect(install).not.toHaveBeenCalled();
  });

  it("does not clear a new Runtime draft when the previous installation settles late", async () => {
    const completion = Promise.withResolvers<PluginInstallation>();
    const { source, operations, controller } = await openSettings(
      vi.fn<Install>(() => completion.promise),
    );
    fireEvent.change(source, { target: { value: "/previous runtime package" } });
    fireEvent.click(screen.getByRole("button", { name: "Install" }));
    const install = vi.fn<Install>(async () => installation);
    act(() => {
      controller.abort();
      client.installedPlugins.access.publishOperations({
        ...operations,
        install,
        signal: new AbortController().signal,
      } as unknown as PackageOperations);
    });
    fireEvent.change(source, { target: { value: "/current runtime package" } });
    act(() => source.focus());
    await act(async () => completion.resolve(installation));

    expect(source.value).toBe("/current runtime package");
    expect(screen.queryByText(/Installed Draft package/)).toBeNull();
    expect(document.activeElement).toBe(source);
    fireEvent.click(screen.getByRole("button", { name: "Install" }));
    expect(await screen.findByText(/Installed Draft package/)).toBeTruthy();
    expect(install).toHaveBeenCalledExactlyOnceWith({ source: "/current runtime package" });
  });

  it("retires a hidden category's popup and does not reopen it on return", async () => {
    await openSettings();
    choose("Reader");
    const size = screen.getByRole("combobox", { name: "Reading size" });
    fireEvent.click(size);
    expect(await screen.findByRole("listbox")).toBeTruthy();
    act(() => client.navigation.go({ settings: "appearance" }));
    await waitFor(() => expect(screen.queryByRole("listbox")).toBeNull());
    expect(screen.getByRole("tabpanel", { name: "Appearance" })).toBeTruthy();
    choose("Reader");
    expect(screen.queryByRole("listbox")).toBeNull();
    expect(within(screen.getByRole("tabpanel", { name: "Reader" })).getByRole("combobox")).toBe(
      size,
    );
  });
});
