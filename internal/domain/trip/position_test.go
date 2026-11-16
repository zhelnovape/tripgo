package trip_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zhelnovape/template/internal/domain/trip"
)

func TestNewPosition(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	tripID := uuid.New()

	tests := []struct {
		name       string
		point      trip.Point
		recordedAt time.Time
		wantField  string
	}{
		{name: "valid", point: ekaterinburg, recordedAt: now.Add(-time.Minute)},
		{name: "recorded now", point: ekaterinburg, recordedAt: now},
		{name: "small clock skew", point: ekaterinburg, recordedAt: now.Add(trip.MaxPositionClockSkew)},
		{name: "boundary coordinates", point: trip.Point{Latitude: -90, Longitude: 180}, recordedAt: now},
		{name: "latitude out of range", point: trip.Point{Latitude: 90.1, Longitude: 0}, recordedAt: now, wantField: "coordinates.latitude"},
		{name: "longitude out of range", point: trip.Point{Latitude: 0, Longitude: -180.1}, recordedAt: now, wantField: "coordinates.longitude"},
		{name: "zero recorded_at", point: ekaterinburg, wantField: "recorded_at"},
		{name: "too far in the future", point: ekaterinburg, recordedAt: now.Add(trip.MaxPositionClockSkew + time.Second), wantField: "recorded_at"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := trip.NewPosition(tripID, tt.point, tt.recordedAt, now)

			if tt.wantField != "" {
				var vErr *trip.ValidationError
				require.True(t, errors.As(err, &vErr))
				assert.Equal(t, tt.wantField, vErr.Field)
				assert.Nil(t, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tripID, got.TripID)
			assert.Equal(t, tt.point, got.Point)
			assert.True(t, tt.recordedAt.Equal(got.RecordedAt))
		})
	}
}

func TestNewPosition_NormalizesToUTC(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	recordedAt := now.In(time.FixedZone("YEKT", 5*60*60))

	got, err := trip.NewPosition(uuid.New(), ekaterinburg, recordedAt, now)

	require.NoError(t, err)
	assert.Equal(t, time.UTC, got.RecordedAt.Location())
}

func TestTrip_EnsureActive(t *testing.T) {
	t.Parallel()

	tr, err := trip.New(uuid.New(), uuid.New(), ekaterinburg, airport, 100, startedAt)
	require.NoError(t, err)
	require.NoError(t, tr.EnsureActive())

	require.NoError(t, tr.Finish(startedAt.Add(time.Hour)))
	require.ErrorIs(t, tr.EnsureActive(), trip.ErrTripCompleted)
}
