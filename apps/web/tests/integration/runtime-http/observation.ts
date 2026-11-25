import { onTestFinished } from "vitest";
import { type FlameClient, type SessionId } from "@flame/runtime-client";
import {
  type GoalStatus,
  type MCPServerID,
  type RunEvent,
  type RuntimeEvent,
  type RuntimeTopic,
} from "@flame/runtime-client/wire";

const runtimeNotReadyStatus = 503;

const readinessPollMilliseconds = 100;

export const defaultTestDeadlineMilliseconds = 5_000;

// A spawned process (the Runtime, its runner, a package backend) starts while
// every first-party backend starts beside it, so its readiness gets the
// startup budget rather than the in-process event deadline.
export const processStartupMilliseconds = 20_000;

function testDeadline(
  detail: string,
  timeoutMilliseconds = defaultTestDeadlineMilliseconds,
): { promise: Promise<never>; release: () => void } {
  let settled = false;
  let release!: () => void;
  let timer: ReturnType<typeof setTimeout> | undefined;
  const promise = new Promise<never>((resolve, reject) => {
    release = () => {
      if (settled) return;
      settled = true;
      if (timer !== undefined) clearTimeout(timer);
      timer = undefined;
      resolve(undefined as never);
    };
    timer = setTimeout(() => {
      timer = undefined;
      settled = true;
      reject(new Error(`timed out waiting for ${detail}`));
    }, timeoutMilliseconds);
  });
  return { promise, release };
}

export async function within<T>(
  promise: Promise<T>,
  detail: string,
  timeoutMilliseconds = defaultTestDeadlineMilliseconds,
): Promise<T> {
  const deadline = testDeadline(detail, timeoutMilliseconds);
  try {
    return await Promise.race([promise, deadline.promise]);
  } finally {
    deadline.release();
  }
}

export async function waitUntilReady(
  baseUrl: string,
  processError: () => string,
  readyBefore = Date.now() + processStartupMilliseconds,
): Promise<void> {
  const signal = AbortSignal.timeout(Math.max(0, readyBefore - Date.now()));
  while (Date.now() < readyBefore) {
    try {
      const response = await fetch(`${baseUrl}/v2/health/ready`, { signal });
      await response.arrayBuffer();
      if (response.ok) return;
      if (response.status !== runtimeNotReadyStatus)
        throw new Error(`unexpected runtime readiness status ${response.status}`);
    } catch (cause) {
      if (signal.aborted)
        throw new Error(`runtime did not become ready: ${processError()}`, { cause });
      if (!(
        cause instanceof TypeError &&
        cause.cause instanceof Error &&
        "code" in cause.cause &&
        cause.cause.code === "ECONNREFUSED"
      ))
        throw cause;
    }
    await new Promise((resolve) => setTimeout(resolve, readinessPollMilliseconds));
  }
  throw new Error(`runtime did not become ready: ${processError()}`);
}

async function observeRuntimeEvent<T>(
  iterator: AsyncIterator<RuntimeEvent>,
  detail: string,
  observe: (event: RuntimeEvent) => T | undefined | Promise<T | undefined>,
): Promise<T> {
  const deadline = testDeadline(detail);
  try {
    while (true) {
      const result = await Promise.race([iterator.next(), deadline.promise]);
      if (result.done) throw new Error(`runtime event stream ended before ${detail}`);
      const observation = await Promise.race([observe(result.value), deadline.promise]);
      if (observation !== undefined) return observation;
    }
  } finally {
    deadline.release();
  }
}

export function nextRuntimeEvent(
  iterator: AsyncIterator<RuntimeEvent>,
  type: RuntimeEvent["type"],
) {
  return observeRuntimeEvent(iterator, type, (event) => (event.type === type ? event : undefined));
}

export async function collectSessionRuntimeEvents(
  iterator: AsyncIterator<RuntimeEvent>,
  types: readonly RuntimeEvent["type"][],
  sessionId: string,
): Promise<RuntimeEvent[]> {
  const pending = new Set<RuntimeEvent["type"]>(types);
  const collected: RuntimeEvent[] = [];
  const deadline = testDeadline(`aggregate reconciliation: ${types.join(", ")}`);
  try {
    while (pending.size > 0) {
      const result = await Promise.race([iterator.next(), deadline.promise]);
      if (result.done)
        throw new Error("runtime event stream ended before aggregate reconciliation");
      const event = result.value;
      if (
        pending.has(event.type) &&
        "sessionIds" in event &&
        event.sessionIds?.includes(sessionId)
      ) {
        pending.delete(event.type);
        collected.push(event);
      }
    }
    return collected;
  } finally {
    deadline.release();
  }
}

export async function watchRuntimeTopic(client: FlameClient, topic: RuntimeTopic) {
  const controller = new AbortController();
  const subscription = await client.runtimeEvents.subscribe({ topics: [topic] }, controller.signal);
  const events = subscription.events[Symbol.asyncIterator]();
  const close = async () => {
    controller.abort();
    await events.return?.();
  };
  onTestFinished(close);
  return { events, close };
}

export async function watchGoal(client: FlameClient, sessionId: SessionId) {
  const { events } = await watchRuntimeTopic(client, "goals.changed");
  return (status?: GoalStatus | null) => {
    const detail = `Goal ${sessionId} change (${status === null ? "cleared" : (status ?? "any status")})`;
    return observeRuntimeEvent(events, detail, async (event) => {
      if (event.type !== "goals.changed" || !event.sessionIds?.includes(sessionId)) return;
      const goal = await client.goals.get(sessionId);
      if (status === undefined || (goal?.status ?? null) === status) return goal;
    });
  };
}

// Managed owners and other servers publish on the same topic, so a test waits
// for the change that names its own user server.
export function nextUserMCPChange(events: AsyncIterator<RuntimeEvent>, server: string) {
  return observeRuntimeEvent(events, `MCP ${server} change`, (event) =>
    event.type === "mcp.changed" &&
    event.servers?.some((id) => id.origin.type === "user" && id.name === server)
      ? event
      : undefined,
  );
}

export function waitForUserMCPConnection(
  client: FlameClient,
  events: AsyncIterator<RuntimeEvent>,
  server: string,
) {
  const matches = (id: MCPServerID) => id.origin.type === "user" && id.name === server;
  return observeRuntimeEvent(events, `MCP ${server} connection`, async (event) => {
    if (event.type !== "mcp.changed" || !event.servers?.some(matches)) return;
    const { data } = await client.mcp.list();
    return data.find(({ id, status }) => matches(id) && status.type === "connected");
  });
}

export async function collectRunEvents(events: AsyncIterable<RunEvent>): Promise<RunEvent[]> {
  const collected: RunEvent[] = [];
  for await (const event of events) collected.push(event);
  return collected;
}

export function originalOpeningEvents<R>(
  opening: import("@flame/runtime-client").RunOpeningResult<R>,
): AsyncIterable<RunEvent> {
  if (opening.observation.type !== "stream") throw new Error("expected an opening stream");
  return opening.observation.events;
}
