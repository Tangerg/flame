import { requireRuntimeEndpoint } from "@flame/runtime-contract/client/endpoint";
import {
  createFlameClient,
  createHttpTransport,
  createPreparedMutationJournal,
  createSidecarClient,
  asRunId,
  type FlameClient,
} from "@flame/runtime-contract/client";
import {
  PROTOCOL_VERSION,
  type DiscoverResponse,
  type RequestMeta,
} from "@flame/runtime-contract/wire";
import {
  type MutationCommand,
  type PreparedMutation,
  type PreparedMutationJournal,
} from "@flame/runtime-contract/client/mutationJournal";
import { CommandStorage } from "./commandStore";

export type Command = Extract<
  MutationCommand,
  { method: "sessions.create" | "runs.start" | "runs.resume" | "runs.cancel" }
>;

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
  readonly #journal: PreparedMutationJournal;
  #discovery?: DiscoverResponse;
  #executing = false;
  #pending?: PreparedMutation;
  #closed?: Promise<void>;

  private constructor(endpoint: string, token: string | undefined, directory: string) {
    this.endpoint = endpoint;
    this.signal = this.#lifetime.signal;
    this.#journal = createPreparedMutationJournal({
      storage: new CommandStorage(directory, endpoint),
      scope: () => this.#discovery?.capabilities.limits.idempotency,
    });
    this.client = createFlameClient(createHttpTransport({ baseUrl: endpoint, localToken: token }), {
      requestMeta: () => REQUEST_META,
      capabilities: () => this.#discovery?.capabilities,
      mutationJournal: {
        reserve: (method, params) => {
          const pending = this.#pending;
          if (!pending) throw new Error("IDE mutations require a prepared command");
          return this.#journal.recover(pending.idempotencyKey, method, params);
        },
        dispose: () => this.#journal.dispose(),
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

  pendingCommands(): PreparedMutation[] {
    return this.#journal.list();
  }

  async execute(command: Command): Promise<{ sessionId?: string; runId?: string }> {
    this.signal.throwIfAborted();
    if (this.#executing) throw new Error("a Runtime command is already in progress");
    if (this.#pending)
      throw new Error("retry the unresolved command before sending another command");
    return this.#executePending(this.#journal.prepare(command));
  }

  async retry(id: string): Promise<{ sessionId?: string; runId?: string }> {
    this.signal.throwIfAborted();
    if (this.#executing) throw new Error("a Runtime command is already in progress");
    const pending = this.#journal.read(id);
    if (!pending) throw new Error("the command was already settled by another client");
    return this.#executePending(pending);
  }

  async #executePending(
    pending: PreparedMutation,
  ): Promise<{ sessionId?: string; runId?: string }> {
    this.#pending = pending;
    this.#executing = true;
    try {
      return await this.#invoke(pending);
    } finally {
      this.#executing = false;
      if (!this.signal.aborted && !this.#journal.read(pending.idempotencyKey)) {
        this.#pending = undefined;
      }
    }
  }

  async #invoke(command: PreparedMutation): Promise<{ sessionId?: string; runId?: string }> {
    switch (command.method) {
      case "sessions.create": {
        const session = await this.client.sessions.create(command.params, this.signal);
        return { sessionId: session.id };
      }
      case "runs.start": {
        const accepted = await this.client.runs.start(command.params, this.signal);
        await accepted.events[Symbol.asyncIterator]().return?.();
        return { sessionId: command.params.sessionId, runId: accepted.result.runId };
      }
      case "runs.resume": {
        const accepted = await this.client.runs.resume(command.params, this.signal);
        await accepted.events[Symbol.asyncIterator]().return?.();
        return { runId: accepted.result.runId };
      }
      case "runs.cancel": {
        const result = await this.client.runs.cancel(
          asRunId(command.params.runId),
          command.params.reason,
        );
        return { runId: result.run.id };
      }
      default:
        throw new Error("IDE does not expose this prepared Runtime command");
    }
  }

  close(): Promise<void> {
    if (this.#closed) return this.#closed;
    this.#lifetime.abort();
    this.#closed = this.client.close();
    return this.#closed;
  }
}
