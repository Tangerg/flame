package main

import (
	"context"
	"errors"
	stdslog "log/slog"
	"os"
	"strings"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/telemetry"
	otelslog "github.com/Tangerg/scope/otel/slog"
	slogbridge "go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

const (
	scopeName               = identity.ProductName
	logLevelEnvironment     = "FLAME_LOG_LEVEL"
	providerShutdownTimeout = 5 * time.Second
)

type telemetryShutdown func(context.Context) error

func configureTelemetry(serviceVersion string) telemetryShutdown {
	level := parseLogLevel(os.Getenv(logLevelEnvironment))

	// slog.Default() is the bridge, not this base logger, so there is no loop: default
	// → bridge → OTel → exporter → base.
	base := stdslog.New(stdslog.NewTextHandler(os.Stderr, &stdslog.HandlerOptions{Level: level}))

	res := resource.NewWithAttributes(telemetry.SchemaURL,
		telemetry.ServiceNameKey.String(scopeName),
		telemetry.ServiceVersionKey.String(serviceVersion),
	)

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithSyncer(otelslog.NewSpanExporter(base)),
	)
	otel.SetTracerProvider(tp)

	reader := sdkmetric.NewPeriodicReader(otelslog.NewMetricExporter(base))
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(reader),
	)
	otel.SetMeterProvider(mp)

	lp := sdklog.NewLoggerProvider(
		sdklog.WithResource(res),
		sdklog.WithProcessor(sdklog.NewSimpleProcessor(otelslog.NewLogExporter(base))),
	)
	otel.SetLoggerProvider(lp)
	stdslog.SetDefault(stdslog.New(minLevelHandler{
		level: level,
		inner: slogbridge.NewHandler(scopeName, slogbridge.WithLoggerProvider(lp)),
	}))

	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return func(ctx context.Context) error {
		shutdownCtx, cancel := context.WithTimeout(ctx, providerShutdownTimeout)
		defer cancel()
		return errors.Join(
			tp.Shutdown(shutdownCtx),
			mp.Shutdown(shutdownCtx),
			lp.Shutdown(shutdownCtx),
		)
	}
}

// The OTel logs bridge does no level filtering, so without this gate
// FLAME_LOG_LEVEL would be ignored.
type minLevelHandler struct {
	level stdslog.Level
	inner stdslog.Handler
}

func (m minLevelHandler) Enabled(_ context.Context, l stdslog.Level) bool { return l >= m.level }

func (m minLevelHandler) Handle(ctx context.Context, r stdslog.Record) error {
	return m.inner.Handle(ctx, r)
}

func (m minLevelHandler) WithAttrs(a []stdslog.Attr) stdslog.Handler {
	return minLevelHandler{level: m.level, inner: m.inner.WithAttrs(a)}
}

func (m minLevelHandler) WithGroup(name string) stdslog.Handler {
	return minLevelHandler{level: m.level, inner: m.inner.WithGroup(name)}
}

func parseLogLevel(s string) stdslog.Level {
	var level stdslog.Level
	if err := level.UnmarshalText([]byte(strings.TrimSpace(s))); err != nil {
		return stdslog.LevelInfo
	}
	return level
}
