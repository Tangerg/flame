import { afterEach, describe, expect, it, vi } from "vitest";
import { QueryObserver } from "@tanstack/react-query";
import { queryClient } from "@/lib/queryClient";
import {
  approveSkillProposal,
  archiveSkill,
  restoreSkill,
  SkillCurationOwner,
} from "./skillCuration";
import type { SkillCurationGateway } from "./ports/skillCurationGateway";
import {
  WORKSPACE_MANAGED_SKILLS_KEY,
  WORKSPACE_SKILLS_KEY,
  WORKSPACE_SKILL_PROPOSALS_KEY,
  type WorkspaceSkill,
} from "./workspaceQueries";
import { rejected } from "@/test/rejected";

let owner: SkillCurationOwner | undefined;
const unsubscribeQueries: Array<() => void> = [];

function observeCatalog<T>(
  queryKey: readonly unknown[],
  initialData: T,
  queryFn: () => Promise<T>,
) {
  const observer = new QueryObserver(queryClient, {
    queryKey,
    queryFn,
    initialData,
    staleTime: Infinity,
    retry: false,
  });
  unsubscribeQueries.push(observer.subscribe(() => undefined));
  return observer;
}

afterEach(() => {
  for (const unsubscribe of unsubscribeQueries.splice(0)) unsubscribe();
  owner?.dispose();
  owner = undefined;
  queryClient.removeQueries({ queryKey: [WORKSPACE_MANAGED_SKILLS_KEY] });
  queryClient.removeQueries({ queryKey: [WORKSPACE_SKILLS_KEY] });
  queryClient.removeQueries({ queryKey: [WORKSPACE_SKILL_PROPOSALS_KEY] });
  vi.restoreAllMocks();
});

describe("skill curation generation", () => {
  it("keeps project discovery authoritative while a personal skill restore refreshes", async () => {
    const project: WorkspaceSkill = {
      name: "review-checklist",
      description: "Project review",
      scope: "project",
    };
    const refreshed = Promise.withResolvers<WorkspaceSkill[]>();
    const read = vi.fn(() => refreshed.promise);
    const discovered = observeCatalog([WORKSPACE_SKILLS_KEY, { cwd: "/repo" }], [project], read);
    queryClient.setQueryData(
      [WORKSPACE_MANAGED_SKILLS_KEY],
      [
        {
          name: project.name,
          description: "Personal review",
          lifecycle: "archived",
        },
      ],
    );
    owner = SkillCurationOwner.install({
      restore: vi.fn().mockResolvedValue(undefined),
    } as unknown as SkillCurationGateway);

    const restoring = restoreSkill(project.name);
    try {
      await vi.waitFor(() => expect(read).toHaveBeenCalledOnce());
      expect(discovered.getCurrentResult().data).toEqual([project]);
    } finally {
      refreshed.resolve([project]);
      await restoring;
    }
    expect(discovered.getCurrentResult().data).toEqual([project]);
  });

  it("refreshes a personal proposal in every workspace that listed it", async () => {
    const handle = {
      workspace: "/repo",
      name: "review",
      revision: "rev-2",
      scope: "user" as const,
    };
    const first = observeCatalog(
      [WORKSPACE_SKILL_PROPOSALS_KEY, { cwd: "/repo" }],
      [handle],
      async () => [],
    );
    const second = observeCatalog(
      [WORKSPACE_SKILL_PROPOSALS_KEY, { cwd: "/other" }],
      [{ ...handle, workspace: "/other" }],
      async () => [],
    );
    owner = SkillCurationOwner.install({
      approveProposal: vi.fn().mockResolvedValue(undefined),
    } as unknown as SkillCurationGateway);

    await approveSkillProposal(handle);

    expect(first.getCurrentResult().data).toEqual([]);
    expect(second.getCurrentResult().data).toEqual([]);
  });

  it("serializes library and proposal decisions that write the same user Skill", async () => {
    const restored = Promise.withResolvers<void>();
    const restore = vi.fn(() => restored.promise);
    const approveProposal = vi.fn().mockResolvedValue(undefined);
    owner = SkillCurationOwner.install({
      restore,
      approveProposal,
    } as unknown as SkillCurationGateway);

    const restoring = restoreSkill("review-checklist");
    const approving = approveSkillProposal({
      workspace: "/repo",
      name: "review-checklist",
      revision: "rev-2",
      scope: "user",
    });
    await vi.waitFor(() => expect(restore).toHaveBeenCalledOnce());

    const approveCallsBeforeRestoreSettled = approveProposal.mock.calls.length;
    restored.resolve();
    await expect(restoring).resolves.toBeUndefined();
    await expect(approving).resolves.toBeUndefined();
    expect(approveCallsBeforeRestoreSettled).toBe(0);
    expect(approveProposal).toHaveBeenCalledOnce();
  });

  it("does not let an old Host archive repair the successor projections", async () => {
    const archived = Promise.withResolvers<void>();
    const archive = vi.fn(() => archived.promise);
    owner = SkillCurationOwner.install({ archive } as unknown as SkillCurationGateway);
    const invalidate = vi.spyOn(queryClient, "invalidateQueries").mockResolvedValue();

    const retired = archiveSkill("review-checklist");
    const retiredSettlement = rejected(retired);
    await vi.waitFor(() => expect(archive).toHaveBeenCalledOnce());
    owner = SkillCurationOwner.install({ archive: vi.fn() } as unknown as SkillCurationGateway);
    archived.resolve();

    await expect(retiredSettlement).resolves.toMatchObject({
      message: "skill_curation_generation_retired",
    });
    expect(invalidate).not.toHaveBeenCalled();
  });

  it("retires in-flight curation on an in-place Runtime generation change", async () => {
    const archived = Promise.withResolvers<void>();
    const archive = vi.fn().mockReturnValueOnce(archived.promise).mockResolvedValueOnce(undefined);
    owner = SkillCurationOwner.install({ archive } as unknown as SkillCurationGateway);

    const retired = rejected(archiveSkill("review-checklist"));
    await vi.waitFor(() => expect(archive).toHaveBeenCalledOnce());
    owner.replaceRuntimeGeneration();

    await expect(retired).resolves.toMatchObject({
      message: "skill_curation_generation_retired",
    });
    await expect(archiveSkill("review-checklist")).resolves.toBeUndefined();
    archived.resolve();
  });

  it("keeps read failure visible without inventing an accepted command's catalog", async () => {
    const skill = {
      name: "review-checklist",
      description: "Review safely",
      scope: "user" as const,
    };
    const failure = new Error("catalog unavailable");
    const discovered = observeCatalog(
      [WORKSPACE_SKILLS_KEY, { cwd: "/repo" }],
      [skill],
      async () => {
        throw failure;
      },
    );
    owner = SkillCurationOwner.install({
      archive: vi.fn().mockResolvedValue(undefined),
    } as unknown as SkillCurationGateway);

    await expect(archiveSkill(skill.name)).resolves.toBeUndefined();

    expect(discovered.getCurrentResult()).toMatchObject({
      isError: true,
      error: failure,
      data: [skill],
    });
  });

  it("refreshes canonical workspace results through the query's original alias", async () => {
    const handle = {
      workspace: "/canonical/repo",
      name: "project-review",
      revision: "rev-1",
      scope: "project" as const,
    };
    const skill = { name: handle.name, description: "Project review", scope: handle.scope };
    const query = { cwd: "/repo-alias" };
    const proposals = observeCatalog(
      [WORKSPACE_SKILL_PROPOSALS_KEY, query],
      [handle],
      async () => [],
    );
    const discovered = observeCatalog<WorkspaceSkill[]>(
      [WORKSPACE_SKILLS_KEY, query],
      [],
      async () => [skill],
    );
    owner = SkillCurationOwner.install({
      approveProposal: vi.fn().mockResolvedValue(undefined),
    } as unknown as SkillCurationGateway);

    await approveSkillProposal(handle);

    expect(proposals.getCurrentResult().data).toEqual([]);
    expect(discovered.getCurrentResult().data).toEqual([skill]);
  });

  it("preserves command failure while refreshing an uncertain durable outcome", async () => {
    const failure = new Error("connection closed after commit");
    const library = observeCatalog(
      [WORKSPACE_MANAGED_SKILLS_KEY],
      [{ name: "review", lifecycle: "active" }],
      async () => [{ name: "review", lifecycle: "archived" }],
    );
    owner = SkillCurationOwner.install({
      archive: vi.fn().mockRejectedValue(failure),
    } as unknown as SkillCurationGateway);

    await expect(archiveSkill("review")).rejects.toBe(failure);

    expect(library.getCurrentResult().data).toEqual([{ name: "review", lifecycle: "archived" }]);
  });

  it("replaces an in-flight first read before publishing the post-command catalog", async () => {
    const beforeArchive = Promise.withResolvers<WorkspaceSkill[]>();
    const read = vi.fn().mockReturnValueOnce(beforeArchive.promise).mockResolvedValue([]);
    const discovered = new QueryObserver<WorkspaceSkill[]>(queryClient, {
      queryKey: [WORKSPACE_SKILLS_KEY, { cwd: "/repo" }],
      queryFn: read,
      retry: false,
    });
    unsubscribeQueries.push(discovered.subscribe(() => undefined));
    owner = SkillCurationOwner.install({
      archive: vi.fn().mockResolvedValue(undefined),
    } as unknown as SkillCurationGateway);

    try {
      await vi.waitFor(() => expect(read).toHaveBeenCalledOnce());
      await archiveSkill("review");
      expect(read).toHaveBeenCalledTimes(2);
      expect(discovered.getCurrentResult().data).toEqual([]);
    } finally {
      beforeArchive.resolve([{ name: "review", description: "Old personal skill", scope: "user" }]);
      await beforeArchive.promise;
    }
    expect(discovered.getCurrentResult().data).toEqual([]);
  });
});
