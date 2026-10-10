import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { QueryClientProvider } from "@tanstack/react-query";
import { DATA_PROVIDER, definePlugin } from "@/plugins/sdk";
import { loadPluginsForTest, resetKernelForTest } from "@/plugins/sdk/testKernel";
import { queryClient } from "@/lib/queryClient";
import {
  WORKSPACE_SKILL_PROPOSALS_KEY,
  type SkillProposal,
} from "../../application/workspaceQueries";
import { SkillCurationOwner } from "../../application/skillCuration";
import { SkillProposalRevisionConflictError } from "../../application/ports/skillCurationGateway";
import { SkillProposalControls } from "./SkillControls";

const notices = vi.hoisted(() => ({ error: vi.fn() }));
vi.mock("@/plugins/sdk", async (original) => ({
  ...(await original<typeof import("@/plugins/sdk")>()),
  notifyError: notices.error,
}));
let owner: SkillCurationOwner | undefined;
afterEach(async () => {
  cleanup();
  owner?.dispose();
  queryClient.clear();
  vi.clearAllMocks();
  await resetKernelForTest();
});

it("submits the content actually reviewed and requires another review after a revision conflict", async () => {
  let proposal: SkillProposal = {
    workspace: "/repo",
    name: "verify",
    scope: "project",
    revision: "revision-one",
    description: "Verify changes",
    instructions: "Original instructions",
    origin: "requested",
    revises: false,
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
    .mockRejectedValueOnce(new SkillProposalRevisionConflictError(new Error("changed")))
    .mockResolvedValueOnce(undefined);
  owner = SkillCurationOwner.install({
    approveProposal: approve,
    rejectProposal: vi.fn(),
    archive: vi.fn(),
    restore: vi.fn(),
  });
  const saved = vi.fn();
  render(
    <QueryClientProvider client={queryClient}>
      <SkillProposalControls cwd="/repo" signal={new AbortController().signal} onSaved={saved} />
    </QueryClientProvider>,
  );
  await waitFor(() =>
    expect(
      (screen.getByRole("button", { name: "Select a proposal" }) as HTMLButtonElement).disabled,
    ).toBe(false),
  );
  fireEvent.click(screen.getByRole("button", { name: "Select a proposal" }));
  fireEvent.click(await screen.findByRole("menuitem", { name: /verify/ }));
  fireEvent.click(screen.getByRole("button", { name: "Approve" }));
  expect(screen.getByRole("dialog").textContent).toContain("Original instructions");
  expect(screen.getByRole("dialog").textContent).toContain("Verify changes");
  proposal = { ...proposal, revision: "revision-two", instructions: "Updated instructions" };
  await act(async () => {
    await queryClient.invalidateQueries({ queryKey: [WORKSPACE_SKILL_PROPOSALS_KEY] });
  });
  expect(screen.getByRole("dialog").textContent).toContain("Original instructions");
  fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Approve" }));
  await waitFor(() => expect(notices.error).toHaveBeenCalledOnce());
  expect(approve).toHaveBeenCalledExactlyOnceWith({
    workspace: "/repo",
    name: "verify",
    scope: "project",
    revision: "revision-one",
  });
  expect(screen.queryByRole("button", { name: "Approve" })).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Select a proposal" }));
  fireEvent.click(await screen.findByRole("menuitem", { name: /verify/ }));
  fireEvent.click(screen.getByRole("button", { name: "Approve" }));
  expect(screen.getByRole("dialog").textContent).toContain("Updated instructions");
  fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Approve" }));
  await waitFor(() =>
    expect(approve).toHaveBeenLastCalledWith({
      workspace: "/repo",
      name: "verify",
      scope: "project",
      revision: "revision-two",
    }),
  );
  await waitFor(() => expect(saved).toHaveBeenCalledTimes(2));
});
