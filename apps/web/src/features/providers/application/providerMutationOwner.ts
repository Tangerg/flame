import { GenerationRetiredError } from "@/lib/asyncOwnership";
import { queryClient, repairCachedProjection } from "@/lib/queryClient";
import { RetirableTaskCohort } from "@/lib/taskQueue";
import { tupleKey } from "@/lib/tupleKey";
import {
  type ProviderGateway,
  type ProviderTestOutcome,
  type ProviderUpdate,
} from "@/features/providers/application/ports/providerGateway";
import {
  type ProviderConfiguration,
  type ProviderRole,
} from "@/features/providers/application/providerModels";
import {
  EMBEDDING_ROLE_KEY,
  MODELS_KEY,
  PROVIDERS_KEY,
  UTILITY_ROLE_KEY,
} from "@/features/providers/application/providerQueries";
interface ProviderMutation<T> {
  execute(): Promise<T>;
  commit(result: T): void;
  repair: readonly string[];
}

class ProviderMutationGeneration {
  readonly #gateway: ProviderGateway;
  readonly #retiredError = new GenerationRetiredError("provider_mutation_generation");
  readonly #cohort = new RetirableTaskCohort(this.#retiredError);

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
    return this.#cohort.run(() => this.#gateway.testProvider(provider));
  }

  errorMessage(error: unknown): string | undefined {
    return this.#gateway.errorMessage(error);
  }

  retire(): void {
    this.#cohort.retire();
  }

  #run<T>(identity: string, mutation: ProviderMutation<T>): Promise<T> {
    return this.#cohort.runSerial(identity, async () => {
      const value = await this.#cohort.run(mutation.execute);
      mutation.commit(value);
      await repairCachedProjection(this.#cohort, mutation.repair);
      this.#cohort.assertCurrent();
      return value;
    });
  }
}

export type ProviderCommands = Pick<
  ProviderMutationGeneration,
  "updateProvider" | "setUtilityRole" | "setEmbeddingRole" | "testProvider" | "errorMessage"
>;

export type ProviderMutationAccess = Pick<
  ProviderMutationOwner,
  "capture" | "materialGeneration" | "assertMaterialGeneration" | "subscribeMaterialGeneration"
>;

export class ProviderMutationOwner {
  #materialGeneration = 0n;
  readonly #materialListeners = new Set<() => void>();
  #generation: ProviderMutationGeneration | null;
  #disposed = false;

  constructor(gateway: ProviderGateway | null) {
    this.#generation = gateway ? new ProviderMutationGeneration(gateway) : null;
  }

  readonly capture = (): ProviderCommands | null => this.#generation;

  readonly materialGeneration = (): bigint => this.#materialGeneration;

  readonly assertMaterialGeneration = (generation: bigint): void => {
    if (this.#disposed || generation !== this.#materialGeneration) {
      throw new GenerationRetiredError("provider_mutation_generation");
    }
  };

  readonly subscribeMaterialGeneration = (listener: () => void): (() => void) => {
    this.#materialListeners.add(listener);
    return () => this.#materialListeners.delete(listener);
  };

  replaceRuntimeGeneration(createGateway: () => ProviderGateway): void {
    if (this.#disposed) return;
    const successor = new ProviderMutationGeneration(createGateway());
    const predecessor = this.#generation;
    this.#generation = successor;
    predecessor?.retire();
    this.#advanceMaterialGeneration();
  }

  retireRuntimeGeneration(): void {
    if (!this.#generation) return;
    const predecessor = this.#generation;
    this.#generation = null;
    predecessor.retire();
    this.#advanceMaterialGeneration();
  }

  dispose(): void {
    if (this.#disposed) return;
    this.#disposed = true;
    this.#generation?.retire();
    this.#generation = null;
    this.#advanceMaterialGeneration();
    this.#materialListeners.clear();
  }

  #advanceMaterialGeneration(): void {
    this.#materialGeneration += 1n;
    for (const listener of this.#materialListeners) listener();
  }
}

function commitProviderSaved(saved: ProviderConfiguration): void {
  queryClient.setQueryData<ProviderConfiguration[]>([PROVIDERS_KEY], (current) =>
    current?.map((provider) => (provider.id === saved.id ? saved : provider)),
  );
}
