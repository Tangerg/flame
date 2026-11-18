import type { MemoryTransport } from "../src/transports/memory";
import type { TransportRequest } from "../src/transport";
import type { Interrupt, RunMetrics, RunRef, StreamEvent } from "@flame/runtime-client/wire";
import type { RunOpeningResult } from "../src/methods";
import type { WireMethodName } from "@flame/runtime-client/methods";
import { RUN_EVENT_METHOD } from "../src/stream";
import { JSONRPC_VERSION, type RpcId, type RpcMessage } from "../src/types";

export function waitForRequest<M extends WireMethodName>(
  t: MemoryTransport,
  method: M,
  matches?: (message: TransportRequest) => boolean,
): Promise<TransportRequest & { method: M }> {
  return t.waitForRequest(method, matches);
}

export function respondSuccess(t: MemoryTransport, id: RpcId, result: unknown): void {
  t.inject({ jsonrpc: JSONRPC_VERSION, id, result } as RpcMessage);
}

function injectNotification(
  t: MemoryTransport,
  requestRpcId: RpcId,
  method: string,
  params: unknown,
): void {
  t.inject({ jsonrpc: JSONRPC_VERSION, method, params }, undefined, requestRpcId);
}

export function injectRunEvent(
  t: MemoryTransport,
  runId: string,
  segmentId: string,
  eventId: string,
  event: StreamEvent,
  requestRpcId: RpcId,
): void {
  injectNotification(t, requestRpcId, RUN_EVENT_METHOD, {
    runId,
    segmentId,
    eventId,
    timestamp: "2026-06-03T00:00:00Z",
    event,
  });
}

export interface SegmentFinish {
  interrupts?: Interrupt[];
  metrics?: RunMetrics;
}

export function finishedSegment(runId: string, finish: SegmentFinish = {}): StreamEvent {
  const waiting = finish.interrupts !== undefined;
  const run: RunRef = {
    id: runId as never,
    sessionId: "ses_1" as never,
    provider: "mock",
    model: "mock",
    status: waiting ? "waiting" : "finished",
    metrics: finish.metrics ?? { steps: 0, activeDurationMillis: 0 },
    capabilities: { requiredFeatures: [], interruptTypes: ["approval", "question"] },
    createdAt: "2026-06-03T00:00:00Z",
    ...(waiting
      ? {}
      : { finishedAt: "2026-06-03T00:00:00Z", outcome: { type: "completed" as const } }),
  };
  return waiting
    ? { type: "segment.finished", run, interrupts: finish.interrupts! }
    : { type: "segment.finished", run };
}

export function injectRunFinished(
  t: MemoryTransport,
  runId: string,
  segmentId: string,
  eventId: string,
  requestRpcId: RpcId,
  finish: SegmentFinish = {},
): void {
  injectRunEvent(t, runId, segmentId, eventId, finishedSegment(runId, finish), requestRpcId);
}

export function openingEvents<R>(
  opening: RunOpeningResult<R>,
): AsyncIterable<import("@flame/runtime-client/wire").RunEvent> {
  if (opening.observation.type !== "stream") throw new Error("expected an opening stream");
  return opening.observation.events;
}

export async function respondNotRecorded(
  t: MemoryTransport,
  operation: "start" | "resume" | "steer",
): Promise<void> {
  const request = await waitForRequest(
    t,
    "runs.getAcceptance",
    (message) => (message.params as { operation?: string }).operation === operation,
  );
  respondSuccess(
    t,
    request.id,
    operation === "steer"
      ? { type: "notRecorded", candidateUserItemId: "item_steer" }
      : { type: "notRecorded" },
  );
}
