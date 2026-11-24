import type {
  RuntimeEvent,
  RuntimeSubscribeRequest,
  RuntimeSubscribeResponse,
  RuntimeTopic,
} from "@flame/runtime-client/wire";
import { createPushPullChannel, type PushPullChannel } from "./channel";
import { RpcConnectionError } from "./errors";
import type { StreamingResult } from "./methods";
import { iterableOf, MAXIMUM_BUFFERED_RUNTIME_EVENTS } from "./stream";

type RuntimeEventSubscription = StreamingResult<RuntimeSubscribeResponse, RuntimeEvent>;

type OpenRuntimeEvents = (
  request: RuntimeSubscribeRequest,
  signal?: AbortSignal,
) => Promise<RuntimeEventSubscription>;

class Subscriber {
  readonly channel: PushPullChannel<RuntimeEvent> = createPushPullChannel({
    capacity: MAXIMUM_BUFFERED_RUNTIME_EVENTS,
  });
  #sequence = 0;

  constructor(private readonly topics: ReadonlySet<RuntimeTopic>) {}

  // A dedicated subscription numbers only the frames it receives, so a shared
  // stream renumbers each subscriber's subset to keep gaps meaning loss.
  deliver(event: RuntimeEvent): boolean {
    const routed = this.#route(event);
    return routed === undefined || this.channel.tryPush({ ...routed, sequence: ++this.#sequence });
  }

  #route(event: RuntimeEvent): RuntimeEvent | undefined {
    if (event.type !== "resync") return this.topics.has(event.type) ? event : undefined;
    const missed = event.topics.filter((topic) => this.topics.has(topic));
    return missed.length > 0
      ? { type: "resync", sequence: event.sequence, topics: missed }
      : undefined;
  }
}

class SharedStream {
  readonly subscribers = new Set<Subscriber>();
  readonly #controller = new AbortController();
  readonly opened: Promise<RuntimeSubscribeResponse>;

  constructor(
    topics: readonly RuntimeTopic[],
    open: OpenRuntimeEvents,
    private readonly ended: (stream: SharedStream) => void,
  ) {
    this.opened = open({ topics: [...topics] }, this.#controller.signal).then(
      ({ result, events }) => {
        void this.#forward(events);
        return result;
      },
      (error: unknown) => {
        this.ended(this);
        throw error;
      },
    );
  }

  leave(subscriber: Subscriber): void {
    if (!this.subscribers.delete(subscriber) || this.subscribers.size > 0) return;
    this.ended(this);
    this.#controller.abort();
  }

  async #forward(events: AsyncIterable<RuntimeEvent>): Promise<void> {
    let failure: unknown = new RpcConnectionError("runtime subscription ended unexpectedly");
    try {
      for await (const event of events) {
        for (const subscriber of this.subscribers) {
          if (subscriber.deliver(event)) continue;
          subscriber.channel.fail(
            new RpcConnectionError("runtime event consumer exceeded its bounded inbox"),
          );
          this.leave(subscriber);
        }
      }
    } catch (error) {
      failure = error;
    }
    this.ended(this);
    for (const subscriber of this.subscribers) subscriber.channel.fail(failure);
    this.subscribers.clear();
  }
}

// Browsers allow six HTTP/1.1 connections per origin and each subscription holds
// one open, so subscribers without file watches share one stream over every
// negotiated topic instead of exhausting the connections their requests need.
export function createRuntimeEventFeed(
  open: OpenRuntimeEvents,
  negotiatedTopics: () => readonly RuntimeTopic[] | undefined,
): { subscribe: OpenRuntimeEvents } {
  let current: SharedStream | undefined;
  const ended = (stream: SharedStream) => {
    if (current === stream) current = undefined;
  };
  return {
    async subscribe(request, signal) {
      const topics = negotiatedTopics();
      if (
        request.watches?.length ||
        topics === undefined ||
        !request.topics.every((topic) => topics.includes(topic))
      )
        return open(request, signal);
      signal?.throwIfAborted();
      const stream = (current ??= new SharedStream(topics, open, ended));
      const subscriber = new Subscriber(new Set(request.topics));
      stream.subscribers.add(subscriber);
      const leave = () => stream.leave(subscriber);
      const abandon = () => {
        subscriber.channel.close();
        leave();
      };
      signal?.addEventListener("abort", abandon, { once: true });
      let result: RuntimeSubscribeResponse;
      try {
        result = await stream.opened;
        signal?.throwIfAborted();
      } catch (error) {
        abandon();
        throw error;
      }
      return {
        result,
        events: iterableOf(subscriber.channel, () => {
          signal?.removeEventListener("abort", abandon);
          leave();
        }),
      };
    },
  };
}
