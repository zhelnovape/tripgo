package positions

import (
	"sync"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

type EnqueueResult string

const (
	ResultQueued    EnqueueResult = "queued"
	ResultDuplicate EnqueueResult = "duplicate"
	ResultDropped   EnqueueResult = "dropped"
)

type Task struct {
	TripID     uuid.UUID
	DriverID   uuid.UUID
	LastSeenAt time.Time

	link trace.Link
	seq  uint64
}

type Queue struct {
	tasks chan Task

	mu      sync.Mutex
	pending map[uuid.UUID]uint64
	nextSeq uint64
	closed  bool
}

func NewQueue(size int) *Queue {
	return &Queue{
		tasks:   make(chan Task, size),
		pending: make(map[uuid.UUID]uint64, size),
	}
}

func (q *Queue) Enqueue(t Task) EnqueueResult {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.closed {
		return ResultDropped
	}
	if _, ok := q.pending[t.TripID]; ok {
		return ResultDuplicate
	}

	q.nextSeq++
	t.seq = q.nextSeq

	select {
	case q.tasks <- t:
		q.pending[t.TripID] = t.seq
		return ResultQueued
	default:
		return ResultDropped
	}
}

func (q *Queue) Cancel(tripID uuid.UUID) {
	q.mu.Lock()
	defer q.mu.Unlock()

	delete(q.pending, tripID)
}

func (q *Queue) Len() int {
	return len(q.tasks)
}

func (q *Queue) actual(t Task) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	seq, ok := q.pending[t.TripID]
	return ok && seq == t.seq
}

func (q *Queue) done(t Task) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if seq, ok := q.pending[t.TripID]; ok && seq == t.seq {
		delete(q.pending, t.TripID)
	}
}

func (q *Queue) close() {
	q.mu.Lock()
	defer q.mu.Unlock()

	if !q.closed {
		q.closed = true
		close(q.tasks)
	}
}
