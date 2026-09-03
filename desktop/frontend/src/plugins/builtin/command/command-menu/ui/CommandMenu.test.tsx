import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { definePlugin } from "@/plugins/sdk";
import { COMMAND, WORKSPACE_VIEW } from "@/plugins/sdk/kernelPoints";
import { loadPluginsForTest } from "@/plugins/sdk/testKernel";
import { drainBrowserTasks } from "@/test/browserTasks";
import { useCommandMenuStore } from "../application/commandMenuState";
import { CommandMenu } from "./CommandMenu";

const newChat = vi.fn();
const openViewInDock = vi.hoisted(() => vi.fn());
const openViewOnCard = vi.hoisted(() => vi.fn());
vi.mock("@/plugins/builtin/workspace/public/navigation", () => ({
  WORKSPACE_SETTINGS_VIEW: "settings",
  openWorkspaceView: openViewOnCard,
  openWorkspaceViewInDock: openViewInDock,
}));

async function withCommands() {
  await loadPluginsForTest(
    definePlugin({
      name: "test.command-menu",
      setup: (ctx) => {
        ctx.contribute(COMMAND, {
          id: "chat.new",
          label: "New chat",
          combo: "Mod+N",
          run: newChat,
        });
        ctx.contribute(COMMAND, { id: "chat.rename", label: "Rename chat", run: () => undefined });
        ctx.contribute(WORKSPACE_VIEW, {
          id: "terminal",
          title: "Terminal",
          icon: "terminal",
          dock: "workspace",
          component: () => null,
        });
        // No dock scope: a full-card view, which is the shape that had no way in at all.
        ctx.contribute(WORKSPACE_VIEW, {
          id: "icon-gallery",
          title: "Icon Gallery",
          icon: "spark",
          component: () => null,
        });
      },
    }),
  );
  useCommandMenuStore.setState({ open: true });
  return render(<CommandMenu />);
}

afterEach(async () => {
  newChat.mockClear();
  openViewInDock.mockClear();
  openViewOnCard.mockClear();
  useCommandMenuStore.setState({ open: false });
  cleanup();
  await drainBrowserTasks();
});

describe("command menu", () => {
  it("lists every registered command and shows the key that runs it", async () => {
    await withCommands();

    const rows = screen.getAllByRole("option");
    expect(rows.map((row) => row.firstElementChild?.nextElementSibling?.textContent)).toEqual([
      "New chat",
      "Rename chat",
      "View: Icon Gallery",
      "View: Terminal",
    ]);
    // Modifier glyphs are platform-dependent; that a combo IS shown is the contract.
    expect(rows[0]?.querySelectorAll("kbd")).toHaveLength(2);
    expect(rows[1]?.querySelectorAll("kbd")).toHaveLength(0);
    expect(rows[2]?.querySelectorAll("kbd")).toHaveLength(0);
  });

  it("filters on the label a person can actually read", async () => {
    await withCommands();

    fireEvent.change(screen.getByRole("combobox"), { target: { value: "rename" } });

    expect(screen.getAllByRole("option").map((row) => row.textContent)).toEqual(["Rename chat"]);
  });

  it("runs the command and closes", async () => {
    await withCommands();

    fireEvent.click(screen.getByRole("option", { name: /New chat/ }));

    expect(newChat).toHaveBeenCalledOnce();
    expect(useCommandMenuStore.getState().open).toBe(false);
  });

  it("lists the workspace views, which nothing else reaches by keyboard", async () => {
    await withCommands();

    fireEvent.change(screen.getByRole("combobox"), { target: { value: "terminal" } });
    const row = screen.getByRole("option");
    expect(row.textContent).toContain("Terminal");

    row.click();

    expect(openViewInDock).toHaveBeenCalledWith("terminal");
  });

  it("opens a view the dock does not carry on the content card", async () => {
    await withCommands();

    fireEvent.change(screen.getByRole("combobox"), { target: { value: "icon gallery" } });
    screen.getByRole("option").click();

    expect(openViewOnCard).toHaveBeenCalledWith("icon-gallery");
    expect(openViewInDock).not.toHaveBeenCalled();
  });
});
