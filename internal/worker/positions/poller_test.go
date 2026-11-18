package positions

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"
)

const (
	staleChecksMetric = "tripgo.stale_checks"
	requestsMetric    = "tripgo.position_requests"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestQueue_Deduplication(t *testing.T) {
	t.Parallel()

	q := NewQueue(10)
	st := staleTrips(1)[0]

	assert.Equal(t, ResultQueued, q.Enqueue(task(st)))
	assert.Equal(t, ResultDuplicate, q.Enqueue(task(st)))
	assert.Equal(t, 1, q.Len())
}

func TestQueue_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	q := NewQueue(100)
	trips := staleTrips(50)

	var wg sync.WaitGroup
	for _, st := range trips {
		wg.Go(func() { q.Enqueue(task(st)) })
		wg.Go(func() { q.Enqueue(task(st)) })
		wg.Go(func() { q.Cancel(st.TripID) })
	}
	wg.Wait()
	q.close()

	actual := make(map[uuid.UUID]int)
	for tk := range q.tasks {
		if q.actual(tk) {
			actual[tk.TripID]++
		}
	}
	for id, n := range actual {
		assert.Equal(t, 1, n, "trip %s has several actual tasks", id)
	}
}

func TestScan_TripAlreadyQueuedIsDuplicate(t *testing.T) {
	t.Parallel()

	e := newEnv(t, 1, 10)
	trips := staleTrips(2)
	e.source.EXPECT().ListStale(gomock.Any(), e.poller.cfg.StaleAfter).Return(trips, nil).Times(2)

	e.poller.scan(context.Background())
	e.poller.scan(context.Background())

	assert.Equal(t, 2, e.queue.Len())
	assert.Equal(t, int64(2), e.count(t, staleChecksMetric, "queued"))
	assert.Equal(t, int64(2), e.count(t, staleChecksMetric, "duplicate"))
	require.NoError(t, e.poller.Stop(context.Background()))
}

func TestScan_QueueOverflowDropsWithoutBlocking(t *testing.T) {
	t.Parallel()

	e := newEnv(t, 1, 2)
	e.source.EXPECT().ListStale(gomock.Any(), gomock.Any()).Return(staleTrips(5), nil)

	e.poller.scan(context.Background())

	assert.Equal(t, 2, e.queue.Len())
	assert.Equal(t, int64(2), e.count(t, staleChecksMetric, "queued"))
	assert.Equal(t, int64(3), e.count(t, staleChecksMetric, "dropped"))
	require.NoError(t, e.poller.Stop(context.Background()))
}

func TestScanner_TickTriggersScan(t *testing.T) {
	t.Parallel()

	e := newEnv(t, 1, 10)
	trips := staleTrips(1)
	gomock.InOrder(
		e.source.EXPECT().ListStale(gomock.Any(), gomock.Any()).Return(nil, errors.New("connection refused")),
		e.source.EXPECT().ListStale(gomock.Any(), gomock.Any()).Return(trips, nil),
		e.source.EXPECT().ListStale(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes(),
	)
	pushed := make(chan uuid.UUID, 1)
	e.pusher.EXPECT().
		RequestPosition(gomock.Any(), trips[0].TripID, trips[0].DriverID).
		DoAndReturn(func(context.Context, uuid.UUID, uuid.UUID) error {
			pushed <- trips[0].TripID
			return nil
		})
	e.start(t)

	e.ticker.tick()
	e.ticker.tick()

	assert.Equal(t, trips[0].TripID, <-pushed)
	require.NoError(t, e.poller.Stop(context.Background()))
	<-e.ticker.stopped
	assert.Equal(t, int64(1), e.count(t, staleChecksMetric, "queued"))
	assert.Equal(t, int64(1), e.count(t, requestsMetric, "success"))
}

func TestPool_PositionBetweenEnqueueAndProcessingCancelsRequest(t *testing.T) {
	t.Parallel()

	e := newEnv(t, 2, 10)
	st := staleTrips(1)[0]
	require.Equal(t, ResultQueued, e.queue.Enqueue(task(st)))

	e.queue.Cancel(st.TripID)
	e.start(t)
	require.NoError(t, e.poller.Stop(context.Background()))

	assert.Equal(t, int64(1), e.count(t, requestsMetric, "cancelled"))
	assert.Equal(t, int64(0), e.count(t, requestsMetric, "success"))
}

func TestPool_StaleCopyAfterRequeueIsSkipped(t *testing.T) {
	t.Parallel()

	e := newEnv(t, 1, 10)
	st := staleTrips(1)[0]
	require.Equal(t, ResultQueued, e.queue.Enqueue(task(st)))
	e.queue.Cancel(st.TripID)
	require.Equal(t, ResultQueued, e.queue.Enqueue(task(st)))

	e.pusher.EXPECT().RequestPosition(gomock.Any(), st.TripID, st.DriverID).Return(nil).Times(1)
	e.start(t)
	require.NoError(t, e.poller.Stop(context.Background()))

	assert.Equal(t, int64(1), e.count(t, requestsMetric, "cancelled"))
	assert.Equal(t, int64(1), e.count(t, requestsMetric, "success"))
}

func TestPool_ProcessesTasksWithFixedNumberOfWorkers(t *testing.T) {
	t.Parallel()

	const (
		workers = 3
		tasks   = 12
	)
	e := newEnv(t, workers, tasks)

	var (
		inFlight, maxInFlight, calls atomic.Int64
		entered                      = make(chan struct{}, tasks)
		release                      = make(chan struct{})
	)
	e.pusher.EXPECT().
		RequestPosition(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(context.Context, uuid.UUID, uuid.UUID) error {
			cur := inFlight.Add(1)
			for {
				prev := maxInFlight.Load()
				if cur <= prev || maxInFlight.CompareAndSwap(prev, cur) {
					break
				}
			}
			entered <- struct{}{}
			<-release
			inFlight.Add(-1)
			calls.Add(1)
			return nil
		}).
		Times(tasks)

	for _, st := range staleTrips(tasks) {
		require.Equal(t, ResultQueued, e.queue.Enqueue(task(st)))
	}
	e.start(t)

	for range workers {
		<-entered
	}
	assert.Equal(t, int64(workers), inFlight.Load())
	close(release)

	require.NoError(t, e.poller.Stop(context.Background()))
	assert.Equal(t, int64(tasks), calls.Load())
	assert.Equal(t, int64(workers), maxInFlight.Load())
	assert.Equal(t, int64(tasks), e.count(t, requestsMetric, "success"))
}

func TestPool_PushErrorDoesNotKillWorker(t *testing.T) {
	t.Parallel()

	e := newEnv(t, 1, 10)
	trips := staleTrips(2)
	gomock.InOrder(
		e.pusher.EXPECT().RequestPosition(gomock.Any(), trips[0].TripID, gomock.Any()).Return(errors.New("push service: 503")),
		e.pusher.EXPECT().RequestPosition(gomock.Any(), trips[1].TripID, gomock.Any()).Return(nil),
	)
	for _, st := range trips {
		require.Equal(t, ResultQueued, e.queue.Enqueue(task(st)))
	}

	e.start(t)
	require.NoError(t, e.poller.Stop(context.Background()))

	assert.Equal(t, int64(1), e.count(t, requestsMetric, "error"))
	assert.Equal(t, int64(1), e.count(t, requestsMetric, "success"))
}

func TestPool_ProcessedTripCanBeQueuedAgain(t *testing.T) {
	t.Parallel()

	e := newEnv(t, 1, 10)
	st := staleTrips(1)[0]
	done := make(chan struct{})
	e.pusher.EXPECT().RequestPosition(gomock.Any(), st.TripID, st.DriverID).
		DoAndReturn(func(context.Context, uuid.UUID, uuid.UUID) error {
			defer close(done)
			assert.Equal(t, ResultDuplicate, e.queue.Enqueue(task(st)))
			return nil
		})
	require.Equal(t, ResultQueued, e.queue.Enqueue(task(st)))

	e.start(t)
	<-done
	require.NoError(t, e.poller.Stop(context.Background()))

	assert.Equal(t, ResultDropped, e.queue.Enqueue(task(st)))
	assert.False(t, e.queue.actual(task(st)))
}

func TestPoller_StopsOnContextCancel(t *testing.T) {
	t.Parallel()

	e := newEnv(t, 4, 10)
	ctx, cancel := context.WithCancel(context.Background())
	e.poller.Start(ctx)

	cancel()
	e.poller.Wait()

	<-e.ticker.stopped
	require.NoError(t, e.poller.Stop(context.Background()))
}

func TestPoller_StopDrainsQueue(t *testing.T) {
	t.Parallel()

	e := newEnv(t, 2, 10)
	e.pusher.EXPECT().RequestPosition(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(5)
	for _, st := range staleTrips(5) {
		require.Equal(t, ResultQueued, e.queue.Enqueue(task(st)))
	}

	e.start(t)
	require.NoError(t, e.poller.Stop(context.Background()))

	assert.Equal(t, 0, e.queue.Len())
	assert.Equal(t, int64(5), e.count(t, requestsMetric, "success"))
}

func TestPoller_StopTimeoutAbortsInFlightRequest(t *testing.T) {
	t.Parallel()

	e := newEnv(t, 1, 10)
	entered := make(chan struct{})
	e.pusher.EXPECT().RequestPosition(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, _, _ uuid.UUID) error {
			close(entered)
			<-ctx.Done()
			return ctx.Err()
		})
	for _, st := range staleTrips(3) {
		require.Equal(t, ResultQueued, e.queue.Enqueue(task(st)))
	}
	e.start(t)
	<-entered

	stopCtx, cancel := context.WithCancel(context.Background())
	cancel()
	err := e.poller.Stop(stopCtx)

	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, int64(1), e.count(t, requestsMetric, "error"))
}

func TestPoller_StopWithoutStart(t *testing.T) {
	t.Parallel()

	e := newEnv(t, 1, 1)

	require.NoError(t, e.poller.Stop(context.Background()))
}
