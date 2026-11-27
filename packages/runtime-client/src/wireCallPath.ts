import type { ResultDelivery, RpcCallOptions, RpcClient } from "./client";
import { cloneJSON } from "@frame/sdk/json";
import { RpcCancellationError, RpcError } from "./errors";
import { captureRequestMeta, checkRequest, requestRequiresIdempotency } from "./request";
import {
  createMutationPromise,
  MutationLookupError,
  mutationHandle,
  mutationSettlementIsUnknown,
  type MutationPromise,
} from "./mutation";
import type {
  MutationJournal,
  MutationReservation,
  PluginCommandInput,
  PluginCommandBinding,
} from "./mutationJournal";
import type { PendingRuntimeCommand, RuntimeCommandMethod } from "./mutationJournalEntry";
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
  isWireStreamingMethodName,
  wireMethodIsPaginated,
  wireMethodReturnsValue,
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

export interface WireCallPathOptions {
  capabilities?: () => ServerCapabilities | null | undefined;
  requestMeta?: () => RequestMeta | undefined;
  mutationJournal: MutationJournal;
}

export interface WireCallPath {
  call: WireCall;
  callUnkeyed: UnkeyedCall;
  queryJSON: RpcClient["callJSON"];
  dispatchCommand: CommandDispatch;
  openMutation: OpenMutation;
  retryCommand<M extends RuntimeCommandMethod>(
    method: M,
    identity: Pick<PendingRuntimeCommand, "idempotencyKey" | "namespace">,
    params: WireParams<M>,
    signal?: AbortSignal,
  ): MutationPromise<WireResult<M>>;
  capturePluginCommand(
    intent: string,
    input: PluginCommandInput,
    prepare: () => Promise<PluginCommandBinding>,
    signal?: AbortSignal,
  ): ReturnType<MutationJournal["capturePluginCommand"]>;
}

interface OriginalMutation {
  readonly idempotencyKey: string;
  reserve(preparedParams: unknown): MutationReservation;
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

export function createWireCallPath(client: RpcClient, options: WireCallPathOptions): WireCallPath {
  const captureMetadata = (supplied?: RequestMeta | null) =>
    captureRequestMeta(supplied === undefined ? options.requestMeta?.() : supplied);

  const prepareUnkeyed = <M extends WireMethodName>(
    method: M,
    params: WireParams<M>,
    suppliedMeta?: RequestMeta | null,
    delivery: ResultDelivery = "value",
  ) => {
    const prepared = checkRequest(method, cloneJSON(params));
    if (
      delivery === "json" &&
      (isWireStreamingMethodName(method) ||
        !wireMethodReturnsValue(method) ||
        requestRequiresIdempotency(method, prepared))
    )
      throw new TypeError("exact JSON delivery requires a unary query result");
    const requestMeta = captureMetadata(suppliedMeta);
    requireCapabilities(
      method,
      prepared,
      options.capabilities?.(),
      requestMeta?.clientCapabilities,
    );
    return { params: prepared, requestMeta };
  };

  const callUnkeyed: UnkeyedCall = async (method, params, callOptions) => {
    const prepared = prepareUnkeyed(method, params, callOptions?.requestMeta);
    return client.call(method, prepared.params, {
      ...callOptions,
      requestMeta: prepared.requestMeta,
    });
  };

  const queryJSON: WireCallPath["queryJSON"] = async (method, params, callOptions) => {
    const prepared = prepareUnkeyed(method, params, callOptions?.requestMeta, "json");
    return client.callJSON(method, prepared.params, {
      ...callOptions,
      requestMeta: prepared.requestMeta,
    });
  };

  // A reservation can outlive the capabilities observed when it was admitted.
  const dispatchCommand: CommandDispatch = async (method, params, reservation, callOptions) => {
    const prepared = checkRequest(method, params);
    const idempotencyNamespace = reservation.authorizeDispatch();
    let phase: DispatchPhase | undefined;
    try {
      return await client.call(method, prepared, {
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
    let reservation: MutationReservation;
    try {
      if (!original && signal?.aborted) throw new RpcCancellationError(signal.reason);
      preparedParams = checkRequest(method, cloneJSON(params));
      if (original) {
        reservation = original.reserve(preparedParams);
      } else {
        reservation = options.mutationJournal.reserve(
          method,
          preparedParams,
          requestedKey,
          captureMetadata(requestMeta),
        );
      }
    } catch (error) {
      const refusedKey = original?.idempotencyKey ?? requestedKey ?? crypto.randomUUID();
      const refused = (): MutationPromise<Result> =>
        mutationHandle(Promise.reject(error), refusedKey, refused);
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

  const retryCommand: WireCallPath["retryCommand"] = (method, identity, params, signal) => {
    const command: PendingRuntimeCommand = {
      method,
      idempotencyKey: identity.idempotencyKey,
      namespace: identity.namespace,
    };
    return openMutation(
      method,
      params,
      (prepared, reservation, attemptSignal) =>
        dispatchCommand(method, prepared, reservation, { signal: attemptSignal }),
      {
        signal,
        original: {
          idempotencyKey: command.idempotencyKey,
          reserve: (prepared) => options.mutationJournal.recoverRuntimeCommand(command, prepared),
        },
      },
    );
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

  return {
    call,
    callUnkeyed,
    queryJSON,
    dispatchCommand,
    openMutation,
    retryCommand,
    capturePluginCommand,
  };
}
