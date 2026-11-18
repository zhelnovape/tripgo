package positions

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/mock/gomock"

	"github.com/zhelnovape/template/internal/model"
)

type fakeTicker struct {
	ch      chan time.Time
	stopped chan struct{}
}

func newFakeTicker() *fakeTicker {
	return &fakeTicker{ch: make(chan time.Time), stopped: make(chan struct{})}
}

func (f *fakeTicker) C() <-chan time.Time { return f.ch }

func (f *fakeTicker) Stop() { close(f.stopped) }

func (f *fakeTicker) tick() { f.ch <- time.Now() }

type env struct {
	poller  *Poller
	queue   *Queue
	source  *MockstaleTripSource
	pusher  *Mockpusher
	ticker  *fakeTicker
	metrics *sdkmetric.ManualReader
}

func newEnv(t *testing.T, workers, queueSize int) *env {
	t.Helper()

	ctrl := gomock.NewController(t)
	e := &env{
		queue:   NewQueue(queueSize),
		source:  NewMockstaleTripSource(ctrl),
		pusher:  NewMockpusher(ctrl),
		ticker:  newFakeTicker(),
		metrics: sdkmetric.NewManualReader(),
	}

	meter := sdkmetric.NewMeterProvider(sdkmetric.WithReader(e.metrics)).Meter("test")
	cfg := Config{
		CheckInterval:  30 * time.Second,
		StaleAfter:     time.Minute,
		Workers:        workers,
		RequestTimeout: time.Second,
	}

	p, err := New(cfg, e.queue, e.source, e.pusher, slog.New(slog.DiscardHandler), noop.NewTracerProvider(), meter,
		WithTicker(func(time.Duration) Ticker { return e.ticker }),
	)
	require.NoError(t, err)
	e.poller = p
	return e
}

func (e *env) start(t *testing.T) {
	t.Helper()

	e.poller.Start(context.Background())
	t.Cleanup(func() { _ = e.poller.Stop(context.Background()) })
}

func (e *env) count(t *testing.T, name, result string) int64 {
	t.Helper()

	var rm metricdata.ResourceMetrics
	require.NoError(t, e.metrics.Collect(context.Background(), &rm))

	want := attribute.String("result", result)
	var total int64
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != name {
				continue
			}
			sum, ok := m.Data.(metricdata.Sum[int64])
			require.True(t, ok)
			for _, dp := range sum.DataPoints {
				if v, ok := dp.Attributes.Value(want.Key); ok && v == want.Value {
					total += dp.Value
				}
			}
		}
	}
	return total
}

func staleTrips(n int) []model.StaleTrip {
	trips := make([]model.StaleTrip, n)
	base := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	for i := range trips {
		trips[i] = model.StaleTrip{TripID: uuid.New(), DriverID: uuid.New(), LastSeenAt: base.Add(time.Duration(i) * time.Second)}
	}
	return trips
}

func task(st model.StaleTrip) Task {
	return Task{TripID: st.TripID, DriverID: st.DriverID, LastSeenAt: st.LastSeenAt}
}
