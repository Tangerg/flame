import { GenerationRetiredError } from "@/lib/asyncOwnership";
import { HOOKS_KEY } from "@/features/hooks/application/hookQueries";
import { repairCachedProjection } from "@/lib/queryClient";
import { RetirableTaskCohort } from "@/lib/taskQueue";
export interface HookTrustGateway {
  setProjectTrust(projectRoot: string, trusted: boolean): Promise<void>;
}

class HookTrustMutationGeneration {
  readonly #gateway: HookTrustGateway;
  readonly #retiredError = new GenerationRetiredError("hook_trust_mutation_generation");
  readonly #cohort = new RetirableTaskCohort(this.#retiredError);

  constructor(gateway: HookTrustGateway) {
    this.#gateway = gateway;
  }

  setProjectTrust(projectRoot: string, trusted: boolean): Promise<void> {
    return this.#cohort.runSerial(projectRoot, async () => {
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
    });
  }

  retire(): void {
    this.#cohort.retire();
  }
}

export type HookTrustCommands = Pick<HookTrustMutationGeneration, "setProjectTrust">;
export type HookTrustAccess = Pick<HookTrustMutationOwner, "capture" | "subscribe">;

export class HookTrustMutationOwner {
  #generation: HookTrustMutationGeneration | null;
  readonly #listeners = new Set<() => void>();
  #disposed = false;

  constructor(gateway: HookTrustGateway | null) {
    this.#generation = gateway ? new HookTrustMutationGeneration(gateway) : null;
  }

  readonly capture = (): HookTrustCommands | null => this.#generation;

  readonly subscribe = (listener: () => void): (() => void) => {
    this.#listeners.add(listener);
    return () => this.#listeners.delete(listener);
  };

  replaceRuntimeGeneration(createGateway: () => HookTrustGateway): void {
    if (this.#disposed) return;
    const successor = new HookTrustMutationGeneration(createGateway());
    const predecessor = this.#generation;
    this.#generation = successor;
    predecessor?.retire();
    this.#publish();
  }

  retireRuntimeGeneration(): void {
    if (!this.#generation) return;
    const predecessor = this.#generation;
    this.#generation = null;
    predecessor.retire();
    this.#publish();
  }

  dispose(): void {
    if (this.#disposed) return;
    this.#disposed = true;
    this.#generation?.retire();
    this.#generation = null;
    this.#publish();
    this.#listeners.clear();
  }

  #publish(): void {
    for (const listener of this.#listeners) listener();
  }
}
