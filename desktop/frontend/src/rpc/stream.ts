// API.md §5 / §10. There is no "run closed" method: the terminal signal is a
// `segment.finished` for the ROOT SEGMENT on the same stream. Tree membership is
// authoritative from the REQUEST ID the transport stamps on every message, never from
// earlier `segment.started` events — those are already gone when a reattach begins.

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

/** Previews have no replay identity and may arrive indefinitely, so saturation drops ONLY
 * previews. */
export const MAXIMUM_BUFFERED_EPHEMERAL_RUN_EVENTS = 256;

/** A consumer omitting discovery must not turn the absence of a negotiated replay promise
 * into unbounded client retention. Overflow stays observable and recoverable. */
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

  /** A subagent's `segment.finished` carries a different segmentId, so it never closes
   *  the tree. */
  isRootFinish(ev: RunEvent): boolean {
    return ev.segmentId === this.rootSegmentId && ev.event.type === "segment.finished";
  }
}

/** Self-cleaning: `cleanup` runs once when the iterator drains or the consumer breaks. */
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
  /** Releases transport registrations WITHOUT discarding buffered channel values. */
  cleanup(): void;
  close(): void;
  fail(error: unknown): void;
  bind(unsub: () => void): void;
}

/** Source termination must release registrations IMMEDIATELY, even when no consumer ever
 * asks the iterator for `done`. Binding is deferred until both registrations exist, so a
 * termination that arrives first is remembered and performed once they attach. */
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
  /** Ends ONLY this stream, without mutating the caller-owned signal. */
  abort(): void;
}

function createStreamLifetime(parent?: AbortSignal): StreamLifetime {
  const controller = new AbortController();
  return {
    signal: parent ? AbortSignal.any([parent, controller.signal]) : controller.signal,
    abort: () => controller.abort(),
  };
}

/** `dispose` exists for the case where the owning call FAILS before anyone iterates
 *  `events`: `iterableOf`'s cleanup only runs on iteration, so the subscription and its
 *  pre-bind buffer would otherwise leak. */
export interface RunEventStream {
  events: AsyncIterable<RunEvent>;
  requestSignal: AbortSignal;
  dispose: () => void;
}

export interface RunEventStreamOptions {
  signal?: AbortSignal;
  /** From `capabilities.limits.runReplay`. Without discovery the SDK applies a bounded
   *  LOCAL envelope rather than claiming a replay promise the Runtime never advertised. */
  replayLimits?: RunReplayLimits;
}

/**
 * Subscribes BEFORE the root segment id is known, buffering until `bind(segmentId)` supplies
 * the terminal identity. Under streamable HTTP the response and its event frames share ONE
 * ordered stream (TRANSPORT.md §6.4), so head events land immediately after the response
 * and subscribing once it resolves would drop them.
 */
export function streamRunEvents(
  client: RpcClient,
  options: RunEventStreamOptions = {},
): RunEventStream & {
  bindRequest: (requestRpcId: RpcId) => void;
  bind: (rootSegmentId: string) => void;
} {
  const lifetime = createStreamLifetime(options.signal);
  // Validated BEFORE any transport registration exists, so a malformed capability cannot
  // leave a half-open stream behind.
  const inbox = new RunEventInbox(options.replayLimits);
  const channel = inbox.channel;
  // A root finish is the final event in its response stream, so retaining only the latest
  // finished segment is sufficient and cannot grow with a wide subagent tree.
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

/** Connection-scoped and LOSSY: no terminal frame, no replay. The stream ends when its POST
 *  stream does, and the consumer resubscribes, treating reconnect as `resync`. */
export interface RuntimeEventStream {
  events: AsyncIterable<RuntimeEvent>;
  requestSignal: AbortSignal;
  dispose: () => void;
}

/** Deliberately small: sustained lag is recovered by ending this generation and
 * resubscribing with an explicit resync, NEVER by retaining a second invalidation log. */
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
