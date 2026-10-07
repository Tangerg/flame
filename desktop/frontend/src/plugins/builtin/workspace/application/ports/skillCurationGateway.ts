import type { SkillProposalScope } from "@flame/runtime-contract/wire";

export interface SkillProposalHandle {
  workspace: string;
  name: string;
  revision: string;
  scope: SkillProposalScope;
}

export interface SkillCurationGateway {
  archive(name: string): Promise<void>;
  restore(name: string): Promise<void>;
  approveProposal(handle: SkillProposalHandle): Promise<void>;
  rejectProposal(handle: SkillProposalHandle): Promise<void>;
}

export class SkillProposalRevisionConflictError extends Error {
  constructor(cause: Error) {
    super(cause.message, { cause });
    this.name = "SkillProposalRevisionConflictError";
  }
}
