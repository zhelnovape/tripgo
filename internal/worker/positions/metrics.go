package positions

import (
	"context"
	"fmt"
	"sync/atomic"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const (
	requestSuccess   = "success"
	requestError     = "error"
	requestCancelled = "cancelled"
)

type metrics struct {
	staleChecks metric.Int64Counter
	requests    metric.Int64Counter
	duration    metric.Float64Histogram
	gauges      metric.Registration
}

func newMetrics(meter metric.Meter, queue *Queue, activeWorkers *atomic.Int64) (*metrics, error) {
	m := &metrics{}
	var err error

	if m.staleChecks, err = meter.Int64Counter("tripgo.stale_checks",
		metric.WithUnit("{trip}"),
		metric.WithDescription("What the scanner did with a stale trip."),
	); err != nil {
		return nil, fmt.Errorf("create stale checks counter: %w", err)
	}

	if m.requests, err = meter.Int64Counter("tripgo.position_requests",
		metric.WithUnit("{request}"),
		metric.WithDescription("Results of position requests to the devices."),
	); err != nil {
		return nil, fmt.Errorf("create position requests counter: %w", err)
	}

	if m.duration, err = meter.Float64Histogram("tripgo.position_requests.duration",
		metric.WithUnit("s"),
		metric.WithDescription("Duration of Push Service calls."),
	); err != nil {
		return nil, fmt.Errorf("create position requests duration histogram: %w", err)
	}

	queueSize, err := meter.Int64ObservableGauge("tripgo.position_requests.queue.size",
		metric.WithUnit("{task}"),
		metric.WithDescription("Tasks waiting in the position requests queue."),
	)
	if err != nil {
		return nil, fmt.Errorf("create queue size gauge: %w", err)
	}

	workersActive, err := meter.Int64ObservableGauge("tripgo.position_requests.workers.active",
		metric.WithUnit("{worker}"),
		metric.WithDescription("Workers busy with a position request."),
	)
	if err != nil {
		return nil, fmt.Errorf("create active workers gauge: %w", err)
	}

	m.gauges, err = meter.RegisterCallback(func(_ context.Context, o metric.Observer) error {
		o.ObserveInt64(queueSize, int64(queue.Len()))
		o.ObserveInt64(workersActive, activeWorkers.Load())
		return nil
	}, queueSize, workersActive)
	if err != nil {
		return nil, fmt.Errorf("register gauges callback: %w", err)
	}

	return m, nil
}

func resultAttr(result string) metric.MeasurementOption {
	return metric.WithAttributes(attribute.String("result", result))
}
