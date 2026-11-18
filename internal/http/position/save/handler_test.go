package save_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	domain "github.com/zhelnovape/template/internal/domain/trip"
	"github.com/zhelnovape/template/internal/generated/api"
	"github.com/zhelnovape/template/internal/http/position/save"
	"github.com/zhelnovape/template/internal/model"
)

var (
	tripID     = uuid.MustParse("1f0a9c62-4a1c-4f2e-9d33-2a4bb0f0b111")
	recordedAt = time.Date(2026, 8, 24, 12, 0, 5, 0, time.UTC)
)

const validBody = `{
	"coordinates": {"latitude": 59.935, "longitude": 30.335},
	"recorded_at": "2026-08-24T12:00:05Z"
}`

func wantInput() model.SavePosition {
	return model.SavePosition{
		TripID:     tripID,
		Point:      domain.Point{Latitude: 59.935, Longitude: 30.335},
		RecordedAt: recordedAt,
	}
}

func serve(t *testing.T, usecase *MockpositionSaver, id, body string) *httptest.ResponseRecorder {
	t.Helper()

	router := chi.NewRouter()
	router.Method(http.MethodPost, "/api/v1/trips/{tripId}/positions", save.New(usecase, slog.New(slog.DiscardHandler)))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/trips/"+id+"/positions", strings.NewReader(body))
	router.ServeHTTP(w, r)
	return w
}

func decodeCode(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()

	assert.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))

	var problem api.Problem
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &problem))
	return problem.Code
}

func TestHandler_Created(t *testing.T) {
	t.Parallel()

	in := wantInput()
	usecase := NewMockpositionSaver(gomock.NewController(t))
	usecase.EXPECT().
		Execute(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ any, got model.SavePosition) (*domain.Position, error) {
			assert.Equal(t, in.TripID, got.TripID)
			assert.Equal(t, in.Point, got.Point)
			assert.True(t, in.RecordedAt.Equal(got.RecordedAt))
			return &domain.Position{ID: 42, TripID: tripID, Point: in.Point, RecordedAt: recordedAt}, nil
		})

	w := serve(t, usecase, tripID.String(), validBody)

	assert.Equal(t, http.StatusCreated, w.Code)
	assert.JSONEq(t, `{
		"id": 42,
		"trip_id": "1f0a9c62-4a1c-4f2e-9d33-2a4bb0f0b111",
		"coordinates": {"latitude": 59.935, "longitude": 30.335},
		"recorded_at": "2026-08-24T12:00:05Z"
	}`, w.Body.String())
}

func TestHandler_InvalidRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		id   string
		body string
	}{
		{name: "invalid trip id", id: "not-a-uuid", body: validBody},
		{name: "broken json", body: `{"coordinates":`},
		{name: "empty body", body: ``},
		{name: "null body", body: `null`},
		{name: "array body", body: `[]`},
		{name: "unknown field", body: strings.Replace(validBody, `"recorded_at"`, `"speed": 10, "recorded_at"`, 1)},
		{name: "missing recorded_at", body: `{"coordinates": {"latitude": 59.935, "longitude": 30.335}}`},
		{name: "null recorded_at", body: strings.Replace(validBody, `"2026-08-24T12:00:05Z"`, `null`, 1)},
		{name: "unparsable recorded_at", body: strings.Replace(validBody, `"2026-08-24T12:00:05Z"`, `"yesterday"`, 1)},
		{name: "missing coordinates", body: `{"recorded_at": "2026-08-24T12:00:05Z"}`},
		{name: "null coordinates", body: strings.Replace(validBody, `{"latitude": 59.935, "longitude": 30.335}`, `null`, 1)},
		{name: "missing latitude", body: strings.Replace(validBody, `"latitude": 59.935, `, ``, 1)},
		{name: "null longitude", body: strings.Replace(validBody, `"longitude": 30.335`, `"longitude": null`, 1)},
		{name: "string latitude", body: strings.Replace(validBody, `59.935`, `"59.935"`, 1)},
		{name: "Latitude next to latitude", body: strings.Replace(validBody, `"latitude": 59.935,`, `"latitude": 59.935, "Latitude": 10,`, 1)},
		{name: "RECORDED_AT instead of recorded_at", body: strings.Replace(validBody, `"recorded_at"`, `"RECORDED_AT"`, 1)},
		{name: "trailing garbage", body: validBody + `xyz`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			id := tt.id
			if id == "" {
				id = tripID.String()
			}
			usecase := NewMockpositionSaver(gomock.NewController(t))

			w := serve(t, usecase, id, tt.body)

			assert.Equal(t, http.StatusBadRequest, w.Code)
			assert.Equal(t, "invalid_request", decodeCode(t, w))
		})
	}
}

func TestHandler_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{
			name:       "validation error",
			err:        fmt.Errorf("new position: %w", &domain.ValidationError{Field: "recorded_at", Reason: "must not be in the future"}),
			wantStatus: http.StatusBadRequest,
			wantCode:   "invalid_request",
		},
		{
			name:       "trip not found",
			err:        fmt.Errorf("get trip: %w", domain.ErrTripNotFound),
			wantStatus: http.StatusNotFound,
			wantCode:   "trip_not_found",
		},
		{
			name:       "trip completed",
			err:        domain.ErrTripCompleted,
			wantStatus: http.StatusConflict,
			wantCode:   "trip_completed",
		},
		{
			name:       "repository error",
			err:        errors.New("dial tcp 10.0.0.5:5432: connection refused"),
			wantStatus: http.StatusInternalServerError,
			wantCode:   "internal_error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			usecase := NewMockpositionSaver(gomock.NewController(t))
			usecase.EXPECT().Execute(gomock.Any(), gomock.Any()).Return(nil, tt.err)

			w := serve(t, usecase, tripID.String(), validBody)

			assert.Equal(t, tt.wantStatus, w.Code)
			assert.Equal(t, tt.wantCode, decodeCode(t, w))
		})
	}
}
