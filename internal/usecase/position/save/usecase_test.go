package save_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.uber.org/mock/gomock"

	"github.com/zhelnovape/template/internal/domain/trip"
	"github.com/zhelnovape/template/internal/model"
	"github.com/zhelnovape/template/internal/usecase/position/save"
)

var (
	now        = time.Date(2026, 10, 28, 12, 30, 0, 0, time.UTC)
	recordedAt = now.Add(-5 * time.Second)
	point      = trip.Point{Latitude: 56.8389, Longitude: 60.6057}
)

type deps struct {
	trips     *MocktripRepository
	positions *MockpositionRepository
	requests  *MockpositionRequestCanceller
	txManager *MocktxManager
	clock     *Mockclock
	metrics   *sdkmetric.ManualReader
}

func newUseCase(t *testing.T) (*save.UseCase, deps) {
	t.Helper()

	ctrl := gomock.NewController(t)
	d := deps{
		trips:     NewMocktripRepository(ctrl),
		positions: NewMockpositionRepository(ctrl),
		requests:  NewMockpositionRequestCanceller(ctrl),
		txManager: NewMocktxManager(ctrl),
		clock:     NewMockclock(ctrl),
		metrics:   sdkmetric.NewManualReader(),
	}

	saved, err := sdkmetric.NewMeterProvider(sdkmetric.WithReader(d.metrics)).
		Meter("test").
		Int64Counter("tripgo.positions.saved")
	require.NoError(t, err)

	d.clock.EXPECT().Now().Return(now).AnyTimes()
	d.txManager.EXPECT().
		Do(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, fn func(ctx context.Context) error) error {
			return fn(ctx)
		}).
		AnyTimes()

	return save.New(d.trips, d.positions, d.requests, d.txManager, d.clock, saved), d
}

func (d deps) savedPositions(t *testing.T) int64 {
	t.Helper()

	var rm metricdata.ResourceMetrics
	require.NoError(t, d.metrics.Collect(context.Background(), &rm))

	var total int64
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			sum, ok := m.Data.(metricdata.Sum[int64])
			require.True(t, ok)
			for _, dp := range sum.DataPoints {
				total += dp.Value
			}
		}
	}
	return total
}

func newActiveTrip(t *testing.T) *trip.Trip {
	t.Helper()

	tr, err := trip.New(uuid.New(), uuid.New(), point, trip.Point{Latitude: 56.7431, Longitude: 60.8027}, 1450, now.Add(-time.Hour))
	require.NoError(t, err)
	return tr
}

func input(tripID uuid.UUID) model.SavePosition {
	return model.SavePosition{TripID: tripID, Point: point, RecordedAt: recordedAt}
}

func TestUseCase_Execute_Success(t *testing.T) {
	t.Parallel()

	uc, d := newUseCase(t)
	active := newActiveTrip(t)

	gomock.InOrder(
		d.trips.EXPECT().Get(gomock.Any(), active.ID).Return(active, nil),
		d.trips.EXPECT().TouchLastPosition(gomock.Any(), active.ID, recordedAt).Return(nil),
		d.positions.EXPECT().
			Save(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, p *trip.Position) error {
				assert.Equal(t, active.ID, p.TripID)
				assert.Equal(t, point, p.Point)
				p.ID = 42
				return nil
			}),
		d.requests.EXPECT().Cancel(active.ID),
	)

	got, err := uc.Execute(context.Background(), input(active.ID))

	require.NoError(t, err)
	assert.Equal(t, int64(42), got.ID)
	assert.True(t, recordedAt.Equal(got.RecordedAt))
	assert.Equal(t, int64(1), d.savedPositions(t))
}

func TestUseCase_Execute_ValidationError(t *testing.T) {
	t.Parallel()

	uc, d := newUseCase(t)
	in := input(uuid.New())
	in.RecordedAt = now.Add(trip.MaxPositionClockSkew + time.Second)

	got, err := uc.Execute(context.Background(), in)

	var vErr *trip.ValidationError
	require.ErrorAs(t, err, &vErr)
	assert.Equal(t, "recorded_at", vErr.Field)
	assert.Nil(t, got)
	assert.Equal(t, int64(0), d.savedPositions(t))
}

func TestUseCase_Execute_TripNotFound(t *testing.T) {
	t.Parallel()

	uc, d := newUseCase(t)
	id := uuid.New()
	d.trips.EXPECT().Get(gomock.Any(), id).Return(nil, trip.ErrTripNotFound)

	got, err := uc.Execute(context.Background(), input(id))

	require.ErrorIs(t, err, trip.ErrTripNotFound)
	assert.Nil(t, got)
}

func TestUseCase_Execute_TripCompleted(t *testing.T) {
	t.Parallel()

	uc, d := newUseCase(t)
	completed := newActiveTrip(t)
	require.NoError(t, completed.Finish(now))
	d.trips.EXPECT().Get(gomock.Any(), completed.ID).Return(completed, nil)

	got, err := uc.Execute(context.Background(), input(completed.ID))

	require.ErrorIs(t, err, trip.ErrTripCompleted)
	assert.Nil(t, got)
	assert.Equal(t, int64(0), d.savedPositions(t))
}

func TestUseCase_Execute_FinishedConcurrently(t *testing.T) {
	t.Parallel()

	uc, d := newUseCase(t)
	active := newActiveTrip(t)
	d.trips.EXPECT().Get(gomock.Any(), active.ID).Return(active, nil)
	d.trips.EXPECT().TouchLastPosition(gomock.Any(), active.ID, recordedAt).Return(trip.ErrTripCompleted)

	got, err := uc.Execute(context.Background(), input(active.ID))

	require.ErrorIs(t, err, trip.ErrTripCompleted)
	assert.Nil(t, got)
}

func TestUseCase_Execute_SaveFails(t *testing.T) {
	t.Parallel()

	uc, d := newUseCase(t)
	active := newActiveTrip(t)
	repoErr := errors.New("connection refused")
	d.trips.EXPECT().Get(gomock.Any(), active.ID).Return(active, nil)
	d.trips.EXPECT().TouchLastPosition(gomock.Any(), active.ID, recordedAt).Return(nil)
	d.positions.EXPECT().Save(gomock.Any(), gomock.Any()).Return(repoErr)

	got, err := uc.Execute(context.Background(), input(active.ID))

	require.ErrorIs(t, err, repoErr)
	assert.Nil(t, got)
	assert.Equal(t, int64(0), d.savedPositions(t))
}
