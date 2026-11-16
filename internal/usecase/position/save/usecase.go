package save

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/metric"

	"github.com/zhelnovape/template/internal/domain/trip"
	"github.com/zhelnovape/template/internal/model"
)

type UseCase struct {
	trips     tripRepository
	positions positionRepository
	txManager txManager
	clock     clock
	saved     metric.Int64Counter
}

func New(
	trips tripRepository,
	positions positionRepository,
	txManager txManager,
	clk clock,
	saved metric.Int64Counter,
) *UseCase {
	return &UseCase{trips: trips, positions: positions, txManager: txManager, clock: clk, saved: saved}
}

func (u *UseCase) Execute(ctx context.Context, in model.SavePosition) (*trip.Position, error) {
	p, err := trip.NewPosition(in.TripID, in.Point, in.RecordedAt, u.clock.Now())
	if err != nil {
		return nil, fmt.Errorf("new position: %w", err)
	}

	err = u.txManager.Do(ctx, func(ctx context.Context) error {
		t, err := u.trips.Get(ctx, in.TripID)
		if err != nil {
			return fmt.Errorf("get trip: %w", err)
		}
		if err := t.EnsureActive(); err != nil {
			return err
		}

		if err := u.trips.TouchLastPosition(ctx, t.ID, p.RecordedAt); err != nil {
			return fmt.Errorf("touch last position: %w", err)
		}
		if err := u.positions.Save(ctx, p); err != nil {
			return fmt.Errorf("save position: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	u.saved.Add(ctx, 1)
	return p, nil
}
