import { GenerationRetiredError, wasGenerationRetired } from "@/lib/asyncOwnership";
import { queryClient, repairCachedProjection } from "@/lib/queryClient";
import { RetirableTaskCohort } from "@/lib/taskQueue";
import {
  type MCPServerInput,
  type MCPServerUpdateInput,
} from "@/features/mcp/application/mcpServerInput";
import {
  MCP_SERVERS_KEY,
  MCP_TOOLS_KEY,
  MCP_EXPOSURE_KEY,
  sameMCPServer,
  userMCPServer,
  type MCPServerID,
  type MCPServerSettings,
} from "@/features/mcp/application/mcpServerQueries";
import { mcpServerLabel } from "@/lib/toolSource";
import {
  type MCPServerGateway,
  type MCPServerTestOutcome,
} from "@/features/mcp/application/ports/mcpServerGateway";
const AUTHORIZATION_ATTEMPT_POLL_MS = 500;

interface MCPServerMutation<T> {
  execute(): Promise<T>;
  commit(result: T): void;
}

type MCPServerImportOutcome = {
  completed: number;
  total: number;
} & ({ status: "completed" | "retired" } | { status: "failed"; error: unknown });

export type MCPServerCommands = Pick<
  MCPServerMutationGeneration,
  | "create"
  | "update"
  | "delete"
  | "test"
  | "importServers"
  | "retired"
  | "setEnabled"
  | "setToolExposure"
  | "reconnect"
  | "authorize"
>;

class MCPServerMutationGeneration {
  readonly #gateway: MCPServerGateway;
  readonly #lifetime = new AbortController();
  readonly #retiredError = new GenerationRetiredError("mcp_server_mutation_generation");
  readonly #cohort = new RetirableTaskCohort(this.#retiredError);

  constructor(gateway: MCPServerGateway) {
    this.#gateway = gateway;
  }

  get retired(): boolean {
    return this.#cohort.retired;
  }

  async importServers(inputs: readonly MCPServerInput[]): Promise<MCPServerImportOutcome> {
    const servers = structuredClone(inputs);
    let completed = 0;
    for (const server of servers) {
      try {
        await this.create(server);
        completed += 1;
      } catch (error) {
        return wasGenerationRetired(error)
          ? { status: "retired", completed, total: servers.length }
          : { status: "failed", error, completed, total: servers.length };
      }
    }
    return { status: "completed", completed, total: servers.length };
  }

  create(input: MCPServerInput): Promise<MCPServerSettings> {
    return this.#run(userMCPServer(input.name), {
      execute: () => this.#gateway.create(input),
      commit: commitMCPServerSaved,
    });
  }

  update(server: MCPServerID, input: MCPServerUpdateInput): Promise<MCPServerSettings> {
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
    return this.#run(server, {
      execute: () => this.#gateway.reconnect(server),
      commit: () => undefined,
    });
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
}

export type MCPServerMutationAccess = Pick<
  MCPServerMutationOwner,
  "capture" | "materialGeneration" | "subscribeMaterialGeneration"
>;

export class MCPServerMutationOwner {
  #materialGeneration = 0n;
  readonly #materialListeners = new Set<() => void>();
  #generation: MCPServerMutationGeneration | null;
  #disposed = false;

  constructor(gateway: MCPServerGateway | null) {
    this.#generation = gateway ? new MCPServerMutationGeneration(gateway) : null;
  }

  readonly capture = (): MCPServerCommands | null => this.#generation;

  readonly materialGeneration = (): bigint => this.#materialGeneration;

  readonly subscribeMaterialGeneration = (listener: () => void): (() => void) => {
    this.#materialListeners.add(listener);
    return () => this.#materialListeners.delete(listener);
  };

  replaceRuntimeGeneration(createGateway: () => MCPServerGateway): void {
    if (this.#disposed) return;
    const successor = new MCPServerMutationGeneration(createGateway());
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
