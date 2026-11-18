package push

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

const (
	pushPath            = "/api/v1/push"
	kindRequestPosition = "REQUEST_POSITION"
	maxDrainedBody      = 64 << 10
)

type Config struct {
	BaseURL        string
	RequestTimeout time.Duration
}

type Telemetry struct {
	TracerProvider trace.TracerProvider
	MeterProvider  metric.MeterProvider
	Propagator     propagation.TextMapPropagator
}

type Client struct {
	http    *http.Client
	pushURL string
}

type pushRequest struct {
	RequestID   string            `json:"request_id"`
	RecipientID uuid.UUID         `json:"recipient_id"`
	Kind        string            `json:"kind"`
	Data        map[string]string `json:"data,omitempty"`
}

func New(cfg Config, tel Telemetry) (*Client, error) {
	pushURL, err := url.JoinPath(cfg.BaseURL, pushPath)
	if err != nil {
		return nil, fmt.Errorf("build push service url: %w", err)
	}

	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   cfg.RequestTimeout,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   cfg.RequestTimeout,
		ResponseHeaderTimeout: cfg.RequestTimeout,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   100,
		IdleConnTimeout:       90 * time.Second,
	}

	return newClient(pushURL, &http.Client{
		Timeout: cfg.RequestTimeout,
		Transport: otelhttp.NewTransport(transport,
			otelhttp.WithTracerProvider(tel.TracerProvider),
			otelhttp.WithMeterProvider(tel.MeterProvider),
			otelhttp.WithPropagators(tel.Propagator),
			otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
				return r.Method + " " + pushPath
			}),
		),
	}), nil
}

func newClient(pushURL string, httpClient *http.Client) *Client {
	return &Client{http: httpClient, pushURL: pushURL}
}

func (c *Client) RequestPosition(ctx context.Context, tripID, driverID uuid.UUID) error {
	return c.send(ctx, pushRequest{
		RequestID:   uuid.NewString(),
		RecipientID: driverID,
		Kind:        kindRequestPosition,
		Data:        map[string]string{"trip_id": tripID.String()},
	})
}

func (c *Client) Close() {
	c.http.CloseIdleConnections()
}

func (c *Client) send(ctx context.Context, body pushRequest) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("encode push request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.pushURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build push request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return &TransportError{Err: err}
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxDrainedBody))
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return &StatusError{StatusCode: resp.StatusCode}
	}
	return nil
}
