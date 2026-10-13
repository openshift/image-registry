package requesttrace_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/openshift/image-registry/pkg/requesttrace"
	"github.com/openshift/image-registry/pkg/testutil"
)

func TestRequests(t *testing.T) {
	ctx := context.Background()
	ctx = testutil.WithTestLogger(ctx, t)

	var req *http.Request

	for _, s := range []string{
		"https://io.docker.com/openshift/origin-v1",
		"https://io.docker.com/openshift/origin-v2",
		"http://io.docker.com/openshift/origin-v1",
		"http://io.docker.com/openshift/origin-v2",
		"https://io.docker.com/openshift1/origin-v1",
		"https://io.docker.com/openshift2/origin-v2",
		"https://docker.com/openshift/origin-v1",
		"https://docker.com/openshift/origin-v2",
	} {
		rt := requesttrace.New(ctx, req)

		newReq, err := http.NewRequest("GET", s, nil)
		if err != nil {
			t.Fatalf("unable to make new request: %v", err)
		}

		if err := rt.ModifyRequest(newReq); err != nil {
			t.Fatalf("unable to modify request: %v", err)
		}

		req = newReq
	}
}

func TestLoopRequests(t *testing.T) {
	ctx := context.Background()
	ctx = testutil.WithTestLogger(ctx, t)

	var req *http.Request

	for i, s := range []string{
		"https://io.docker.com/openshift/origin-v1",
		"https://io.docker.com/openshift/origin-v2",
		"https://io.docker.com/openshift/origin-v3",
		"https://io.docker.com/openshift/origin-v1",
		"https://io.docker.com/openshift/origin-v4",
	} {
		rt := requesttrace.New(ctx, req)

		newReq, err := http.NewRequest("GET", s, nil)
		if err != nil {
			t.Fatalf("unable to make new request: %v", err)
		}

		err = rt.ModifyRequest(newReq)

		if i < 3 && err != nil {
			t.Fatalf("unable to modify request: %v", err)
		}

		if i == 3 && err == nil {
			t.Fatalf("error expected in the loop")
		}

		req = newReq
	}
}
