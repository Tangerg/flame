// Imported ONLY from `*.test.ts`; production code never sees this module.

import type { MemoryTransport } from "./memory";
import type { TransportRequest } from "../transport";
import type { RunMetrics, SegmentOutcome, StreamEvent } from "@flame/runtime-contract/wire";
import type { WireMethodName } from "@flame/runtime-contract/methods";
import { RUN_EVENT_METHOD } from "../stream";
import { JSONRPC_VERSION, type RpcId, type RpcMessage } from "../types";

// Outbound (client → server) — synchronisation helpers

/** Polls the outbox microtask-by-microtask for ~50 ticks. Use it to grab the id the client
 *  allocated, then answer with {@link respondSuccess} / {@link respondError}. */
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

// Inbound (server → client) — message synthesis

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

/** Inject a `notifications.run.event` carrying a v2 StreamEvent (§5). A
 *  fixed timestamp keeps fixtures stable. The
 *  envelope carries BOTH runId and segmentId — the stream tree keys on the
 *  segmentId (a resume opens a new segment of the same run). */
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

/** Inject a `segment.finished` StreamEvent for the root segment — terminates the
 *  stream (v2 has no separate "closed" method, §5). */
export function injectRunFinished(
  t: MemoryTransport,
  runId: string,
  segmentId: string,
  eventId: string,
  requestRpcId: RpcId,
  outcome: SegmentOutcome = { type: "completed" },
  metrics: RunMetrics = { steps: 0, activeDurationMillis: 0 },
): void {
  injectRunEvent(
    t,
    runId,
    segmentId,
    eventId,
    { type: "segment.finished", contextTokens: 0, outcome, metrics },
    requestRpcId,
  );
}
