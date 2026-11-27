import {
  type CapturedToolInteraction,
  type ComponentContext,
  type InteractionSubmission,
  type ToolInteractionInput,
} from "@flame/component-sdk";
import { asRunId, type FlameClient } from "@flame/runtime-client";
import { parseJSON } from "@frame/sdk/json";
import { type AgentInput, type AgentInputSettlement } from "@/features/agent/public/input";

export type SubmitCapturedAgentInput = (
  sessionId: string,
  input: AgentInput,
  signal: AbortSignal,
) => Promise<AgentInputSettlement>;

type IdleInteraction = { type: "idle" };
type AwaitingInteraction = {
  type: "awaiting";
  command: Extract<AgentInputSettlement, { status: "unknown" }>;
};
type InteractionState = IdleInteraction | AwaitingInteraction | { type: "observing" };

const IDLE_INTERACTION: IdleInteraction = { type: "idle" };

export function captureToolInteraction(
  runtime: FlameClient,
  submitInput: SubmitCapturedAgentInput,
  source: Readonly<{ runId: string; itemId: string }>,
  signal: AbortSignal,
  track: ComponentContext["track"],
): CapturedToolInteraction {
  const captured = Object.freeze({ ...source });
  let state: InteractionState = IDLE_INTERACTION;

  function project(result: AgentInputSettlement): InteractionSubmission {
    if (result.status !== "unknown") {
      state = IDLE_INTERACTION;
      return Object.freeze({ ...result });
    }
    state = { type: "awaiting", command: result };
    const idempotencyKey = result.idempotencyKey;
    return Object.freeze({
      status: "unknown",
      idempotencyKey,
      recover(): Promise<InteractionSubmission> {
        signal.throwIfAborted();
        const pending = state;
        if (pending.type !== "awaiting" || pending.command.idempotencyKey !== idempotencyKey)
          throw new Error("tool interaction no longer awaits this command");
        return observe(() => pending.command.recover(), pending);
      },
    });
  }

  async function observe(
    perform: () => Promise<AgentInputSettlement>,
    previous: IdleInteraction | AwaitingInteraction,
  ): Promise<InteractionSubmission> {
    state = { type: "observing" };
    try {
      return await track(async () => project(await perform()));
    } catch (error) {
      state = previous;
      throw error;
    }
  }

  return Object.freeze({
    source: captured,
    submit(input: ToolInteractionInput): Promise<InteractionSubmission> {
      signal.throwIfAborted();
      if (state.type === "observing")
        throw new Error("tool interaction is already being submitted");
      if (state.type === "awaiting")
        throw new Error("recover the original tool interaction before submitting again");
      if (!input.action.trim()) throw new Error("tool interaction action is empty");
      parseJSON(input.payloadJSON);
      // Keep the user's exact JSON source, including numeric lexemes. This is an
      // ordinary input, not a command name or a new persisted transcript field.
      const prepared: AgentInput = {
        parts: [
          {
            kind: "text",
            text: [
              "Tool result interaction",
              `Source Run: ${JSON.stringify(captured.runId)}`,
              `Source Item: ${JSON.stringify(captured.itemId)}`,
              `Action: ${JSON.stringify(input.action)}`,
              "Form payload (JSON):",
              input.payloadJSON,
            ].join("\n"),
          },
        ],
      };
      return observe(async () => {
        const run = await runtime.runs.get(asRunId(captured.runId), signal);
        signal.throwIfAborted();
        return submitInput(run.sessionId, prepared, signal);
      }, IDLE_INTERACTION);
    },
  });
}
