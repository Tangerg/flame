import { createPushPullChannel, type PushPullChannel } from "./channel";
import type { RpcClient } from "./client";
import type { RpcId } from "./types";
import {
  runEventIsReplayable,
  type RunEvent,
  type RunReplayLimits,
  type RuntimeEvent,
} from "@flame/runtime-contract/wire";
import { RUNTIME_SUBSCRIBE_METHOD } from "./transport";
import { NOTIFICATIONS_RUN_EVENT, NOTIFICATIONS_RUNTIME_EVENT } from "@flame/runtime-contract/wire";
import { RpcConnectionError } from "./errors";

export const RUN_EVENT_METHOD = NOTIFICATIONS_RUN_EVENT;
export const RUNTIME_EVENT_METHOD = NOTIFICATIONS_RUNTIME_EVENT;

interface RunReplayBudget {
  maxEvents: number;
  maxBytes: number;
}

class RunReplayMemory {
  private readonly delivered = new Map<string, number>();
  private readonly encoder = new TextEncoder();
  private readonly maxEvents: number;
  private readonly maxBytes: number;
  private retainedBytes = 0;

  constructor(budget: RunReplayBudget) {
    this.maxEvents = budget.maxEvents;
    this.maxBytes = budget.maxBytes;
    for (const [name, value] of [
      ["event", budget.maxEvents],
      ["byte", budget.maxBytes],
    ] as const) {
      if (!Number.isSafeInteger(value) || value <= 0) {
        throw new RangeError(`run replay ${name} capacity must be a positive safe integer`);
      }
    }
  }

  alreadyDelivered(eventId: string): boolean {
    if (this.delivered.has(eventId)) return true;
    const bytes = this.encoder.encode(eventId).byteLength;
    this.delivered.set(eventId, bytes);
    this.retainedBytes += bytes;
    while (this.delivered.size > this.maxEvents || this.retainedBytes > this.maxBytes) {
      const oldest = this.delivered.keys().next();
      if (oldest.done) break;
      const oldestBytes = this.delivered.get(oldest.value);
      this.delivered.delete(oldest.value);
      this.retainedBytes -= oldestBytes ?? 0;
    }
    return false;
  }
}

export const MAXIMUM_BUFFERED_EPHEMERAL_RUN_EVENTS = 256;

const MAXIMUM_UNNEGOTIATED_REPLAY_IDENTITIES = 2_048;
const MAXIMUM_UNNEGOTIATED_REPLAY_ID_BYTES = 16 * 1024 * 1024;

type RunEventAdmission = "accepted" | "duplicate" | "ephemeralDropped" | "authoritativeOverflow";

class RunEventInbox {
  readonly channel: PushPullChannel<RunEvent>;
  private readonly replayMemory: RunReplayMemory;

  constructor(limits?: RunReplayLimits) {
    const replayBudget: RunReplayBudget = limits ?? {
      maxEvents: MAXIMUM_UNNEGOTIATED_REPLAY_IDENTITIES,
      maxBytes: MAXIMUM_UNNEGOTIATED_REPLAY_ID_BYTES,
    };
    this.replayMemory = new RunReplayMemory(replayBudget);
    const capacity = replayBudget.maxEvents + MAXIMUM_BUFFERED_EPHEMERAL_RUN_EVENTS;
    if (!Number.isSafeInteger(capacity)) {
      throw new RangeError("run event inbox capacity must be a positive safe integer");
    }
    this.channel = createPushPullChannel<RunEvent>({ capacity });
  }

  admit(event: RunEvent): RunEventAdmission {
    const replayable = runEventIsReplayable(event.event.type) === true;
    if (replayable && this.replayMemory.alreadyDelivered(event.eventId)) return "duplicate";
    if (this.channel.tryPush(event)) return "accepted";
    return replayable ? "authoritativeOverflow" : "ephemeralDropped";
  }
}

class BoundRunResponse {
  constructor(private readonly rootSegmentId: string) {}

  isRootFinish(ev: RunEvent): boolean {
    return ev.segmentId === this.rootSegmentId && ev.event.type === "segment.finished";
  }
}

function iterableOf<T>(channel: PushPullChannel<T>, cleanup: () => void): AsyncIterable<T> {
  return {
    [Symbol.asyncIterator]() {
      const inner = channel.iterator();
      return {
        [Symbol.asyncIterator]() {
          return this;
        },
        next: async (): Promise<IteratorResult<T>> => {
          try {
            const result = await inner.next();
            if (result.done) cleanup();
            return result;
          } catch (error) {
            cleanup();
            throw error;
          }
        },
        return: async (): Promise<IteratorResult<T>> => {
          channel.close();
          cleanup();
          return { value: undefined as never, done: true };
        },
      };
    },
  };
}

interface StreamLifecycle {
  cleanup(): void;
  close(): void;
  fail(error: unknown): void;
  bind(unsub: () => void): void;
}

function createStreamLifecycle<T>(
  channel: PushPullChannel<T>,
  lifetime: StreamLifetime,
): StreamLifecycle {
  const signal = lifetime.signal;
  let bound = false;
  let cleanupRequested = false;
  let cleaned = false;
  let unsub: () => void = () => undefined;

  const cleanup = () => {
    if (cleaned) return;
    if (!bound) {
      cleanupRequested = true;
      return;
    }
    cleaned = true;
    unsub();
    signal.removeEventListener("abort", onAbort);
    lifetime.abort();
  };
  const onAbort = () => {
    channel.close();
    cleanup();
  };

  if (signal.aborted) onAbort();
  else signal.addEventListener("abort", onAbort, { once: true });

  return {
    cleanup,
    close: () => {
      channel.close();
      cleanup();
    },
    fail: (error) => {
      channel.fail(error);
      cleanup();
    },
    bind: (nextUnsub) => {
      if (bound) throw new Error("stream lifecycle is already bound");
      bound = true;
      unsub = nextUnsub;
      if (cleanupRequested) cleanup();
    },
  };
}

interface StreamLifetime {
  signal: AbortSignal;
  abort(): void;
}

function createStreamLifetime(parent?: AbortSignal): StreamLifetime {
  const controller = new AbortController();
  return {
    signal: parent ? AbortSignal.any([parent, controller.signal]) : controller.signal,
    abort: () => controller.abort(),
  };
}

export interface RunEventStream {
  events: AsyncIterable<RunEvent>;
  requestSignal: AbortSignal;
  dispose: () => void;
}

export interface RunEventStreamOptions {
  signal?: AbortSignal;
  replayLimits?: RunReplayLimits;
}

export function streamRunEvents(
  client: RpcClient,
  options: RunEventStreamOptions = {},
): RunEventStream & {
  bindRequest: (requestRpcId: RpcId) => void;
  bind: (rootSegmentId: string) => void;
} {
  const lifetime = createStreamLifetime(options.signal);
  const inbox = new RunEventInbox(options.replayLimits);
  const channel = inbox.channel;
  let latestFinishedBeforeBind: string | undefined;
  let ownerRequestRpcId: RpcId | undefined;
  let response: BoundRunResponse | null = null;
  const lifecycle = createStreamLifecycle(channel, lifetime);

  const unsubEvents = client.subscribe(RUN_EVENT_METHOD, {
    next(event, requestRpcId) {
      if (channel.closed || requestRpcId !== ownerRequestRpcId) return;
      const admission = inbox.admit(event);
      if (admission === "authoritativeOverflow") {
        lifecycle.fail(
          new RpcConnectionError(
            "run event consumer exceeded its bounded inbox before an authoritative event",
          ),
        );
        return;
      }
      if (admission !== "accepted" || event.event.type !== "segment.finished") return;
      if (response === null) latestFinishedBeforeBind = event.segmentId;
      else if (response.isRootFinish(event)) lifecycle.close();
    },
    error: (error, requestRpcId) => {
      if (requestRpcId !== undefined && requestRpcId !== ownerRequestRpcId) return;
      lifecycle.fail(error);
    },
  });
  const unsubDown = client.onStreamEnd((event) => {
    if (channel.closed || event.requestRpcId !== ownerRequestRpcId) return;
    if (event.error) lifecycle.fail(event.error);
    else lifecycle.close();
  });

  const bind = (rootSegmentId: string): void => {
    if (response !== null) return;
    response = new BoundRunResponse(rootSegmentId);
    if (latestFinishedBeforeBind === rootSegmentId) lifecycle.close();
    latestFinishedBeforeBind = undefined;
  };

  lifecycle.bind(() => {
    unsubEvents();
    unsubDown();
  });
  return {
    events: iterableOf(channel, lifecycle.cleanup),
    requestSignal: lifetime.signal,
    bindRequest: (requestRpcId) => {
      if (ownerRequestRpcId !== undefined) {
        throw new Error("run event stream is already bound to a request");
      }
      ownerRequestRpcId = requestRpcId;
    },
    bind,
    dispose: lifecycle.close,
  };
}

export interface RuntimeEventStream {
  events: AsyncIterable<RuntimeEvent>;
  requestSignal: AbortSignal;
  dispose: () => void;
}

export const MAXIMUM_BUFFERED_RUNTIME_EVENTS = 64;

export function streamRuntimeEvents(
  client: RpcClient,
  signal?: AbortSignal,
): RuntimeEventStream & { bindRequest: (requestRpcId: RpcId) => void } {
  const lifetime = createStreamLifetime(signal);
  const channel = createPushPullChannel<RuntimeEvent>({
    capacity: MAXIMUM_BUFFERED_RUNTIME_EVENTS,
  });
  let ownerRequestRpcId: RpcId | undefined;
  const lifecycle = createStreamLifecycle(channel, lifetime);
  const unsubEvents = client.subscribe(RUNTIME_EVENT_METHOD, {
    next(params, requestRpcId) {
      if (channel.closed || requestRpcId !== ownerRequestRpcId) return;
      if (!channel.tryPush(params.event)) {
        lifecycle.fail(new RpcConnectionError("runtime event consumer exceeded its bounded inbox"));
      }
    },
    error: (error, requestRpcId) => {
      if (requestRpcId !== undefined && requestRpcId !== ownerRequestRpcId) return;
      lifecycle.fail(error);
    },
  });
  const unsubDown = client.onStreamEnd((event) => {
    if (channel.closed) return;
    if (event.method !== RUNTIME_SUBSCRIBE_METHOD || event.requestRpcId !== ownerRequestRpcId) {
      return;
    }
    if (event.error) lifecycle.fail(event.error);
    else lifecycle.close();
  });
  lifecycle.bind(() => {
    unsubEvents();
    unsubDown();
  });
  return {
    events: iterableOf(channel, lifecycle.cleanup),
    requestSignal: lifetime.signal,
    bindRequest: (requestRpcId) => {
      if (ownerRequestRpcId !== undefined) {
        throw new Error("runtime event stream is already bound to a request");
      }
      ownerRequestRpcId = requestRpcId;
    },
    dispose: lifecycle.close,
  };
}
