package trip

import (
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	domain "github.com/zhelnovape/template/internal/domain/trip"
)

func scanTrip(row pgx.Row) (*domain.Trip, error) {
	var (
		t      domain.Trip
		status string
	)

	err := row.Scan(
		&t.ID, &t.UserID, &t.DriverID,
		&t.Start.Latitude, &t.Start.Longitude,
		&t.End.Latitude, &t.End.Longitude,
		&t.Price, &status,
		&t.StartedAt, &t.FinishedAt, &t.LastPositionAt,
		&t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	t.Status = domain.Status(status)
	t.StartedAt = t.StartedAt.UTC()
	t.FinishedAt = utcOrNil(t.FinishedAt)
	t.LastPositionAt = utcOrNil(t.LastPositionAt)
	t.CreatedAt = t.CreatedAt.UTC()
	t.UpdatedAt = t.UpdatedAt.UTC()
	return &t, nil
}

func utcOrNil(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	utc := t.UTC()
	return &utc
}

func statusOrNil(s *domain.Status) *string {
	if s == nil {
		return nil
	}
	value := string(*s)
	return &value
}

func isActiveDriverViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) &&
		pgErr.Code == pgUniqueViolation &&
		pgErr.ConstraintName == activeDriverIndex
}
