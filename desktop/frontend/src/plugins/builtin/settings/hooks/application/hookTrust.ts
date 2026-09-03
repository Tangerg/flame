import { GenerationRetiredError } from "@/lib/asyncOwnership";
import { HOOKS_KEY } from "./hookQueries";
import { createPublicationSlot } from "@/lib/publicationSlot";
import { repairCachedProjection } from "@/lib/queryClient";
import { RetirableTaskCohort, SerialTaskChain } from "@/lib/taskQueue";

export interface HookTrustGateway {
  setProjectTrust(projectRoot: string, trusted: boolean): Promise<void>;
}

class HookTrustMutationGeneration {
  readonly #gateway: HookTrustGateway;
  readonly #retiredError = new GenerationRetiredError("hook_trust_mutation_generation");
  readonly #cohort = new RetirableTaskCohort(this.#retiredError);
  readonly #chain = new SerialTaskChain();

  constructor(gateway: HookTrustGateway) {
    this.#gateway = gateway;
  }

  setProjectTrust(projectRoot: string, trusted: boolean): Promise<void> {
    return this.#chain.chain(projectRoot, (tail) =>
      this.#cohort.settle(tail).then(async () => {
        this.#cohort.assertCurrent();
        try {
          await this.#cohort.settle(this.#gateway.setProjectTrust(projectRoot, trusted));
        } catch (error) {
          if (error === this.#retiredError) throw error;
          await repairCachedProjection(this.#cohort, [HOOKS_KEY]);
          this.#cohort.assertCurrent();
          throw error;
        }
        this.#cohort.assertCurrent();
        await repairCachedProjection(this.#cohort, [HOOKS_KEY]);
        this.#cohort.assertCurrent();
      }),
    );
  }

  retire(): void {
    this.#cohort.retire();
    this.#chain.clear();
  }
}

export class HookTrustMutationOwner {
  readonly #gateway: HookTrustGateway;
  #generation: HookTrustMutationGeneration;
  #disposed = false;

  private constructor(gateway: HookTrustGateway) {
    this.#gateway = gateway;
    this.#generation = new HookTrustMutationGeneration(gateway);
  }

  static install(gateway: HookTrustGateway): HookTrustMutationOwner {
    const owner = new HookTrustMutationOwner(gateway);
    hookTrustPublication.publish(owner, (predecessor) => predecessor.dispose());
    return owner;
  }

  static current(): HookTrustMutationOwner {
    const owner = hookTrustPublication.current();
    if (!owner || owner.#disposed) throw new Error("Hook trust mutation owner is not installed");
    return owner;
  }

  setProjectTrust(projectRoot: string, trusted: boolean): Promise<void> {
    return this.#generation.setProjectTrust(projectRoot, trusted);
  }

  replaceRuntimeGeneration(): void {
    if (this.#disposed || !hookTrustPublication.owns(this)) return;
    const predecessor = this.#generation;
    this.#generation = new HookTrustMutationGeneration(this.#gateway);
    predecessor.retire();
  }

  dispose(): void {
    if (this.#disposed) return;
    this.#disposed = true;
    this.#generation.retire();
    hookTrustPublication.withdraw(this);
  }
}

const hookTrustPublication = createPublicationSlot<HookTrustMutationOwner>();

export function setHookTrust(projectRoot: string, trusted: boolean): Promise<void> {
  return HookTrustMutationOwner.current().setProjectTrust(projectRoot, trusted);
}
