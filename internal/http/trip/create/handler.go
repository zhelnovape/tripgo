package create

import (
	"io"
	"log/slog"
	"net/http"

	"github.com/zhelnovape/template/internal/generated/api"
	"github.com/zhelnovape/template/internal/http/domainerror"
	"github.com/zhelnovape/template/internal/http/request"
	"github.com/zhelnovape/template/internal/http/response"
	"github.com/zhelnovape/template/internal/http/trip"
)

type Handler struct {
	usecase tripCreator
	logger  *slog.Logger
}

func New(usecase tripCreator, logger *slog.Logger) *Handler {
	return &Handler{usecase: usecase, logger: logger}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	req, err := decode(r.Body)
	if err != nil {
		domainerror.Write(w, r, h.logger, err)
		return
	}

	t, err := h.usecase.Execute(r.Context(), toModel(req))
	if err != nil {
		domainerror.Write(w, r, h.logger, err)
		return
	}

	w.Header().Set("Location", "/api/v1/trips/"+t.ID.String())
	response.JSON(w, http.StatusCreated, trip.ToAPI(t))
}

func decode(body io.Reader) (api.TripData, error) {
	var req api.TripData

	obj, err := request.DecodeJSON(body, &req)
	if err != nil {
		return req, err
	}
	if err := obj.Require("", "user_id", "driver_id", "start_point", "end_point", "price"); err != nil {
		return req, err
	}

	for _, name := range []string{"start_point", "end_point"} {
		point, err := obj.Nested(name)
		if err != nil {
			return req, err
		}
		if err := point.Require(name+".", "latitude", "longitude"); err != nil {
			return req, err
		}
	}
	return req, nil
}
