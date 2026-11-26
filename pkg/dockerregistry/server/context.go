package server

import (
	"context"
	"errors"
	"net/http"
	"sync"

	dcontext "github.com/distribution/distribution/v3/context"

	"github.com/openshift/image-registry/pkg/dockerregistry/server/audit"
	"github.com/openshift/image-registry/pkg/dockerregistry/server/client"
)

// errNoRequest is returned when a context does not carry an *http.Request.
var errNoRequest = errors.New("no http request in context")

// requestFrom returns the *http.Request stored in ctx by
// distribution's dcontext.WithRequest.
func requestFrom(ctx context.Context) (*http.Request, error) {
	r, ok := ctx.Value("http.request").(*http.Request)
	if !ok || r == nil {
		return nil, errNoRequest
	}
	return r, nil
}

// authContexts holds the context produced by AccessController.Authorized for
// each in-flight request. Authorized cannot return a context, so its values are
// keyed by the original *http.Request, which both Authorized and later handlers
// reach through the "http.request" context value. Entries are removed when the
// request context is done.
var authContexts sync.Map // *http.Request -> context.Context

// storeAuthContext records authCtx as the authorization result for r.
func storeAuthContext(r *http.Request, authCtx context.Context) {
	orig, err := requestFrom(r.Context())
	if err != nil {
		orig = r
	}
	authContexts.Store(orig, authCtx)
	context.AfterFunc(orig.Context(), func() { authContexts.Delete(orig) })
}

// authContext falls back to the request's stored authorization context for any
// value the handler context does not carry, so downstream lookups (the audit
// logger, the user-info entries and the private auth keys) see what
// AccessController.authorize produced.
type authContext struct {
	context.Context
	auth context.Context
}

// Value falls through to the authorization context when the handler context
// returns nil. A nil value deliberately stored under a key is therefore
// indistinguishable from an absent key and is masked by the auth context;
// context.Context offers no presence check, and nothing in this package stores
// a nil value.
func (c authContext) Value(key interface{}) interface{} {
	if v := c.Context.Value(key); v != nil {
		return v
	}
	return c.auth.Value(key)
}

// withAuthContext wraps ctx so value lookups it cannot satisfy fall through to
// the authorization context stored for the request carried by ctx. If no such
// context exists, ctx is returned unchanged.
func withAuthContext(ctx context.Context) context.Context {
	r, err := requestFrom(ctx)
	if err != nil {
		return ctx
	}
	authCtx, ok := authContexts.Load(r)
	if !ok {
		return ctx
	}
	wrapped := authContext{Context: ctx, auth: authCtx.(context.Context)}
	// The handler context's own logger shadows the auth context's, so the
	// user-info fields are added again.
	return dcontext.WithLogger(wrapped, dcontext.GetLogger(wrapped, audit.AuditUserEntry, audit.AuditUserIDEntry))
}

// authValue looks up key in ctx and, failing that, in the authorization
// context stored for the request carried by ctx.
func authValue(ctx context.Context, key contextKey) interface{} {
	return withAuthContext(ctx).Value(key)
}

type contextKey string

const (
	// appMiddlewareKey is for testing purposes.
	appMiddlewareKey contextKey = "appMiddleware"

	// userClientKey is the key for a origin's client with the current user's
	// credentials in Contexts.
	userClientKey contextKey = "userClient"

	// authPerformedKey is the key to indicate that authentication was
	// performed in Contexts.
	authPerformedKey contextKey = "authPerformed"

	// deferredErrorsKey is the key for deferred errors in Contexts.
	deferredErrorsKey contextKey = "deferredErrors"
)

func appMiddlewareFrom(ctx context.Context) appMiddleware {
	am, _ := ctx.Value(appMiddlewareKey).(appMiddleware)
	return am
}

// withUserClient returns a new Context with the origin's client.
// This client should have the current user's credentials
func withUserClient(parent context.Context, userClient client.Interface) context.Context {
	return context.WithValue(parent, userClientKey, userClient)
}

// userClientFrom returns the origin's client stored in ctx, if any.
func userClientFrom(ctx context.Context) (client.Interface, bool) {
	userClient, ok := authValue(ctx, userClientKey).(client.Interface)
	return userClient, ok
}

// withAuthPerformed returns a new Context with indication that authentication
// was performed.
func withAuthPerformed(parent context.Context) context.Context {
	return context.WithValue(parent, authPerformedKey, true)
}

// authPerformed reports whether ctx has indication that authentication was
// performed.
func authPerformed(ctx context.Context) bool {
	authPerformed, ok := authValue(ctx, authPerformedKey).(bool)
	return ok && authPerformed
}

// withDeferredErrors returns a new Context that carries deferred errors.
func withDeferredErrors(parent context.Context, errs deferredErrors) context.Context {
	return context.WithValue(parent, deferredErrorsKey, errs)
}

// deferredErrorsFrom returns the deferred errors stored in ctx, if any.
func deferredErrorsFrom(ctx context.Context) (deferredErrors, bool) {
	errs, ok := authValue(ctx, deferredErrorsKey).(deferredErrors)
	return errs, ok
}
