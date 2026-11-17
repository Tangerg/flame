import type {
  ComponentContext,
  InteractionSubmission,
} from "@flame/component-sdk";
import {
  isErrorType,
  mutationSettlementIsUnknown,
  OperationFailure,
  pluginMethod,
  type PluginCommandRecovery,
} from "@flame/runtime-client";
import { parseJSON, stringifyJSON } from "@frame/sdk/json";
import type { CallPluginRequest } from "@flame/runtime-client/wire";
import type { ActionPayload } from "@a2ui/web_core";
import { Presentation, limits } from "../contracts/document";

export interface InteractionState {
  writeOperation(id: string, value: unknown): void;
  resetDraft(surfaceId: string): void;
}

type Binding = Omit<CallPluginRequest, "input">;
type Review = {
  binding: Binding;
  input: string;
  decide(approved: boolean): void;
};
type Recovery = { label: string; key?: string } & (
  { recover(): Promise<void> } | { error: unknown }
);

export type InteractionView = Readonly<
  | { status: "idle" }
  | { status: "running"; label: string }
  | { status: "review"; review: Review }
  | { status: "unknown"; recoveries: readonly Recovery[] }
  | { status: "failed"; error: unknown }
  | { status: "complete" }
>;

export interface InteractionSnapshot {
  readonly action: InteractionView;
}

class Unconfirmed extends Error {
  constructor(
    readonly recovery: Recovery,
    cause?: unknown,
  ) {
    super("genui: recover the original action before continuing", { cause });
  }
}

class CancelledReview extends Error {}

export class InteractionController {
  readonly #context: ComponentContext;
  readonly #presentation: Presentation;
  readonly #state: InteractionState;
  readonly #listeners = new Set<() => void>();
  #snapshot: InteractionSnapshot = {
    action: { status: "idle" },
  };
  #initialized = false;

  constructor(
    context: ComponentContext,
    presentation: Presentation,
    state: InteractionState,
  ) {
    this.#context = context;
    this.#presentation = presentation;
    this.#state = state;
  }

  readonly snapshot = (): InteractionSnapshot => this.#snapshot;
  readonly subscribe = (listener: () => void): (() => void) => {
    this.#listeners.add(listener);
    return () => {
      this.#listeners.delete(listener);
    };
  };

  get blocked(): boolean {
    return (
      this.#snapshot.action.status === "running" ||
      this.#snapshot.action.status === "review" ||
      this.#snapshot.action.status === "unknown"
    );
  }

  #publish(view: InteractionView): void {
    this.#commit({ ...this.#snapshot, action: view });
  }

  #commit(snapshot: InteractionSnapshot): void {
    if (this.#context.signal.aborted) return;
    this.#snapshot = snapshot;
    for (const listener of this.#listeners) listener();
  }

  async initialize(): Promise<void> {
    if (this.#initialized) return;
    this.#initialized = true;
    await this.#guard(
      "initialize",
      async () => {
        if (await this.#pending()) return;
      },
      false,
    );
  }

  async execute(action: ActionPayload): Promise<void> {
    if (this.blocked) return;
    const captured = structuredClone(action);
    await this.#guard(captured.name, async () => {
      this.#presentation.action(captured);
      if (captured.name === "resetDraft") {
        this.#state.resetDraft(captured.surfaceId);
      } else if (captured.name === "followup") {
        if (
          !this.#context.interaction ||
          this.#context.interaction.source.runId !==
            this.#context.toolResult?.runId ||
          this.#context.interaction.source.itemId !==
            this.#context.toolResult?.id
        )
          throw new Error(
            "genui: original conversation interaction is unavailable",
          );
        const { message, payload } = captured.context;
        if (
          typeof message !== "string" ||
          !message.trim() ||
          payload === undefined
        )
          throw new Error(
            "genui: followup requires explicit message and payload",
          );
        const payloadJSON = stringifyJSON(payload);
        if (
          new TextEncoder().encode(message + payloadJSON).byteLength >
          limits.followupBytes
        )
          throw new Error("genui: followup exceeds model context capacity");
        await this.#followup(
          await this.#context.interaction.submit({
            action: message,
            payloadJSON,
          }),
          captured.name,
        );
      } else {
        await this.#operation(captured.name, captured.context);
      }
    });
  }

  async #guard(
    label: string,
    run: () => Promise<void>,
    completed = true,
  ): Promise<void> {
    this.#publish({ status: "running", label });
    try {
      this.#context.signal.throwIfAborted();
      await run();
      if (this.#snapshot.action.status === "running")
        this.#publish({ status: completed ? "complete" : "idle" });
    } catch (error) {
      if (this.#context.signal.aborted) return;
      if (error instanceof Unconfirmed)
        this.#publish({ status: "unknown", recoveries: [error.recovery] });
      else if (error instanceof CancelledReview)
        this.#publish({ status: "idle" });
      else this.#publish({ status: "failed", error });
    }
  }

  async recover(recovery: Recovery): Promise<void> {
    if (
      this.#snapshot.action.status !== "unknown" ||
      !this.#snapshot.action.recoveries.includes(recovery) ||
      !("recover" in recovery)
    )
      return;
    await this.#guard(recovery.label, async () => {
      await recovery.recover();
      await this.#pending();
    });
  }

  async #followup(result: InteractionSubmission, label: string): Promise<void> {
    this.#context.signal.throwIfAborted();
    if (result.status === "unknown") {
      const recovery: Recovery = {
        label,
        key: result.idempotencyKey,
        recover: async () => {
          let settled: InteractionSubmission;
          try {
            settled = await result.recover();
          } catch (cause) {
            throw new Unconfirmed(recovery, cause);
          }
          await this.#followup(settled, label);
        },
      };
      throw new Unconfirmed(recovery);
    }
    if (result.status === "retired")
      throw new Error("genui: the original conversation input was retired");
  }

  async #intent(actionId: string, operationId: string): Promise<string> {
    const tool = this.#context.toolResult;
    if (!tool)
      throw new Error("genui: an operation requires its captured tool source");
    const identity = stringifyJSON([
      "flame.genui",
      tool.runId,
      tool.id,
      actionId,
      operationId,
    ]);
    const digest = await crypto.subtle.digest(
      "SHA-256",
      new TextEncoder().encode(identity),
    );
    return (
      "genui:" +
      Array.from(new Uint8Array(digest), (byte) =>
        byte.toString(16).padStart(2, "0"),
      ).join("")
    );
  }

  async #pending(): Promise<boolean> {
    this.#context.signal.throwIfAborted();
    const operations = this.#presentation
      .operations()
      .filter(({ binding }) => pluginMethod(binding).kind === "command");
    if (!operations.length) return false;
    const intents = await Promise.all(
      operations.map(({ request }) =>
        this.#intent(request.id, request.id).then((intent) => ({
          intent,
          operationId: request.id,
        })),
      ),
    );
    this.#context.signal.throwIfAborted();
    const recoveries = this.#context.runtime.plugins
      .pendingCommands()
      .flatMap((command) => {
        const captured = intents.find(({ intent }) =>
          command.matchesIntent(intent),
        );
        if (!captured) return [];
        this.#state.writeOperation(captured.operationId, {
          status: "unknown",
          idempotencyKey: command.idempotencyKey,
        });
        return [
          {
            label: captured.operationId,
            key: command.idempotencyKey,
            recover: () => this.#recoverCommand(captured.operationId, command),
          },
        ];
      });
    if (!recoveries.length) return false;
    this.#publish({ status: "unknown", recoveries });
    return true;
  }

  async #recoverCommand(
    id: string,
    command: PluginCommandRecovery,
  ): Promise<void> {
    this.#context.signal.throwIfAborted();
    try {
      const data = await this.#context.track(() =>
        command.recover((value) => value, this.#context.signal),
      );
      this.#context.signal.throwIfAborted();
      this.#state.writeOperation(id, { status: "success", data });
    } catch (cause) {
      if (this.#context.signal.aborted) throw cause;
      if (
        !(cause instanceof OperationFailure) &&
        !isErrorType(cause, "plugin_command_retired")
      )
        throw new Unconfirmed(
          {
            label: id,
            key: command.idempotencyKey,
            recover: () => this.#recoverCommand(id, command),
          },
          cause,
        );
      this.#state.writeOperation(id, {
        status: "error",
        error: cause instanceof Error ? cause.message : String(cause),
      });
      throw cause;
    }
  }

  #retainCommand(id: string, intent: string): Recovery {
    try {
      const original = this.#context.runtime.plugins
        .pendingCommands()
        .find((command) => command.matchesIntent(intent));
      if (!original)
        throw new Error(
          "genui: the original command handle is unavailable; inspect the existing command recovery view",
        );
      return {
        label: id,
        key: original.idempotencyKey,
        recover: () => this.#recoverCommand(id, original),
      };
    } catch (error) {
      return { label: id, error };
    }
  }

  async #review(binding: Binding, input: string): Promise<void> {
    const approved = await new Promise<boolean>((resolve) => {
      const abort = () => resolve(false);
      this.#context.signal.addEventListener("abort", abort, { once: true });
      this.#publish({
        status: "review",
        review: {
          binding,
          input,
          decide: (value) => {
            this.#context.signal.removeEventListener("abort", abort);
            resolve(value);
          },
        },
      });
    });
    this.#context.signal.throwIfAborted();
    if (!approved) throw new CancelledReview();
    this.#publish({ status: "running", label: binding.method });
  }

  async #operation(
    id: string,
    inputValue: Record<string, unknown>,
  ): Promise<void> {
    this.#context.signal.throwIfAborted();
    const { binding } = this.#presentation.operation(id);
    const method = pluginMethod(binding);
    const input = stringifyJSON(inputValue);
    if (method.kind === "command") await this.#review(binding, input);
    const consume = (chunk: unknown) => {
      this.#context.signal.throwIfAborted();
      this.#state.writeOperation(id, { status: "loading", chunk });
    };
    const { contract, method: name, ...route } = binding;
    const intent = method.kind === "command" ? await this.#intent(id, id) : "";
    this.#context.signal.throwIfAborted();
    this.#state.writeOperation(id, { status: "loading" });
    try {
      let data: unknown;
      if (method.kind === "command") {
        const command = { contract, method: name, input };
        data = await this.#context.track(() =>
          method.streaming
            ? this.#context.runtime.plugins.stream(
                intent,
                command,
                async () => route,
                consume,
                (value) => value,
                this.#context.signal,
              )
            : this.#context.runtime.plugins.command(
                intent,
                command,
                async () => route,
                (value) => value,
                this.#context.signal,
              ),
        );
      } else if (method.streaming) {
        data = await this.#context.track(() =>
          this.#context.runtime.plugins.queryStream(
            { ...binding, input },
            consume,
            (value) => value,
            this.#context.signal,
          ),
        );
      } else {
        const reply = await this.#context.track(() =>
          this.#context.runtime.plugins.call(
            { ...binding, input },
            { signal: this.#context.signal },
          ),
        );
        if (reply.failure)
          throw new OperationFailure(reply.failure.code, reply.failure.message);
        if (reply.output === undefined)
          throw new Error("genui: query omitted its output");
        data = parseJSON(reply.output);
      }
      this.#context.signal.throwIfAborted();
      this.#state.writeOperation(id, { status: "success", data });
    } catch (cause) {
      if (this.#context.signal.aborted) throw cause;
      if (method.kind === "command" && mutationSettlementIsUnknown(cause)) {
        this.#state.writeOperation(id, { status: "unknown" });
        // Retain the actual original handle. Re-entering command() after
        // another view settles the journal entry could create a new command.
        throw new Unconfirmed(this.#retainCommand(id, intent), cause);
      }
      this.#state.writeOperation(id, {
        status: "error",
        error: cause instanceof Error ? cause.message : String(cause),
      });
      throw cause;
    }
  }
}
