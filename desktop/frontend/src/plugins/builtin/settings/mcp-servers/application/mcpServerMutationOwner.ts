import { GenerationRetiredError } from "@/lib/asyncOwnership";
import { createPublicationSlot } from "@/lib/publicationSlot";
import { queryClient, repairCachedProjection } from "@/lib/queryClient";
import { RetirableTaskCohort } from "@/lib/taskQueue";
import type { MCPServerInput } from "./mcpServerInput";
import {
  MCP_SERVERS_KEY,
  MCP_TOOLS_KEY,
  MCP_EXPOSURE_KEY,
  sameMCPServer,
  userMCPServer,
  type MCPServerID,
  type MCPServerSettings,
} from "./mcpServerQueries";
import { mcpServerLabel } from "@/lib/toolSource";
import type { MCPServerGateway, MCPServerTestOutcome } from "./ports/mcpServerGateway";

const AUTHORIZATION_ATTEMPT_POLL_MS = 500;

interface MCPServerMutation<T> {
  execute(): Promise<T>;
  commit(result: T): void;
}

class MCPServerMutationGeneration {
  readonly #gateway: MCPServerGateway;
  readonly #lifetime = new AbortController();
  readonly #retiredError = new GenerationRetiredError("mcp_server_mutation_generation");
  readonly #cohort = new RetirableTaskCohort(this.#retiredError);
  readonly #reconnects = new Map<string, Promise<void>>();

  constructor(gateway: MCPServerGateway) {
    this.#gateway = gateway;
  }

  create(input: MCPServerInput): Promise<MCPServerSettings> {
    return this.#run(userMCPServer(input.name), {
      execute: () => this.#gateway.create(input),
      commit: commitMCPServerSaved,
    });
  }

  update(server: MCPServerID, input: MCPServerInput): Promise<MCPServerSettings> {
    return this.#run(server, {
      execute: () => this.#gateway.update(server, input),
      commit: commitMCPServerSaved,
    });
  }

  setEnabled(server: MCPServerID, enabled: boolean): Promise<MCPServerSettings> {
    return this.#run(server, {
      execute: () => this.#gateway.setEnabled(server, enabled),
      commit: commitMCPServerSaved,
    });
  }

  delete(server: MCPServerID): Promise<void> {
    return this.#run(server, {
      execute: () => this.#gateway.delete(server),
      commit: () => removeMCPServer(server),
    });
  }

  setToolExposure(server: MCPServerID, name: string, disabled: boolean): Promise<void> {
    return this.#run(server, {
      execute: () => this.#gateway.setToolExposure(server, name, disabled),
      commit: () => undefined,
    });
  }

  reconnect(server: MCPServerID): Promise<void> {
    const key = mcpServerLabel(server);
    const admitted = this.#reconnects.get(key);
    if (admitted) return admitted;

    const reconnect = this.#run(server, {
      execute: () => this.#gateway.reconnect(server),
      commit: () => undefined,
    });
    this.#reconnects.set(key, reconnect);
    void reconnect.then(
      () => this.#forgetReconnect(key, reconnect),
      () => this.#forgetReconnect(key, reconnect),
    );
    return reconnect;
  }

  test(input: MCPServerInput): Promise<MCPServerTestOutcome> {
    return this.#cohort.run(() => this.#gateway.test(input));
  }

  async authorize(server: MCPServerID, callerSignal?: AbortSignal): Promise<void> {
    const signal = callerSignal
      ? AbortSignal.any([callerSignal, this.#lifetime.signal])
      : this.#lifetime.signal;
    let attempt = await this.#cohort.run(() =>
      this.#gateway.createAuthorizationAttempt(server, signal),
    );
    while (attempt.status === "pending") {
      await this.#cohort.settle(authorizationPollDelay(signal));
      attempt = await this.#cohort.run(() =>
        this.#gateway.getAuthorizationAttempt(attempt.id, signal),
      );
    }
    await repairCachedProjection(this.#cohort, [MCP_SERVERS_KEY, MCP_TOOLS_KEY, MCP_EXPOSURE_KEY]);
    this.#cohort.assertCurrent();
    if (attempt.status === "failed") throw new Error(attempt.error);
  }

  retire(): void {
    if (this.#cohort.retired) return;
    this.#lifetime.abort(this.#retiredError);
    this.#cohort.retire();
    this.#reconnects.clear();
  }

  #run<T>(server: MCPServerID, mutation: MCPServerMutation<T>): Promise<T> {
    return this.#cohort.runSerial(mcpServerLabel(server), async () => {
      const value = await this.#cohort.run(mutation.execute);
      mutation.commit(value);
      await repairCachedProjection(this.#cohort, [
        MCP_SERVERS_KEY,
        MCP_TOOLS_KEY,
        MCP_EXPOSURE_KEY,
      ]);
      this.#cohort.assertCurrent();
      return value;
    });
  }

  #forgetReconnect(key: string, reconnect: Promise<void>): void {
    if (this.#reconnects.get(key) === reconnect) this.#reconnects.delete(key);
  }
}

export class MCPServerMutationOwner {
  static #materialGeneration = 0n;
  static readonly #materialListeners = new Set<() => void>();

  #generation: MCPServerMutationGeneration;
  #disposed = false;

  private constructor(gateway: MCPServerGateway) {
    this.#generation = new MCPServerMutationGeneration(gateway);
  }

  static install(gateway: MCPServerGateway): MCPServerMutationOwner {
    const owner = new MCPServerMutationOwner(gateway);
    mcpServerMutationPublication.publish(owner, (predecessor) => predecessor.dispose());
    MCPServerMutationOwner.#advanceMaterialGeneration();
    return owner;
  }

  static current(): MCPServerMutationOwner {
    const owner = mcpServerMutationPublication.current();
    if (!owner || owner.#disposed) throw new Error("MCP server mutation owner is not installed");
    return owner;
  }

  static materialGeneration(): bigint {
    return MCPServerMutationOwner.#materialGeneration;
  }

  static subscribeMaterialGeneration(listener: () => void): () => void {
    MCPServerMutationOwner.#materialListeners.add(listener);
    return () => MCPServerMutationOwner.#materialListeners.delete(listener);
  }

  create(input: MCPServerInput): Promise<MCPServerSettings> {
    return this.#generation.create(input);
  }

  update(server: MCPServerID, input: MCPServerInput): Promise<MCPServerSettings> {
    return this.#generation.update(server, input);
  }

  setEnabled(server: MCPServerID, enabled: boolean): Promise<MCPServerSettings> {
    return this.#generation.setEnabled(server, enabled);
  }

  delete(server: MCPServerID): Promise<void> {
    return this.#generation.delete(server);
  }

  setToolExposure(server: MCPServerID, name: string, disabled: boolean): Promise<void> {
    return this.#generation.setToolExposure(server, name, disabled);
  }

  reconnect(server: MCPServerID): Promise<void> {
    return this.#generation.reconnect(server);
  }

  test(input: MCPServerInput): Promise<MCPServerTestOutcome> {
    return this.#generation.test(input);
  }

  authorize(server: MCPServerID, signal?: AbortSignal): Promise<void> {
    return this.#generation.authorize(server, signal);
  }

  replaceRuntimeGeneration(createGateway: () => MCPServerGateway): void {
    if (this.#disposed || !mcpServerMutationPublication.owns(this)) return;
    const predecessor = this.#generation;
    this.#generation = new MCPServerMutationGeneration(createGateway());
    predecessor.retire();
    MCPServerMutationOwner.#advanceMaterialGeneration();
  }

  dispose(): void {
    if (this.#disposed) return;
    this.#disposed = true;
    this.#generation.retire();
    if (mcpServerMutationPublication.withdraw(this)) {
      MCPServerMutationOwner.#advanceMaterialGeneration();
    }
  }

  static #advanceMaterialGeneration(): void {
    MCPServerMutationOwner.#materialGeneration += 1n;
    for (const listener of MCPServerMutationOwner.#materialListeners) listener();
  }
}

const mcpServerMutationPublication = createPublicationSlot<MCPServerMutationOwner>();

function commitMCPServerSaved(saved: MCPServerSettings): void {
  queryClient.setQueryData<MCPServerSettings[]>([MCP_SERVERS_KEY], (current) => {
    if (!current) return current;
    const index = current.findIndex((server) => sameMCPServer(server.id, saved.id));
    if (index < 0) return [...current, saved];
    return current.map((server) => (sameMCPServer(server.id, saved.id) ? saved : server));
  });
}

function removeMCPServer(removed: MCPServerID): void {
  queryClient.setQueryData<MCPServerSettings[]>([MCP_SERVERS_KEY], (current) =>
    current?.filter((server) => !sameMCPServer(server.id, removed)),
  );
}

function authorizationPollDelay(signal: AbortSignal): Promise<void> {
  if (signal.aborted) return Promise.reject(signal.reason);
  return new Promise((resolve, reject) => {
    const timer = setTimeout(done, AUTHORIZATION_ATTEMPT_POLL_MS);
    function done(): void {
      signal.removeEventListener("abort", aborted);
      resolve();
    }
    function aborted(): void {
      clearTimeout(timer);
      reject(signal.reason);
    }
    signal.addEventListener("abort", aborted, { once: true });
  });
}
