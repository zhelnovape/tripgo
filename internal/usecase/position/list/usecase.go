package list

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/zhelnovape/template/internal/domain/trip"
)

type UseCase struct {
	trips     tripRepository
	positions positionRepository
}

func New(trips tripRepository, positions positionRepository) *UseCase {
	return &UseCase{trips: trips, positions: positions}
}

func (u *UseCase) Execute(ctx context.Context, tripID uuid.UUID) ([]trip.Position, error) {
	if _, err := u.trips.Get(ctx, tripID); err != nil {
		return nil, fmt.Errorf("get trip: %w", err)
	}

	positions, err := u.positions.ListByTrip(ctx, tripID)
	if err != nil {
		return nil, fmt.Errorf("list positions: %w", err)
	}
	return positions, nil
}
