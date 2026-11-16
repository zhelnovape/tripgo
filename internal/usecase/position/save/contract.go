//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=${GOFILE} -destination=mocks_test.go -package=${GOPACKAGE}_test
package save

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/zhelnovape/template/internal/domain/trip"
)

type (
	tripRepository interface {
		Get(ctx context.Context, id uuid.UUID) (*trip.Trip, error)
		TouchLastPosition(ctx context.Context, tripID uuid.UUID, recordedAt time.Time) error
	}

	positionRepository interface {
		Save(ctx context.Context, p *trip.Position) error
	}

	txManager interface {
		Do(ctx context.Context, fn func(ctx context.Context) error) error
	}

	clock interface {
		Now() time.Time
	}
)
