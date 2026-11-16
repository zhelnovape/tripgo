package position

import (
	"context"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	domain "github.com/zhelnovape/template/internal/domain/trip"
	"github.com/zhelnovape/template/internal/infra/postgres/transaction"
)

const positionsTable = "trip_positions"

var psql = sq.StatementBuilder.PlaceholderFormat(sq.Dollar)

type Repository struct {
	db           querier
	queryTimeout time.Duration
}

func New(db querier, queryTimeout time.Duration) *Repository {
	return &Repository{db: db, queryTimeout: queryTimeout}
}

func (r *Repository) Save(ctx context.Context, p *domain.Position) error {
	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	query, args, err := psql.
		Insert(positionsTable).
		Columns("trip_id", "latitude", "longitude", "recorded_at").
		Values(p.TripID, p.Point.Latitude, p.Point.Longitude, p.RecordedAt).
		Suffix("RETURNING id, recorded_at, created_at").
		ToSql()
	if err != nil {
		return fmt.Errorf("build insert position query: %w", err)
	}

	if err := r.conn(ctx).QueryRow(ctx, query, args...).Scan(&p.ID, &p.RecordedAt, &p.CreatedAt); err != nil {
		return fmt.Errorf("execute insert position: %w", err)
	}
	p.RecordedAt = p.RecordedAt.UTC()
	return nil
}

func (r *Repository) ListByTrip(ctx context.Context, tripID uuid.UUID) ([]domain.Position, error) {
	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	query, args, err := psql.
		Select("id", "trip_id", "latitude", "longitude", "recorded_at", "created_at").
		From(positionsTable).
		Where(sq.Eq{"trip_id": tripID}).
		OrderBy("recorded_at", "id").
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build select positions query: %w", err)
	}

	rows, err := r.conn(ctx).Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("execute select positions: %w", err)
	}

	positions, err := pgx.CollectRows(rows, scanPosition)
	if err != nil {
		return nil, fmt.Errorf("scan positions: %w", err)
	}
	return positions, nil
}

func (r *Repository) conn(ctx context.Context) querier {
	if tx, ok := transaction.FromContext(ctx); ok {
		return tx
	}
	return r.db
}

func scanPosition(row pgx.CollectableRow) (domain.Position, error) {
	var p domain.Position
	err := row.Scan(&p.ID, &p.TripID, &p.Point.Latitude, &p.Point.Longitude, &p.RecordedAt, &p.CreatedAt)
	p.RecordedAt = p.RecordedAt.UTC()
	return p, err
}
