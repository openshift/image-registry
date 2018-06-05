package errors

import (
	"net/http"

	errcode "github.com/docker/distribution/registry/api/errcode"
)

const errGroup = "openshift"

var (
	ErrorCodePullthroughManifest = errcode.Register(errGroup, errcode.ErrorDescriptor{
		Value:   "OPENSHIFT_PULLTHROUGH_MANIFEST",
		Message: "unable to pull manifest from %s: %v",
		// We have to use an error code within the range [400, 499].
		// Otherwise the error message with not be shown by the client.
		HTTPStatusCode: http.StatusNotFound,
	})
)
