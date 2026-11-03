package steps

import (
	"context"
)

type Op int

const (
	Create Op = iota + 1
	Update
	Delete
)

type Handler[T any] interface {
	Namespace(ns string)
	Op(op Op)
	Handle(ctx context.Context, id string, in *map[string]any) (T, error)
}
