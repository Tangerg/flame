import { afterEach, describe, expect, it, vi } from "vitest";
import { resetContainer, setContainer } from "@/main/container";
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
  await resetContainer();
});

describe("runtimeSkillCurationGateway", () => {
  it.each(["approveProposal", "rejectProposal"] as const)(
    "%ss against the workspace that supplied the reviewed proposal",
    async (decision) => {
      const approveProposal = vi.fn().mockResolvedValue(undefined);
      const rejectProposal = vi.fn().mockResolvedValue(undefined);
      const open = vi.fn().mockResolvedValue({
        skills: { approveProposal, rejectProposal },
      });
      setContainer({
        client: () =>
          ({
            skills: {},
            workspaces: { open },
          }) as unknown as FlameClient,
      });
      installation = installSkillCurationGateway();

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
      setContainer({ client: () => ({ workspaces: { open } }) as unknown as FlameClient });
      installation = installSkillCurationGateway();
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
      setContainer({
        client: () => ({ skills: { archive, restore } }) as unknown as FlameClient,
      });
      installation = installSkillCurationGateway();

      await (decision === "archive" ? archiveSkill : restoreSkill)("verify");

      expect(decision === "archive" ? archive : restore).toHaveBeenCalledWith("verify");
    },
  );
});
