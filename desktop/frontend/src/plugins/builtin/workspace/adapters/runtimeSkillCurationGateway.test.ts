import { rejected } from "@/test/rejected";
import { afterEach, describe, expect, it, vi } from "vitest";
import { RpcError, type FlameClient } from "@flame/runtime-contract/client";
import {
  approveSkillProposal,
  archiveSkill,
  rejectSkillProposal,
  restoreSkill,
} from "../application/skillCuration";
import { SkillProposalRevisionConflictError } from "../application/ports/skillCurationGateway";
import { installSkillCurationGateway } from "./runtimeSkillCurationGateway";

let installation: ReturnType<typeof installSkillCurationGateway> | undefined;

afterEach(async () => {
  installation?.dispose();
  installation = undefined;
});

describe("runtimeSkillCurationGateway", () => {
  it("captures a replacement client at the Runtime generation boundary", async () => {
    const response = Promise.withResolvers<void>();
    const retiredArchive = vi.fn(() => response.promise);
    const successorArchive = vi.fn().mockResolvedValue(undefined);
    let client = { skills: { archive: retiredArchive } } as unknown as FlameClient;
    installation = installSkillCurationGateway(() => client);

    const retired = rejected(archiveSkill("verify"));
    await vi.waitFor(() => expect(retiredArchive).toHaveBeenCalledOnce());
    client = { skills: { archive: successorArchive } } as unknown as FlameClient;
    installation.replaceRuntimeGeneration();
    await expect(retired).resolves.toMatchObject({ message: "skill_curation_generation_retired" });
    await archiveSkill("verify");
    expect(successorArchive).toHaveBeenCalledExactlyOnceWith("verify");
    expect(retiredArchive).toHaveBeenCalledOnce();
    response.resolve();
  });

  it.each(["approveProposal", "rejectProposal"] as const)(
    "%ss against the workspace that supplied the reviewed proposal",
    async (decision) => {
      const approveProposal = vi.fn().mockResolvedValue(undefined);
      const rejectProposal = vi.fn().mockResolvedValue(undefined);
      const open = vi.fn().mockResolvedValue({
        skills: { approveProposal, rejectProposal },
      });
      const runtimeClient = () =>
        ({
          skills: {},
          workspaces: { open },
        }) as unknown as FlameClient;
      installation = installSkillCurationGateway(() => runtimeClient());

      await (decision === "approveProposal" ? approveSkillProposal : rejectSkillProposal)({
        workspace: "/work/reviewed",
        name: "verify",
        revision: "rev_1",
        scope: "project",
      });

      expect(open).toHaveBeenCalledWith({ path: "/work/reviewed" });
      const expected = { name: "verify", revision: "rev_1", scope: "project" };
      expect(
        decision === "approveProposal" ? approveProposal : rejectProposal,
      ).toHaveBeenCalledWith(expected);
    },
  );

  it.each(["approveProposal", "rejectProposal"] as const)(
    "translates %s conflicts without retrying a newer revision",
    async (decision) => {
      const cause = new RpcError({
        code: -32009,
        message: "stale proposal",
        data: { type: "revision_conflict" },
      });
      const command = vi.fn().mockRejectedValue(cause);
      const open = vi.fn().mockResolvedValue({ skills: { [decision]: command } });
      const runtimeClient = () => ({ workspaces: { open } }) as unknown as FlameClient;
      installation = installSkillCurationGateway(() => runtimeClient());
      await expect(
        (decision === "approveProposal" ? approveSkillProposal : rejectSkillProposal)({
          workspace: "/repo",
          name: "verify",
          scope: "project",
          revision: "reviewed",
        }),
      ).rejects.toBeInstanceOf(SkillProposalRevisionConflictError);
      expect(command).toHaveBeenCalledExactlyOnceWith({
        name: "verify",
        scope: "project",
        revision: "reviewed",
      });
    },
  );

  it.each(["archive", "restore"] as const)(
    "maps library %s through the captured client",
    async (decision) => {
      const archive = vi.fn().mockResolvedValue(undefined);
      const restore = vi.fn().mockResolvedValue(undefined);
      const runtimeClient = () => ({ skills: { archive, restore } }) as unknown as FlameClient;
      installation = installSkillCurationGateway(() => runtimeClient());

      await (decision === "archive" ? archiveSkill : restoreSkill)("verify");

      expect(decision === "archive" ? archive : restore).toHaveBeenCalledWith("verify");
    },
  );
});
