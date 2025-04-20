// Package tracing provides tracing utilities for the application.
package tracing

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"

	"celeryToGo/internal/observability/logging"
)

// Config contains configuration for the tracer.
type Config struct {
	// ServiceName is the name of the service.
	ServiceName string

	// ServiceVersion is the version of the service.
	ServiceVersion string

	// Environment is the environment the service is running in.
	Environment string

	// OTLPEndpoint is the endpoint for the OTLP exporter.
	OTLPEndpoint string

	// Enabled indicates whether tracing is enabled.
	Enabled bool

	// SampleRate is the rate at which traces are sampled (0.0 - 1.0).
	SampleRate float64
}

// Tracer is the interface for tracing.
type Tracer interface {
	// Start starts a new span.
	Start(ctx context.Context, spanName string, opts ...trace.SpanStartOption) (context.Context, trace.Span)

	// Shutdown shuts down the tracer.
	Shutdown(ctx context.Context) error
}

// OpenTelemetryTracer is an implementation of the Tracer interface using OpenTelemetry.
type OpenTelemetryTracer struct {
	tracer trace.Tracer
	tp     *sdktrace.TracerProvider
	config Config
}

// NewOpenTelemetryTracer creates a new OpenTelemetryTracer.
func NewOpenTelemetryTracer(config Config) (*OpenTelemetryTracer, error) {
	if !config.Enabled {
		// Return a no-op tracer if tracing is disabled
		return &OpenTelemetryTracer{
			tracer: trace.NewNoopTracerProvider().Tracer(""),
			config: config,
		}, nil
	}

	// Set default values
	if config.ServiceName == "" {
		config.ServiceName = "celery-go-worker"
	}
	if config.ServiceVersion == "" {
		config.ServiceVersion = "unknown"
	}
	if config.Environment == "" {
		config.Environment = "development"
	}
	if config.OTLPEndpoint == "" {
		config.OTLPEndpoint = "localhost:4317"
	}
	if config.SampleRate <= 0 {
		config.SampleRate = 1.0
	}

	// Create a resource describing the service
	res, err := resource.New(context.Background(),
		resource.WithAttributes(
			semconv.ServiceNameKey.String(config.ServiceName),
			semconv.ServiceVersionKey.String(config.ServiceVersion),
			attribute.String("environment", config.Environment),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Create the OTLP exporter
	client := otlptracegrpc.NewClient(
		otlptracegrpc.WithEndpoint(config.OTLPEndpoint),
		otlptracegrpc.WithInsecure(),
	)
	exporter, err := otlptrace.New(context.Background(), client)
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP exporter: %w", err)
	}

	// Create the trace provider
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.TraceIDRatioBased(config.SampleRate)),
		sdktrace.WithBatcher(exporter,
			sdktrace.WithMaxExportBatchSize(512),
			sdktrace.WithBatchTimeout(5*time.Second),
		),
		sdktrace.WithResource(res),
	)

	// Set the global trace provider
	otel.SetTracerProvider(tp)

	// Set the global propagator
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// Create the tracer
	tracer := tp.Tracer(config.ServiceName)

	logging.Info("OpenTelemetry tracer initialized")

	return &OpenTelemetryTracer{
		tracer: tracer,
		tp:     tp,
		config: config,
	}, nil
}

// Start starts a new span.
func (t *OpenTelemetryTracer) Start(ctx context.Context, spanName string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return t.tracer.Start(ctx, spanName, opts...)
}

// Shutdown shuts down the tracer.
func (t *OpenTelemetryTracer) Shutdown(ctx context.Context) error {
	if t.tp == nil {
		return nil
	}
	return t.tp.Shutdown(ctx)
}

// DefaultTracer is the default tracer for the application.
var DefaultTracer Tracer

// InitTracer initializes the default tracer.
func InitTracer(config Config) error {
	tracer, err := NewOpenTelemetryTracer(config)
	if err != nil {
		return err
	}
	DefaultTracer = tracer
	return nil
}

// Start starts a new span using the default tracer.
func Start(ctx context.Context, spanName string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	if DefaultTracer == nil {
		// Initialize a no-op tracer if not initialized
		DefaultTracer = &OpenTelemetryTracer{
			tracer: trace.NewNoopTracerProvider().Tracer(""),
		}
	}
	return DefaultTracer.Start(ctx, spanName, opts...)
}

// Shutdown shuts down the default tracer.
func Shutdown(ctx context.Context) error {
	if DefaultTracer == nil {
		return nil
	}
	return DefaultTracer.Shutdown(ctx)
}

// SpanFromContext returns the current span from the context.
func SpanFromContext(ctx context.Context) trace.Span {
	return trace.SpanFromContext(ctx)
}

// ContextWithSpan returns a new context with the given span.
func ContextWithSpan(ctx context.Context, span trace.Span) context.Context {
	return trace.ContextWithSpan(ctx, span)
}

// AddEvent adds an event to the current span.
func AddEvent(ctx context.Context, name string, attrs ...attribute.KeyValue) {
	span := trace.SpanFromContext(ctx)
	span.AddEvent(name, trace.WithAttributes(attrs...))
}

// SetAttributes sets attributes on the current span.
func SetAttributes(ctx context.Context, attrs ...attribute.KeyValue) {
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attrs...)
}

// RecordError records an error on the current span.
func RecordError(ctx context.Context, err error, opts ...trace.EventOption) {
	span := trace.SpanFromContext(ctx)
	span.RecordError(err, opts...)
}