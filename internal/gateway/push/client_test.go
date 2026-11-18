package push

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
)

var (
	tripID   = uuid.MustParse("1f0a9c62-4a1c-4f2e-9d33-2a4bb0f0b111")
	driverID = uuid.MustParse("8860b315-ec86-42eb-a17c-7c163d721ff5")
)

func newTestClient(t *testing.T, baseURL string, timeout time.Duration) *Client {
	t.Helper()

	c, err := New(Config{BaseURL: baseURL, RequestTimeout: timeout}, Telemetry{
		TracerProvider: tracenoop.NewTracerProvider(),
		MeterProvider:  metricnoop.NewMeterProvider(),
		Propagator:     propagation.TraceContext{},
	})
	require.NoError(t, err)
	t.Cleanup(c.Close)
	return c
}

func TestClient_RequestPosition_Accepted(t *testing.T) {
	t.Parallel()

	var got pushRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, pushPath, r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&got))
		w.WriteHeader(http.StatusAccepted)
		_, _ = io.WriteString(w, `{"message_id":"m-1","accepted_at":"2026-08-24T12:00:05Z"}`)
	}))
	t.Cleanup(srv.Close)

	err := newTestClient(t, srv.URL, time.Second).RequestPosition(context.Background(), tripID, driverID)

	require.NoError(t, err)
	assert.NotEmpty(t, got.RequestID)
	assert.Equal(t, driverID, got.RecipientID)
	assert.Equal(t, "REQUEST_POSITION", got.Kind)
	assert.Equal(t, map[string]string{"trip_id": tripID.String()}, got.Data)
}

func TestClient_RequestPosition_StatusErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		status        int
		wantTemporary bool
	}{
		{status: http.StatusBadRequest},
		{status: http.StatusNotFound},
		{status: http.StatusTooManyRequests, wantTemporary: true},
		{status: http.StatusInternalServerError, wantTemporary: true},
		{status: http.StatusServiceUnavailable, wantTemporary: true},
	}

	for _, tt := range tests {
		t.Run(http.StatusText(tt.status), func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
			}))
			t.Cleanup(srv.Close)

			err := newTestClient(t, srv.URL, time.Second).RequestPosition(context.Background(), tripID, driverID)

			var statusErr *StatusError
			require.ErrorAs(t, err, &statusErr)
			assert.Equal(t, tt.status, statusErr.StatusCode)
			assert.Equal(t, tt.wantTemporary, statusErr.Temporary())

			var transportErr *TransportError
			assert.False(t, errors.As(err, &transportErr))
		})
	}
}

func TestClient_RequestPosition_TransportError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.NotFoundHandler())
	baseURL := srv.URL
	srv.Close()

	err := newTestClient(t, baseURL, time.Second).RequestPosition(context.Background(), tripID, driverID)

	var transportErr *TransportError
	require.ErrorAs(t, err, &transportErr)
	var statusErr *StatusError
	assert.False(t, errors.As(err, &statusErr))
}

func TestClient_RequestPosition_Timeout(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(func() {
		close(release)
		srv.Close()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := newTestClient(t, srv.URL, time.Minute).RequestPosition(ctx, tripID, driverID)

	var transportErr *TransportError
	require.ErrorAs(t, err, &transportErr)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

type trackedBody struct {
	io.Reader
	closed *atomic.Bool
}

func (b trackedBody) Close() error {
	b.closed.Store(true)
	return nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestClient_AlwaysClosesBody(t *testing.T) {
	t.Parallel()

	for _, status := range []int{http.StatusAccepted, http.StatusBadRequest, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Parallel()

			var closed atomic.Bool
			c := newClient("http://push.test"+pushPath, &http.Client{
				Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode: status,
						Body:       trackedBody{Reader: strings.NewReader(`{"error":"x"}`), closed: &closed},
						Header:     make(http.Header),
					}, nil
				}),
			})

			_ = c.RequestPosition(context.Background(), tripID, driverID)

			assert.True(t, closed.Load())
		})
	}
}

func TestClient_PropagatesTraceContextAndCreatesSpan(t *testing.T) {
	t.Parallel()

	var traceparent string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traceparent = r.Header.Get("traceparent")
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(srv.Close)

	spans := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spans))
	c, err := New(Config{BaseURL: srv.URL, RequestTimeout: time.Second}, Telemetry{
		TracerProvider: tp,
		MeterProvider:  metricnoop.NewMeterProvider(),
		Propagator:     propagation.TraceContext{},
	})
	require.NoError(t, err)
	t.Cleanup(c.Close)

	ctx, parent := tp.Tracer("test").Start(context.Background(), "worker")
	require.NoError(t, c.RequestPosition(ctx, tripID, driverID))
	parent.End()

	assert.Contains(t, traceparent, parent.SpanContext().TraceID().String())
	ended := spans.Ended()
	require.Len(t, ended, 2)
	assert.Equal(t, "POST "+pushPath, ended[0].Name())
	assert.Equal(t, parent.SpanContext().SpanID(), ended[0].Parent().SpanID())
}
