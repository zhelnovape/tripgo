package list

import (
	"log/slog"
	"net/http"

	"github.com/zhelnovape/template/internal/http/domainerror"
	"github.com/zhelnovape/template/internal/http/position"
	"github.com/zhelnovape/template/internal/http/request"
	"github.com/zhelnovape/template/internal/http/response"
)

type Handler struct {
	usecase positionLister
	logger  *slog.Logger
}

func New(usecase positionLister, logger *slog.Logger) *Handler {
	return &Handler{usecase: usecase, logger: logger}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	tripID, err := request.TripID(r)
	if err != nil {
		domainerror.Write(w, r, h.logger, err)
		return
	}

	positions, err := h.usecase.Execute(r.Context(), tripID)
	if err != nil {
		domainerror.Write(w, r, h.logger, err)
		return
	}

	response.JSON(w, http.StatusOK, position.ListToAPI(positions))
}
