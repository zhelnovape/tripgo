package list_test

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	domain "github.com/zhelnovape/template/internal/domain/trip"
	"github.com/zhelnovape/template/internal/generated/api"
	"github.com/zhelnovape/template/internal/http/position/list"
)

var tripID = uuid.MustParse("1f0a9c62-4a1c-4f2e-9d33-2a4bb0f0b111")

func serve(t *testing.T, usecase *MockpositionLister, id string) *httptest.ResponseRecorder {
	t.Helper()

	router := chi.NewRouter()
	router.Method(http.MethodGet, "/api/v1/trips/{tripId}/positions", list.New(usecase, slog.New(slog.DiscardHandler)))

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/trips/"+id+"/positions", http.NoBody))
	return w
}

func decodeCode(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()

	var problem api.Problem
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &problem))
	return problem.Code
}

func TestHandler_OK(t *testing.T) {
	t.Parallel()

	usecase := NewMockpositionLister(gomock.NewController(t))
	usecase.EXPECT().Execute(gomock.Any(), tripID).Return([]domain.Position{
		{ID: 1, TripID: tripID, Point: domain.Point{Latitude: 59.93, Longitude: 30.33}, RecordedAt: time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)},
		{ID: 2, TripID: tripID, Point: domain.Point{Latitude: 59.94, Longitude: 30.34}, RecordedAt: time.Date(2026, 8, 24, 12, 0, 5, 0, time.UTC)},
	}, nil)

	w := serve(t, usecase, tripID.String())

	assert.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"positions": [
		{"id": 1, "trip_id": "1f0a9c62-4a1c-4f2e-9d33-2a4bb0f0b111", "coordinates": {"latitude": 59.93, "longitude": 30.33}, "recorded_at": "2026-08-24T12:00:00Z"},
		{"id": 2, "trip_id": "1f0a9c62-4a1c-4f2e-9d33-2a4bb0f0b111", "coordinates": {"latitude": 59.94, "longitude": 30.34}, "recorded_at": "2026-08-24T12:00:05Z"}
	]}`, w.Body.String())
}

func TestHandler_EmptyRoute(t *testing.T) {
	t.Parallel()

	usecase := NewMockpositionLister(gomock.NewController(t))
	usecase.EXPECT().Execute(gomock.Any(), tripID).Return(nil, nil)

	w := serve(t, usecase, tripID.String())

	assert.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"positions": []}`, w.Body.String())
}

func TestHandler_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		id         string
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "invalid trip id", id: "42", wantStatus: http.StatusBadRequest, wantCode: "invalid_request"},
		{name: "trip not found", err: domain.ErrTripNotFound, wantStatus: http.StatusNotFound, wantCode: "trip_not_found"},
		{name: "repository error", err: errors.New("connection refused"), wantStatus: http.StatusInternalServerError, wantCode: "internal_error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			usecase := NewMockpositionLister(gomock.NewController(t))
			id := tt.id
			if id == "" {
				id = tripID.String()
				usecase.EXPECT().Execute(gomock.Any(), tripID).Return(nil, tt.err)
			}

			w := serve(t, usecase, id)

			assert.Equal(t, tt.wantStatus, w.Code)
			assert.Equal(t, tt.wantCode, decodeCode(t, w))
		})
	}
}
