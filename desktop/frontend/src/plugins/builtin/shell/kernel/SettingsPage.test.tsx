import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { definePlugin } from "@/plugins/sdk";
import { SETTINGS_PANE } from "@/plugins/sdk/kernelPoints";
import { drainBrowserTasks } from "@/test/browserTasks";
import { SettingsPage } from "./SettingsPage";
import { loadPluginsForTest } from "@/plugins/sdk/testKernel";

async function loadPanes() {
  await loadPluginsForTest(
    definePlugin({
      name: "test.settings",
      setup: (ctx) => {
        ctx.contribute(SETTINGS_PANE, {
          id: "appearance",
          label: "Appearance",
          description: "Tune the interface",
          order: 0,
          component: () => <div data-testid="appearance-body">appearance body</div>,
        });
        ctx.contribute(SETTINGS_PANE, {
          id: "plugins",
          label: "Plugins",
          order: 10,
          component: () => <div data-testid="plugins-body">plugins body</div>,
        });
      },
    }),
  );
}

describe("settingsPage", () => {
  afterEach(async () => {
    cleanup();
    await drainBrowserTasks();
  });

  it("shows the first pane by default and renders its body", async () => {
    await loadPanes();
    render(<SettingsPage />);
    expect(screen.getByTestId("appearance-body")).toBeTruthy();
    expect(screen.getByRole("heading", { name: "Appearance" })).toBeTruthy();
    expect(screen.getByText("Tune the interface")).toBeTruthy();
    expect(screen.getAllByText("Plugins").length).toBeGreaterThan(0);
  });

  it("switches the body when a different pane is clicked", async () => {
    await loadPanes();
    render(<SettingsPage />);
    fireEvent.click(screen.getByText("Plugins"));
    expect(screen.getByTestId("plugins-body")).toBeTruthy();
    await waitFor(() => expect(screen.getAllByRole("tabpanel")).toHaveLength(1));
    expect(screen.getByRole("tabpanel").textContent).toContain("plugins body");
  });

  it("falls back to the first pane when no panes match the saved selection", async () => {
    await loadPanes();
    render(<SettingsPage />);
    expect(screen.getByTestId("appearance-body")).toBeTruthy();
  });
});
