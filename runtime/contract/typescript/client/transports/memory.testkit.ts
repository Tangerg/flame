import type { MemoryTransport } from "./memory";
import type { TransportRequest } from "../transport";
import type { Interrupt, RunMetrics, RunRef, StreamEvent } from "@flame/runtime-contract/wire";
import type { WireMethodName } from "@flame/runtime-contract/methods";
import { RUN_EVENT_METHOD } from "../stream";
import { JSONRPC_VERSION, type RpcId, type RpcMessage } from "../types";

export async function waitForRequest<M extends WireMethodName>(
  t: MemoryTransport,
  method: M,
): Promise<TransportRequest & { method: M }> {
  for (let attempt = 0; attempt < 50; attempt++) {
    const found = t
      .outbox()
      .find((message): message is TransportRequest & { method: M } => message.method === method);
    if (found) return found;
    await new Promise((r) => setTimeout(r, 0));
  }
  throw new Error(`timeout waiting for outbound Request "${method}"`);
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
    protocolProfile: { requiredFeatures: [], interruptTypes: ["approval", "question"] },
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
