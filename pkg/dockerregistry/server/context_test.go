package server

import (
	"context"
	"net/http"
	"testing"
	"time"

	dcontext "github.com/distribution/distribution/v3/context"
	"github.com/sirupsen/logrus"

	"github.com/openshift/image-registry/pkg/dockerregistry/server/audit"
)

func withAppMiddleware(parent context.Context, am appMiddleware) context.Context {
	return context.WithValue(parent, appMiddlewareKey, am)
}

func TestWithAuthContextFallsThrough(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "https://registry/v2/", nil)
	if err != nil {
		t.Fatal(err)
	}

	authCtx := WithUserInfoLogger(context.Background(), "alice", "42")
	authCtx = audit.WithLogger(authCtx, audit.GetLogger(authCtx))
	storeAuthContext(req, authCtx)

	// The handler context carries the request and its own logger, as
	// distribution provides it.
	handlerCtx := dcontext.WithRequest(context.Background(), req)
	handlerCtx = dcontext.WithLogger(handlerCtx, dcontext.GetLogger(handlerCtx))
	ctx := withAuthContext(handlerCtx)

	if !audit.LoggerExists(ctx) {
		t.Error("expected the audit logger to be visible through the auth context")
	}
	if got := ctx.Value(audit.AuditUserEntry); got != "alice" {
		t.Errorf("expected user entry %q, got %#v", "alice", got)
	}
	fields := dcontext.GetLogger(ctx).(*logrus.Entry).Data
	if fields[audit.AuditUserEntry] != "alice" || fields[audit.AuditUserIDEntry] != "42" {
		t.Errorf("expected user-info logger fields, got %v", fields)
	}
}

func TestStoreAuthContextRemovedWhenRequestDone(t *testing.T) {
	reqCtx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, "https://registry/v2/", nil)
	if err != nil {
		t.Fatal(err)
	}

	storeAuthContext(req, context.Background())
	if _, ok := authContexts.Load(req); !ok {
		t.Fatal("expected a stored auth context")
	}

	cancel()
	// AfterFunc runs the cleanup in its own goroutine.
	for i := 0; i < 100; i++ {
		if _, ok := authContexts.Load(req); !ok {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Error("expected the auth context to be removed once the request is done")
}

func TestWithAuthContextNoStoredContext(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "https://registry/v2/", nil)
	if err != nil {
		t.Fatal(err)
	}

	ctx := withAuthContext(dcontext.WithRequest(context.Background(), req))

	if audit.LoggerExists(ctx) {
		t.Error("expected no audit logger without a stored auth context")
	}
	if got := ctx.Value(audit.AuditUserEntry); got != nil {
		t.Errorf("expected nil user entry, got %#v", got)
	}
}
