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
          keywords: ["Hot reload"],
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

  it("keeps the open pane and offers a way out when nothing matches", async () => {
    await loadPanes();
    render(<SettingsPage />);
    const search = screen.getByRole("searchbox");
    fireEvent.change(search, { target: { value: "zzzz" } });

    expect(screen.getByTestId("appearance-body")).toBeTruthy();
    expect(screen.getByRole("status").textContent).toContain("zzzz");
    fireEvent.click(screen.getByRole("button", { name: /clear search/i }));
    expect(screen.getAllByRole("tab")).toHaveLength(2);
  });

  it("finds a pane by a field it contains, not only by its title", async () => {
    await loadPanes();
    render(<SettingsPage />);
    fireEvent.change(screen.getByRole("searchbox"), { target: { value: "hot reload" } });
    expect(screen.getAllByRole("tab").map((tab) => tab.textContent)).toEqual(["Plugins"]);
  });
});
