import type { RpcCallOptions, RpcClient } from "./client";
import { RpcError } from "./errors";
import { checkRequest, requestRequiresIdempotency } from "./request";
import { createMutationPromise, type MutationPromise } from "./mutation";
import type { MutationJournal, MutationReservation } from "./mutationJournal";
import { unnegotiated } from "./preflight";
import {
  createAutoPagingPromise,
  SDK_PAGINATION_POLICY,
  type AutoPagingPromise,
  type CursorPage,
} from "./pagination";
import type { RequestMeta, ServerCapabilities } from "@flame/runtime-client/wire";
import {
  wireMethodIsPaginated,
  type WireMethodName,
  type WireMutationMethodName,
  type WirePaginatedMethodName,
  type WireParams,
  type WireResult,
} from "@flame/runtime-client/methods";

type WirePerform = <M extends WireMethodName>(
  method: M,
  params: WireParams<M>,
  options?: RpcCallOptions,
) => Promise<WireResult<M>>;

type WireInvokeResult<M extends WireMethodName> = M extends WireMutationMethodName
  ? MutationPromise<WireResult<M>>
  : Promise<WireResult<M>>;

type WireInvoke = <M extends WireMethodName>(
  method: M,
  params: WireParams<M>,
  options?: RpcCallOptions,
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
  options?: RpcCallOptions,
) => WireCallResult<M>;

export interface MethodsOptions {
  capabilities?: () => ServerCapabilities | null | undefined;
  requestMeta?: () => RequestMeta | undefined;
  mutationJournal?: MutationJournal;
}

export interface WireCallPath {
  call: WireCall;
  perform: WirePerform;
  openMutation: OpenMutation;
}

type OpenMutation = <M extends WireMethodName, Result>(
  method: M,
  params: WireParams<M>,
  execute: (
    idempotencyKey: string,
    attempt: MutationDelivery,
    preparedParams: WireParams<M>,
  ) => Promise<Result>,
  signal?: AbortSignal,
  requestedKey?: string,
  journalKey?: string,
  reserveOriginal?: () => MutationReservation,
) => MutationPromise<Result>;

interface MutationDelivery {
  signal?: AbortSignal;
  readonly requestMeta: RequestMeta | null;
  readonly steerItemId: string | null;
  bindSteerItem(userItemId: string): void;
  lookupNamespace(): string;
  authorizeDispatch(): string;
}

export function createWireCallPath(
  client: RpcClient,
  options: MethodsOptions & { mutationJournal: MutationJournal },
): WireCallPath {
  const refuse = <M extends WireMethodName>(
    method: M,
    params: WireParams<M>,
    requestMeta?: RequestMeta | null,
  ): void => {
    const missing = unnegotiated(
      method,
      params,
      options.capabilities?.(),
      requestMeta?.clientCapabilities,
    );
    if (missing.length === 0) return;
    throw new RpcError({
      message: `${method} requires ${missing.join(", ")}`,
      data: {
        type: "capability_not_negotiated",
        requiredCapabilities: missing.map((name) => ({
          type: "feature",
          name,
        })),
      },
    });
  };

  const perform: WirePerform = async (method, params, callOptions) => {
    const requestMeta =
      callOptions?.requestMeta === undefined ? options.requestMeta?.() : callOptions.requestMeta;
    refuse(method, params, requestMeta);
    const effectiveOptions =
      options.requestMeta !== undefined || callOptions?.requestMeta !== undefined
        ? { ...callOptions, requestMeta: requestMeta ?? null }
        : callOptions;
    return client.call(method, params, effectiveOptions);
  };

  const openMutation = <M extends WireMethodName, Result>(
    method: M,
    params: WireParams<M>,
    execute: (
      idempotencyKey: string,
      attempt: MutationDelivery,
      preparedParams: WireParams<M>,
    ) => Promise<Result>,
    signal?: AbortSignal,
    requestedKey?: string,
    journalKey?: string,
    reserveOriginal?: () => MutationReservation,
  ): MutationPromise<Result> => {
    const preparedParams = structuredClone(params);
    const preferredJournalKey = requestedKey ?? journalKey ?? crypto.randomUUID();
    let reservation: MutationReservation;
    try {
      checkRequest(method, preparedParams);
      reservation = reserveOriginal
        ? reserveOriginal()
        : options.mutationJournal.reserve(
            method,
            preparedParams,
            preferredJournalKey,
            structuredClone(options.requestMeta?.() ?? null),
          );
    } catch (error) {
      const failedKey = requestedKey ?? preferredJournalKey;
      const refused = (): MutationPromise<Result> =>
        Object.defineProperties(Promise.reject(error), {
          idempotencyKey: { enumerable: true, value: failedKey },
          retry: { enumerable: true, value: refused },
        }) as MutationPromise<Result>;
      return refused();
    }
    const create = () =>
      createMutationPromise(
        (idempotencyKey, attempt) => {
          return execute(
            idempotencyKey,
            {
              ...attempt,
              requestMeta: reservation.requestMeta,
              get steerItemId() {
                return reservation.steerItemId;
              },
              bindSteerItem: (userItemId) => {
                reservation.bindSteerItem(userItemId);
              },
              lookupNamespace: () => reservation.lookupNamespace(),
              authorizeDispatch: () => reservation.authorizeDispatch(),
            },
            preparedParams,
          );
        },
        reservation.idempotencyKey,
        { signal },
      );
    return reservation.track(create);
  };

  const invoke = (<M extends WireMethodName>(
    method: M,
    params: WireParams<M>,
    callOptions?: RpcCallOptions,
  ): WireInvokeResult<M> => {
    if (!requestRequiresIdempotency(method, params)) {
      return perform(method, params, callOptions) as WireInvokeResult<M>;
    }
    const { signal, idempotencyKey, ...stableCallOptions } = callOptions ?? {};
    return openMutation(
      method,
      params,
      (idempotencyKey, attempt, preparedParams) => {
        const idempotencyNamespace = attempt.authorizeDispatch();
        return perform(method, preparedParams, {
          ...stableCallOptions,
          ...(attempt.signal ? { signal: attempt.signal } : {}),
          idempotencyKey,
          ...(idempotencyNamespace ? { idempotencyNamespace } : {}),
        });
      },
      signal,
      idempotencyKey,
    ) as WireInvokeResult<M>;
  }) as WireInvoke;

  const call = (<M extends WireMethodName>(
    method: M,
    params: WireParams<M>,
    callOptions?: RpcCallOptions,
  ): WireCallResult<M> => {
    if (wireMethodIsPaginated(method)) {
      const query = structuredClone(params);
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

  return { call, perform, openMutation };
}
