import type { RpcCallOptions, RpcClient } from "./client";
import { cloneJSON } from "@frame/sdk/json";
import { RpcCancellationError, RpcError } from "./errors";
import { captureRequestMeta, checkRequest, requestRequiresIdempotency } from "./request";
import {
  createMutationPromise,
  MutationLookupError,
  mutationSettlementIsUnknown,
  type MutationPromise,
} from "./mutation";
import type {
  MutationJournal,
  MutationReservation,
  PluginCommandInput,
  PluginCommandBinding,
} from "./mutationJournal";
import { requireCapabilities } from "./preflight";
import type { DispatchPhase } from "./durableMutationJournal";
import {
  createAutoPagingPromise,
  SDK_PAGINATION_POLICY,
  type AutoPagingPromise,
  type CursorPage,
} from "./pagination";
import type { RequestMeta, ServerCapabilities } from "@flame/runtime-client/wire";
import {
  WIRE_METHOD_POLICY,
  wireMethodIsPaginated,
  type WireMethodName,
  type WireMutationMethodName,
  type WirePaginatedMethodName,
  type WireParams,
  type WireResult,
} from "@flame/runtime-client/methods";

type UnkeyedCall = <M extends WireMethodName>(
  method: M,
  params: WireParams<M>,
  options?: RpcCallOptions<WireResult<M>>,
) => Promise<WireResult<M>>;

type CommandCallOptions<Result> = Omit<
  RpcCallOptions<Result>,
  "idempotencyKey" | "idempotencyNamespace" | "requestMeta" | "beforeDispatch"
>;

type CommandDispatch = <M extends WireMethodName>(
  method: M,
  params: WireParams<M>,
  reservation: MutationReservation,
  options?: CommandCallOptions<WireResult<M>>,
) => Promise<WireResult<M>>;

type WireInvokeResult<M extends WireMethodName> = M extends WireMutationMethodName
  ? MutationPromise<WireResult<M>>
  : Promise<WireResult<M>>;

type WireInvoke = <M extends WireMethodName>(
  method: M,
  params: WireParams<M>,
  options?: RpcCallOptions<WireResult<M>>,
) => WireInvokeResult<M>;

type PaginatedWireCall<M extends WirePaginatedMethodName> =
  WireResult<M> extends CursorPage ? AutoPagingPromise<WireResult<M>> : never;

type WireCallResult<M extends WireMethodName> = M extends WirePaginatedMethodName
  ? PaginatedWireCall<M>
  : M extends WireMutationMethodName
    ? MutationPromise<WireResult<M>>
    : Promise<WireResult<M>>;

export type WireCall = <M extends WireMethodName>(
  method: M,
  params: WireParams<M>,
  options?: RpcCallOptions<WireResult<M>>,
) => WireCallResult<M>;

export interface MethodsOptions {
  capabilities?: () => ServerCapabilities | null | undefined;
  requestMeta?: () => RequestMeta | undefined;
  mutationJournal: MutationJournal;
}

export interface WireCallPath {
  call: WireCall;
  callUnkeyed: UnkeyedCall;
  dispatchCommand: CommandDispatch;
  openMutation: OpenMutation;
  capturePluginCommand(
    intent: string,
    input: PluginCommandInput,
    prepare: () => Promise<PluginCommandBinding>,
    signal?: AbortSignal,
  ): ReturnType<MutationJournal["capturePluginCommand"]>;
}

interface OriginalMutation {
  readonly idempotencyKey: string;
  reserve(): MutationReservation;
}

type MutationOpeningOptions = {
  signal?: AbortSignal;
} & (
  | { original: OriginalMutation; requestedKey?: never; requestMeta?: never }
  | { original?: undefined; requestedKey?: string; requestMeta?: RequestMeta | null }
);

type OpenMutation = <M extends WireMethodName, Result>(
  method: M,
  params: WireParams<M>,
  execute: (
    preparedParams: WireParams<M>,
    reservation: MutationReservation,
    signal?: AbortSignal,
  ) => Promise<Result>,
  options?: MutationOpeningOptions,
) => MutationPromise<Result>;

export function createWireCallPath(client: RpcClient, options: MethodsOptions): WireCallPath {
  const callUnkeyed: UnkeyedCall = async (method, params, callOptions) => {
    const requestMeta =
      callOptions?.requestMeta === undefined ? options.requestMeta?.() : callOptions.requestMeta;
    requireCapabilities(method, params, options.capabilities?.(), requestMeta?.clientCapabilities);
    const effectiveOptions =
      options.requestMeta !== undefined || callOptions?.requestMeta !== undefined
        ? { ...callOptions, requestMeta: requestMeta ?? null }
        : callOptions;
    return client.call(method, params, effectiveOptions);
  };

  // A reservation can outlive the capabilities observed when it was admitted.
  const dispatchCommand: CommandDispatch = async (method, params, reservation, callOptions) => {
    const idempotencyNamespace = reservation.authorizeDispatch();
    let phase: DispatchPhase | undefined;
    try {
      return await client.call(method, params, {
        ...callOptions,
        requestMeta: reservation.requestMeta,
        idempotencyKey: reservation.idempotencyKey,
        idempotencyNamespace,
        beforeDispatch: () => {
          phase = reservation.markDispatched();
        },
      });
    } catch (error) {
      if (
        phase === "replay" &&
        WIRE_METHOD_POLICY[method].idempotency === "replayResponse" &&
        error instanceof RpcError &&
        !error.recordedOutcome &&
        !mutationSettlementIsUnknown(error)
      ) {
        throw new MutationLookupError(error);
      }
      throw error;
    }
  };

  const openMutation = <M extends WireMethodName, Result>(
    method: M,
    params: WireParams<M>,
    execute: (
      preparedParams: WireParams<M>,
      reservation: MutationReservation,
      signal?: AbortSignal,
    ) => Promise<Result>,
    opening: MutationOpeningOptions = {},
  ): MutationPromise<Result> => {
    const { signal, requestedKey, original, requestMeta } = opening;
    let preparedParams: WireParams<M>;
    const preferredJournalKey = original?.idempotencyKey ?? requestedKey ?? crypto.randomUUID();
    let reservation: MutationReservation;
    try {
      if (!original && signal?.aborted) throw new RpcCancellationError();
      preparedParams = checkRequest(method, cloneJSON(params));
      if (original) {
        reservation = original.reserve();
      } else {
        const metadata = captureRequestMeta(
          requestMeta === undefined ? (options.requestMeta?.() ?? null) : requestMeta,
        );
        reservation = options.mutationJournal.reserve(
          method,
          preparedParams,
          preferredJournalKey,
          metadata,
        );
      }
    } catch (error) {
      const refused = (): MutationPromise<Result> =>
        Object.defineProperties(Promise.reject(error), {
          idempotencyKey: { enumerable: true, value: preferredJournalKey },
          retry: { enumerable: true, value: refused },
        }) as MutationPromise<Result>;
      return refused();
    }
    const create = () =>
      createMutationPromise(
        (_, attempt) => execute(preparedParams, reservation, attempt.signal),
        reservation.idempotencyKey,
        { signal },
      );
    return reservation.track(create);
  };

  const invoke = (<M extends WireMethodName>(
    method: M,
    params: WireParams<M>,
    callOptions?: RpcCallOptions<WireResult<M>>,
  ): WireInvokeResult<M> => {
    if (!requestRequiresIdempotency(method, params)) {
      return callUnkeyed(method, params, callOptions) as WireInvokeResult<M>;
    }
    const { signal, idempotencyKey, requestMeta, ...stableCallOptions } = callOptions ?? {};
    return openMutation(
      method,
      params,
      (preparedParams, reservation, attemptSignal) =>
        dispatchCommand(method, preparedParams, reservation, {
          ...stableCallOptions,
          signal: attemptSignal,
        }),
      { signal, requestedKey: idempotencyKey, requestMeta },
    ) as WireInvokeResult<M>;
  }) as WireInvoke;

  const call = (<M extends WireMethodName>(
    method: M,
    params: WireParams<M>,
    callOptions?: RpcCallOptions<WireResult<M>>,
  ): WireCallResult<M> => {
    if (wireMethodIsPaginated(method)) {
      const query = checkRequest(method, cloneJSON(params));
      const initialCursor = (query as { cursor?: string }).cursor;
      return createAutoPagingPromise<CursorPage>(
        (cursor) => {
          const continuation = { ...query, cursor } as WireParams<M> & {
            cursor?: string;
          };
          if (cursor === undefined) delete continuation.cursor;
          return invoke<M>(method, continuation, callOptions) as unknown as Promise<CursorPage>;
        },
        SDK_PAGINATION_POLICY,
        initialCursor,
      ) as unknown as WireCallResult<M>;
    }
    return invoke(method, params, callOptions) as WireCallResult<M>;
  }) as WireCall;

  const capturePluginCommand: WireCallPath["capturePluginCommand"] = (
    intent,
    input,
    prepare,
    signal,
  ) =>
    options.mutationJournal.capturePluginCommand(
      intent,
      input,
      prepare,
      options.requestMeta,
      signal,
    );

  return { call, callUnkeyed, dispatchCommand, openMutation, capturePluginCommand };
}
