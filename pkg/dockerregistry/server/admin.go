package server

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"

	restclient "k8s.io/client-go/rest"

	"github.com/docker/distribution"
	dcontext "github.com/docker/distribution/context"
	"github.com/docker/distribution/registry/api/errcode"
	"github.com/docker/distribution/registry/api/v2"
	"github.com/docker/distribution/registry/auth"
	"github.com/docker/distribution/registry/handlers"
	"github.com/docker/distribution/registry/storage"
	storagedriver "github.com/docker/distribution/registry/storage/driver"
	gorillahandlers "github.com/gorilla/handlers"
	"github.com/opencontainers/go-digest"

	"github.com/openshift/image-registry/pkg/dockerregistry/server/api"
	"github.com/openshift/image-registry/pkg/dockerregistry/server/cache"
)

var (
	pruneService string = "image-registry-hardprune"
)

func (app *App) registerBlobHandler(dockerApp *handlers.App) {
	adminRouter := dockerApp.NewRoute().PathPrefix(api.AdminPrefix).Subrouter()
	pruneAccessRecords := func(*http.Request) []auth.Access {
		return []auth.Access{
			{
				Resource: auth.Resource{
					Type: "admin",
				},
				Action: "prune",
			},
		}
	}

	dockerApp.RegisterRoute(
		"admin-blobs",
		// DELETE /admin/blobs/<digest>
		adminRouter.Path(api.AdminPath).Methods("DELETE"),
		// handler
		app.blobDispatcher,
		// repo name not required in url
		handlers.NameNotRequired,
		// custom access records
		pruneAccessRecords,
	)
}

// blobDispatcher takes the request context and builds the appropriate handler
// for handling blob requests.
func (app *App) blobDispatcher(ctx *handlers.Context, r *http.Request) http.Handler {
	reference := dcontext.GetStringValue(ctx, "vars.digest")
	dgst, _ := digest.Parse(reference)

	blobHandler := &blobHandler{
		Cache:   app.cache,
		Context: ctx,
		driver:  app.driver,
		Digest:  dgst,
	}

	return gorillahandlers.MethodHandler{
		"DELETE": http.HandlerFunc(blobHandler.Delete),
	}
}

// blobHandler handles http operations on blobs.
type blobHandler struct {
	*handlers.Context

	driver storagedriver.StorageDriver
	Digest digest.Digest
	Cache  cache.DigestCache
}

// Delete deletes the blob from the storage backend.
func (bh *blobHandler) Delete(w http.ResponseWriter, req *http.Request) {
	defer func() {
		// TODO(dmage): log error?
		_ = req.Body.Close()
	}()

	if len(bh.Digest) == 0 {
		bh.Errors = append(bh.Errors, v2.ErrorCodeBlobUnknown)
		return
	}

	err := bh.Cache.Remove(bh.Digest)
	if err != nil {
		dcontext.GetLogger(bh).Errorf("blobHandler: ignore error: unable to remove %q from cache: %v", bh.Digest, err)
	}

	vacuum := storage.NewVacuum(bh.Context, bh.driver)

	err = vacuum.RemoveBlob(bh.Digest.String())
	if err != nil {
		// ignore not found error
		switch t := err.(type) {
		case storagedriver.PathNotFoundError:
		case errcode.Error:
			if t.Code != v2.ErrorCodeBlobUnknown {
				bh.Errors = append(bh.Errors, err)
				return
			}
		default:
			if err != distribution.ErrBlobUnknown {
				detail := fmt.Sprintf("error deleting blob %q: %v", bh.Digest, err)
				err = errcode.ErrorCodeUnknown.WithDetail(detail)
				bh.Errors = append(bh.Errors, err)
				return
			}
		}
		dcontext.GetLogger(bh).Infof("blobHandler: ignoring %T error: %v", err, err)
	}

	query := req.URL.Query()

	if len(query.Get("forwarded")) == 0 {
		go bh.propagateRequest(*req.URL)
	}

	w.WriteHeader(http.StatusNoContent)
}

func (bh *blobHandler) propagateRequest(rurl url.URL) {
	registryClient := &http.Client{
		Transport: http.DefaultTransport,
	}

	if rurl.Scheme == "https" {
		registryClient.Transport, _ = restclient.TransportFor(&restclient.Config{
			TLSClientConfig: restclient.TLSClientConfig{Insecure: true},
		})
	}

	var err error

	hostname := os.Getenv("HOSTNAME")
	if len(hostname) == 0 {
		hostname, err = os.Hostname()
		if err != nil {
			dcontext.GetLogger(bh).Errorf("propagateRequest: hostname error: %s", err)
			return
		}
	}

	hostAddrs, err := net.LookupHost(hostname)
	if err != nil {
		dcontext.GetLogger(bh).Errorf("propagateRequest: lookup hostname error: %s", err)
		return
	}

	addrs, err := net.LookupHost(pruneService)
	if err != nil {
		dcontext.GetLogger(bh).Errorf("propagateRequest: lookup error: %s", err)
		return
	}

	q := rurl.Query()
	q.Set("forwarded", "1")

	rurl.RawQuery = q.Encode()

	for _, addr := range addrs {
		if inSlice(addr, hostAddrs) {
			continue
		}

		rurl.Host = addr

		req, err := http.NewRequest("DELETE", rurl.String(), nil)
		if err != nil {
			dcontext.GetLogger(bh).Errorf("propagateRequest: unable to make request: %s", err)
			return
		}

		resp, err := registryClient.Do(req)
		if err != nil {
			dcontext.GetLogger(bh).Errorf("propagateRequest: unable to send request: %s", err)
			return
		}
		_ = resp.Body.Close()
	}
}

func inSlice(s string, arr []string) bool {
	for _, elem := range arr {
		if s == elem {
			return true
		}
	}
	return false
}
