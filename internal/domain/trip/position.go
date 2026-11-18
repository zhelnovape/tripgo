package trip

import (
	"time"

	"github.com/google/uuid"
)

const MaxPositionClockSkew = time.Minute

type Position struct {
	ID         int64
	TripID     uuid.UUID
	Point      Point
	RecordedAt time.Time
	CreatedAt  time.Time
}

func NewPosition(tripID uuid.UUID, point Point, recordedAt, now time.Time) (*Position, error) {
	if err := validatePoint("coordinates", point); err != nil {
		return nil, err
	}
	if recordedAt.IsZero() {
		return nil, &ValidationError{Field: "recorded_at", Reason: "is required"}
	}
	if recordedAt.After(now.Add(MaxPositionClockSkew)) {
		return nil, &ValidationError{Field: "recorded_at", Reason: "must not be in the future"}
	}

	return &Position{
		TripID:     tripID,
		Point:      point,
		RecordedAt: recordedAt.UTC(),
	}, nil
}
