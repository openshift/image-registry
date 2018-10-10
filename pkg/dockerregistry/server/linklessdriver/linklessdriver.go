package linklessdriver

import (
	"context"
	"strings"

	storagedriver "github.com/docker/distribution/registry/storage/driver"
)

type nullWriter struct {
	size int64
}

var _ storagedriver.FileWriter = &nullWriter{}

func (w *nullWriter) Write(p []byte) (n int, err error) {
	n = len(p)
	w.size += int64(n)
	return
}

func (w *nullWriter) Close() error {
	return nil
}

func (w *nullWriter) Cancel() error {
	return nil
}

func (w *nullWriter) Commit() error {
	return nil
}

func (w *nullWriter) Size() int64 {
	return w.size
}

type linklessStorageDriver struct {
	storagedriver.StorageDriver
}

var _ storagedriver.StorageDriver = &linklessStorageDriver{}

func StorageDriver(driver storagedriver.StorageDriver) storagedriver.StorageDriver {
	return &linklessStorageDriver{
		StorageDriver: driver,
	}
}

func (l *linklessStorageDriver) PutContent(ctx context.Context, path string, content []byte) error {
	if strings.HasSuffix(path, "/link") {
		return nil
	}
	return l.StorageDriver.PutContent(ctx, path, content)
}

func (l *linklessStorageDriver) Writer(ctx context.Context, path string, append bool) (storagedriver.FileWriter, error) {
	if strings.HasSuffix(path, "/link") {
		return &nullWriter{}, nil
	}
	return l.StorageDriver.Writer(ctx, path, append)
}
