// Package telemetry supplies bounded OTLP instrumentation for hosted services.
package telemetry

import (
	"context"
	"net/http"
	"runtime"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func Start(ctx context.Context, name string) func(context.Context) error {
	resources, err := resource.New(ctx, resource.WithFromEnv(), resource.WithAttributes(attribute.String("service.name", name)))
	if err != nil {
		resources = resource.NewWithAttributes("", attribute.String("service.name", name))
	}
	traceExporter, traceErr := otlptracehttp.New(ctx)
	metricExporter, metricErr := otlpmetrichttp.New(ctx)
	var traces *sdktrace.TracerProvider
	var metrics *sdkmetric.MeterProvider
	if traceErr == nil {
		traces = sdktrace.NewTracerProvider(sdktrace.WithBatcher(traceExporter), sdktrace.WithResource(resources))
		otel.SetTracerProvider(traces)
	}
	if metricErr == nil {
		metrics = sdkmetric.NewMeterProvider(sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter)), sdkmetric.WithResource(resources))
		otel.SetMeterProvider(metrics)
	}
	otel.SetTextMapPropagator(propagation.TraceContext{})
	_, _ = otel.Meter("authrail.hosted").Int64ObservableGauge("authrail.process.heap.bytes",
		metric.WithInt64Callback(func(_ context.Context, observer metric.Int64Observer) error {
			var memory runtime.MemStats
			runtime.ReadMemStats(&memory)
			observer.Observe(int64(memory.HeapAlloc))
			return nil
		}))
	_, _ = otel.Meter("authrail.hosted").Int64ObservableGauge("authrail.process.goroutines",
		metric.WithInt64Callback(func(_ context.Context, observer metric.Int64Observer) error {
			observer.Observe(int64(runtime.NumGoroutine()))
			return nil
		}))
	return func(ctx context.Context) error {
		if traces != nil {
			_ = traces.Shutdown(ctx)
		}
		if metrics != nil {
			_ = metrics.Shutdown(ctx)
		}
		return nil
	}
}

// HTTP records stable operation names, never raw paths, query strings or bodies.
func HTTP(operation string, next http.Handler) http.Handler {
	tracer := otel.Tracer("authrail.hosted")
	duration, _ := otel.Meter("authrail.hosted").Float64Histogram("authrail.hosted.operation.duration", metric.WithUnit("s"))
	count, _ := otel.Meter("authrail.hosted").Int64Counter("authrail.hosted.operations")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health/live" || r.URL.Path == "/health/ready" {
			next.ServeHTTP(w, r)
			return
		}
		ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
		ctx, span := tracer.Start(ctx, operation)
		defer span.End()
		start := time.Now()
		response := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(response, r.WithContext(ctx))
		if response.status == 0 {
			response.status = http.StatusOK
		}
		outcome := "ok"
		if response.status >= 500 {
			outcome = "error"
			span.SetStatus(codes.Error, "request failed")
		} else if response.status >= 400 {
			outcome = "denied"
		}
		span.SetAttributes(attribute.Int("http.response.status_code", response.status))
		labels := metric.WithAttributes(attribute.String("operation", operation), attribute.String("outcome", outcome))
		duration.Record(ctx, time.Since(start).Seconds(), labels)
		count.Add(ctx, 1, labels)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *statusWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}
func (w *statusWriter) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(body)
}
func (w *statusWriter) Flush() {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	_ = http.NewResponseController(w.ResponseWriter).Flush()
}
