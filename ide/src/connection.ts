import { requireRuntimeEndpoint } from "@flame/runtime-contract/client/endpoint";
import {
  createFlameClient,
  createHttpTransport,
  createMutationJournal,
  createSidecarClient,
  asRunId,
  type FlameClient,
} from "@flame/runtime-contract/client";
import { mutationSettlementIsUnknown } from "@flame/runtime-contract/client/mutation";
import {
  PROTOCOL_VERSION,
  type DiscoverResponse,
  type RequestMeta,
} from "@flame/runtime-contract/wire";
import {
  MutationJournalStorageError,
  MutationJournalOwnershipError,
  MutationJournalScopeUnavailableError,
  type MutationJournal,
} from "@flame/runtime-contract/client/mutationJournal";
import { authorizeCommand, CommandStore, type Command, type PendingCommand } from "./commandStore";

const REQUEST_META: RequestMeta = {
  protocolVersion: PROTOCOL_VERSION,
  clientInfo: { name: "flame-ide", version: "0.1.0" },
  clientCapabilities: {
    features: { subagents: { enabled: true } },
    interruptTypes: ["approval", "question"],
    excludedEphemeralEvents: ["segment.progress", "item.delta"],
  },
};

export class Connection {
  readonly client: FlameClient;
  readonly endpoint: string;
  readonly signal: AbortSignal;
  readonly #lifetime = new AbortController();
  readonly #commands: CommandStore;
  #discovery?: DiscoverResponse;
  #executing = false;
  #pending?: PendingCommand;
  #journal?: MutationJournal;
  #closed?: Promise<void>;

  private constructor(endpoint: string, token: string | undefined, directory: string) {
    this.endpoint = endpoint;
    this.signal = this.#lifetime.signal;
    this.#commands = new CommandStore(directory, endpoint);
    this.client = createFlameClient(createHttpTransport({ baseUrl: endpoint, localToken: token }), {
      requestMeta: () => REQUEST_META,
      capabilities: () => this.#discovery?.capabilities,
      mutationJournal: {
        reserve: (method, params) => {
          const pending = this.#pending;
          if (!pending) throw new Error("IDE mutations require a prepared command");
          authorizeCommand(pending, this.discovery.capabilities.limits.idempotency.namespace);
          const reservation = this.#journal?.reserve(method, params, pending.id);
          if (!reservation) throw new Error("Runtime omitted the command replay scope");
          return {
            ...reservation,
            authorizeAttempt: () => {
              authorizeCommand(pending, this.discovery.capabilities.limits.idempotency.namespace);
              return reservation.authorizeAttempt();
            },
          };
        },
        dispose: () => this.#journal?.dispose(),
      },
    });
  }

  static async open(
    endpoint: string,
    token: string | undefined,
    directory: string,
    signal?: AbortSignal,
  ): Promise<Connection> {
    const connection = new Connection(requireRuntimeEndpoint(endpoint), token, directory);
    const opening = signal ? AbortSignal.any([signal, connection.signal]) : connection.signal;
    try {
      const info = await createSidecarClient({ baseUrl: connection.endpoint }).info(opening);
      if (info.protocolVersion !== PROTOCOL_VERSION)
        throw new Error(
          `Runtime protocol ${info.protocolVersion} does not match ${PROTOCOL_VERSION}`,
        );
      const discovery = await connection.client.runtime.discover(opening);
      if (
        discovery.protocolVersion !== PROTOCOL_VERSION ||
        discovery.serverInfo.instanceId !== info.server.instanceId
      ) {
        throw new Error("Runtime changed while connecting; connect again");
      }
      connection.#discovery = discovery;
      return connection;
    } catch (error) {
      await connection.close();
      throw error;
    }
  }

  get discovery(): DiscoverResponse {
    if (!this.#discovery) throw new Error("Runtime is not connected");
    return this.#discovery;
  }

  pendingCommands(): PendingCommand[] {
    return this.#commands.list();
  }

  async execute(command: Command): Promise<{ sessionId?: string; runId?: string }> {
    this.signal.throwIfAborted();
    if (this.#executing) throw new Error("a Runtime command is already in progress");
    if (this.#pending)
      throw new Error("retry the unresolved command before sending another command");
    const { namespace, retentionSeconds } = this.discovery.capabilities.limits.idempotency;
    return this.#executePending(this.#commands.prepare(namespace, retentionSeconds, command));
  }

  async retry(id: string): Promise<{ sessionId?: string; runId?: string }> {
    this.signal.throwIfAborted();
    if (this.#executing) throw new Error("a Runtime command is already in progress");
    const pending = this.#commands.read(id);
    if (!pending) throw new Error("the command was already settled by another client");
    return this.#executePending(pending);
  }

  async #executePending(pending: PendingCommand): Promise<{ sessionId?: string; runId?: string }> {
    authorizeCommand(pending, this.discovery.capabilities.limits.idempotency.namespace);
    this.#journal?.dispose();
    const entries = new Map<string, unknown>();
    this.#journal = createMutationJournal({
      storage: {
        get: (key) => structuredClone(entries.get(key)),
        set: (key, value) => {
          entries.set(key, structuredClone(value));
        },
        remove: (key) => {
          entries.delete(key);
        },
        keys: () => [...entries.keys()],
      },
      scope: () => this.#discovery?.capabilities.limits.idempotency,
    });
    this.#pending = pending;
    this.#executing = true;
    try {
      let result;
      try {
        result = await this.#invoke(pending.command);
      } catch (error) {
        if (
          !this.signal.aborted &&
          !mutationSettlementIsUnknown(error) &&
          !(error instanceof MutationJournalStorageError) &&
          !(error instanceof MutationJournalOwnershipError) &&
          !(error instanceof MutationJournalScopeUnavailableError)
        ) {
          this.#commands.settle(pending.id);
          this.#pending = undefined;
        }
        throw error;
      }
      this.signal.throwIfAborted();
      this.#commands.settle(pending.id);
      this.#pending = undefined;
      return result;
    } finally {
      this.#executing = false;
    }
  }

  async #invoke(command: Command): Promise<{ sessionId?: string; runId?: string }> {
    switch (command.kind) {
      case "createSession": {
        const session = await this.client.sessions.create(command.params, this.signal);
        return { sessionId: session.id };
      }
      case "start": {
        const accepted = await this.client.runs.start(command.params, this.signal);
        await accepted.events[Symbol.asyncIterator]().return?.();
        return { sessionId: command.params.sessionId, runId: accepted.result.runId };
      }
      case "resume": {
        const accepted = await this.client.runs.resume(command.params, this.signal);
        await accepted.events[Symbol.asyncIterator]().return?.();
        return { runId: accepted.result.runId };
      }
      case "cancel": {
        const result = await this.client.runs.cancel(
          asRunId(command.params.runId),
          command.params.reason,
        );
        return { runId: result.run.id };
      }
    }
  }

  close(): Promise<void> {
    if (this.#closed) return this.#closed;
    this.#lifetime.abort();
    this.#closed = this.client.close();
    return this.#closed;
  }
}
