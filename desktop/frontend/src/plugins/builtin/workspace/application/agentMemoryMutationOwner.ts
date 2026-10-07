import { GenerationRetiredError } from "@/lib/asyncOwnership";
import { createPublicationSlot } from "@/lib/publicationSlot";
import { queryClient, repairCachedProjection } from "@/lib/queryClient";
import { RetirableTaskCohort } from "@/lib/taskQueue";
import { tupleKey } from "@/lib/tupleKey";
import type {
  AgentMemoryAddInput,
  AgentMemoryReviewDecision,
  AgentMemoryGateway,
} from "./ports/agentMemoryGateway";
import {
  WORKSPACE_AGENT_MEMORY_KEY,
  type AgentMemoryEntry,
  type AgentMemoryQuery,
} from "./workspaceQueries";

interface AgentMemoryMutation<T> {
  execute(): Promise<T>;
  commit?(result: T): void;
}

class AgentMemoryMutationGeneration {
  readonly #gateway: AgentMemoryGateway;
  readonly #retiredError = new GenerationRetiredError("agent_memory_mutation_generation");
  readonly #cohort = new RetirableTaskCohort(this.#retiredError);

  constructor(gateway: AgentMemoryGateway) {
    this.#gateway = gateway;
  }

  review(id: string, decision: AgentMemoryReviewDecision): Promise<void> {
    return this.#run(id, { execute: () => this.#gateway.review(id, decision) });
  }

  updateContent(id: string, content: string): Promise<void> {
    return this.#run(id, {
      execute: () => this.#gateway.updateContent(id, content),
      commit: commitAgentMemoryItem,
    }).then(() => undefined);
  }

  setPinned(id: string, pinned: boolean): Promise<void> {
    return this.#run(id, {
      execute: () => this.#gateway.setPinned(id, pinned),
      commit: commitAgentMemoryItem,
    }).then(() => undefined);
  }

  delete(id: string): Promise<void> {
    return this.#run(id, {
      execute: () => this.#gateway.delete(id),
      commit: () => removeAgentMemoryItem(id),
    });
  }

  add(input: AgentMemoryAddInput): Promise<AgentMemoryEntry> {
    return this.#run(tupleKey("add", input.scope, input.cwd ?? "", input.content), {
      execute: () => this.#gateway.add(input),
      commit: (saved) => commitAddedAgentMemory(input, saved),
    });
  }

  retire(): void {
    this.#cohort.retire();
  }

  #run<T>(identity: string, mutation: AgentMemoryMutation<T>): Promise<T> {
    return this.#cohort.runSerial(identity, async () => {
      this.#cohort.assertCurrent();
      const value = await this.#cohort.settle(mutation.execute());
      this.#cohort.assertCurrent();
      mutation.commit?.(value);
      await repairCachedProjection(this.#cohort, [WORKSPACE_AGENT_MEMORY_KEY]);
      this.#cohort.assertCurrent();
      return value;
    });
  }
}

export class AgentMemoryMutationOwner {
  #generation: AgentMemoryMutationGeneration;
  #disposed = false;

  private constructor(gateway: AgentMemoryGateway) {
    this.#generation = new AgentMemoryMutationGeneration(gateway);
  }

  static install(gateway: AgentMemoryGateway): AgentMemoryMutationOwner {
    const owner = new AgentMemoryMutationOwner(gateway);
    agentMemoryMutationPublication.publish(owner, (predecessor) => predecessor.dispose());
    return owner;
  }

  static current(): AgentMemoryMutationOwner {
    const owner = agentMemoryMutationPublication.current();
    if (!owner || owner.#disposed) throw new Error("Agent memory mutation owner is not installed");
    return owner;
  }

  review(id: string, decision: AgentMemoryReviewDecision): Promise<void> {
    return this.#generation.review(id, decision);
  }

  updateContent(id: string, content: string): Promise<void> {
    return this.#generation.updateContent(id, content);
  }

  setPinned(id: string, pinned: boolean): Promise<void> {
    return this.#generation.setPinned(id, pinned);
  }

  delete(id: string): Promise<void> {
    return this.#generation.delete(id);
  }

  add(input: AgentMemoryAddInput): Promise<AgentMemoryEntry> {
    return this.#generation.add(input);
  }

  replaceRuntimeGeneration(createGateway: () => AgentMemoryGateway): void {
    if (this.#disposed || !agentMemoryMutationPublication.owns(this)) return;
    const predecessor = this.#generation;
    this.#generation = new AgentMemoryMutationGeneration(createGateway());
    predecessor.retire();
  }

  dispose(): void {
    if (this.#disposed) return;
    this.#disposed = true;
    this.#generation.retire();
    agentMemoryMutationPublication.withdraw(this);
  }
}

const agentMemoryMutationPublication = createPublicationSlot<AgentMemoryMutationOwner>();

export function agentMemoryQuery(scope: AgentMemoryQuery["scope"], cwd?: string): AgentMemoryQuery {
  return scope === "user" ? { scope } : { scope, cwd };
}

function commitAgentMemoryItem(saved: AgentMemoryEntry): void {
  queryClient.setQueriesData<AgentMemoryEntry[]>(
    { queryKey: [WORKSPACE_AGENT_MEMORY_KEY] },
    (current) => {
      if (!current) return current;
      const index = current.findIndex((item) => item.id === saved.id);
      if (index < 0) return current;
      return current.map((item) => (item.id === saved.id ? saved : item));
    },
  );
}

function commitAddedAgentMemory(input: AgentMemoryAddInput, saved: AgentMemoryEntry): void {
  const query = agentMemoryQuery(input.scope, input.cwd);
  queryClient.setQueryData<AgentMemoryEntry[]>([WORKSPACE_AGENT_MEMORY_KEY, query], (current) =>
    current ? [saved, ...current] : current,
  );
}

function removeAgentMemoryItem(id: string): void {
  queryClient.setQueriesData<AgentMemoryEntry[]>(
    { queryKey: [WORKSPACE_AGENT_MEMORY_KEY] },
    (current) => current?.filter((item) => item.id !== id),
  );
}
