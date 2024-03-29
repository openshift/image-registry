package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"

	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	dcontext "github.com/distribution/distribution/v3/context"
	"github.com/distribution/distribution/v3/registry/api/errcode"
	v2 "github.com/distribution/distribution/v3/registry/api/v2"
	"github.com/distribution/distribution/v3/registry/handlers"

	imageapiv1 "github.com/openshift/api/image/v1"

	gorillahandlers "github.com/gorilla/handlers"
	"github.com/openshift/image-registry/pkg/dockerregistry/server/client"
	rerrors "github.com/openshift/image-registry/pkg/errors"
	"github.com/openshift/library-go/pkg/image/imageutil"
	imageref "github.com/openshift/library-go/pkg/image/reference"
)

const (
	errGroup             = "registry.api.v2"
	defaultSchemaVersion = 2
)

// signature represents a Docker image signature.
type signature struct {
	// Version specifies the schema version
	Version int `json:"schemaVersion"`
	// Name must be in "sha256:<digest>@signatureName" format
	Name string `json:"name"`
	// Type is optional, of not set it will be defaulted to "AtomicImageV1"
	Type string `json:"type"`
	// Content contains the signature
	Content []byte `json:"content"`
}

// signatureList represents list of Docker image signatures.
type signatureList struct {
	Signatures []signature `json:"signatures"`
}

var (
	ErrorCodeSignatureInvalid = errcode.Register(errGroup, errcode.ErrorDescriptor{
		Value:          "SIGNATURE_INVALID",
		Message:        "invalid image signature",
		HTTPStatusCode: http.StatusBadRequest,
	})

	ErrorCodeSignatureAlreadyExists = errcode.Register(errGroup, errcode.ErrorDescriptor{
		Value:          "SIGNATURE_EXISTS",
		Message:        "image signature already exists",
		HTTPStatusCode: http.StatusConflict,
	})
)

type signatureHandler struct {
	ctx           *handlers.Context
	reference     imageref.DockerImageReference
	isImageClient client.ImageStreamImagesNamespacer
}

// NewSignatureDispatcher provides a function that handles the GET and PUT
// requests for signature endpoint.
func NewSignatureDispatcher(isImageClient client.ImageStreamImagesNamespacer) func(*handlers.Context, *http.Request) http.Handler {
	return func(ctx *handlers.Context, r *http.Request) http.Handler {
		reference, _ := imageref.Parse(
			dcontext.GetStringValue(ctx, "vars.name") + "@" + dcontext.GetStringValue(ctx, "vars.digest"),
		)
		signatureHandler := &signatureHandler{
			ctx:           ctx,
			isImageClient: isImageClient,
			reference:     reference,
		}
		return gorillahandlers.MethodHandler{
			"GET": http.HandlerFunc(signatureHandler.Get),
			"PUT": http.HandlerFunc(signatureHandler.Put),
		}
	}
}

func (s *signatureHandler) Put(w http.ResponseWriter, r *http.Request) {
	dcontext.GetLogger(s.ctx).Debugf("(*signatureHandler).Put")
	if len(s.reference.String()) == 0 {
		s.handleError(s.ctx, v2.ErrorCodeNameInvalid.WithDetail("missing image name or image ID"), w)
		return
	}

	client, ok := userClientFrom(s.ctx)
	if !ok {
		s.handleError(s.ctx, errcode.ErrorCodeUnknown.WithDetail("unable to get origin client"), w)
		return
	}

	sig := signature{}
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		s.handleError(s.ctx, ErrorCodeSignatureInvalid.WithDetail(err.Error()), w)
		return
	}
	if err := json.Unmarshal(body, &sig); err != nil {
		s.handleError(s.ctx, ErrorCodeSignatureInvalid.WithDetail(err.Error()), w)
		return
	}

	if len(sig.Type) == 0 {
		sig.Type = imageapiv1.ImageSignatureTypeAtomicImageV1
	}
	if sig.Version != defaultSchemaVersion {
		s.handleError(s.ctx, ErrorCodeSignatureInvalid.WithDetail(errors.New("only schemaVersion=2 is currently supported")), w)
		return
	}
	newSig := &imageapiv1.ImageSignature{Content: sig.Content, Type: sig.Type}
	newSig.Name = sig.Name

	_, err = client.ImageSignatures().Create(s.ctx, newSig, metav1.CreateOptions{})
	switch {
	case err == nil:
	case kapierrors.IsUnauthorized(err):
		s.handleError(s.ctx, errcode.ErrorCodeUnauthorized.WithDetail(err.Error()), w)
		return
	case kapierrors.IsBadRequest(err):
		s.handleError(s.ctx, ErrorCodeSignatureInvalid.WithDetail(err.Error()), w)
		return
	case kapierrors.IsNotFound(err):
		w.WriteHeader(http.StatusNotFound)
		return
	case kapierrors.IsAlreadyExists(err):
		s.handleError(s.ctx, ErrorCodeSignatureAlreadyExists.WithDetail(err.Error()), w)
		return
	default:
		s.handleError(s.ctx, errcode.ErrorCodeUnknown.WithDetail(fmt.Sprintf("unable to create image %s signature: %v", s.reference.String(), err)), w)
		return
	}

	// Return just 201 with no body.
	// TODO: The docker registry actually returns the Location header
	w.WriteHeader(http.StatusCreated)
	dcontext.GetLogger(s.ctx).Debugf("(*signatureHandler).Put signature successfully added to %s", s.reference.String())
}

func (s *signatureHandler) Get(w http.ResponseWriter, req *http.Request) {
	dcontext.GetLogger(s.ctx).Debugf("(*signatureHandler).Get")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if len(s.reference.String()) == 0 {
		s.handleError(s.ctx, v2.ErrorCodeNameInvalid.WithDetail("missing image name or image ID"), w)
		return
	}

	if len(s.reference.ID) == 0 {
		s.handleError(s.ctx, v2.ErrorCodeNameInvalid.WithDetail("the image ID must be specified (sha256:<digest>"), w)
		return
	}

	image, err := s.isImageClient.ImageStreamImages(s.reference.Namespace).Get(s.ctx, imageutil.JoinImageStreamImage(s.reference.Name, s.reference.ID), metav1.GetOptions{})
	switch {
	case err == nil:
	case kapierrors.IsUnauthorized(err):
		s.handleError(s.ctx, errcode.ErrorCodeUnauthorized.WithDetail(fmt.Sprintf("not authorized to get image %q signature: %v", s.reference.String(), err)), w)
		return
	case kapierrors.IsNotFound(err):
		w.WriteHeader(http.StatusNotFound)
		return
	default:
		s.handleError(s.ctx, errcode.ErrorCodeUnknown.WithDetail(fmt.Sprintf("unable to get image %q signature: %v", s.reference.String(), err)), w)
		return
	}

	// Transform the OpenShift ImageSignature into Registry signature object.
	signatures := signatureList{Signatures: []signature{}}
	for _, s := range image.Image.Signatures {
		signatures.Signatures = append(signatures.Signatures, signature{
			Version: defaultSchemaVersion,
			Name:    s.Name,
			Type:    s.Type,
			Content: s.Content,
		})
	}

	if data, err := json.Marshal(signatures); err != nil {
		s.handleError(s.ctx, errcode.ErrorCodeUnknown.WithDetail(fmt.Sprintf("failed to serialize image signature %v", err)), w)
	} else {
		// TODO(dmage): log error?
		_, _ = w.Write(data)
	}
}

func (s *signatureHandler) handleError(ctx context.Context, err error, w http.ResponseWriter) {
	rerrors.Handle(ctx, "signature response completed with error", err)
	ctx, w = dcontext.WithResponseWriter(ctx, w)
	if serveErr := errcode.ServeJSON(w, err); serveErr != nil {
		dcontext.GetResponseLogger(ctx).Errorf("error sending error response: %v", serveErr)
		return
	}
}
