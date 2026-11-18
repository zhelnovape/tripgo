package model

import (
	"time"

	"github.com/google/uuid"

	"github.com/zhelnovape/template/internal/domain/trip"
)

type SavePosition struct {
	TripID     uuid.UUID
	Point      trip.Point
	RecordedAt time.Time
}
