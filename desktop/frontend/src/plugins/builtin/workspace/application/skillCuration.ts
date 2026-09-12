import { GenerationRetiredError } from "@/lib/asyncOwnership";
import { createPublicationSlot } from "@/lib/publicationSlot";
import { replaceCachedRead } from "@/lib/queryClient";
import { RetirableTaskCohort, SerialTaskChain } from "@/lib/taskQueue";
import { tupleKey } from "@/lib/tupleKey";
import type { SkillCurationGateway, SkillProposalHandle } from "./ports/skillCurationGateway";
import {
  WORKSPACE_MANAGED_SKILLS_KEY,
  WORKSPACE_SKILLS_KEY,
  WORKSPACE_SKILL_PROPOSALS_KEY,
} from "./workspaceQueries";

class SkillCurationGeneration {
  readonly #gateway: SkillCurationGateway;
  readonly #cohort = new RetirableTaskCohort(
    new GenerationRetiredError("skill_curation_generation"),
  );
  readonly #chain = new SerialTaskChain();

  constructor(gateway: SkillCurationGateway) {
    this.#gateway = gateway;
  }

  archive(name: string): Promise<void> {
    return this.#run(userSkillIdentity(name), () => this.#gateway.archive(name));
  }

  restore(name: string): Promise<void> {
    return this.#run(userSkillIdentity(name), () => this.#gateway.restore(name));
  }

  approveProposal(handle: SkillProposalHandle): Promise<void> {
    return this.#run(proposalIdentity(handle), () => this.#gateway.approveProposal(handle));
  }

  rejectProposal(handle: SkillProposalHandle): Promise<void> {
    return this.#run(proposalIdentity(handle), () => this.#gateway.rejectProposal(handle));
  }

  retire(): void {
    this.#cohort.retire();
    this.#chain.clear();
  }

  #run(identity: string, execute: () => Promise<void>): Promise<void> {
    return this.#chain.chain(identity, (tail) =>
      this.#cohort.settle(tail).then(async () => {
        this.#cohort.assertCurrent();
        try {
          await this.#cohort.settle(execute());
        } finally {
          if (!this.#cohort.retired) {
            // Only Runtime can resolve discovery precedence, library order, and
            // proposal revisions. A failed command may also have committed.
            await Promise.all(
              [
                WORKSPACE_SKILLS_KEY,
                WORKSPACE_MANAGED_SKILLS_KEY,
                WORKSPACE_SKILL_PROPOSALS_KEY,
              ].map((key) => this.#cohort.settle(replaceCachedRead({ queryKey: [key] }))),
            );
          }
        }
        this.#cohort.assertCurrent();
      }),
    );
  }
}

/** Owns Skill library curation and proposal review for one exact Plugin Host
 * and Runtime generation. Both command families write the same Skill identity,
 * so they deliberately share one resource-partitioned tail. */
export class SkillCurationOwner {
  readonly #gateway: SkillCurationGateway;
  #generation: SkillCurationGeneration;
  #disposed = false;

  private constructor(gateway: SkillCurationGateway) {
    this.#gateway = gateway;
    this.#generation = new SkillCurationGeneration(gateway);
  }

  static install(gateway: SkillCurationGateway): SkillCurationOwner {
    const owner = new SkillCurationOwner(gateway);
    skillCurationPublication.publish(owner, (predecessor) => predecessor.dispose());
    return owner;
  }

  static current(): SkillCurationOwner {
    const owner = skillCurationPublication.current();
    if (!owner || owner.#disposed) throw new Error("Skill curation owner is not installed");
    return owner;
  }

  archive(name: string): Promise<void> {
    return this.#generation.archive(name);
  }

  restore(name: string): Promise<void> {
    return this.#generation.restore(name);
  }

  approveProposal(handle: SkillProposalHandle): Promise<void> {
    return this.#generation.approveProposal(handle);
  }

  rejectProposal(handle: SkillProposalHandle): Promise<void> {
    return this.#generation.rejectProposal(handle);
  }

  replaceRuntimeGeneration(): void {
    if (this.#disposed || !skillCurationPublication.owns(this)) return;
    const predecessor = this.#generation;
    this.#generation = new SkillCurationGeneration(this.#gateway);
    predecessor.retire();
  }

  dispose(): void {
    if (this.#disposed) return;
    this.#disposed = true;
    this.#generation.retire();
    skillCurationPublication.withdraw(this);
  }
}

const skillCurationPublication = createPublicationSlot<SkillCurationOwner>();

export function archiveSkill(name: string): Promise<void> {
  return SkillCurationOwner.current().archive(name);
}

export function restoreSkill(name: string): Promise<void> {
  return SkillCurationOwner.current().restore(name);
}

export function approveSkillProposal(handle: SkillProposalHandle): Promise<void> {
  return SkillCurationOwner.current().approveProposal(handle);
}

export function rejectSkillProposal(handle: SkillProposalHandle): Promise<void> {
  return SkillCurationOwner.current().rejectProposal(handle);
}

function userSkillIdentity(name: string): string {
  return tupleKey("user", name);
}

function proposalIdentity(handle: SkillProposalHandle): string {
  return handle.scope === "user"
    ? userSkillIdentity(handle.name)
    : tupleKey("project", handle.workspace, handle.name);
}
