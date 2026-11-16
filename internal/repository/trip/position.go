package trip

import (
	"context"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"

	domain "github.com/zhelnovape/template/internal/domain/trip"
)

func (r *Repository) TouchLastPosition(ctx context.Context, tripID uuid.UUID, recordedAt time.Time) error {
	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	query, args, err := psql.
		Update(tripsTable).
		Set("last_position_at", sq.Expr("GREATEST(last_position_at, ?::timestamptz)", recordedAt)).
		Set("updated_at", sq.Expr("now()")).
		Where(sq.Eq{"id": tripID, statusColumn: string(domain.StatusActive)}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build touch last position query: %w", err)
	}

	tag, err := r.conn(ctx).Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("execute touch last position: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrTripCompleted
	}
	return nil
}
