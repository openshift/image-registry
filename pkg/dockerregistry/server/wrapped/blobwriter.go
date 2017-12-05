package wrapped

import (
	"github.com/docker/distribution"
	"github.com/docker/distribution/context"
)

// blobWriter wraps a distribution.BlobWriter.
type blobWriter struct {
	distribution.BlobWriter
	wrapper Wrapper
}

func (bw *blobWriter) Commit(ctx context.Context, provisional distribution.Descriptor) (canonical distribution.Descriptor, err error) {
	err = bw.wrapper(ctx, "BlobWriter.Commit", func(ctx context.Context) error {
		canonical, err = bw.BlobWriter.Commit(ctx, provisional)
		return err
	})
	return
}

func (bw *blobWriter) Cancel(ctx context.Context) error {
	return bw.wrapper(ctx, "BlobWriter.Cancel", func(ctx context.Context) error {
		return bw.BlobWriter.Cancel(ctx)
	})
}
