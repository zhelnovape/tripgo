package save

import (
	"io"
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	domain "github.com/zhelnovape/template/internal/domain/trip"
	"github.com/zhelnovape/template/internal/generated/api"
	"github.com/zhelnovape/template/internal/http/domainerror"
	"github.com/zhelnovape/template/internal/http/position"
	"github.com/zhelnovape/template/internal/http/request"
	"github.com/zhelnovape/template/internal/http/response"
	"github.com/zhelnovape/template/internal/model"
)

type Handler struct {
	usecase positionSaver
	logger  *slog.Logger
}

func New(usecase positionSaver, logger *slog.Logger) *Handler {
	return &Handler{usecase: usecase, logger: logger}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	tripID, err := request.TripID(r)
	if err != nil {
		domainerror.Write(w, r, h.logger, err)
		return
	}

	req, err := decode(r.Body)
	if err != nil {
		domainerror.Write(w, r, h.logger, err)
		return
	}

	p, err := h.usecase.Execute(r.Context(), toModel(tripID, req))
	if err != nil {
		domainerror.Write(w, r, h.logger, err)
		return
	}

	response.JSON(w, http.StatusCreated, position.ToAPI(p))
}

func decode(body io.Reader) (api.PositionData, error) {
	var req api.PositionData

	obj, err := request.DecodeJSON(body, &req)
	if err != nil {
		return req, err
	}
	if err := obj.Require("", "coordinates", "recorded_at"); err != nil {
		return req, err
	}

	coordinates, err := obj.Nested("coordinates")
	if err != nil {
		return req, err
	}
	if err := coordinates.Require("coordinates.", "latitude", "longitude"); err != nil {
		return req, err
	}
	return req, nil
}

func toModel(tripID uuid.UUID, req api.PositionData) model.SavePosition {
	return model.SavePosition{
		TripID:     tripID,
		Point:      domain.Point{Latitude: req.Coordinates.Latitude, Longitude: req.Coordinates.Longitude},
		RecordedAt: req.RecordedAt,
	}
}
