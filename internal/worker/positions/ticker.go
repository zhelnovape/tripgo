package positions

import "time"

type Ticker interface {
	C() <-chan time.Time
	Stop()
}

type TickerFactory func(d time.Duration) Ticker

type realTicker struct {
	t *time.Ticker
}

func newRealTicker(d time.Duration) Ticker {
	return &realTicker{t: time.NewTicker(d)}
}

func (r *realTicker) C() <-chan time.Time {
	return r.t.C
}

func (r *realTicker) Stop() {
	r.t.Stop()
}
