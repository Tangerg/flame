import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { Host } from "dougong";
import { afterEach, describe, expect, it } from "vitest";
import { reportPluginError, startKernel, stopKernel, usePluginErrorStore } from "@/plugins/sdk";
import { trackInstalledPlugin } from "@/plugins/sdk/kernel";
import { PluginsPane } from "./PluginsPane";

let host: Host | undefined;

afterEach(async () => {
  cleanup();
  usePluginErrorStore.getState().clearAll();
  if (!host) return;
  const owned = host;
  host = undefined;
  await stopKernel(owned);
});

describe("PluginsPane installation facts", () => {
  it("shows and clears kernel errors without requiring an installed plugin", async () => {
    host = await startKernel([]);
    trackInstalledPlugin(host, "flame.builtin.example");
    reportPluginError("kernel", "setup", new Error("Lifecycle hook failed"), "hook stack");
    const view = render(<PluginsPane />);

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
    host = await startKernel([]);
    trackInstalledPlugin(host, "flame.builtin.example");

    const view = render(<PluginsPane />);

    expect(screen.getByText("flame.builtin.example")).toBeTruthy();
    view.unmount();
  });
});
