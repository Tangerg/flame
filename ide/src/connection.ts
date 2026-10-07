import { requireRuntimeEndpoint } from "@flame/runtime-contract/client/endpoint";
import { errorMessage } from "@flame/runtime-contract/client/errors";
import {
  createFlameClient,
  createHttpTransport,
  createPreparedMutationJournal,
  createSidecarClient,
  asRunId,
  isErrorType,
  RpcError,
  type FlameClient,
} from "@flame/runtime-contract/client";
import {
  PROTOCOL_VERSION,
  type DiscoverResponse,
  type RequestMeta,
  type PluginInstallation,
} from "@flame/runtime-contract/wire";
import {
  type MutationCommand,
  type PreparedMutation,
  type PreparedMutationJournal,
} from "@flame/runtime-contract/client/mutationJournal";
import type { WireMutationMethodName } from "@flame/runtime-contract/methods";
import { CommandStorage } from "./commandStore";

export type Command = Extract<
  MutationCommand,
  {
    method:
      | "sessions.create"
      | "runs.start"
      | "runs.resume"
      | "runs.cancel"
      | Extract<WireMutationMethodName, `plugins.${string}`>;
  }
>;

export interface CommandResult {
  sessionId?: string;
  pluginResult?: PluginInstallation;
}

const REQUEST_META: RequestMeta = {
  protocolVersion: PROTOCOL_VERSION,
  clientInfo: { name: "flame-ide", version: "0.1.0" },
  clientCapabilities: {
    features: { subagents: { enabled: true } },
    interruptTypes: ["approval", "question"],
    excludedEphemeralEvents: ["segment.progress", "item.delta"],
  },
};

export function commandFailureMessage(error: unknown): string {
  if (isErrorType(error, "plugin_changed"))
    return "A plugin changed while the run was being prepared. Nothing was started; send it again to retry.";
  if (error instanceof RpcError && error.data.detail) return error.data.detail;
  return errorMessage(error);
}

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

  async execute(command: Command): Promise<CommandResult> {
    this.signal.throwIfAborted();
    if (this.#executing) throw new Error("a Runtime command is already in progress");
    if (this.#pending)
      throw new Error("retry the unresolved command before sending another command");
    return this.#executePending(this.#journal.prepare(command));
  }

  async retry(id: string): Promise<CommandResult> {
    this.signal.throwIfAborted();
    if (this.#executing) throw new Error("a Runtime command is already in progress");
    const pending = this.#journal.read(id);
    if (!pending) throw new Error("the command was already settled by another client");
    return this.#executePending(pending);
  }

  async #executePending(pending: PreparedMutation): Promise<CommandResult> {
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

  async #invoke(command: PreparedMutation): Promise<CommandResult> {
    switch (command.method) {
      case "sessions.create": {
        const session = await this.client.sessions.create(command.params, this.signal);
        return { sessionId: session.id };
      }
      case "runs.start": {
        const accepted = await this.client.runs.start(command.params, this.signal);
        await accepted.events[Symbol.asyncIterator]().return?.();
        return { sessionId: command.params.sessionId };
      }
      case "runs.resume": {
        const accepted = await this.client.runs.resume(command.params, this.signal);
        await accepted.events[Symbol.asyncIterator]().return?.();
        return {};
      }
      case "runs.cancel":
        await this.client.runs.cancel(asRunId(command.params.runId), command.params.reason);
        return {};
      case "plugins.install":
        return { pluginResult: await this.client.plugins.install(command.params) };
      case "plugins.stage":
        return { pluginResult: await this.client.plugins.stage(command.params) };
      case "plugins.select":
        return { pluginResult: await this.client.plugins.select(command.params) };
      case "plugins.approve":
        return { pluginResult: await this.client.plugins.approve(command.params) };
      case "plugins.configure":
        return { pluginResult: await this.client.plugins.configure(command.params) };
      case "plugins.setEnablement":
        return { pluginResult: await this.client.plugins.setEnablement(command.params) };
      case "plugins.revoke":
        return { pluginResult: await this.client.plugins.revoke(command.params.installationId) };
      case "plugins.uninstall":
        await this.client.plugins.uninstall(command.params.installationId);
        return {};
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
