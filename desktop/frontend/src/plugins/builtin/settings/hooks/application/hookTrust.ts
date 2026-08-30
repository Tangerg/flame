import { HOOKS_KEY } from "./hookQueries";
import { createPublicationSlot } from "@/lib/publicationSlot";
import { queryClient } from "@/lib/queryClient";
import { RetirableTaskCohort, SerialTaskChain } from "@/lib/taskQueue";

export interface HookTrustGateway {
  setProjectTrust(projectRoot: string, trusted: boolean): Promise<void>;
}

class HookTrustMutationRetiredError extends Error {
  override readonly name = "HookTrustMutationRetiredError";

  constructor() {
    super("hook_trust_mutation_generation_retired");
  }
}

class HookTrustMutationGeneration {
  readonly #gateway: HookTrustGateway;
  readonly #retiredError = new HookTrustMutationRetiredError();
  readonly #cohort = new RetirableTaskCohort(this.#retiredError);
  readonly #chain = new SerialTaskChain();

  constructor(gateway: HookTrustGateway) {
    this.#gateway = gateway;
  }

  setProjectTrust(projectRoot: string, trusted: boolean): Promise<void> {
    return this.#chain.chain(projectRoot, (tail) =>
      this.#settle(tail).then(async () => {
        this.#assertCurrent();
        try {
          await this.#settle(this.#gateway.setProjectTrust(projectRoot, trusted));
        } catch (error) {
          if (error === this.#retiredError) throw error;
          await this.#repairProjection();
          this.#assertCurrent();
          throw error;
        }
        this.#assertCurrent();
        await this.#repairProjection();
        this.#assertCurrent();
      }),
    );
  }

  retire(): void {
    this.#cohort.retire();
    this.#chain.clear();
  }

  async #repairProjection(): Promise<void> {
    try {
      await this.#settle(queryClient.invalidateQueries({ queryKey: [HOOKS_KEY] }));
    } catch (error) {
      if (error === this.#retiredError) throw error;
      // The Runtime command already settled. Hook events and the next read
      // remain the authoritative projection repair path.
    }
  }

  #settle<T>(operation: Promise<T>): Promise<T> {
    return this.#cohort.settle(operation);
  }

  #assertCurrent(): void {
    this.#cohort.assertCurrent();
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

export function hookTrustMutationWasRetired(error: unknown): boolean {
  return error instanceof HookTrustMutationRetiredError;
}
