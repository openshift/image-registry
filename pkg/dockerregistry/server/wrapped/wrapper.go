package wrapped

import "github.com/docker/distribution/context"

// Wrapper is a user defined function that wraps a method and can control the
// execution flow, modify its context and handle errors.
type Wrapper func(ctx context.Context, funcname string, f func(ctx context.Context) error) error
