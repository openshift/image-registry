package errors

import (
	"context"

	dcontext "github.com/distribution/distribution/v3/context"
	errcode "github.com/distribution/distribution/v3/registry/api/errcode"
)

type errMessageKey struct{}

func (errMessageKey) String() string { return "err.message" }

type errDetailKey struct{}

func (errDetailKey) String() string { return "err.detail" }

func Handle(ctx context.Context, message string, err error) {
	ctx = context.WithValue(ctx, errMessageKey{}, err)
	switch err := err.(type) {
	case errcode.Error:
		ctx = context.WithValue(ctx, errDetailKey{}, err.Detail)
	}
	dcontext.GetLogger(ctx, errMessageKey{}, errDetailKey{}).Error(message)
}
