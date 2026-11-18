package list_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/zhelnovape/template/internal/domain/trip"
	"github.com/zhelnovape/template/internal/usecase/position/list"
)

func newUseCase(t *testing.T) (*list.UseCase, *MocktripRepository, *MockpositionRepository) {
	t.Helper()

	ctrl := gomock.NewController(t)
	trips := NewMocktripRepository(ctrl)
	positions := NewMockpositionRepository(ctrl)
	return list.New(trips, positions), trips, positions
}

func TestUseCase_Execute_Success(t *testing.T) {
	t.Parallel()

	uc, trips, positions := newUseCase(t)
	tripID := uuid.New()
	want := []trip.Position{
		{ID: 1, TripID: tripID, RecordedAt: time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)},
		{ID: 2, TripID: tripID, RecordedAt: time.Date(2026, 10, 28, 12, 0, 5, 0, time.UTC)},
	}
	trips.EXPECT().Get(gomock.Any(), tripID).Return(&trip.Trip{ID: tripID}, nil)
	positions.EXPECT().ListByTrip(gomock.Any(), tripID).Return(want, nil)

	got, err := uc.Execute(context.Background(), tripID)

	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestUseCase_Execute_TripNotFound(t *testing.T) {
	t.Parallel()

	uc, trips, _ := newUseCase(t)
	tripID := uuid.New()
	trips.EXPECT().Get(gomock.Any(), tripID).Return(nil, trip.ErrTripNotFound)

	got, err := uc.Execute(context.Background(), tripID)

	require.ErrorIs(t, err, trip.ErrTripNotFound)
	assert.Nil(t, got)
}

func TestUseCase_Execute_ListFails(t *testing.T) {
	t.Parallel()

	uc, trips, positions := newUseCase(t)
	tripID := uuid.New()
	repoErr := errors.New("connection refused")
	trips.EXPECT().Get(gomock.Any(), tripID).Return(&trip.Trip{ID: tripID}, nil)
	positions.EXPECT().ListByTrip(gomock.Any(), tripID).Return(nil, repoErr)

	got, err := uc.Execute(context.Background(), tripID)

	require.ErrorIs(t, err, repoErr)
	assert.Nil(t, got)
}
