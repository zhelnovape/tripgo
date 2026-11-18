package position

import (
	domain "github.com/zhelnovape/template/internal/domain/trip"
	"github.com/zhelnovape/template/internal/generated/api"
)

func ToAPI(p *domain.Position) api.TripPosition {
	return api.TripPosition{
		Id:          p.ID,
		TripId:      p.TripID,
		Coordinates: api.Coordinates{Latitude: p.Point.Latitude, Longitude: p.Point.Longitude},
		RecordedAt:  p.RecordedAt,
	}
}

func ListToAPI(positions []domain.Position) api.TripPositionsResponse {
	resp := api.TripPositionsResponse{Positions: make([]api.TripPosition, 0, len(positions))}
	for i := range positions {
		resp.Positions = append(resp.Positions, ToAPI(&positions[i]))
	}
	return resp
}
