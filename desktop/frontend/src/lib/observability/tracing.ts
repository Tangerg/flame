import { context, type Span, SpanKind, SpanStatusCode, trace } from "@opentelemetry/api";

const TRACER_NAME = "flame-frontend";

export function startRunSpan(attrs: Record<string, string | number | boolean>): Span {
  return trace.getTracer(TRACER_NAME).startSpan("agent.run", {
    kind: SpanKind.INTERNAL,
    attributes: attrs,
  });
}

export function withSpan<T>(span: Span, fn: () => T): T {
  return context.with(trace.setSpan(context.active(), span), fn);
}

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
