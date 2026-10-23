import type React from "react";
import { fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { t } from "@/lib/i18n/index";
import { usePluginErrorStore } from "@/extensions/errors";
import type { WorkspaceViewSpec } from "@/features/workspace/public/views";
import { WorkspaceViewBody } from "@/features/shell/workbench/panel/WorkspaceViewBody";

const installed = vi.hoisted(() => ({ views: [] as WorkspaceViewSpec[] }));

vi.mock("@/features/workspace/public/views", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/features/workspace/public/views")>()),
  useWorkspaceViews: () => installed.views,
}));

afterEach(() => {
  installed.views = [];
});

describe("workspaceViewBody", () => {
  it("names a failing installed view by its own title and recovers it in place", () => {
    const spy = vi.spyOn(console, "error").mockImplementation(() => {});
    let fail = true;
    function DiffView(): React.ReactNode {
      if (fail) throw new Error("diff exploded");
      return <p>diff reader</p>;
    }
    installed.views = [{ id: "diff", title: "Diff", component: DiffView }];
    render(<WorkspaceViewBody viewId="diff" />);

    const region = screen.getByRole("region", { name: "Diff" });
    const alert = within(region).getByRole("alert");
    expect(within(alert).getByText("Diff")).toBeTruthy();
    expect(usePluginErrorStore.getState().log[0]!.plugin).toBe("workspace:diff");

    const retry = within(alert).getByRole("button", { name: /retry/i });
    retry.focus();
    fail = false;
    fireEvent.click(retry);

    expect(within(region).getByText("diff reader")).toBeTruthy();
    expect(document.activeElement).toBe(region);
    spy.mockRestore();
  });

  it("keeps the generic region for a view that is not installed", () => {
    render(<WorkspaceViewBody viewId="missing" />);

    const region = screen.getByRole("region", { name: t("plugins.mainView") });
    expect(within(region).getByText(t("workspace.view.unavailable.title"))).toBeTruthy();
    expect(within(region).queryByRole("alert")).toBeNull();
  });
});
