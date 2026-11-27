import { cloneJSON } from "@frame/sdk/json";
import { wireAcknowledgementCheck, type WireParams } from "@flame/runtime-client/methods";
import { checkRequest } from "./request";
import type { RpcCallOptions } from "./client";
import {
  MutationLookupError,
  mutationHandle,
  observedMutation,
  type MutationPromise,
} from "./mutation";
import type { WireCallPath, WireCallPathOptions } from "./wireCallPath";
import type {
  ResumeRunRequest,
  ResumeRunResponse,
  StartRunResponse,
  SteerRunResponse,
  SteerRunRequest,
  RunEvent,
  RunAcceptance,
  GetRunAcceptanceRequest,
  StartRunRequest,
} from "@flame/runtime-client/wire";
import {
  callOrDispose,
  streamRunEvents,
  type RunStreamTarget,
  type StreamingResult,
} from "./stream";
import type { PendingRunCommand } from "./mutationJournal";
import { RpcCancellationError, RpcProtocolError } from "./errors";

import type { MutationJournal } from "./mutationJournal";

export interface RunOpeningResult<R> {
  result: R;
  observation:
    | { type: "stream"; events: AsyncIterable<RunEvent> }
    | { type: "snapshot"; sessionId: string }
    | { type: "retired"; sessionId: string };
}

export type SteerCommandResult = SteerRunResponse & { type: "accepted" | "retired" };

export interface SteerMutationPromise extends MutationPromise<SteerCommandResult> {
  /** An opaque Runtime nomination, cached before dispatch; it is not acceptance. */
  readonly inputIdentity: Promise<string>;
  retry(options?: { signal?: AbortSignal }): SteerMutationPromise;
}

type RecordedAcceptance = Exclude<RunAcceptance, { type: "notRecorded" }>;
type RecordedInputAcceptance = RecordedAcceptance & { userItemId: string };

interface CommandObservation {
  readonly idempotencyKey: string;
  readonly observation:
    | { type: "unknown"; cause: unknown }
    | Extract<RunAcceptance, { type: "notRecorded" }>
    | {
        type: "recorded";
        acceptance: RecordedAcceptance;
        acknowledge(): void;
      };
}

export type RunCommandRecovery = CommandObservation &
  (
    | ({ operation: "start"; params: StartRunRequest } & Pick<
        MutationPromise<RunOpeningResult<StartRunResponse>>,
        "retry"
      >)
    | ({ operation: "resume"; params: ResumeRunRequest } & Pick<
        MutationPromise<RunOpeningResult<ResumeRunResponse>>,
        "retry"
      >)
    | ({ operation: "steer"; params: SteerRunRequest; userItemId: string | null } & Pick<
        SteerMutationPromise,
        "retry"
      >)
  );

export interface RunCommands {
  attach<R extends RunStreamTarget>(
    signal: AbortSignal | undefined,
    request: (options: Pick<RpcCallOptions<R>, "signal" | "observe">) => Promise<R>,
  ): Promise<StreamingResult<R, RunEvent>>;
  open(
    method: "runs.start",
    params: StartRunRequest,
    signal?: AbortSignal,
  ): MutationPromise<RunOpeningResult<StartRunResponse>>;
  open(
    method: "runs.resume",
    params: ResumeRunRequest,
    signal?: AbortSignal,
  ): MutationPromise<RunOpeningResult<ResumeRunResponse>>;
  steer(params: SteerRunRequest, signal?: AbortSignal): SteerMutationPromise;
  recover(signal?: AbortSignal): Promise<RunCommandRecovery[]>;
}

function openingFromReceipt(receipt: RecordedInputAcceptance): RunOpeningResult<StartRunResponse>;
function openingFromReceipt(receipt: RecordedAcceptance): RunOpeningResult<ResumeRunResponse>;
function openingFromReceipt(receipt: RecordedAcceptance): RunOpeningResult<ResumeRunResponse> {
  const { runId, segmentId, userItemId, sessionId } = receipt;
  return {
    result: { runId, segmentId, ...(userItemId ? { userItemId } : {}) },
    observation: { type: receipt.type === "retired" ? "retired" : "snapshot", sessionId },
  };
}

function requireSteerNomination(receipt: RecordedAcceptance, nominated: string | null): void {
  if (nominated !== null && nominated !== receipt.userItemId)
    throw new RpcProtocolError("runs.getAcceptance result", [
      { path: "$.userItemId", detail: "receipt differs from the nominated input identity" },
    ]);
}

function withSteerIdentity(
  mutation: MutationPromise<SteerCommandResult>,
  inputIdentity: Promise<string>,
): SteerMutationPromise {
  return Object.defineProperty(mutation, "inputIdentity", {
    enumerable: true,
    value: inputIdentity,
  }) as SteerMutationPromise;
}

function observeRecordedSteer(key: string, receipt: RecordedInputAcceptance): SteerMutationPromise {
  const result: SteerCommandResult = { type: receipt.type, userItemId: receipt.userItemId };
  return withSteerIdentity(observedMutation(key, result), Promise.resolve(result.userItemId));
}

// Run commands carry durable acceptance: start, resume and steer first ask the
// Runtime whether their original key was accepted, and only an unrecorded
// command dispatches and attaches its event stream.
export function createRunCommands(
  {
    callUnkeyed,
    dispatchCommand,
    openMutation,
  }: Pick<WireCallPath, "callUnkeyed" | "dispatchCommand" | "openMutation">,
  mutationJournal: MutationJournal,
  capabilities: WireCallPathOptions["capabilities"],
): RunCommands {
  const runEventStreamOptions = (signal?: AbortSignal) => ({
    signal,
    replayLimits: capabilities?.()?.limits.runReplay,
  });

  const attachRunStream = async <R extends RunStreamTarget>(
    signal: AbortSignal | undefined,
    request: (options: Pick<RpcCallOptions<R>, "signal" | "observe">) => Promise<R>,
  ): Promise<StreamingResult<R, RunEvent>> => {
    const stream = streamRunEvents(runEventStreamOptions(signal));
    const result = await callOrDispose(stream, () =>
      request({ signal: stream.signal, observe: stream.observe }),
    );
    return { result, events: stream.events };
  };

  const queryAcceptance = <Operation extends GetRunAcceptanceRequest["operation"]>(
    operation: Operation,
    params: WireParams<`runs.${Operation}`>,
    namespace: string,
    key: string,
    signal?: AbortSignal,
  ) => {
    const acknowledgement = wireAcknowledgementCheck(`runs.${operation}`, params);
    return callUnkeyed("runs.getAcceptance", { namespace, key, operation }, { signal })
      .then((receipt) => {
        if (receipt.type === "notRecorded") return receipt;
        const mismatch = acknowledgement?.(receipt);
        if (mismatch !== undefined)
          throw new RpcProtocolError("runs.getAcceptance result", [
            { path: `$.${mismatch}`, detail: "must match the original command" },
          ]);
        if (
          operation !== "resume" &&
          (!receipt.userItemId || (operation === "steer" && !receipt.input?.length))
        )
          throw new RpcProtocolError("runs.getAcceptance result", [
            {
              path: "$",
              detail: `accepted ${operation} omitted its original input identity or content`,
            },
          ]);
        return receipt;
      })
      .catch((cause: unknown) => {
        // A canceled lookup is still a cancellation: the journal's own dispatch
        // record, not this read, decides whether the reservation may be released.
        if (cause instanceof RpcCancellationError) throw cause;
        throw new MutationLookupError(cause);
      });
  };

  function openRun(
    method: "runs.start",
    params: StartRunRequest,
    signal?: AbortSignal,
    original?: PendingRunCommand,
  ): MutationPromise<RunOpeningResult<StartRunResponse>>;
  function openRun(
    method: "runs.resume",
    params: ResumeRunRequest,
    signal?: AbortSignal,
    original?: PendingRunCommand,
  ): MutationPromise<RunOpeningResult<ResumeRunResponse>>;
  function openRun(
    method: "runs.start" | "runs.resume",
    params: StartRunRequest | ResumeRunRequest,
    signal?: AbortSignal,
    original?: PendingRunCommand,
  ): MutationPromise<RunOpeningResult<ResumeRunResponse>> {
    return openMutation(
      method,
      params,
      async (preparedParams, reservation, attemptSignal) => {
        const namespace = reservation.lookupNamespace();
        const acceptance = await queryAcceptance(
          method === "runs.start" ? "start" : "resume",
          preparedParams,
          namespace,
          reservation.idempotencyKey,
          attemptSignal,
        );
        if (acceptance.type !== "notRecorded") return openingFromReceipt(acceptance);
        const opened = await attachRunStream<ResumeRunResponse>(attemptSignal, (options) =>
          dispatchCommand(method, preparedParams, reservation, options),
        );
        return { result: opened.result, observation: { type: "stream", events: opened.events } };
      },
      { signal, original },
    );
  }

  const openSteer = (
    params: SteerRunRequest,
    signal?: AbortSignal,
    original?: PendingRunCommand,
  ): SteerMutationPromise => {
    let publishedNomination: string | undefined;
    const observers = new Set<ReturnType<typeof Promise.withResolvers<string>>>();
    const publishNomination = async (userItemId: string) => {
      publishedNomination = userItemId;
      const published = [...observers];
      for (const observer of observers) observer.resolve(userItemId);
      observers.clear();
      await Promise.all(published.map((observer) => observer.promise));
    };
    const expose = (mutation: MutationPromise<SteerCommandResult>): SteerMutationPromise => {
      const identity = Promise.withResolvers<string>();
      if (publishedNomination !== undefined) identity.resolve(publishedNomination);
      else observers.add(identity);
      // Observing nomination is optional; the command still reports every failure.
      identity.promise.catch(() => undefined);
      const exposed = mutation.then(
        (result) => result,
        (error: unknown) => {
          observers.delete(identity);
          identity.reject(error);
          throw error;
        },
      );
      return withSteerIdentity(
        mutationHandle(exposed, mutation.idempotencyKey, (options) =>
          expose(mutation.retry(options)),
        ),
        identity.promise,
      );
    };
    const mutation = openMutation(
      "runs.steer",
      params,
      async (prepared, reservation, attemptSignal): Promise<SteerCommandResult> => {
        const boundItem = reservation.steerItemId;
        if (boundItem !== null) await publishNomination(boundItem);
        const namespace = reservation.lookupNamespace();
        const acceptance = await queryAcceptance(
          "steer",
          prepared,
          namespace,
          reservation.idempotencyKey,
          attemptSignal,
        );
        const userItemId =
          acceptance.type === "notRecorded"
            ? acceptance.candidateUserItemId
            : acceptance.userItemId;
        if (!userItemId)
          throw new RpcProtocolError("runs.getAcceptance result", [
            { path: "$", detail: "Steer lookup omitted its input identity" },
          ]);
        if (acceptance.type !== "notRecorded") {
          requireSteerNomination(acceptance, boundItem);
          // The Runtime receipt can outlive the local entry another observer settled.
          await publishNomination(userItemId);
          return { type: acceptance.type, userItemId };
        }
        const nomination = reservation.bindSteerItem(userItemId);
        await publishNomination(nomination);
        const acknowledged = await dispatchCommand("runs.steer", prepared, reservation, {
          signal: attemptSignal,
        });
        if (acknowledged.userItemId !== nomination)
          throw new RpcProtocolError("runs.steer result", [
            { path: "$.userItemId", detail: "ACK differs from the nominated input identity" },
          ]);
        return { type: "accepted", ...acknowledged };
      },
      { signal, original },
    );
    return expose(mutation);
  };

  const recover = async (signal?: AbortSignal) =>
    Promise.all(
      mutationJournal.pendingCommands().map(async (command): Promise<RunCommandRecovery> => {
        let observation: RunCommandRecovery["observation"];
        try {
          const acceptance = await queryAcceptance(
            command.operation,
            command.params,
            command.namespace,
            command.idempotencyKey,
            signal,
          );
          if (command.operation === "steer" && acceptance.type !== "notRecorded")
            requireSteerNomination(acceptance, command.steerItemId);
          observation =
            acceptance.type === "notRecorded"
              ? acceptance
              : { type: "recorded", acceptance, acknowledge: command.acknowledge };
        } catch (cause) {
          observation = { type: "unknown", cause };
        }
        const { idempotencyKey } = command;
        const observed = { idempotencyKey, observation };
        const recorded = observation.type === "recorded" ? observation.acceptance : undefined;
        // A recorded Start or Steer reached this point through the input identity check.
        switch (command.operation) {
          case "start": {
            const receipt =
              recorded &&
              observedMutation(
                idempotencyKey,
                openingFromReceipt(recorded as RecordedInputAcceptance),
              );
            return {
              ...observed,
              operation: "start",
              params: checkRequest("runs.start", cloneJSON(command.params)),
              retry: (options) =>
                receipt ?? openRun("runs.start", command.params, options?.signal, command),
            };
          }
          case "resume": {
            const receipt =
              recorded && observedMutation(idempotencyKey, openingFromReceipt(recorded));
            return {
              ...observed,
              operation: "resume",
              params: checkRequest("runs.resume", cloneJSON(command.params)),
              retry: (options) =>
                receipt ?? openRun("runs.resume", command.params, options?.signal, command),
            };
          }
          case "steer": {
            const receipt =
              recorded && observeRecordedSteer(idempotencyKey, recorded as RecordedInputAcceptance);
            return {
              ...observed,
              operation: "steer",
              userItemId: command.steerItemId,
              params: checkRequest("runs.steer", cloneJSON(command.params)),
              retry: (options) => receipt ?? openSteer(command.params, options?.signal, command),
            };
          }
        }
      }),
    );

  return { attach: attachRunStream, open: openRun, steer: openSteer, recover };
}
