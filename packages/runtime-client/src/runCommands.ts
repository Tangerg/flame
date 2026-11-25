import { cloneJSON } from "@frame/sdk/json";
import { wireAcknowledgementCheck, type WireParams } from "@flame/runtime-client/methods";
import { checkRequest } from "./request";
import type { RpcCallOptions, RpcClient } from "./client";
import { MutationLookupError, type MutationPromise } from "./mutation";
import { type MethodsOptions } from "./wireCallPath";
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
import { streamRunEvents } from "./stream";
import { MutationJournalScopeUnavailableError } from "./mutationJournal";
import type { PendingRunCommand } from "./mutationJournal";
import { RpcCancellationError, RpcProtocolError } from "./errors";

import { callOrDispose, type StreamingResult } from "./stream";
import type { MutationJournal } from "./mutationJournal";
import type { WireCallPath } from "./wireCallPath";

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

interface CommandObservation {
  readonly idempotencyKey: string;
  readonly observation:
    | { type: "unknown"; cause: unknown }
    | Extract<RunAcceptance, { type: "notRecorded" }>
    | {
        type: "recorded";
        acceptance: Exclude<RunAcceptance, { type: "notRecorded" }>;
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
  attach<R extends { segmentId: string }>(
    signal: AbortSignal | undefined,
    request: (options: Pick<RpcCallOptions, "signal" | "onRequestRpcId">) => Promise<R>,
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

// Run commands carry durable acceptance: start, resume and steer first ask the
// Runtime whether their original key was accepted, and only an unrecorded
// command dispatches and attaches its event stream.
export function createRunCommands(
  client: RpcClient,
  { perform, openMutation }: Pick<WireCallPath, "perform" | "openMutation">,
  mutationJournal: MutationJournal,
  capabilities: MethodsOptions["capabilities"],
): RunCommands {
  const runEventStreamOptions = (signal?: AbortSignal) => ({
    signal,
    replayLimits: capabilities?.()?.limits.runReplay,
  });

  const attachRunStream = async <R extends { segmentId: string }>(
    signal: AbortSignal | undefined,
    request: (options: Pick<RpcCallOptions, "signal" | "onRequestRpcId">) => Promise<R>,
  ): Promise<StreamingResult<R, RunEvent>> => {
    const stream = streamRunEvents(client, runEventStreamOptions(signal));
    const result = await callOrDispose(stream, () =>
      request({ signal: stream.requestSignal, onRequestRpcId: stream.bindRequest }),
    );
    stream.bind(result.segmentId);
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
    return perform("runs.getAcceptance", { namespace, key, operation }, { signal })
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
      async (idempotencyKey, attempt, preparedParams) => {
        const namespace = attempt.lookupNamespace();
        if (!namespace)
          throw new MutationJournalScopeUnavailableError(
            "run opening requires the Runtime store identity",
          );
        const acceptance = await queryAcceptance(
          method === "runs.start" ? "start" : "resume",
          preparedParams,
          namespace,
          idempotencyKey,
          attempt.signal,
        );
        if (acceptance.type !== "notRecorded") {
          return {
            result: {
              runId: acceptance.runId,
              segmentId: acceptance.segmentId,
              ...(acceptance.userItemId ? { userItemId: acceptance.userItemId } : {}),
            },
            observation: {
              type: acceptance.type === "retired" ? "retired" : "snapshot",
              sessionId: acceptance.sessionId,
            },
          };
        }
        const idempotencyNamespace = attempt.authorizeDispatch();
        const opened = await attachRunStream(attempt.signal, (options) =>
          perform(method, preparedParams, {
            ...options,
            idempotencyKey,
            idempotencyNamespace,
            requestMeta: attempt.requestMeta,
            beforeDispatch: attempt.markDispatched,
          }),
        );
        return { result: opened.result, observation: { type: "stream", events: opened.events } };
      },
      {
        signal,
        journalKey: original?.idempotencyKey,
        reserveOriginal: original?.reserve,
      },
    );
  }

  const openSteer = (
    params: SteerRunRequest,
    signal?: AbortSignal,
    original?: PendingRunCommand,
  ): SteerMutationPromise => {
    let knownIdentity: string | undefined;
    const observers = new Set<ReturnType<typeof Promise.withResolvers<string>>>();
    const publishIdentity = async (userItemId: string) => {
      if (knownIdentity !== undefined && knownIdentity !== userItemId)
        throw new RpcProtocolError("runs.steer identity", [
          { path: "$.userItemId", detail: "Runtime changed the original input identity" },
        ]);
      knownIdentity = userItemId;
      const published = [...observers];
      for (const observer of observers) observer.resolve(userItemId);
      observers.clear();
      await Promise.all(published.map((observer) => observer.promise));
    };
    const expose = (mutation: MutationPromise<SteerCommandResult>): SteerMutationPromise => {
      const identity = Promise.withResolvers<string>();
      if (knownIdentity !== undefined) identity.resolve(knownIdentity);
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
      return Object.defineProperties(exposed, {
        idempotencyKey: { enumerable: true, value: mutation.idempotencyKey },
        inputIdentity: { enumerable: true, value: identity.promise },
        retry: {
          enumerable: true,
          value: (options?: { signal?: AbortSignal }) => expose(mutation.retry(options)),
        },
      }) as SteerMutationPromise;
    };
    const mutation = openMutation(
      "runs.steer",
      params,
      async (key, attempt, prepared): Promise<SteerCommandResult> => {
        if (attempt.steerItemId !== null) await publishIdentity(attempt.steerItemId);
        const namespace = attempt.lookupNamespace();
        if (!namespace)
          throw new MutationJournalScopeUnavailableError(
            "steer requires the Runtime store identity",
          );
        const acceptance = await queryAcceptance("steer", prepared, namespace, key, attempt.signal);
        const userItemId =
          acceptance.type === "notRecorded"
            ? acceptance.candidateUserItemId
            : acceptance.userItemId;
        if (!userItemId)
          throw new RpcProtocolError("runs.getAcceptance result", [
            { path: "$", detail: "Steer lookup omitted its input identity" },
          ]);
        attempt.bindSteerItem(userItemId);
        await publishIdentity(userItemId);
        if (acceptance.type !== "notRecorded") return { type: acceptance.type, userItemId };
        const acknowledged = await perform("runs.steer", prepared, {
          signal: attempt.signal,
          idempotencyKey: key,
          idempotencyNamespace: attempt.authorizeDispatch(),
          requestMeta: attempt.requestMeta,
          beforeDispatch: attempt.markDispatched,
        });
        if (acknowledged.userItemId !== userItemId)
          throw new RpcProtocolError("runs.steer result", [
            { path: "$.userItemId", detail: "ACK differs from the nominated input identity" },
          ]);
        return { type: "accepted", ...acknowledged };
      },
      {
        signal,
        journalKey: original?.idempotencyKey,
        reserveOriginal: original?.reserve,
      },
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
          observation =
            acceptance.type === "notRecorded"
              ? acceptance
              : { type: "recorded", acceptance, acknowledge: command.acknowledge };
        } catch (cause) {
          observation = { type: "unknown", cause };
        }
        const observed = {
          idempotencyKey: command.idempotencyKey,
          observation,
        };
        switch (command.operation) {
          case "start":
            return {
              ...observed,
              operation: "start",
              params: checkRequest("runs.start", cloneJSON(command.params)),
              retry: (options) => openRun("runs.start", command.params, options?.signal, command),
            };
          case "resume":
            return {
              ...observed,
              operation: "resume",
              params: checkRequest("runs.resume", cloneJSON(command.params)),
              retry: (options) => openRun("runs.resume", command.params, options?.signal, command),
            };
          case "steer":
            return {
              ...observed,
              operation: "steer",
              userItemId: command.steerItemId,
              params: checkRequest("runs.steer", cloneJSON(command.params)),
              retry: (options) => openSteer(command.params, options?.signal, command),
            };
        }
      }),
    );

  return { attach: attachRunStream, open: openRun, steer: openSteer, recover };
}
