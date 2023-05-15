package requesttrace

import (
	"context"
	"fmt"
	"net/http"
	"net/textproto"

	dcontext "github.com/distribution/distribution/v3/context"
)

const (
	requestHeader = "X-Registry-Request-URL"
)

type requestTracer struct {
	ctx context.Context
	req *http.Request
}

func New(ctx context.Context, req *http.Request) *requestTracer {
	return &requestTracer{
		ctx: ctx,
		req: req,
	}
}

func (rt *requestTracer) ModifyRequest(req *http.Request) (err error) {
	if rt.req != nil {
		for _, k := range rt.req.Header[textproto.CanonicalMIMEHeaderKey(requestHeader)] {
			if k == req.URL.String() {
				err = fmt.Errorf("Request to %q is denied because a loop is detected", k)
				dcontext.GetLogger(rt.ctx).Error(err.Error())
				return
			}
			req.Header.Add(requestHeader, k)
		}
	}
	req.Header.Add(requestHeader, req.URL.String())
	return
}
