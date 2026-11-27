import type {
  ComponentContext,
  InteractionSubmission,
} from "@flame/component-sdk";
import {
  OperationFailure,
  pluginMethod,
  type PluginCommand,
} from "@flame/runtime-client";
import { parseJSON, stringifyJSON } from "@frame/sdk/json";
import type { PluginCallRoute } from "@flame/runtime-client/wire";
import type { ActionPayload } from "@a2ui/web_core";
import { BUILTIN_ACTIONS, Presentation, limits } from "../contracts/document";

export interface InteractionState {
  writeOperation(id: string, value: unknown): void;
  resetDraft(surfaceId: string): void;
}

type Review = {
  binding: PluginCallRoute;
  input: string;
  approve(): void;
  cancel(): void;
};
type RecoveryView = { label: string; key: string };
type FollowupRecovery = RecoveryView & {
  recover(): Promise<void>;
};

type ForegroundView =
  | { status: "idle" }
  | { status: "running" }
  | { status: "review"; review: Review }
  | { status: "failed"; error: unknown }
  | { status: "complete" };

type Foreground =
  | ForegroundView
  | { status: "followupUnconfirmed"; recovery: FollowupRecovery };
type Outcome = "idle" | "complete";
type InteractionView = Readonly<
  ForegroundView | { status: "unknown"; recoveries: readonly RecoveryView[] }
>;

const initializing = { status: "running" } as const;

class UnconfirmedFollowup extends Error {
  constructor(
    readonly recovery: FollowupRecovery,
    cause?: unknown,
  ) {
    super("genui: recover the original action before continuing", { cause });
  }
}

class CanceledReview extends Error {}

export class InteractionController {
  readonly #context: ComponentContext;
  readonly #presentation: Presentation;
  readonly #state: InteractionState;
  readonly #listeners = new Set<() => void>();
  #foreground: Foreground = initializing;
  #view: InteractionView = initializing;
  #commandIntents = new Map<string, string>();
  #initialization?: Promise<void>;

  constructor(
    context: ComponentContext,
    presentation: Presentation,
    state: InteractionState,
  ) {
    this.#context = context;
    this.#presentation = presentation;
    this.#state = state;
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

  #publish(view: Foreground): void {
    this.#foreground = view;
    this.#project();
  }

  #project = (): void => {
    if (this.#context.signal.aborted) return;
    const foreground = this.#foreground;
    let view: InteractionView;
    if (foreground.status === "followupUnconfirmed") {
      const { label, key } = foreground.recovery;
      view = { status: "unknown", recoveries: [{ label, key }] };
    } else if (
      foreground.status === "running" ||
      foreground.status === "review"
    ) {
      view = foreground;
    } else {
      view = foreground;
      try {
        const originals = this.#commands();
        if (originals.some(({ command }) => command.state === "active")) {
          view = { status: "running" };
        } else if (originals.length) {
          view = {
            status: "unknown",
            recoveries: originals.map(({ id, command }) => ({
              label: id,
              key: command.idempotencyKey,
            })),
          };
        }
      } catch (error) {
        view = { status: "failed", error };
      }
    }
    this.#view = view;
    for (const listener of this.#listeners) listener();
  };

  connect(): () => void {
    const stop = this.#context.runtime.plugins.subscribeCommands(this.#project);
    this.#initialization ??= this.#initialize();
    this.#project();
    return stop;
  }

  async #initialize(): Promise<void> {
    try {
      const commands = this.#presentation
        .operations()
        .filter(({ binding }) => pluginMethod(binding).kind === "command");
      const intents = await Promise.all(
        commands.map(
          async ({ request: { id } }) =>
            [id, await this.#intent(id, id)] as const,
        ),
      );
      if (this.#context.signal.aborted) return;
      this.#commandIntents = new Map(intents);
      this.#publish({ status: "idle" });
    } catch (error) {
      this.#publish({ status: "failed", error });
    }
  }

  #commands(): { id: string; command: PluginCommand }[] {
    if (!this.#commandIntents.size) return [];
    return this.#context.runtime.plugins.commands().flatMap((command) => {
      for (const [id, intent] of this.#commandIntents)
        if (command.matchesIntent(intent)) return [{ id, command }];
      return [];
    });
  }

  async execute(action: ActionPayload): Promise<void> {
    if (this.blocked) return;
    const captured = structuredClone(action);
    await this.#guard(async () => {
      this.#presentation.action(captured);
      if (captured.name === BUILTIN_ACTIONS.resetDraft) {
        this.#state.resetDraft(captured.surfaceId);
      } else if (captured.name === BUILTIN_ACTIONS.followup) {
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
        return this.#operation(captured.name, captured.context);
      }
      return "complete";
    });
  }

  async #guard(run: () => Promise<Outcome>): Promise<void> {
    this.#publish({ status: "running" });
    try {
      this.#context.signal.throwIfAborted();
      this.#publish({ status: await run() });
    } catch (error) {
      if (this.#context.signal.aborted) return;
      if (error instanceof UnconfirmedFollowup)
        this.#publish({
          status: "followupUnconfirmed",
          recovery: error.recovery,
        });
      else if (error instanceof CanceledReview)
        this.#publish({ status: "idle" });
      else this.#publish({ status: "failed", error });
    }
  }

  async recover(key: string): Promise<void> {
    const view = this.#view;
    if (view.status !== "unknown") return;
    if (!view.recoveries.some((entry) => entry.key === key)) return;
    const foreground = this.#foreground;
    await this.#guard(async () => {
      if (foreground.status === "followupUnconfirmed") {
        await foreground.recovery.recover();
        return "complete";
      }
      const original = this.#commands().find(
        ({ command }) => command.idempotencyKey === key,
      );
      if (original?.command.state !== "recoverable") return "idle";
      const { id, command } = original;
      return this.#observeOperation(id, () =>
        this.#context.track((signal) =>
          command.recover((value) => value, signal),
        ),
      );
    });
  }

  async #followup(result: InteractionSubmission, label: string): Promise<void> {
    this.#context.signal.throwIfAborted();
    if (result.status === "unknown") {
      const recovery: FollowupRecovery = {
        label,
        key: result.idempotencyKey,
        recover: async () => {
          let settled: InteractionSubmission;
          try {
            settled = await result.recover();
          } catch (cause) {
            throw new UnconfirmedFollowup(recovery, cause);
          }
          await this.#followup(settled, label);
        },
      };
      throw new UnconfirmedFollowup(recovery);
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

  async #review(binding: PluginCallRoute, input: string): Promise<void> {
    const approved = await new Promise<boolean>((resolve) => {
      const abort = () => resolve(false);
      this.#context.signal.addEventListener("abort", abort, { once: true });
      const decide = (approved: boolean) => {
        this.#context.signal.removeEventListener("abort", abort);
        resolve(approved);
      };
      this.#publish({
        status: "review",
        review: {
          binding,
          input,
          approve: () => decide(true),
          cancel: () => decide(false),
        },
      });
    });
    this.#context.signal.throwIfAborted();
    if (!approved) throw new CanceledReview();
    this.#publish({ status: "running" });
  }

  async #operation(
    id: string,
    inputValue: Record<string, unknown>,
  ): Promise<Outcome> {
    this.#context.signal.throwIfAborted();
    const { binding: route } = this.#presentation.operation(id);
    const method = pluginMethod(route);
    const streaming = method.delivery === "plugins.stream";
    const input = stringifyJSON(inputValue);
    const consume = (chunk: unknown) => {
      this.#context.signal.throwIfAborted();
      this.#state.writeOperation(id, { status: "loading", chunk });
    };
    if (method.kind === "command") {
      const intent = this.#commandIntents.get(id);
      if (intent === undefined)
        throw new Error("genui: the captured command intent is unavailable");
      await this.#review(route, input);
      if (this.#commands().length) return "idle";
      const { contract, method: name, ...binding } = route;
      const command = { contract, method: name, input };
      return this.#observeOperation(id, () =>
        this.#context.track((signal) =>
          streaming
            ? this.#context.runtime.plugins.stream(
                intent,
                command,
                async () => binding,
                consume,
                (value) => value,
                signal,
              )
            : this.#context.runtime.plugins.command(
                intent,
                command,
                async () => binding,
                (value) => value,
                signal,
              ),
        ),
      );
    }
    return this.#observeOperation(id, () =>
      this.#context.track(async (signal) => {
        if (streaming)
          return this.#context.runtime.plugins.queryStream(
            { route, input },
            consume,
            (value) => value,
            signal,
          );
        const reply = await this.#context.runtime.plugins.call(
          { route, input },
          { signal },
        );
        if (reply.failure)
          throw new OperationFailure(reply.failure.code, reply.failure.message);
        if (reply.output === undefined)
          throw new Error("genui: query omitted its output");
        return parseJSON(reply.output);
      }),
    );
  }

  async #observeOperation(
    id: string,
    attempt: () => Promise<unknown>,
  ): Promise<Outcome> {
    this.#state.writeOperation(id, { status: "loading" });
    try {
      const data = await attempt();
      this.#context.signal.throwIfAborted();
      this.#state.writeOperation(id, { status: "success", data });
      return "complete";
    } catch (cause) {
      this.#context.signal.throwIfAborted();
      if (
        this.#commandIntents.has(id) &&
        this.#commands().some((entry) => entry.id === id)
      ) {
        this.#state.writeOperation(id, undefined);
        return "idle";
      }
      this.#state.writeOperation(id, {
        status: "error",
        error: cause instanceof Error ? cause.message : String(cause),
      });
      throw cause;
    }
  }
}
