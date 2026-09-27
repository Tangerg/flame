import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { QueryClientProvider } from "@tanstack/react-query";
import { DATA_PROVIDER, definePlugin } from "@/plugins/sdk";
import { loadPluginsForTest } from "@/plugins/sdk/testKernel";
import { queryClient } from "@/lib/queryClient";
import {
  WORKSPACE_SKILLS_KEY,
  WORKSPACE_SKILL_DETAIL_KEY,
} from "../../application/workspaceQueries";
import { AvailableSkills } from "./AvailableSkills";

const workspace = vi.hoisted(() => ({ cwd: "/project-a" }));
vi.mock("@/plugins/builtin/agent/public/session", () => ({
  useActiveSessionWorkspace: () => ({ status: "ready", cwd: workspace.cwd }),
}));
vi.mock("../../application/workspaceCapabilities", () => ({
  useWorkspaceCapability: () => true,
}));

afterEach(() => {
  cleanup();
  queryClient.clear();
  workspace.cwd = "/project-a";
});

it("keeps usable skills beside diagnostics and loads exact source details only when opened", async () => {
  const detail = vi.fn(async (params?: unknown) => ({
    name: "verify",
    description: "Verify changes",
    scope: "project",
    path: `${(params as { cwd: string }).cwd}/.flame/skills/verify/SKILL.md`,
    revision: "a".repeat(64),
    instructions: `Instructions for ${(params as { cwd: string }).cwd}`,
  }));
  await loadPluginsForTest(
    definePlugin({
      name: "test.discovered-skills",
      setup(ctx) {
        ctx.contribute(DATA_PROVIDER, {
          key: WORKSPACE_SKILLS_KEY,
          fetcher: async () => ({
            skills: [{ name: "verify", description: "Verify changes", scope: "project" }],
            diagnostics: [{ name: "broken", detail: "Repair SKILL.md." }],
          }),
        });
        ctx.contribute(DATA_PROVIDER, { key: WORKSPACE_SKILL_DETAIL_KEY, fetcher: detail });
      },
    }),
  );
  const app = () => (
    <QueryClientProvider client={queryClient}>
      <AvailableSkills />
    </QueryClientProvider>
  );
  const view = render(app());
  expect(await screen.findByText("verify")).toBeDefined();
  const unreadable = screen.getByRole("alert");
  expect(unreadable.textContent).toContain("broken");
  expect(unreadable.textContent).toContain("Repair SKILL.md.");
  expect(detail).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "Read instructions" }));
  expect(await screen.findByText("Instructions for /project-a")).toBeDefined();
  expect(screen.getByTitle("/project-a/.flame/skills/verify/SKILL.md")).toBeDefined();
  expect(screen.getByTitle(`Content hash ${"a".repeat(64)}`)).toBeDefined();

  workspace.cwd = "/project-b";
  view.rerender(app());
  await waitFor(() => expect(screen.queryByText("Instructions for /project-a")).toBeNull());
  expect(detail).toHaveBeenCalledTimes(1);
  fireEvent.click(await screen.findByRole("button", { name: "Read instructions" }));
  expect(await screen.findByText("Instructions for /project-b")).toBeDefined();
});
