package imagestream

import (
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/registry/api/errcode"
	disterrors "github.com/docker/distribution/registry/api/v2"

	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	imageapiv1 "github.com/openshift/api/image/v1"

	"github.com/openshift/image-registry/pkg/dockerregistry/server/client"
	quotautil "github.com/openshift/image-registry/pkg/origin-common/quota/util"
)

// cachedImageStreamGetter wraps a master API client for getting image streams with a cache.
type cachedImageStreamGetter struct {
	ctx               context.Context
	namespace         string
	name              string
	isNamespacer      client.ImageStreamsNamespacer
	cachedImageStream *imageapiv1.ImageStream
}

func (g *cachedImageStreamGetter) get() (*imageapiv1.ImageStream, error) {
	if g.cachedImageStream != nil {
		context.GetLogger(g.ctx).Debugf("(*cachedImageStreamGetter).getImageStream: returning cached copy")
		return g.cachedImageStream, nil
	}
	is, err := g.isNamespacer.ImageStreams(g.namespace).Get(g.name, metav1.GetOptions{})
	if err != nil {
		context.GetLogger(g.ctx).Errorf("failed to get image stream: %v", err)
		switch {
		case kerrors.IsNotFound(err):
			return nil, disterrors.ErrorCodeNameUnknown.WithDetail(err)
		case kerrors.IsForbidden(err), kerrors.IsUnauthorized(err), quotautil.IsErrorQuotaExceeded(err):
			return nil, errcode.ErrorCodeDenied.WithDetail(err)
		default:
			return nil, errcode.ErrorCodeUnknown.WithDetail(err)
		}
	}

	context.GetLogger(g.ctx).Debugf("(*cachedImageStreamGetter).getImageStream: got image stream %s/%s", is.Namespace, is.Name)
	g.cachedImageStream = is
	return is, nil
}

func (g *cachedImageStreamGetter) cacheImageStream(is *imageapiv1.ImageStream) {
	context.GetLogger(g.ctx).Debugf("(*cachedImageStreamGetter).cacheImageStream: got image stream %s/%s", is.Namespace, is.Name)
	g.cachedImageStream = is
}
