// Id allocation, response correlation and notification dispatch (API.md §1).

import { errorMessage, RpcError, RpcProtocolError, RpcTransportError } from "./errors";
import {
  MAXIMUM_RUN_EVENT_ID_CHARACTERS,
  NOTIFICATIONS_RUN_EVENT,
  RUN_EVENT_ID_PREFIX,
  runEventReliability,
  type ProblemData,
  type RequestMeta,
} from "@flame/runtime-contract/wire";
import type {
  Transport,
  TransportEvent,
  TransportRequest,
  TransportResponseMetadata,
} from "./transport";
import {
  wireMethodAcceptsReplayCursor,
  wireMethodRequiresIdempotency,
  wireMethodReturnsValue,
  type WireMethodName,
  type WireParams,
  type WireResult,
} from "@flame/runtime-contract/methods";
import {
  isWireNotificationName,
  validateMethodResult,
  validateNotificationParams,
  validateWire,
  type WireNotificationName,
  type WireNotificationParams,
} from "@flame/runtime-contract/validate";
import type { RpcId, RpcMessage } from "./types";
import { JSONRPC_VERSION, isErrorResponse, isNotification, isResponse } from "./types";
import { ExactSequence } from "@/foundation/exactSequence";

export interface NotificationObserver<M extends WireNotificationName = WireNotificationName> {
  next(params: WireNotificationParams[M], requestRpcId: RpcId): void;
  /** A protocol error names its response stream; a connection failure has no request owner
   *  and terminates every observer. */
  error(error: RpcProtocolError | RpcTransportError, requestRpcId?: RpcId): void;
}

export type StreamEndHandler = (event: Extract<TransportEvent, { type: "streamEnd" }>) => void;

function notificationEventType(params: unknown): unknown {
  if (params === null || typeof params !== "object" || Array.isArray(params)) return undefined;
  const event = (params as Record<string, unknown>).event;
  if (event === null || typeof event !== "object" || Array.isArray(event)) return undefined;
  return (event as Record<string, unknown>).type;
}

export interface RpcClientOptions {
  requestMeta?: () => RequestMeta | undefined;
}

export interface RpcCallOptions {
  signal?: AbortSignal;
  idempotencyKey?: string;
  /** Refused BEFORE business admission when it no longer matches discovery. */
  idempotencyNamespace?: string;
  /** The last event this client FOLDED (§5.5). Replayed from just after it, or refused when
   *  not addressable — the caller's signal to cold-read. */
  lastEventId?: string;
  /** A SNAPSHOT, so preflight and the emitted request stay on one client declaration even
   *  when the metadata provider is dynamic. */
  requestMeta?: RequestMeta | null;
  /** Bind a stream owner before Transport.send can deliver its first frame. */
  onRequestRpcId?: (id: RpcId) => void;
}

export interface RpcClient {
  call<M extends WireMethodName>(
    method: M,
    params: WireParams<M>,
    options?: RpcCallOptions,
  ): Promise<WireResult<M>>;
  /** Returns an unsubscribe fn. */
  subscribe<M extends WireNotificationName>(
    method: M,
    observer: NotificationObserver<M>,
  ): () => void;
  onStreamEnd(handler: StreamEndHandler): () => void;
  close(): Promise<void>;
}

interface Pending {
  method: WireMethodName;
  resolve: (value: unknown) => void;
  reject: (err: unknown) => void;
}

export function createRpcClient(transport: Transport, options: RpcClientOptions = {}): RpcClient {
  // Arbitrary precision: an integer id would repeat at the safe-integer boundary.
  const requestIds = new ExactSequence();
  const pending = new Map<RpcId, Pending>();
  const subscribers = new Map<WireNotificationName, Set<NotificationObserver>>();
  const streamEndHandlers = new Set<StreamEndHandler>();
  let closed = false;
  let closePromise: Promise<void> | undefined;

  function failAllPending(failure: RpcTransportError): void {
    for (const { reject } of pending.values()) reject(failure);
    pending.clear();
  }

  function failConnection(failure: RpcTransportError): void {
    closed = true;
    failAllPending(failure);
    for (const observers of subscribers.values()) {
      for (const observer of observers) observer.error(failure);
    }
    subscribers.clear();
    streamEndHandlers.clear();
  }

  // Whether the stream throws OR closes cleanly, no further Responses arrive, so every
  // in-flight request must settle. Handling only the throw path hangs them on a clean EOS.
  const receiveLoop = (async () => {
    try {
      for await (const event of transport.recv()) {
        dispatchInbound(event);
      }
      failConnection(new RpcTransportError("transport stream ended"));
    } catch (err) {
      failConnection(new RpcTransportError(`transport recv() failed: ${errorMessage(err)}`));
    }
  })();

  function dispatchInbound(event: TransportEvent): void {
    if (event.type === "requestError") {
      const entry = pending.get(event.rpcId);
      if (!entry) return;
      pending.delete(event.rpcId);
      entry.reject(event.error);
      return;
    }
    if (event.type === "streamEnd") {
      for (const handler of streamEndHandlers) handler(event);
      return;
    }
    if (isResponse(event.message) && event.message.id !== event.requestRpcId) {
      // The SOURCE request is authoritative, not the envelope id: a transport merges many
      // response bodies into one channel, so a malformed frame from request A
      // could otherwise settle request B and strand A.
      const entry = pending.get(event.requestRpcId);
      if (entry) {
        pending.delete(event.requestRpcId);
        entry.reject(
          new RpcProtocolError(
            `${entry.method} response`,
            [
              {
                path: `${entry.method}.response.id`,
                detail: `must match request id ${event.requestRpcId}`,
              },
            ],
            event.metadata?.requestId,
          ),
        );
      }
      return;
    }
    dispatchMessage(event.message, event.requestRpcId, event.metadata);
  }

  function dispatchMessage(
    msg: RpcMessage,
    requestRpcId: RpcId,
    metadata?: TransportResponseMetadata,
  ): void {
    if (isResponse(msg)) {
      const entry = pending.get(msg.id);
      if (!entry) return; // unsolicited or already settled — drop silently
      pending.delete(msg.id);
      if (isErrorResponse(msg)) {
        const payload = msg.error;
        if (payload.data !== undefined) {
          const violations = validateWire("ProblemData", payload.data);
          if (violations.length > 0) {
            entry.reject(
              new RpcProtocolError(`${entry.method} error data`, violations, metadata?.requestId),
            );
            return;
          }
        }
        entry.reject(
          new RpcError(
            {
              code: payload.code,
              message: payload.message,
              data: payload.data as ProblemData | undefined,
            },
            metadata?.requestId,
          ),
        );
      } else {
        const result = msg.result;
        const violations = validateMethodResult(entry.method, result);
        if (violations.length > 0) {
          entry.reject(
            new RpcProtocolError(`${entry.method} result`, violations, metadata?.requestId),
          );
          return;
        }
        entry.resolve(wireMethodReturnsValue(entry.method) ? result : undefined);
      }
      return;
    }
    if (isNotification(msg)) {
      if (!isWireNotificationName(msg.method)) return;
      const handlers = subscribers.get(msg.method);
      if (!handlers) return;
      const violations = validateNotificationParams(msg.method, msg.params);
      if (violations.length > 0) {
        const failure = new RpcProtocolError(
          `${msg.method} params`,
          violations,
          metadata?.requestId,
        );
        if (
          msg.method === NOTIFICATIONS_RUN_EVENT &&
          runEventReliability(notificationEventType(msg.params)) === "ephemeral"
        ) {
          console.warn(`[rpc] dropping invalid ephemeral notification: ${failure.message}`);
          return;
        }
        for (const observer of handlers) observer.error(failure, requestRpcId);
        return;
      }
      for (const observer of handlers) {
        try {
          observer.next(msg.params as WireNotificationParams[typeof msg.method], requestRpcId);
        } catch (err) {
          // A subscriber must never crash the dispatch loop.
          console.error(`[rpc] notification handler for "${msg.method}" threw:`, err);
        }
      }
      return;
    }
    // The protocol has no server→client RPC (API.md §1.1), so this is always a mismatch.
    console.warn("[rpc] dropping unexpected server-initiated Request", msg);
  }

  function paramsWithMeta<P>(
    params: P | undefined,
    meta: RequestMeta | null | undefined = options.requestMeta?.(),
  ): unknown {
    if (!meta) return params;
    if (params === undefined) return { _meta: meta };
    if (params !== null && typeof params === "object" && !Array.isArray(params)) {
      return Object.assign({}, params, { _meta: meta });
    }
    return params;
  }

  async function call<M extends WireMethodName>(
    method: M,
    params: WireParams<M>,
    callOptions: RpcCallOptions = {},
  ): Promise<WireResult<M>> {
    if (closed) throw new RpcTransportError("client closed");
    const requiresIdempotency = wireMethodRequiresIdempotency(method);
    if (callOptions.idempotencyKey !== undefined && !requiresIdempotency) {
      throw new TypeError(`${method} does not accept an idempotency key`);
    }
    if (
      callOptions.idempotencyNamespace !== undefined &&
      callOptions.idempotencyKey === undefined
    ) {
      throw new TypeError("An idempotency namespace requires an idempotency key");
    }
    if (callOptions.lastEventId !== undefined) {
      if (!wireMethodAcceptsReplayCursor(method)) {
        throw new TypeError(`${method} does not accept a run replay cursor`);
      }
      if (requiresIdempotency && callOptions.idempotencyKey === undefined) {
        throw new TypeError("A run command replay cursor requires an idempotency key");
      }
      if (callOptions.lastEventId !== "") {
        if (callOptions.lastEventId.length > MAXIMUM_RUN_EVENT_ID_CHARACTERS) {
          throw new TypeError(
            `Run replay cursor exceeds ${MAXIMUM_RUN_EVENT_ID_CHARACTERS} characters`,
          );
        }
        if (!callOptions.lastEventId.startsWith(RUN_EVENT_ID_PREFIX)) {
          throw new TypeError("Run replay cursor has invalid event-id framing");
        }
      }
    }
    const id = requestIds.issue().toString();
    callOptions.onRequestRpcId?.(id);
    const req: TransportRequest = {
      jsonrpc: JSONRPC_VERSION,
      id,
      method,
      ...(() => {
        const withMeta = paramsWithMeta(params, callOptions.requestMeta);
        return withMeta !== undefined ? { params: withMeta } : {};
      })(),
    };

    return new Promise<WireResult<M>>((resolve, reject) => {
      const { signal } = callOptions;
      // Aborting the transport request propagates cancellation through the
      // server request context; no second cancellation protocol is needed.
      const onAbort = () => {
        if (!pending.has(id)) return;
        pending.delete(id);
        reject(new RpcTransportError("aborted"));
      };
      // Detach the abort listener once the request settles by any path —
      // otherwise a long-lived / shared signal accumulates one dead
      // listener per completed call ({ once: true } only fires on abort).
      const detach = () => signal?.removeEventListener("abort", onAbort);
      pending.set(id, {
        method,
        resolve: (value) => {
          detach();
          (resolve as (v: unknown) => void)(value);
        },
        reject: (err) => {
          detach();
          reject(err);
        },
      });

      if (signal) {
        if (signal.aborted) {
          onAbort();
          return;
        }
        signal.addEventListener("abort", onAbort, { once: true });
      }

      transport
        .send(req, signal, {
          idempotencyKey: callOptions.idempotencyKey,
          idempotencyNamespace: callOptions.idempotencyNamespace,
          lastEventId: callOptions.lastEventId,
        })
        .catch((err) => {
          if (!pending.has(id)) return; // already aborted/settled
          pending.delete(id);
          detach();
          reject(err);
        });
    });
  }

  function subscribe<M extends WireNotificationName>(
    method: M,
    observer: NotificationObserver<M>,
  ): () => void {
    // The map is heterogeneous by method. Erasure happens once, here; dispatch only
    // invokes the observer after the generated validator for this same key succeeds.
    const validatedObserver = observer as NotificationObserver;
    let set = subscribers.get(method);
    if (!set) {
      set = new Set();
      subscribers.set(method, set);
    }
    set.add(validatedObserver);
    return () => {
      const current = subscribers.get(method);
      if (!current) return;
      current.delete(validatedObserver);
      if (current.size === 0) subscribers.delete(method);
    };
  }

  function onStreamEnd(handler: StreamEndHandler): () => void {
    streamEndHandlers.add(handler);
    return () => streamEndHandlers.delete(handler);
  }

  function close(): Promise<void> {
    closePromise ??= (async () => {
      // `closed` is the request-admission / correlation state and can already
      // be true because recv() ended. Transport ownership is independent: the
      // public close contract must still run and join its one teardown.
      if (!closed) failConnection(new RpcTransportError("client closed"));
      let closeFailure: unknown;
      try {
        await transport.close();
      } catch (error) {
        closeFailure = error;
      }
      // The receive pump is created by RpcClient, not by Transport. Transport
      // close makes recv() terminal; joining the consumer here guarantees that
      // close() owns the complete lifecycle and no final iterator continuation
      // escapes into the next client/test generation.
      await receiveLoop;
      if (closeFailure !== undefined) throw closeFailure;
    })();
    return closePromise;
  }

  return { call, subscribe, onStreamEnd, close };
}
