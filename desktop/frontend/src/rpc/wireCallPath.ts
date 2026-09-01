// Four things happen between a caller and the socket that a binding line like
// `call("sessions.get", { sessionId })` does not show: a capability preflight that refuses
// locally what the server said it cannot do, an idempotency key from the durable journal so
// a retry is the SAME command, a mutation promise carrying that key and a `retry`, and
// cursor auto-paging.

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

// From the GENERATED table, so a rename in the Registry is a compile error rather than a
// runtime `method_not_found`.
export type WirePerform = <M extends WireMethodName>(
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
  /**
   * What the server said it can do, or null before discovery — the capability
   * preflight reads it before each call. Omit it and every call goes out, leaving
   * the runtime to refuse what it cannot do.
   */
  capabilities?: () => ServerCapabilities | null | undefined;
  /**
   * Metadata attached to the next request. The factory reads it once per call,
   * using the same snapshot for capability preflight and emission.
   */
  requestMeta?: () => RequestMeta | undefined;
  /** Optional durable owner for unresolved command identities. The RPC SDK
   * remains storage-agnostic; Desktop supplies the adapter at composition. */
  mutationJournal?: MutationJournal;
}

/** What a mutation-shaped binding needs beyond `call`: `runs.start` subscribes before the
 *  POST, so it opens its own mutation around a stream it already holds. */
export interface WireCallPath {
  call: WireCall;
  perform: WirePerform;
  openMutation: OpenMutation;
}

export type OpenMutation = <M extends WireMethodName, Result>(
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
  // Every outbound call passes the preflight, because the alternative is a
  // round-trip whose only possible answer is the refusal we already hold.
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
      // This is the same typed refusal the runtime would return, with every gap in
      // one frame. Manufacturing a detail here would put runtime words in a local
      // refusal, so the UI still owns the prose.
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
