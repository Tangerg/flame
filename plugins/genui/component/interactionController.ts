import type {
  ComponentContext,
  InteractionSubmission,
} from "@flame/component-sdk";
import {
  isErrorType,
  mutationSettlementIsUnknown,
  OperationFailure,
  parseJSON,
  pluginMethod,
  stringifyJSON,
  type PluginCommandRecovery,
} from "@flame/runtime-client";
import type { CallPluginRequest } from "@flame/runtime-client/wire";
import { Presentation, type Action } from "../contracts/document";

export interface InteractionState {
  readState(id: string): unknown;
  writeState(id: string, value: unknown): void;
  readOperation(id: string): unknown;
  writeOperation(id: string, value: unknown): void;
}

export interface InteractionEffects {
  copy(text: string): Promise<void>;
  open(url: string): void;
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

export type InteractionView =
  | { status: "idle" }
  | { status: "running"; label: string }
  | { status: "review"; review: Review }
  | { status: "unknown"; recoveries: readonly Recovery[] }
  | { status: "failed"; error: unknown }
  | { status: "complete" };

class Unconfirmed extends Error {
  constructor(
    readonly recovery: Recovery,
    cause?: unknown,
  ) {
    super("genui: recover the original action before continuing", { cause });
  }

  followedBy(continuation: () => Promise<void>): Unconfirmed {
    const original = this.recovery;
    if (!("recover" in original)) return this;
    return new Unconfirmed(
      {
        ...this.recovery,
        recover: async () => {
          try {
            await original.recover();
          } catch (cause) {
            if (cause instanceof Unconfirmed)
              throw cause.followedBy(continuation);
            throw cause;
          }
          await continuation();
        },
      },
      this,
    );
  }
}

class CancelledReview extends Error {}

export class InteractionController {
  readonly #context: ComponentContext;
  readonly #presentation: Presentation;
  readonly #state: InteractionState;
  readonly #effects: InteractionEffects;
  readonly #listeners = new Set<() => void>();
  #view: InteractionView = { status: "idle" };
  #initialized = false;

  constructor(
    context: ComponentContext,
    presentation: Presentation,
    state: InteractionState,
    effects: InteractionEffects,
  ) {
    this.#context = context;
    this.#presentation = presentation;
    this.#state = state;
    this.#effects = effects;
  }

  readonly snapshot = (): InteractionView => this.#view;
  readonly subscribe = (listener: () => void): (() => void) => {
    this.#listeners.add(listener);
    return () => {
      this.#listeners.delete(listener);
    };
  };

  get blocked(): boolean {
    return (
      this.#view.status === "running" ||
      this.#view.status === "review" ||
      this.#view.status === "unknown"
    );
  }

  #publish(view: InteractionView): void {
    if (this.#context.signal.aborted) return;
    this.#view = view;
    for (const listener of this.#listeners) listener();
  }

  async initialize(): Promise<void> {
    if (this.#initialized) return;
    this.#initialized = true;
    await this.#guard(
      "initialize",
      async () => {
        if (await this.#pending()) return;
        for (const { request } of this.#presentation.operations()) {
          if (request.auto)
            await this.#operation(request.id, "automatic-query");
        }
      },
      false,
    );
  }

  async execute(actionId: string): Promise<void> {
    if (this.blocked) return;
    await this.#guard(actionId, async () => {
      const action = this.#presentation
        .snapshot()
        .actions.find((value) => value.id === actionId);
      if (!action) throw new Error("genui: action is not declared");
      if (
        this.#presentation.validateAction(
          actionId,
          this.#state.readState,
          this.#state.readOperation,
        ).length
      )
        throw new Error("genui: correct invalid form values before submitting");
      await this.#steps(action, 0);
    });
  }

  reset(fields?: string[]): void {
    if (this.blocked || this.#context.signal.aborted) return;
    this.#reset(fields);
    this.#publish({ status: "idle" });
  }

  update(id: string, value: unknown): void {
    if (this.#context.signal.aborted) return;
    try {
      this.#state.writeState(id, this.#presentation.validateState(id, value));
    } catch (error) {
      this.#publish({ status: "failed", error });
    }
  }

  #reset(fields?: string[]): void {
    const initial = this.#presentation.snapshot().state;
    if (fields?.some((id) => !initial.some((field) => field.id === id))) {
      throw new Error("genui: reset refers to undeclared local state");
    }
    for (const field of initial) {
      if (fields === undefined || fields.includes(field.id))
        this.#state.writeState(field.id, field.value);
    }
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
      if (this.#view.status === "running")
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
      this.#view.status !== "unknown" ||
      !this.#view.recoveries.includes(recovery) ||
      !("recover" in recovery)
    )
      return;
    await this.#guard(recovery.label, async () => {
      await recovery.recover();
      await this.#pending();
    });
  }

  async #steps(action: Action, index: number): Promise<void> {
    for (let current = index; current < action.steps.length; current += 1) {
      this.#context.signal.throwIfAborted();
      const step = action.steps[current]!;
      try {
        switch (step.type) {
          case "set":
            this.#state.writeState(
              step.state,
              this.#presentation.validateState(
                step.state,
                this.#evaluate(step.value),
              ),
            );
            break;
          case "reset":
            this.#reset(step.states);
            break;
          case "operation":
            await this.#operation(step.operation, action.id);
            break;
          case "followup": {
            if (!this.#context.interaction)
              throw new Error("genui: conversation interaction is unavailable");
            const message = this.#evaluate(step.message);
            if (typeof message !== "string" || !message.trim())
              throw new Error("genui: followup message must be nonempty text");
            const payload = Object.fromEntries(
              this.#presentation
                .snapshot()
                .state.map(({ id }) => [id, this.#state.readState(id)]),
            );
            await this.#followup(
              await this.#context.interaction.submit({
                action: message,
                payloadJSON: stringifyJSON(payload),
              }),
              action.id,
            );
            break;
          }
          case "copy": {
            const value = this.#evaluate(step.text);
            if (typeof value !== "string")
              throw new Error("genui: copy action requires text");
            await this.#effects.copy(value);
            break;
          }
          case "openUrl": {
            const value = this.#evaluate(step.url);
            if (typeof value !== "string")
              throw new Error("genui: navigation action requires a URL");
            this.#effects.open(value);
            break;
          }
        }
      } catch (cause) {
        if (cause instanceof Unconfirmed)
          throw cause.followedBy(() => this.#steps(action, current + 1));
        throw cause;
      }
    }
  }

  #evaluate(expression: Parameters<Presentation["evaluate"]>[0]): unknown {
    return this.#presentation.evaluate(
      expression,
      this.#state.readState,
      this.#state.readOperation,
    );
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
      this.#presentation.snapshot().actions.flatMap((action) =>
        action.steps.flatMap((step) => {
          if (
            step.type !== "operation" ||
            !operations.some(({ request }) => request.id === step.operation)
          )
            return [];
          return [
            this.#intent(action.id, step.operation).then((intent) => ({
              intent,
              operationId: step.operation,
            })),
          ];
        }),
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
      const data = await this.#context.track(
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

  async #operation(id: string, actionId: string): Promise<void> {
    this.#context.signal.throwIfAborted();
    const { request, binding } = this.#presentation.operation(id);
    const method = pluginMethod(binding);
    const input = stringifyJSON(this.#evaluate(request.input));
    if (method.kind === "command") await this.#review(binding, input);
    const consume = (chunk: unknown) => {
      this.#context.signal.throwIfAborted();
      this.#state.writeOperation(id, { status: "loading", chunk });
    };
    const { contract, method: name, ...route } = binding;
    const intent =
      method.kind === "command" ? await this.#intent(actionId, id) : "";
    this.#context.signal.throwIfAborted();
    this.#state.writeOperation(id, { status: "loading" });
    try {
      let data: unknown;
      if (method.kind === "command") {
        const command = { contract, method: name, input };
        data = await this.#context.track(
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
        data = await this.#context.track(
          this.#context.runtime.plugins.queryStream(
            { ...binding, input },
            consume,
            (value) => value,
            this.#context.signal,
          ),
        );
      } else {
        const reply = await this.#context.track(
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
