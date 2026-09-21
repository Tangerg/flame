import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { QueryClientProvider } from "@tanstack/react-query";
import { DATA_PROVIDER, definePlugin } from "@/plugins/sdk";
import { loadPluginsForTest } from "@/plugins/sdk/testKernel";
import { queryClient } from "@/lib/queryClient";
import {
  WORKSPACE_SKILL_PROPOSALS_KEY,
  type SkillProposal,
} from "../../application/workspaceQueries";
import { SkillCurationOwner } from "../../application/skillCuration";
import { SkillProposalRevisionConflictError } from "../../application/ports/skillCurationGateway";
import { SkillProposals } from "./SkillProposals";

const notices = vi.hoisted(() => ({ error: vi.fn() }));
vi.mock("@/plugins/sdk", async (original) => ({
  ...(await original<typeof import("@/plugins/sdk")>()),
  notifyError: notices.error,
}));
vi.mock("@/plugins/builtin/agent/public/session", () => ({
  useActiveSessionWorkspace: () => ({ status: "ready", cwd: "/repo" }),
}));
let owner: SkillCurationOwner | undefined;
afterEach(() => {
  cleanup();
  owner?.dispose();
  queryClient.clear();
  vi.clearAllMocks();
});

it("keeps the review open after a conflict and requires an explicit decision on the refreshed revision", async () => {
  let proposal: SkillProposal = {
    workspace: "/repo",
    name: "verify",
    scope: "project",
    revision: "revision-one",
    description: "Verify changes",
    instructions: "Original instructions",
    origin: "requested",
    revises: false,
    sourceSession: "",
  };
  await loadPluginsForTest(
    definePlugin({
      name: "test.skill-proposals",
      setup(ctx) {
        ctx.contribute(DATA_PROVIDER, {
          key: WORKSPACE_SKILL_PROPOSALS_KEY,
          fetcher: async () => [proposal],
        });
      },
    }),
  );
  const approve = vi
    .fn()
    .mockImplementationOnce(async () => {
      proposal = { ...proposal, revision: "revision-two", instructions: "Updated instructions" };
      throw new SkillProposalRevisionConflictError(new Error("changed"));
    })
    .mockResolvedValueOnce(undefined);
  owner = SkillCurationOwner.install({
    approveProposal: approve,
    rejectProposal: vi.fn(),
    archive: vi.fn(),
    restore: vi.fn(),
  });
  render(
    <QueryClientProvider client={queryClient}>
      <SkillProposals />
    </QueryClientProvider>,
  );
  fireEvent.click(await screen.findByRole("button", { name: "Read instructions" }));
  const disclosure = screen.getByRole("button", { name: "Hide instructions" });
  fireEvent.click(screen.getByRole("button", { name: "Approve" }));
  await waitFor(() => expect(notices.error).toHaveBeenCalledOnce());
  expect(screen.getByRole("button", { name: "Hide instructions" })).toBe(disclosure);
  expect(screen.getByText("Updated instructions")).toBeDefined();
  expect(approve).toHaveBeenCalledTimes(1);
  fireEvent.click(screen.getByRole("button", { name: "Approve" }));
  await waitFor(() =>
    expect(approve).toHaveBeenLastCalledWith({
      workspace: "/repo",
      name: "verify",
      scope: "project",
      revision: "revision-two",
    }),
  );
});
