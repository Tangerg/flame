import type { RpcCallOptions, RpcClient } from "./client";
import { RpcError } from "./errors";
import { createMutationPromise, type MutationPromise } from "./mutation";
import type { MutationJournal } from "./mutationJournal";
import { unnegotiated } from "./preflight";
import {
  createAutoPagingPromise,
  SDK_PAGINATION_POLICY,
  type AutoPagingPromise,
  type CursorPage,
} from "./pagination";
import type { RequestMeta, ServerCapabilities } from "@flame/runtime-contract/wire";
import {
  wireMethodIsPaginated,
  wireMethodRequiresIdempotency,
  type WireMethodName,
  type WireMutationMethodName,
  type WirePaginatedMethodName,
  type WireParams,
  type WireResult,
} from "@flame/runtime-contract/methods";

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
    attempt: { signal?: AbortSignal; idempotencyNamespace?: string },
  ) => Promise<Result>,
  signal?: AbortSignal,
  requestedKey?: string,
  journalKey?: string,
) => MutationPromise<Result>;

export function createWireCallPath(client: RpcClient, options: MethodsOptions): WireCallPath {
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
        requiredCapabilities: missing.map((name) => ({ type: "feature", name })),
      },
    });
  };

  const perform: WirePerform = async (method, params, callOptions) => {
    const ownsRequestMeta = options.requestMeta !== undefined;
    const requestMeta = ownsRequestMeta ? options.requestMeta?.() : callOptions?.requestMeta;
    refuse(method, params, requestMeta);
    const effectiveOptions = ownsRequestMeta
      ? { ...callOptions, requestMeta: requestMeta ?? null }
      : callOptions;
    return client.call(method, params, effectiveOptions);
  };

  const openMutation = <M extends WireMethodName, Result>(
    method: M,
    params: WireParams<M>,
    execute: (
      idempotencyKey: string,
      attempt: { signal?: AbortSignal; idempotencyNamespace?: string },
    ) => Promise<Result>,
    signal?: AbortSignal,
    requestedKey?: string,
    journalKey?: string,
  ): MutationPromise<Result> => {
    const preferredJournalKey = journalKey ?? crypto.randomUUID();
    let reservation: ReturnType<MutationJournal["reserve"]>;
    try {
      reservation =
        requestedKey !== undefined
          ? undefined
          : options.mutationJournal?.reserve(method, params, preferredJournalKey);
    } catch (error) {
      const failedKey = requestedKey ?? preferredJournalKey;
      const retry = (retryOptions?: { signal?: AbortSignal }): MutationPromise<Result> =>
        openMutation(
          method,
          params,
          execute,
          retryOptions === undefined ? signal : retryOptions.signal,
          requestedKey,
          preferredJournalKey,
        );
      return Object.defineProperties(Promise.reject(error), {
        idempotencyKey: { enumerable: true, value: failedKey },
        retry: { enumerable: true, value: retry },
      }) as MutationPromise<Result>;
    }
    const mutation = createMutationPromise(
      (idempotencyKey, attempt) => {
        const idempotencyNamespace = reservation?.authorizeAttempt();
        return execute(idempotencyKey, { ...attempt, idempotencyNamespace });
      },
      requestedKey ?? reservation?.idempotencyKey,
      { signal },
    );
    return reservation?.track(mutation) ?? mutation;
  };

  const invoke = (<M extends WireMethodName>(
    method: M,
    params: WireParams<M>,
    callOptions?: RpcCallOptions,
  ): WireInvokeResult<M> => {
    if (!wireMethodRequiresIdempotency(method)) {
      return perform(method, params, callOptions) as WireInvokeResult<M>;
    }
    const { signal, idempotencyKey, ...stableCallOptions } = callOptions ?? {};
    return openMutation(
      method,
      params,
      (idempotencyKey, attempt) =>
        perform(method, params, {
          ...stableCallOptions,
          ...(attempt.signal ? { signal: attempt.signal } : {}),
          idempotencyKey,
          ...(attempt.idempotencyNamespace
            ? { idempotencyNamespace: attempt.idempotencyNamespace }
            : {}),
        }),
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
      const initialCursor = (params as { cursor?: string }).cursor;
      return createAutoPagingPromise<CursorPage>(
        (cursor) => {
          const continuation = { ...params, cursor } as WireParams<M> & { cursor?: string };
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
