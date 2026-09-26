import { getContainer } from "@/main/container";
import { isErrorType, type FlameClient } from "@flame/runtime-contract/client";
import { SkillCurationOwner } from "../application/skillCuration";
import {
  SkillProposalRevisionConflictError,
  type SkillCurationGateway,
} from "../application/ports/skillCurationGateway";

function runtimeSkillCurationGateway(client: FlameClient): SkillCurationGateway {
  return {
    archive: (name) => client.skills.archive(name),
    restore: (name) => client.skills.restore(name),
    async approveProposal(handle) {
      const { workspace, ...ref } = handle;
      const resources = await client.workspaces.open({ path: workspace });
      try {
        await resources.skills.approveProposal(ref);
      } catch (error) {
        if (isErrorType(error, "revision_conflict"))
          throw new SkillProposalRevisionConflictError(error);
        throw error;
      }
    },
    async rejectProposal(handle) {
      const { workspace, ...ref } = handle;
      const resources = await client.workspaces.open({ path: workspace });
      try {
        await resources.skills.rejectProposal(ref);
      } catch (error) {
        if (isErrorType(error, "revision_conflict"))
          throw new SkillProposalRevisionConflictError(error);
        throw error;
      }
    },
  };
}

export function installSkillCurationGateway() {
  const gateway = runtimeSkillCurationGateway(getContainer().client());
  const owner = SkillCurationOwner.install(gateway);
  return {
    replaceRuntimeGeneration: () => owner.replaceRuntimeGeneration(),
    dispose: () => owner.dispose(),
  };
}
