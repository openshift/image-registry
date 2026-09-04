// Package context re-exports the request-scoped context helpers that live in
// the unexported package internal/dcontext.
//
// Upstream distribution moved this package to internal/dcontext in v3, which
// hides it from external modules. openshift/image-registry depends on
// GetLogger and the related helpers to obtain the request-scoped logger that
// the registry handlers install. That logger is stored under an unexported
// context key, so a copy of the package in a consumer would never find it and
// would silently lose the request id, user and URI log fields. Every symbol
// here is a type alias or a thin wrapper that forwards to internal/dcontext.
//
// Drop this package when upstream exports an equivalent logger accessor or
// when image-registry stops depending on it.
package context

import (
	"context"
	"net/http"
	"time"

	"github.com/distribution/distribution/v3/internal/dcontext"
)

// Common errors used with this package.
var (
	ErrNoRequestContext        = dcontext.ErrNoRequestContext
	ErrNoResponseWriterContext = dcontext.ErrNoResponseWriterContext
)

// Logger provides a leveled-logging interface.
type Logger = dcontext.Logger

// Background returns a non-nil, empty Context. The background context
// provides a single key, "instance.id" that is globally unique to the
// process.
func Background() context.Context {
	return dcontext.Background()
}

// WithValues returns a context that proxies lookups through a map. Only
// supports string keys.
func WithValues(ctx context.Context, m map[string]any) context.Context {
	return dcontext.WithValues(ctx, m)
}

// WithRequest places the request on the context. The context of the request
// is assigned a unique id, available at "http.request.id". The request itself
// is available at "http.request". Other common attributes are available under
// the prefix "http.request.". If a request is already present on the context,
// this method will panic.
func WithRequest(ctx context.Context, r *http.Request) context.Context {
	return dcontext.WithRequest(ctx, r)
}

// GetRequestID attempts to resolve the current request id, if possible. An
// empty string is returned if it is not available on the context.
func GetRequestID(ctx context.Context) string {
	return dcontext.GetRequestID(ctx)
}

// WithResponseWriter returns a new context and response writer that makes
// interesting response statistics available within the context.
func WithResponseWriter(ctx context.Context, w http.ResponseWriter) (context.Context, http.ResponseWriter) {
	return dcontext.WithResponseWriter(ctx, w)
}

// GetResponseWriter returns the http.ResponseWriter from the provided
// context. If not present, ErrNoResponseWriterContext is returned.
func GetResponseWriter(ctx context.Context) (http.ResponseWriter, error) {
	return dcontext.GetResponseWriter(ctx)
}

// WithVars extracts gorilla/mux vars and makes them available on the returned
// context. Variables are available at keys with the prefix "vars.".
func WithVars(ctx context.Context, r *http.Request) context.Context {
	return dcontext.WithVars(ctx, r)
}

// GetRequestLogger returns a logger that contains fields from the request in
// the current context. If the request is not available in the context, no
// fields will display. Request loggers can safely be pushed onto the context.
func GetRequestLogger(ctx context.Context) Logger {
	return dcontext.GetRequestLogger(ctx)
}

// GetResponseLogger reads the current response stats and builds a logger.
// Because the values are read at call time, pushing a logger returned from
// this function on the context will lead to missing or invalid data. Only
// call this at the end of a request, after the response has been written.
func GetResponseLogger(ctx context.Context) Logger {
	return dcontext.GetResponseLogger(ctx)
}

// WithLogger creates a new context with provided logger.
func WithLogger(ctx context.Context, logger Logger) context.Context {
	return dcontext.WithLogger(ctx, logger)
}

// GetLoggerWithField returns a logger instance with the specified field key
// and value without affecting the context. Extra specified keys will be
// resolved from the context.
func GetLoggerWithField(ctx context.Context, key, value any, keys ...any) Logger {
	return dcontext.GetLoggerWithField(ctx, key, value, keys...)
}

// GetLoggerWithFields returns a logger instance with the specified fields
// without affecting the context. Extra specified keys will be resolved from
// the context.
func GetLoggerWithFields(ctx context.Context, fields map[any]any, keys ...any) Logger {
	return dcontext.GetLoggerWithFields(ctx, fields, keys...)
}

// GetLogger returns the logger from the current context, if present. If one
// or more keys are provided, they will be resolved on the context and
// included in the logger.
func GetLogger(ctx context.Context, keys ...any) Logger {
	return dcontext.GetLogger(ctx, keys...)
}

// SetDefaultLogger sets the default logger upon which to base new loggers.
func SetDefaultLogger(logger Logger) {
	dcontext.SetDefaultLogger(logger)
}

// WithTrace allocates a traced timing span in a new context. This allows a
// caller to track the time between calling WithTrace and the returned done
// function.
func WithTrace(ctx context.Context) (context.Context, func(format string, a ...any)) {
	return dcontext.WithTrace(ctx)
}

// Since looks up key, which should be a time.Time, and returns the duration
// since that time. If the key is not found, the value returned will be zero.
func Since(ctx context.Context, key any) time.Duration {
	return dcontext.Since(ctx, key)
}

// GetStringValue returns a string value from the context. The empty string
// will be returned if not found.
func GetStringValue(ctx context.Context, key any) string {
	return dcontext.GetStringValue(ctx, key)
}

// WithVersion stores the application version in the context. The new context
// gets a logger to ensure log messages are marked with the application
// version.
func WithVersion(ctx context.Context, version string) context.Context {
	return dcontext.WithVersion(ctx, version)
}

// GetVersion returns the application version from the context. An empty
// string may returned if the version was not set on the context.
func GetVersion(ctx context.Context) string {
	return dcontext.GetVersion(ctx)
}
