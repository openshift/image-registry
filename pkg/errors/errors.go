package errors

import (
	"fmt"
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

// Error provides a wrapper around error.
type Error interface {
	error
	Code() string
	Message() string
	Unwrap() error

	// internal is an unexported method to prevent external implementations.
	internal()
}

type registryError struct {
	code    string
	message string
	err     error
}

func NewError(code, msg string, err error) Error {
	return &registryError{
		code:    code,
		message: msg,
		err:     err,
	}
}

func (e registryError) Error() string {
	if e.err != nil {
		return fmt.Sprintf("%s: %s: %s", e.code, e.message, e.err.Error())
	}
	return fmt.Sprintf("%s: %s", e.code, e.message)
}

func (e registryError) Code() string {
	return e.code
}

func (e registryError) Message() string {
	return e.message
}

func (e registryError) Unwrap() error {
	return e.err
}

func (registryError) internal() {}
