package pluto

import (
	"context"

	"github.com/jackc/pgx/v5"
)

func valueOrNull[T any](ptr *T) interface{} {
	if ptr == nil {
		return nil
	}
	return *ptr
}

type TxFunc func(ctx context.Context, tx pgx.Tx) error
