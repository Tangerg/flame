import {
  errorMessage,
  RpcError,
  RpcProtocolError,
  RpcTransportError,
  RpcConnectionError,
} from "./errors";
import { checkRequest, requestRequiresIdempotency } from "./request";
import { stringifyJSON } from "@qimu/sdk/json";
import {
  MAXIMUM_RUN_EVENT_ID_CHARACTERS,
  PROBLEM_CODES,
  NOTIFICATIONS_RUN_EVENT,
  RUN_EVENT_ID_PREFIX,
  runEventReliability,
  type ProblemData,
  type RequestMeta,
} from "@flame/runtime-client/wire";
import type {
  Transport,
  TransportEvent,
  TransportRequest,
  TransportResponseMetadata,
} from "./transport";
import {
  wireMethodAcceptsReplayCursor,
  wireMethodReturnsValue,
  isWireStreamingMethodName,
  type WireMethodName,
  type WireParams,
  type WireResult,
} from "@flame/runtime-client/methods";
import {
  isWireNotificationName,
  projectMethodResult,
  projectNotificationParams,
  projectWire,
  validateMethodResult,
  validateNotificationParams,
  validateWire,
  type WireNotificationName,
  type WireNotificationParams,
} from "./generated/wire.validate.generated";
import type { RpcId, RpcMessage } from "./types";
import { JSONRPC_VERSION, isErrorResponse, isNotification, isResponse } from "./types";

export interface NotificationObserver<M extends WireNotificationName = WireNotificationName> {
  next(params: WireNotificationParams[M], requestRpcId: RpcId): void | Promise<void>;
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
  idempotencyNamespace?: string;
  lastEventId?: string;
  requestMeta?: RequestMeta | null;
  onRequestRpcId?: (id: RpcId) => void;
}

export interface RpcClient {
  readonly closedSignal: AbortSignal;
  call<M extends WireMethodName>(
    method: M,
    params: WireParams<M>,
    options?: RpcCallOptions,
  ): Promise<WireResult<M>>;
  callJSON<M extends WireMethodName>(
    method: M,
    params: WireParams<M>,
    options?: Pick<RpcCallOptions, "signal" | "requestMeta">,
  ): Promise<string>;
  subscribe<M extends WireNotificationName>(
    method: M,
    observer: NotificationObserver<M>,
  ): () => void;
  onStreamEnd(handler: StreamEndHandler): () => void;
  close(): Promise<void>;
}

interface Pending {
  method: WireMethodName;
  exactJSON: boolean;
  resolve: (value: unknown) => void;
  reject: (err: unknown) => void;
}

export function createRpcClient(transport: Transport, options: RpcClientOptions = {}): RpcClient {
  let lastRequestId = 0n;
  const pending = new Map<RpcId, Pending>();
  const subscribers = new Map<WireNotificationName, Set<NotificationObserver>>();
  const streamEndHandlers = new Set<StreamEndHandler>();
  const consumers = new Set<Promise<void>>();
  const connection = new AbortController();
  let closePromise: Promise<void> | undefined;

  function failAllPending(failure: RpcTransportError): void {
    for (const { reject } of pending.values()) reject(failure);
    pending.clear();
  }

  function failConnection(failure: RpcTransportError): void {
    connection.abort(failure);
    failAllPending(failure);
    for (const observers of subscribers.values()) {
      for (const observer of observers) observer.error(failure);
    }
    subscribers.clear();
    streamEndHandlers.clear();
  }

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
    const delivery = dispatchMessage(event.message, event.requestRpcId, event.metadata);
    if (event.consumed) event.consumed(delivery ?? Promise.resolve());
    else if (delivery)
      void delivery.catch((cause: unknown) =>
        failConnection(new RpcConnectionError(errorMessage(cause))),
      );
  }

  function dispatchMessage(
    msg: RpcMessage,
    requestRpcId: RpcId,
    metadata?: TransportResponseMetadata,
  ): Promise<void> | undefined {
    if (isResponse(msg)) {
      const entry = pending.get(msg.id);
      if (!entry) return;
      pending.delete(msg.id);
      if (isErrorResponse(msg)) {
        const payload = msg.error;
        const violations = validateWire("ProblemData", payload.data);
        if (violations.length === 0) {
          const problem = payload.data as ProblemData;
          const expectedCode = PROBLEM_CODES[problem.type as keyof typeof PROBLEM_CODES];
          if (
            expectedCode === undefined ||
            payload.code !== expectedCode ||
            payload.message !== problem.type
          ) {
            violations.push({
              path: `${entry.method}.error`,
              detail: "code and message must identify the declared problem type",
            });
          }
        }
        if (violations.length > 0) {
          entry.reject(
            new RpcProtocolError(`${entry.method} error`, violations, metadata?.requestId),
          );
          return;
        }
        let problem: ProblemData;
        try {
          problem = projectWire("ProblemData", payload.data) as ProblemData;
        } catch (cause) {
          const error = new RpcProtocolError(
            `${entry.method} error`,
            [{ path: "$", detail: "number cannot be represented by the typed wire projection" }],
            metadata?.requestId,
          );
          error.cause = cause;
          entry.reject(error);
          return;
        }
        entry.reject(
          new RpcError(
            {
              code: payload.code,
              message: payload.message,
              data: problem,
            },
            metadata?.requestId,
          ),
        );
      } else {
        const result = msg.result;
        let violations;
        try {
          violations = validateMethodResult(entry.method, result);
        } catch (cause) {
          const error = new RpcProtocolError(
            `${entry.method} result`,
            [{ path: "$", detail: "numeric evidence cannot be validated" }],
            metadata?.requestId,
          );
          error.cause = cause;
          entry.reject(error);
          return;
        }
        if (violations.length > 0) {
          entry.reject(
            new RpcProtocolError(`${entry.method} result`, violations, metadata?.requestId),
          );
          return;
        }
        try {
          entry.resolve(
            entry.exactJSON
              ? stringifyJSON(result, true)
              : wireMethodReturnsValue(entry.method)
                ? projectMethodResult(entry.method, result)
                : undefined,
          );
        } catch (cause) {
          const error = new RpcProtocolError(
            `${entry.method} result`,
            [{ path: "$", detail: "number cannot be represented by the typed wire projection" }],
            metadata?.requestId,
          );
          error.cause = cause;
          entry.reject(error);
        }
      }
      return;
    }
    if (isNotification(msg)) {
      if (!isWireNotificationName(msg.method)) return;
      const handlers = subscribers.get(msg.method);
      if (!handlers) return;
      let params: WireNotificationParams[typeof msg.method] | undefined;
      let violations;
      try {
        violations = validateNotificationParams(msg.method, msg.params);
        if (violations.length === 0)
          params = projectNotificationParams(
            msg.method,
            msg.params,
          ) as WireNotificationParams[typeof msg.method];
      } catch {
        violations = [
          { path: "$", detail: "number cannot be represented by the typed wire projection" },
        ];
      }
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
      const deliveries: Promise<void>[] = [];
      for (const observer of handlers) {
        try {
          const delivery = observer.next(params!, requestRpcId);
          if (delivery) {
            const consumed = delivery.catch((cause: unknown) => {
              observer.error(
                new RpcConnectionError(`notification consumer failed: ${errorMessage(cause)}`),
                requestRpcId,
              );
            });
            consumers.add(consumed);
            void consumed.then(
              () => consumers.delete(consumed),
              () => consumers.delete(consumed),
            );
            deliveries.push(consumed);
          }
        } catch (err) {
          observer.error(
            new RpcConnectionError(`notification consumer failed: ${errorMessage(err)}`),
            requestRpcId,
          );
        }
      }
      return Promise.all(deliveries).then(() => undefined);
    }
    console.warn("[rpc] dropping unexpected server-initiated Request", msg);
  }

  async function request<M extends WireMethodName>(
    method: M,
    params: WireParams<M>,
    callOptions: RpcCallOptions = {},
    exactJSON = false,
  ): Promise<WireResult<M> | string> {
    if (connection.signal.aborted) throw new RpcTransportError("client closed");
    const requiresIdempotency = requestRequiresIdempotency(method, params);
    if (
      exactJSON &&
      (requiresIdempotency || isWireStreamingMethodName(method) || !wireMethodReturnsValue(method))
    )
      throw new TypeError("exact JSON delivery requires a unary query result");
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
    const requestMeta =
      callOptions.requestMeta === undefined ? options.requestMeta?.() : callOptions.requestMeta;
    checkRequest(method, params, requestMeta);
    const id = (++lastRequestId).toString();
    callOptions.onRequestRpcId?.(id);
    if (connection.signal.aborted) throw new RpcTransportError("client closed");
    const req: TransportRequest = {
      jsonrpc: JSONRPC_VERSION,
      id,
      method,
      params: requestMeta ? { ...params, _meta: requestMeta } : params,
    };

    return new Promise<WireResult<M> | string>((resolve, reject) => {
      const { signal } = callOptions;
      const rejectPending = (error: unknown) => {
        const request = pending.get(id);
        if (!request) return;
        pending.delete(id);
        request.reject(error);
      };
      const onAbort = () => rejectPending(new RpcTransportError("aborted"));
      const detach = () => signal?.removeEventListener("abort", onAbort);
      pending.set(id, {
        method,
        exactJSON,
        resolve: (value) => {
          detach();
          resolve(value as WireResult<M> | string);
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

      try {
        void transport
          .send(req, signal, {
            idempotencyKey: callOptions.idempotencyKey,
            idempotencyNamespace: callOptions.idempotencyNamespace,
            lastEventId: callOptions.lastEventId,
          })
          .catch(rejectPending);
      } catch (error) {
        rejectPending(error);
      }
    });
  }

  function call<M extends WireMethodName>(
    method: M,
    params: WireParams<M>,
    options?: RpcCallOptions,
  ): Promise<WireResult<M>> {
    return request(method, params, options) as Promise<WireResult<M>>;
  }
  function callJSON<M extends WireMethodName>(
    method: M,
    params: WireParams<M>,
    options?: Pick<RpcCallOptions, "signal" | "requestMeta">,
  ): Promise<string> {
    return request(method, params, options, true) as Promise<string>;
  }

  function subscribe<M extends WireNotificationName>(
    method: M,
    observer: NotificationObserver<M>,
  ): () => void {
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
      if (!connection.signal.aborted) failConnection(new RpcTransportError("client closed"));
      let closeFailure: unknown;
      try {
        await transport.close();
      } catch (error) {
        closeFailure = error;
      }
      await receiveLoop;
      await Promise.all(consumers);
      if (closeFailure !== undefined) throw closeFailure;
    })();
    return closePromise;
  }

  return { call, callJSON, subscribe, onStreamEnd, close, closedSignal: connection.signal };
}
