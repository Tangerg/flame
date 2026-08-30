// SPAN GRANULARITY IS DELIBERATELY COARSE: one span per agent run and one per RPC call,
// NEVER per StreamEvent or token. A run streams ~30 events/sec, so per-event spans would be
// thousands per run; that signal belongs in metrics.
//
// For everything ABOVE rpc/ — the rpc layer cannot import this and instruments its own
// CLIENT span directly against @opentelemetry/api.

import { context, type Span, SpanKind, SpanStatusCode, trace } from "@opentelemetry/api";

const TRACER_NAME = "flame-frontend";

/** Open a span for one agent run. Coarse: covers the whole run (start →
 *  finish), the parent the RPC CLIENT spans nest under. */
export function startRunSpan(attrs: Record<string, string | number | boolean>): Span {
  return trace.getTracer(TRACER_NAME).startSpan("agent.run", {
    kind: SpanKind.INTERNAL,
    attributes: attrs,
  });
}

/** Run `fn` with `span` as the active span, so anything it dispatches
 *  synchronously (the RPC call → transport.send) parents under it and
 *  inherits its trace context for `traceparent` injection. */
export function withSpan<T>(span: Span, fn: () => T): T {
  return context.with(trace.setSpan(context.active(), span), fn);
}

/** Pass the error to mark the span failed. */
export function endSpan(span: Span, err?: unknown): void {
  if (err !== undefined && err !== null) {
    span.setStatus({
      code: SpanStatusCode.ERROR,
      message: err instanceof Error ? err.message : String(err),
    });
  } else {
    span.setStatus({ code: SpanStatusCode.OK });
  }
  span.end();
}
