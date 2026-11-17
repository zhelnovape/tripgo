package positions

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

const tracerName = "github.com/zhelnovape/template/internal/worker/positions"

type Config struct {
	CheckInterval  time.Duration
	StaleAfter     time.Duration
	Workers        int
	RequestTimeout time.Duration
}

type Option func(*Poller)

func WithTicker(f TickerFactory) Option {
	return func(p *Poller) { p.newTicker = f }
}

type Poller struct {
	cfg       Config
	queue     *Queue
	source    staleTripSource
	pusher    pusher
	newTicker TickerFactory
	logger    *slog.Logger
	tracer    trace.Tracer
	metrics   *metrics

	activeWorkers atomic.Int64

	startOnce     sync.Once
	stopOnce      sync.Once
	stopErr       error
	cancelScanner context.CancelFunc
	cancelWorkers context.CancelFunc
	scannerDone   chan struct{}
	workers       sync.WaitGroup
}

func New(
	cfg Config,
	queue *Queue,
	source staleTripSource,
	pusher pusher,
	logger *slog.Logger,
	tp trace.TracerProvider,
	meter metric.Meter,
	opts ...Option,
) (*Poller, error) {
	if cfg.Workers <= 0 {
		return nil, fmt.Errorf("workers must be positive, got %d", cfg.Workers)
	}

	p := &Poller{
		cfg:         cfg,
		queue:       queue,
		source:      source,
		pusher:      pusher,
		newTicker:   newRealTicker,
		logger:      logger,
		tracer:      tp.Tracer(tracerName),
		scannerDone: make(chan struct{}),
	}
	for _, opt := range opts {
		opt(p)
	}

	m, err := newMetrics(meter, queue, &p.activeWorkers)
	if err != nil {
		return nil, err
	}
	p.metrics = m

	return p, nil
}

func (p *Poller) Start(ctx context.Context) {
	p.startOnce.Do(func() {
		var scannerCtx, workersCtx context.Context
		scannerCtx, p.cancelScanner = context.WithCancel(ctx)
		workersCtx, p.cancelWorkers = context.WithCancel(ctx)

		p.workers.Add(p.cfg.Workers)
		for i := range p.cfg.Workers {
			go p.runWorker(workersCtx, i)
		}
		go p.runScanner(scannerCtx)

		p.logger.InfoContext(ctx, "position poller started",
			"workers", p.cfg.Workers,
			"queue_size", cap(p.queue.tasks),
			"check_interval", p.cfg.CheckInterval,
			"stale_after", p.cfg.StaleAfter,
		)
	})
}

func (p *Poller) Wait() {
	<-p.scannerDone
	p.workers.Wait()
}

func (p *Poller) Stop(ctx context.Context) error {
	p.stopOnce.Do(func() {
		p.stopErr = p.stop(ctx)
	})
	return p.stopErr
}

func (p *Poller) stop(ctx context.Context) error {
	if p.cancelScanner == nil {
		return p.metrics.gauges.Unregister()
	}

	p.cancelScanner()
	<-p.scannerDone
	p.logger.InfoContext(ctx, "position scanner stopped")

	p.queue.close()

	workersDone := make(chan struct{})
	go func() {
		p.workers.Wait()
		close(workersDone)
	}()

	var err error
	select {
	case <-workersDone:
		p.logger.InfoContext(ctx, "position workers drained the queue and stopped")
	case <-ctx.Done():
		p.cancelWorkers()
		<-workersDone
		err = fmt.Errorf("drain position requests queue: %w", ctx.Err())
		p.logger.WarnContext(ctx, "position workers stopped before draining the queue", "left", p.queue.Len())
	}
	p.cancelWorkers()

	return errors.Join(err, p.metrics.gauges.Unregister())
}

func (p *Poller) runScanner(ctx context.Context) {
	defer close(p.scannerDone)

	ticker := p.newTicker(p.cfg.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C():
			p.scan(ctx)
		}
	}
}

func (p *Poller) scan(ctx context.Context) {
	ctx, span := p.tracer.Start(ctx, "positions.scan")
	defer span.End()

	trips, err := p.source.ListStale(ctx, p.cfg.StaleAfter)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "list stale trips")
		p.logger.ErrorContext(ctx, "list stale trips failed", "error", err)
		return
	}
	span.SetAttributes(attribute.Int("tripgo.stale_trips", len(trips)))

	link := trace.LinkFromContext(ctx)
	for _, t := range trips {
		result := p.queue.Enqueue(Task{
			TripID:     t.TripID,
			DriverID:   t.DriverID,
			LastSeenAt: t.LastSeenAt,
			link:       link,
		})
		p.metrics.staleChecks.Add(ctx, 1, resultAttr(string(result)))

		if result == ResultDropped {
			p.logger.WarnContext(ctx, "position request dropped: queue is full",
				"trip_id", t.TripID,
				"last_seen_at", t.LastSeenAt,
			)
		}
	}
}

func (p *Poller) runWorker(ctx context.Context, id int) {
	defer p.workers.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case task, ok := <-p.queue.tasks:
			if !ok {
				return
			}
			if ctx.Err() != nil {
				return
			}
			p.handle(ctx, id, task)
		}
	}
}

func (p *Poller) handle(ctx context.Context, workerID int, task Task) {
	if !p.queue.actual(task) {
		p.metrics.requests.Add(ctx, 1, resultAttr(requestCancelled))
		return
	}
	defer p.queue.done(task)

	p.activeWorkers.Add(1)
	defer p.activeWorkers.Add(-1)

	ctx, span := p.tracer.Start(ctx, "positions.request_position",
		trace.WithNewRoot(),
		trace.WithLinks(task.link),
		trace.WithAttributes(
			attribute.String("tripgo.trip_id", task.TripID.String()),
			attribute.Int("tripgo.worker_id", workerID),
		),
	)
	defer span.End()

	reqCtx, cancel := context.WithTimeout(ctx, p.cfg.RequestTimeout)
	defer cancel()

	started := time.Now()
	err := p.pusher.RequestPosition(reqCtx, task.TripID, task.DriverID)
	elapsed := time.Since(started).Seconds()

	result := requestSuccess
	if err != nil {
		result = requestError
		span.RecordError(err)
		span.SetStatus(codes.Error, "request position")
		p.logger.WarnContext(ctx, "position request failed", "trip_id", task.TripID, "error", err)
	}

	p.metrics.duration.Record(ctx, elapsed, resultAttr(result))
	p.metrics.requests.Add(ctx, 1, resultAttr(result))
}
