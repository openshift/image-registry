package server

import (
	"net/http"

	dcontext "github.com/distribution/distribution/v3/context"
	"github.com/distribution/distribution/v3/registry/auth"
	"github.com/distribution/distribution/v3/registry/handlers"

	"github.com/openshift/image-registry/pkg/dockerregistry/server/api"
	"github.com/openshift/image-registry/pkg/dockerregistry/server/client"
)

// RegisterSignatureHandler registers the Docker image signature extension to Docker
// registry.
func RegisterSignatureHandler(app *handlers.App, isImageClient client.ImageStreamImagesNamespacer) {
	extensionsRouter := app.NewRoute().PathPrefix(api.ExtensionsPrefix).Subrouter()
	var (
		getSignatureAccess = func(r *http.Request) []auth.Access {
			return []auth.Access{
				{
					Resource: auth.Resource{
						Type: "signature",
						Name: dcontext.GetStringValue(dcontext.WithVars(app, r), "vars.name"),
					},
					Action: "get",
				},
			}
		}
		putSignatureAccess = func(r *http.Request) []auth.Access {
			return []auth.Access{
				{
					Resource: auth.Resource{
						Type: "signature",
						Name: dcontext.GetStringValue(dcontext.WithVars(app, r), "vars.name"),
					},
					Action: "put",
				},
			}
		}
	)
	app.RegisterRoute(
		"extensions-signatures-get",
		extensionsRouter.Path(api.SignaturesPath).Methods("GET"),
		NewSignatureDispatcher(isImageClient),
		handlers.NameRequired,
		getSignatureAccess,
	)
	app.RegisterRoute(
		"extensions-signatures-put",
		extensionsRouter.Path(api.SignaturesPath).Methods("PUT"),
		NewSignatureDispatcher(isImageClient),
		handlers.NameRequired,
		putSignatureAccess,
	)
}
