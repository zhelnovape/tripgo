//go:build integration

package position_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/suite"

	domain "github.com/zhelnovape/template/internal/domain/trip"
	"github.com/zhelnovape/template/internal/infra/postgres/transaction"
	positionrepo "github.com/zhelnovape/template/internal/repository/position"
	triprepo "github.com/zhelnovape/template/internal/repository/trip"
)

const queryTimeout = 3 * time.Second

type RepositorySuite struct {
	suite.Suite
	pool      *pgxpool.Pool
	trips     *triprepo.Repository
	positions *positionrepo.Repository
	txManager *transaction.Manager
}

func TestRepositorySuite(t *testing.T) {
	suite.Run(t, new(RepositorySuite))
}

func (s *RepositorySuite) SetupSuite() {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		databaseURL = os.Getenv("DATABASE_URL")
	}
	s.Require().NotEmpty(databaseURL, "set TEST_DATABASE_URL or DATABASE_URL")

	pool, err := pgxpool.New(context.Background(), databaseURL)
	s.Require().NoError(err)
	s.Require().NoError(pool.Ping(context.Background()))

	s.pool = pool
	s.trips = triprepo.New(pool, queryTimeout)
	s.positions = positionrepo.New(pool, queryTimeout)
	s.txManager = transaction.New(pool, queryTimeout)
}

func (s *RepositorySuite) TearDownSuite() {
	s.pool.Close()
}

func (s *RepositorySuite) newTrip() *domain.Trip {
	tr, err := domain.New(
		uuid.New(),
		uuid.New(),
		domain.Point{Latitude: 56.8389, Longitude: 60.6057},
		domain.Point{Latitude: 56.7431, Longitude: 60.8027},
		1450,
		time.Now().UTC().Truncate(time.Microsecond),
	)
	s.Require().NoError(err)
	s.Require().NoError(s.trips.Save(context.Background(), tr))

	s.T().Cleanup(func() {
		_, err := s.pool.Exec(context.Background(), "DELETE FROM trips WHERE id = $1", tr.ID)
		s.NoError(err)
	})
	return tr
}

func (s *RepositorySuite) newPosition(tripID uuid.UUID, recordedAt time.Time) *domain.Position {
	p, err := domain.NewPosition(tripID, domain.Point{Latitude: 59.93, Longitude: 30.33}, recordedAt, recordedAt)
	s.Require().NoError(err)
	return p
}

func (s *RepositorySuite) lastPositionAt(tripID uuid.UUID) *time.Time {
	tr, err := s.trips.Get(context.Background(), tripID)
	s.Require().NoError(err)
	return tr.LastPositionAt
}

func (s *RepositorySuite) savePosition(p *domain.Position) error {
	return s.txManager.Do(context.Background(), func(ctx context.Context) error {
		if err := s.trips.TouchLastPosition(ctx, p.TripID, p.RecordedAt); err != nil {
			return err
		}
		return s.positions.Save(ctx, p)
	})
}

func (s *RepositorySuite) TestSaveUpdatesLastPositionAt() {
	tr := s.newTrip()
	recordedAt := time.Now().UTC().Truncate(time.Microsecond)
	p := s.newPosition(tr.ID, recordedAt)

	s.Require().NoError(s.savePosition(p))

	s.Positive(p.ID)
	s.False(p.CreatedAt.IsZero())
	got := s.lastPositionAt(tr.ID)
	s.Require().NotNil(got)
	s.True(recordedAt.Equal(*got))
}

func (s *RepositorySuite) TestLatePositionDoesNotAgeTrip() {
	tr := s.newTrip()
	fresh := time.Now().UTC().Truncate(time.Microsecond)
	late := fresh.Add(-time.Minute)

	s.Require().NoError(s.savePosition(s.newPosition(tr.ID, fresh)))
	s.Require().NoError(s.savePosition(s.newPosition(tr.ID, late)))

	got := s.lastPositionAt(tr.ID)
	s.Require().NotNil(got)
	s.True(fresh.Equal(*got))

	positions, err := s.positions.ListByTrip(context.Background(), tr.ID)
	s.Require().NoError(err)
	s.Len(positions, 2)
}

func (s *RepositorySuite) TestTouchCompletedTrip() {
	ctx := context.Background()
	tr := s.newTrip()
	s.Require().NoError(tr.Finish(time.Now().UTC().Truncate(time.Microsecond)))
	s.Require().NoError(s.trips.Finish(ctx, tr))

	err := s.trips.TouchLastPosition(ctx, tr.ID, time.Now().UTC())

	s.Require().ErrorIs(err, domain.ErrTripCompleted)
	s.Nil(s.lastPositionAt(tr.ID))
}

func (s *RepositorySuite) TestSaveRollsBackAtomically() {
	tr := s.newTrip()
	errAfterInsert := errors.New("something went wrong after insert")
	p := s.newPosition(tr.ID, time.Now().UTC().Truncate(time.Microsecond))

	err := s.txManager.Do(context.Background(), func(ctx context.Context) error {
		if err := s.trips.TouchLastPosition(ctx, p.TripID, p.RecordedAt); err != nil {
			return err
		}
		if err := s.positions.Save(ctx, p); err != nil {
			return err
		}
		return errAfterInsert
	})

	s.Require().ErrorIs(err, errAfterInsert)
	s.Nil(s.lastPositionAt(tr.ID))
	positions, err := s.positions.ListByTrip(context.Background(), tr.ID)
	s.Require().NoError(err)
	s.Empty(positions)
}

func (s *RepositorySuite) TestListSortedByRecordedAtThenID() {
	tr := s.newTrip()
	base := time.Now().UTC().Truncate(time.Microsecond).Add(-time.Hour)

	third := s.newPosition(tr.ID, base.Add(2*time.Second))
	first := s.newPosition(tr.ID, base)
	secondA := s.newPosition(tr.ID, base.Add(time.Second))
	secondB := s.newPosition(tr.ID, base.Add(time.Second))
	for _, p := range []*domain.Position{third, first, secondA, secondB} {
		s.Require().NoError(s.savePosition(p))
	}

	got, err := s.positions.ListByTrip(context.Background(), tr.ID)

	s.Require().NoError(err)
	s.Require().Len(got, 4)
	wantIDs := []int64{first.ID, secondA.ID, secondB.ID, third.ID}
	for i, p := range got {
		s.Equal(wantIDs[i], p.ID)
		s.Equal(tr.ID, p.TripID)
	}
}

func (s *RepositorySuite) TestListEmpty() {
	tr := s.newTrip()

	got, err := s.positions.ListByTrip(context.Background(), tr.ID)

	s.Require().NoError(err)
	s.Empty(got)
}
