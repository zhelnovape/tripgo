package model

import (
	"time"

	"github.com/google/uuid"
)

type StaleTrip struct {
	TripID     uuid.UUID
	DriverID   uuid.UUID
	LastSeenAt time.Time
}
