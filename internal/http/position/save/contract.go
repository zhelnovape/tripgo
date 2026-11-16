//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=${GOFILE} -destination=mocks_test.go -package=${GOPACKAGE}_test
package save

import (
	"context"

	domain "github.com/zhelnovape/template/internal/domain/trip"
	"github.com/zhelnovape/template/internal/model"
)

type positionSaver interface {
	Execute(ctx context.Context, in model.SavePosition) (*domain.Position, error)
}
