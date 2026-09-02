import { createPublicationSlot } from "@/lib/publicationSlot";
import { queryClient, repairCachedProjection } from "@/lib/queryClient";
import { RetirableTaskCohort, SerialTaskChain } from "@/lib/taskQueue";
import { tupleKey } from "@/lib/tupleKey";
import type { ProviderGateway, ProviderTestOutcome, ProviderUpdate } from "./ports/providerGateway";
import type { ProviderConfiguration, ProviderRole } from "./providerModels";
import { EMBEDDING_ROLE_KEY, MODELS_KEY, PROVIDERS_KEY, UTILITY_ROLE_KEY } from "./providerQueries";

class ProviderMutationRetiredError extends Error {
  override readonly name = "ProviderMutationRetiredError";

  constructor() {
    super("provider_mutation_generation_retired");
  }
}

interface ProviderMutation<T> {
  execute(): Promise<T>;
  commit(result: T): void;
  repair: readonly string[];
}

class ProviderMutationGeneration {
  readonly #gateway: ProviderGateway;
  readonly #retiredError = new ProviderMutationRetiredError();
  readonly #cohort = new RetirableTaskCohort(this.#retiredError);
  readonly #chain = new SerialTaskChain();

  constructor(gateway: ProviderGateway) {
    this.#gateway = gateway;
  }

  updateProvider(input: ProviderUpdate): Promise<ProviderConfiguration> {
    return this.#run(tupleKey("provider", input.provider), {
      execute: () => this.#gateway.updateProvider(input),
      commit: commitProviderSaved,
      repair: [PROVIDERS_KEY, MODELS_KEY],
    });
  }

  setUtilityRole(role: ProviderRole): Promise<ProviderRole> {
    return this.#run("utility-role", {
      execute: () => this.#gateway.setUtilityRole(role),
      commit: (saved) => queryClient.setQueryData([UTILITY_ROLE_KEY], saved),
      repair: [UTILITY_ROLE_KEY],
    });
  }

  setEmbeddingRole(role: ProviderRole): Promise<ProviderRole> {
    return this.#run("embedding-role", {
      execute: () => this.#gateway.setEmbeddingRole(role),
      commit: (saved) => queryClient.setQueryData([EMBEDDING_ROLE_KEY], saved),
      repair: [EMBEDDING_ROLE_KEY],
    });
  }

  testProvider(provider: string): Promise<ProviderTestOutcome> {
    return this.#execute(() => this.#gateway.testProvider(provider));
  }

  retire(): void {
    this.#cohort.retire();
    this.#chain.clear();
  }

  #run<T>(identity: string, mutation: ProviderMutation<T>): Promise<T> {
    return this.#chain.chain(identity, (tail) =>
      this.#settle(tail).then(async () => {
        const value = await this.#execute(mutation.execute);
        mutation.commit(value);
        await this.#repairProjection(mutation.repair);
        this.#assertCurrent();
        return value;
      }),
    );
  }

  async #execute<T>(operation: () => Promise<T>): Promise<T> {
    this.#assertCurrent();
    const value = await this.#settle(operation());
    this.#assertCurrent();
    return value;
  }

  #repairProjection(keys: readonly string[]): Promise<void> {
    return repairCachedProjection(this.#cohort, keys);
  }

  #settle<T>(operation: Promise<T>): Promise<T> {
    return this.#cohort.settle(operation);
  }

  #assertCurrent(): void {
    this.#cohort.assertCurrent();
  }
}

/** Owns Provider commands and their cache projection for one exact Plugin Host
 * and Runtime generation. */
export class ProviderMutationOwner {
  static #materialGeneration = 0n;
  static readonly #materialListeners = new Set<() => void>();

  readonly #gateway: ProviderGateway;
  #generation: ProviderMutationGeneration;
  #disposed = false;

  private constructor(gateway: ProviderGateway) {
    this.#gateway = gateway;
    this.#generation = new ProviderMutationGeneration(gateway);
  }

  static install(gateway: ProviderGateway): ProviderMutationOwner {
    const owner = new ProviderMutationOwner(gateway);
    providerMutationPublication.publish(owner, (predecessor) => predecessor.dispose());
    ProviderMutationOwner.#advanceMaterialGeneration();
    return owner;
  }

  static current(): ProviderMutationOwner {
    const owner = providerMutationPublication.current();
    if (!owner || owner.#disposed) throw new Error("Provider mutation owner is not installed");
    return owner;
  }

  static materialGeneration(): bigint {
    return ProviderMutationOwner.#materialGeneration;
  }

  static subscribeMaterialGeneration(listener: () => void): () => void {
    ProviderMutationOwner.#materialListeners.add(listener);
    return () => ProviderMutationOwner.#materialListeners.delete(listener);
  }

  updateProvider(input: ProviderUpdate): Promise<ProviderConfiguration> {
    return this.#generation.updateProvider(input);
  }

  setUtilityRole(role: ProviderRole): Promise<ProviderRole> {
    return this.#generation.setUtilityRole(role);
  }

  setEmbeddingRole(role: ProviderRole): Promise<ProviderRole> {
    return this.#generation.setEmbeddingRole(role);
  }

  testProvider(provider: string): Promise<ProviderTestOutcome> {
    return this.#generation.testProvider(provider);
  }

  errorMessage(error: unknown): string | undefined {
    return this.#gateway.errorMessage(error);
  }

  replaceRuntimeGeneration(): void {
    if (this.#disposed || !providerMutationPublication.owns(this)) return;
    const predecessor = this.#generation;
    this.#generation = new ProviderMutationGeneration(this.#gateway);
    predecessor.retire();
    ProviderMutationOwner.#advanceMaterialGeneration();
  }

  dispose(): void {
    if (this.#disposed) return;
    this.#disposed = true;
    this.#generation.retire();
    if (providerMutationPublication.withdraw(this)) {
      ProviderMutationOwner.#advanceMaterialGeneration();
    }
  }

  static #advanceMaterialGeneration(): void {
    ProviderMutationOwner.#materialGeneration += 1n;
    for (const listener of ProviderMutationOwner.#materialListeners) listener();
  }
}

const providerMutationPublication = createPublicationSlot<ProviderMutationOwner>();

export function providerMutationWasRetired(error: unknown): boolean {
  return error instanceof ProviderMutationRetiredError;
}

function commitProviderSaved(saved: ProviderConfiguration): void {
  queryClient.setQueryData<ProviderConfiguration[]>([PROVIDERS_KEY], (current) =>
    current?.map((provider) => (provider.id === saved.id ? saved : provider)),
  );
}
