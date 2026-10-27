import {
  type CapturedToolInteraction,
  type InteractionSubmission,
  type ToolInteractionInput,
} from "@flame/component-sdk";
import { asRunId, parseJSON, type FlameClient } from "@flame/runtime-client";
import { type AgentInput, type AgentInputSettlement } from "@/features/agent/public/input";

export type SubmitCapturedAgentInput = (
  sessionId: string,
  input: AgentInput,
  signal: AbortSignal,
) => Promise<AgentInputSettlement>;

export function captureToolInteraction(
  runtime: FlameClient,
  submitInput: SubmitCapturedAgentInput,
  source: Readonly<{ runId: string; itemId: string }>,
  signal: AbortSignal,
  track: <T>(operation: Promise<T>) => Promise<T>,
): CapturedToolInteraction {
  const captured = Object.freeze({ ...source });
  let active = false;
  let unknown: Extract<InteractionSubmission, { status: "unknown" }> | undefined;

  const project = (result: AgentInputSettlement): InteractionSubmission => {
    if (result.status !== "unknown") {
      unknown = undefined;
      return Object.freeze({ ...result });
    }
    const retained: Extract<InteractionSubmission, { status: "unknown" }> = Object.freeze({
      status: "unknown",
      idempotencyKey: result.idempotencyKey,
      recover: () => {
        signal.throwIfAborted();
        if (active) throw new Error("tool interaction is already awaiting its original command");
        active = true;
        const recovery = Promise.resolve()
          .then(() => result.recover())
          .then(project)
          .finally(() => {
            active = false;
          });
        return track(recovery);
      },
    });
    unknown = retained;
    return retained;
  };

  return Object.freeze({
    source: captured,
    submit(input: ToolInteractionInput): Promise<InteractionSubmission> {
      signal.throwIfAborted();
      if (active) throw new Error("tool interaction is already being submitted");
      if (unknown) throw new Error("recover the original tool interaction before submitting again");
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
      active = true;
      const submission = (async () => {
        const run = await runtime.runs.get(asRunId(captured.runId), signal);
        signal.throwIfAborted();
        return project(await submitInput(run.sessionId, prepared, signal));
      })().finally(() => {
        active = false;
      });
      return track(submission);
    },
  });
}
