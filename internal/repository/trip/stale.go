package trip

import (
	"context"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"

	domain "github.com/zhelnovape/template/internal/domain/trip"
	"github.com/zhelnovape/template/internal/model"
)

const lastSeenExpr = "COALESCE(last_position_at, started_at)"

func (r *Repository) ListStale(ctx context.Context, staleAfter time.Duration) ([]model.StaleTrip, error) {
	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	query, args, err := psql.
		Select("id", "driver_id", lastSeenExpr).
		From(tripsTable).
		Where(sq.Eq{statusColumn: string(domain.StatusActive)}).
		Where(sq.Expr(lastSeenExpr+" < now() - ?::bigint * interval '1 microsecond'", staleAfter.Microseconds())).
		OrderBy(lastSeenExpr, "id").
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build select stale trips query: %w", err)
	}

	rows, err := r.conn(ctx).Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("execute select stale trips: %w", err)
	}

	trips, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (model.StaleTrip, error) {
		var t model.StaleTrip
		err := row.Scan(&t.TripID, &t.DriverID, &t.LastSeenAt)
		t.LastSeenAt = t.LastSeenAt.UTC()
		return t, err
	})
	if err != nil {
		return nil, fmt.Errorf("scan stale trips: %w", err)
	}
	return trips, nil
}
