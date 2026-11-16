//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=${GOFILE} -destination=mocks_test.go -package=${GOPACKAGE}_test
package list

import (
	"context"

	"github.com/google/uuid"

	"github.com/zhelnovape/template/internal/domain/trip"
)

type (
	tripRepository interface {
		Get(ctx context.Context, id uuid.UUID) (*trip.Trip, error)
	}

	positionRepository interface {
		ListByTrip(ctx context.Context, tripID uuid.UUID) ([]trip.Position, error)
	}
)
