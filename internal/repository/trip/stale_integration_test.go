//go:build integration

package trip_test

import (
	"context"
	"time"

	"github.com/google/uuid"

	domain "github.com/zhelnovape/template/internal/domain/trip"
	"github.com/zhelnovape/template/internal/model"
)

const staleAfter = time.Minute

func (s *RepositorySuite) saveTripStartedAt(startedAt time.Time) *domain.Trip {
	tr := s.newTrip()
	tr.StartedAt = startedAt.UTC().Truncate(time.Microsecond)
	s.Require().NoError(s.repo.Save(context.Background(), tr))
	return tr
}

func (s *RepositorySuite) staleByID() map[uuid.UUID]model.StaleTrip {
	trips, err := s.repo.ListStale(context.Background(), staleAfter)
	s.Require().NoError(err)

	byID := make(map[uuid.UUID]model.StaleTrip, len(trips))
	for _, t := range trips {
		byID[t.TripID] = t
	}
	return byID
}

func (s *RepositorySuite) TestListStale() {
	ctx := context.Background()
	now := time.Now().UTC()

	withoutPositions := s.saveTripStartedAt(now.Add(-2 * time.Hour))

	fresh := s.saveTripStartedAt(now.Add(-2 * time.Hour))
	s.Require().NoError(s.repo.TouchLastPosition(ctx, fresh.ID, now.Add(-10*time.Second)))

	silent := s.saveTripStartedAt(now.Add(-3 * time.Hour))
	s.Require().NoError(s.repo.TouchLastPosition(ctx, silent.ID, now.Add(-5*time.Minute)))

	justStarted := s.saveTripStartedAt(now.Add(-10 * time.Second))

	completed := s.saveTripStartedAt(now.Add(-2 * time.Hour))
	s.Require().NoError(completed.Finish(now))
	s.Require().NoError(s.repo.Finish(ctx, completed))

	got := s.staleByID()

	s.Contains(got, withoutPositions.ID)
	s.Contains(got, silent.ID)
	s.NotContains(got, fresh.ID)
	s.NotContains(got, justStarted.ID)
	s.NotContains(got, completed.ID)

	s.Equal(withoutPositions.DriverID, got[withoutPositions.ID].DriverID)
	s.True(withoutPositions.StartedAt.Equal(got[withoutPositions.ID].LastSeenAt))
	s.True(now.Add(-5 * time.Minute).Truncate(time.Microsecond).Equal(got[silent.ID].LastSeenAt))
}

func (s *RepositorySuite) TestListStaleOrderedBySilence() {
	now := time.Now().UTC()
	newer := s.saveTripStartedAt(now.Add(-10 * time.Minute))
	oldest := s.saveTripStartedAt(now.Add(-time.Hour))
	middle := s.saveTripStartedAt(now.Add(-30 * time.Minute))

	trips, err := s.repo.ListStale(context.Background(), staleAfter)
	s.Require().NoError(err)

	var order []uuid.UUID
	for _, t := range trips {
		if t.TripID == newer.ID || t.TripID == oldest.ID || t.TripID == middle.ID {
			order = append(order, t.TripID)
		}
	}
	s.Equal([]uuid.UUID{oldest.ID, middle.ID, newer.ID}, order)

	for i := 1; i < len(trips); i++ {
		s.False(trips[i].LastSeenAt.Before(trips[i-1].LastSeenAt))
	}
}
