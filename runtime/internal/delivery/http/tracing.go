package http

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

var tracer = otel.Tracer("github.com/Tangerg/flame/runtime/internal/delivery/http")

func recordError(ctx context.Context, spanName string, err error, attrs ...attribute.KeyValue) {
	if err == nil {
		return
	}
	_, span := tracer.Start(ctx, spanName,
		trace.WithAttributes(attrs...),
	)
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
	span.End()
}
