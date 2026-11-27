import { canonicalJSON } from "@frame/sdk/json";
import {
  errorMessage,
  RpcError,
  RpcProtocolError,
  RpcTransportError,
  RpcCancellationError,
  RpcConnectionError,
} from "./errors";
import { requestRequiresIdempotency } from "./request";
import {
  MAXIMUM_RUN_EVENT_ID_CHARACTERS,
  PROBLEM_CODES,
  NOTIFICATIONS_RUN_EVENT,
  RUN_EVENT_ID_PREFIX,
  runEventReliability,
  type RPCFailureData,
  type RequestMeta,
} from "@flame/runtime-client/wire";
import type {
  Transport,
  TransportMessageMetadata,
  TransportRequest,
  TransportResponseMetadata,
} from "./transport";
import {
  wireAcknowledgementCheck,
  wireMethodAcceptsReplayCursor,
  wireMethodReturnsValue,
  isWireStreamingMethodName,
  type WireMethodName,
  type WireParams,
  type WireResult,
} from "@flame/runtime-client/methods";
import {
  projectMethodResult,
  projectNotificationParams,
  projectWire,
  validateMethodResult,
  validateNotificationParams,
  validateWire,
  type WireNotificationName,
  type WireNotificationParams,
} from "./generated/wire.validate.generated";
import {
  JSONRPC_VERSION,
  isResponse,
  type RpcErrorPayload,
  type RpcId,
  type RpcMessage,
  type RpcNotification,
} from "./rpcMessage";

interface StreamConsumer<M extends WireNotificationName> {
  next(
    params: WireNotificationParams[M],
    metadata?: TransportMessageMetadata,
  ): void | Promise<void>;
  fail(error: Error): void;
  end(error?: Error): void;
}

export interface StreamObservation {
  deliver(message: RpcNotification, metadata?: TransportMessageMetadata): void | Promise<void>;
  end(error?: Error): void;
}

export function observeNotifications<M extends WireNotificationName>(
  method: M,
  consumer: StreamConsumer<M>,
): StreamObservation {
  return {
    deliver(message, metadata) {
      if (message.method !== method) {
        consumer.fail(
          new RpcProtocolError(
            method,
            [{ path: "$.method", detail: "does not match the request's notification method" }],
            metadata?.requestId,
          ),
        );
        return;
      }

      let params: WireNotificationParams[M];
      try {
        const violations = validateNotificationParams(method, message.params);
        if (violations.length > 0)
          throw new RpcProtocolError(`${method} params`, violations, metadata?.requestId);
        params = projectNotificationParams(method, message.params) as WireNotificationParams[M];
      } catch (cause) {
        const failure =
          cause instanceof RpcProtocolError
            ? cause
            : protocolFailure(
                `${method} params`,
                "number cannot be represented by the typed wire projection",
                metadata?.requestId,
                cause,
              );
        if (
          method === NOTIFICATIONS_RUN_EVENT &&
          runEventReliability(notificationEventType(message.params)) === "ephemeral"
        ) {
          console.warn(`[rpc] dropping invalid ephemeral notification: ${failure.message}`);
        } else {
          consumer.fail(failure);
        }
        return;
      }

      const failDelivery = (cause: unknown) => {
        const failure = new RpcConnectionError(
          `notification consumer failed: ${errorMessage(cause)}`,
          metadata?.requestId,
        );
        failure.cause = cause;
        consumer.fail(failure);
      };
      try {
        return consumer.next(params, metadata)?.catch(failDelivery);
      } catch (cause) {
        failDelivery(cause);
      }
    },
    end: consumer.end,
  };
}

function notificationEventType(params: unknown): unknown {
  if (params === null || typeof params !== "object" || Array.isArray(params)) return undefined;
  const event = (params as Record<string, unknown>).event;
  if (event === null || typeof event !== "object" || Array.isArray(event)) return undefined;
  return (event as Record<string, unknown>).type;
}

export interface RpcCallOptions<Result = unknown> {
  signal?: AbortSignal;
  idempotencyKey?: string;
  idempotencyNamespace?: string;
  lastEventId?: string;
  requestMeta?: RequestMeta | null;
  observe?: (result: Result) => StreamObservation;
  // Durable command admission must finish before transport can observe the request.
  beforeDispatch?: () => void;
}

export interface RpcClient {
  call<M extends WireMethodName>(
    method: M,
    params: WireParams<M>,
    options?: RpcCallOptions<WireResult<M>>,
  ): Promise<WireResult<M>>;
  callJSON<M extends WireMethodName>(
    method: M,
    params: WireParams<M>,
    options?: Pick<RpcCallOptions, "signal" | "requestMeta">,
  ): Promise<string>;
  close(): Promise<void>;
}

export type ResultDelivery = "value" | "json";

interface AwaitingResponse {
  state: "awaiting";
  method: WireMethodName;
  delivery: ResultDelivery;
  acknowledgement: ReturnType<typeof wireAcknowledgementCheck>;
  resolve: (value: unknown) => void;
  reject: (err: unknown) => void;
}

interface ObservingStream {
  state: "observing";
  observation: StreamObservation;
  detach: () => void;
}

type RequestState = AwaitingResponse | ObservingStream;

function protocolFailure(
  subject: string,
  detail: string,
  requestId: string | undefined,
  cause: unknown,
): RpcProtocolError {
  const error = new RpcProtocolError(subject, [{ path: "$", detail }], requestId);
  error.cause = cause;
  return error;
}

// A problem response must name a declared problem type with its own code.
function problemViolations(method: WireMethodName, payload: RpcErrorPayload) {
  const violations = validateWire("RPCFailureData", payload.data);
  if (violations.length > 0) return violations;
  const { problem } = payload.data as RPCFailureData;
  const expectedCode = PROBLEM_CODES[problem.type as keyof typeof PROBLEM_CODES];
  if (
    expectedCode !== undefined &&
    payload.code === expectedCode &&
    payload.message === problem.type
  )
    return violations;
  return [
    {
      path: `${method}.error`,
      detail: "code and message must identify the declared problem type",
    },
  ];
}

function rejectWithProblem(
  entry: AwaitingResponse,
  payload: RpcErrorPayload,
  requestId: string | undefined,
): void {
  const violations = problemViolations(entry.method, payload);
  if (violations.length > 0) {
    entry.reject(new RpcProtocolError(`${entry.method} error`, violations, requestId));
    return;
  }
  let data: RPCFailureData;
  try {
    data = projectWire("RPCFailureData", payload.data) as RPCFailureData;
  } catch (cause) {
    entry.reject(
      protocolFailure(
        `${entry.method} error`,
        "number cannot be represented by the typed wire projection",
        requestId,
        cause,
      ),
    );
    return;
  }
  entry.reject(
    new RpcError(
      {
        code: payload.code,
        message: payload.message,
        data: data.problem,
        recordedOutcome: data.recordedOutcome,
      },
      requestId,
    ),
  );
}

function settleResult(
  entry: AwaitingResponse,
  result: unknown,
  requestId: string | undefined,
): void {
  let violations;
  try {
    violations = validateMethodResult(entry.method, result);
  } catch (cause) {
    entry.reject(
      protocolFailure(
        `${entry.method} result`,
        "numeric evidence cannot be validated",
        requestId,
        cause,
      ),
    );
    return;
  }
  if (violations.length > 0) {
    entry.reject(new RpcProtocolError(`${entry.method} result`, violations, requestId));
    return;
  }
  const mismatch = entry.acknowledgement?.(result);
  if (mismatch !== undefined) {
    entry.reject(
      new RpcProtocolError(
        `${entry.method} result`,
        [{ path: `${entry.method}.result.${mismatch}`, detail: "must match the request" }],
        requestId,
      ),
    );
    return;
  }
  try {
    entry.resolve(
      entry.delivery === "json"
        ? canonicalJSON(result)
        : wireMethodReturnsValue(entry.method)
          ? projectMethodResult(entry.method, result)
          : undefined,
    );
  } catch (cause) {
    entry.reject(
      protocolFailure(
        `${entry.method} result`,
        "number cannot be represented by the typed wire projection",
        requestId,
        cause,
      ),
    );
  }
}

function assertCallOptions<M extends WireMethodName>(
  method: M,
  params: WireParams<M>,
  callOptions: RpcCallOptions<WireResult<M>>,
): void {
  const requiresIdempotency = requestRequiresIdempotency(method, params);
  if (callOptions.observe && !isWireStreamingMethodName(method))
    throw new TypeError("notification observation requires a streaming method");
  if (callOptions.idempotencyKey !== undefined && !requiresIdempotency) {
    throw new TypeError(`${method} does not accept an idempotency key`);
  }
  if (callOptions.idempotencyNamespace !== undefined && callOptions.idempotencyKey === undefined) {
    throw new TypeError("An idempotency namespace requires an idempotency key");
  }
  if (callOptions.lastEventId !== undefined)
    assertReplayCursor(
      method,
      requiresIdempotency,
      callOptions.idempotencyKey,
      callOptions.lastEventId,
    );
}

function assertReplayCursor(
  method: WireMethodName,
  requiresIdempotency: boolean,
  idempotencyKey: string | undefined,
  lastEventId: string,
): void {
  if (!wireMethodAcceptsReplayCursor(method)) {
    throw new TypeError(`${method} does not accept a run replay cursor`);
  }
  if (requiresIdempotency && idempotencyKey === undefined) {
    throw new TypeError("A run command replay cursor requires an idempotency key");
  }
  if (lastEventId === "") return;
  if (lastEventId.length > MAXIMUM_RUN_EVENT_ID_CHARACTERS) {
    throw new TypeError(`Run replay cursor exceeds ${MAXIMUM_RUN_EVENT_ID_CHARACTERS} characters`);
  }
  if (!lastEventId.startsWith(RUN_EVENT_ID_PREFIX)) {
    throw new TypeError("Run replay cursor has invalid event-id framing");
  }
}

export function createRpcClient(transport: Transport): RpcClient {
  let lastRequestId = 0n;
  const requests = new Map<RpcId, RequestState>();
  const consumers = new Set<Promise<void>>();
  const observerFailures: unknown[] = [];
  let connectionFailure: RpcTransportError | undefined;
  let closePromise: Promise<void> | undefined;

  function requireOpen(): void {
    if (connectionFailure) throw connectionFailure;
  }

  function endRequest(
    request: RequestState,
    error?: Error,
    metadata?: TransportResponseMetadata,
  ): void {
    if (request.state === "awaiting") {
      request.reject(
        error ??
          new RpcTransportError(
            "event stream ended before the call's response",
            undefined,
            metadata?.requestId,
          ),
      );
      return;
    }
    request.detach();
    try {
      request.observation.end(error);
    } catch (cause) {
      observerFailures.push(cause);
    }
  }

  function failConnection(failure: RpcTransportError): void {
    if (connectionFailure) return;
    connectionFailure = failure;
    const outstanding = [...requests.values()];
    requests.clear();
    for (const request of outstanding) endRequest(request, failure);
  }

  const receiveLoop = (async () => {
    try {
      for await (const event of transport.recv()) {
        const request = requests.get(event.requestRpcId);
        if (event.type === "streamEnd") {
          if (request) {
            requests.delete(event.requestRpcId);
            endRequest(request, event.error, event.metadata);
          }
          continue;
        }

        let delivered: void | Promise<void>;
        try {
          delivered =
            request && dispatchMessage(request, event.message, event.requestRpcId, event.metadata);
        } catch (cause) {
          event.consumed?.(Promise.reject(cause));
          throw cause;
        }
        if (delivered) {
          consumers.add(delivered);
          void delivered.then(
            () => consumers.delete(delivered),
            () => consumers.delete(delivered),
          );
        }
        if (event.consumed) event.consumed(delivered ?? Promise.resolve());
        else if (delivered)
          void delivered.catch((cause: unknown) =>
            failConnection(new RpcConnectionError(errorMessage(cause))),
          );
      }
      failConnection(new RpcTransportError("transport stream ended"));
    } catch (err) {
      failConnection(new RpcTransportError(`transport recv() failed: ${errorMessage(err)}`));
    }
  })();

  function dispatchMessage(
    request: RequestState,
    message: RpcMessage,
    requestRpcId: RpcId,
    metadata?: TransportMessageMetadata,
  ): void | Promise<void> {
    if (request.state === "observing") {
      if (!isResponse(message)) return request.observation.deliver(message, metadata);
      requests.delete(requestRpcId);
      endRequest(
        request,
        new RpcProtocolError(
          "event stream",
          [{ path: "$", detail: "must contain only notifications after acknowledgement" }],
          metadata?.requestId,
        ),
      );
      return;
    }

    if (!isResponse(message)) {
      request.reject(
        new RpcProtocolError(
          `${request.method} response`,
          [{ path: "$", detail: "must acknowledge the call before any other message" }],
          metadata?.requestId,
        ),
      );
      return;
    }
    if (message.id !== requestRpcId) {
      request.reject(
        new RpcProtocolError(
          `${request.method} response`,
          [{ path: "$.id", detail: `must match request id ${requestRpcId}` }],
          metadata?.requestId,
        ),
      );
      return;
    }
    if ("error" in message) {
      rejectWithProblem(request, message.error, metadata?.requestId);
    } else {
      settleResult(request, message.result, metadata?.requestId);
    }
  }

  async function request<M extends WireMethodName>(
    method: M,
    params: WireParams<M>,
    callOptions: RpcCallOptions<WireResult<M>> = {},
    delivery: ResultDelivery = "value",
  ): Promise<WireResult<M> | string> {
    requireOpen();
    assertCallOptions(method, params, callOptions);
    const { requestMeta } = callOptions;
    const id = (++lastRequestId).toString();
    const req: TransportRequest = {
      jsonrpc: JSONRPC_VERSION,
      id,
      method,
      params: requestMeta ? { ...params, _meta: requestMeta } : params,
    };

    return new Promise<WireResult<M> | string>((resolve, reject) => {
      const { signal, observe } = callOptions;
      const detach = () => signal?.removeEventListener("abort", onAbort);
      const rejectAwaiting = (error: unknown) => {
        requests.delete(id);
        detach();
        reject(error);
      };
      const onAbort = () => {
        const request = requests.get(id);
        if (request?.state === "awaiting") rejectAwaiting(new RpcCancellationError(signal?.reason));
        else requests.delete(id);
      };
      requests.set(id, {
        state: "awaiting",
        method,
        delivery,
        acknowledgement: wireAcknowledgementCheck(method, req.params),
        resolve: (value) => {
          if (observe) {
            let observation: StreamObservation;
            try {
              observation = observe(value as WireResult<M>);
            } catch (error) {
              rejectAwaiting(error);
              return;
            }
            requests.set(id, { state: "observing", observation, detach });
          } else {
            requests.delete(id);
            detach();
          }
          resolve(value as WireResult<M> | string);
        },
        reject: rejectAwaiting,
      });

      if (signal) {
        if (signal.aborted) {
          onAbort();
          return;
        }
        signal.addEventListener("abort", onAbort, { once: true });
      }

      const sendFailed = (error: unknown) => {
        const request = requests.get(id);
        if (request?.state === "awaiting") request.reject(error);
      };
      try {
        callOptions.beforeDispatch?.();
        void transport
          .send(req, signal, {
            idempotencyKey: callOptions.idempotencyKey,
            idempotencyNamespace: callOptions.idempotencyNamespace,
            lastEventId: callOptions.lastEventId,
          })
          .catch(sendFailed);
      } catch (error) {
        sendFailed(error);
      }
    });
  }

  function call<M extends WireMethodName>(
    method: M,
    params: WireParams<M>,
    options?: RpcCallOptions<WireResult<M>>,
  ): Promise<WireResult<M>> {
    return request(method, params, options) as Promise<WireResult<M>>;
  }
  function callJSON<M extends WireMethodName>(
    method: M,
    params: WireParams<M>,
    options?: Pick<RpcCallOptions, "signal" | "requestMeta">,
  ): Promise<string> {
    return request(method, params, options, "json") as Promise<string>;
  }

  function close(): Promise<void> {
    if (closePromise) return closePromise;
    const completion = Promise.withResolvers<void>();
    closePromise = completion.promise;
    failConnection(new RpcTransportError("client closed"));
    void (async () => {
      const failures: unknown[] = [];
      try {
        await transport.close();
      } catch (error) {
        failures.push(error);
      }
      try {
        await receiveLoop;
      } catch (error) {
        failures.push(error);
      }
      for (const outcome of await Promise.allSettled(consumers))
        if (outcome.status === "rejected") failures.push(outcome.reason);
      failures.push(...observerFailures);
      if (failures.length === 1) throw failures[0];
      if (failures.length > 1) throw new AggregateError(failures, "rpc client shutdown failed");
    })().then(completion.resolve, completion.reject);
    return closePromise;
  }

  return { call, callJSON, close };
}
