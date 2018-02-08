package metrics

import (
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/docker/distribution"
	"github.com/docker/distribution/context"

	"github.com/openshift/image-registry/pkg/dockerregistry/server/wrapped"
)

const (
	registryNamespace = "openshift"
	registrySubsystem = "registry"
)

var (
	registryAPIRequests *prometheus.HistogramVec
)

// Register the metrics.
func Register() {
	registryAPIRequests = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: registryNamespace,
			Subsystem: registrySubsystem,
			Name:      "request_duration_seconds",
			Help:      "Request latency summary in microseconds for each operation",
		},
		[]string{"operation", "name"},
	)
	prometheus.MustRegister(registryAPIRequests)
}

// Timer is a helper type to time functions.
type Timer interface {
	// Stop records the duration passed since the Timer was created with NewTimer.
	Stop()
}

// NewTimer wraps the HistogramVec and used to track amount of time passed since the Timer was created.
func NewTimer(collector *prometheus.HistogramVec, labels []string) Timer {
	return &metricTimer{
		collector: collector,
		labels:    labels,
		startTime: time.Now(),
	}
}

type metricTimer struct {
	collector *prometheus.HistogramVec
	labels    []string
	startTime time.Time
}

func (m *metricTimer) Stop() {
	m.collector.WithLabelValues(m.labels...).Observe(float64(time.Since(m.startTime)) / float64(time.Second))
}

func newWrapper(reponame string) wrapped.Wrapper {
	return func(ctx context.Context, funcname string, f func(ctx context.Context) error) error {
		defer NewTimer(registryAPIRequests, []string{strings.ToLower(funcname), reponame}).Stop()
		return f(ctx)
	}
}

// NewBlobStore wraps a distribution.BlobStore to collect statistics.
func NewBlobStore(bs distribution.BlobStore, reponame string) distribution.BlobStore {
	return wrapped.NewBlobStore(bs, newWrapper(reponame))
}

// NewManifestService wraps a distribution.ManifestService to collect statistics
func NewManifestService(ms distribution.ManifestService, reponame string) distribution.ManifestService {
	return wrapped.NewManifestService(ms, newWrapper(reponame))
}

// NewTagService wraps a distribution.TagService to collect statistics
func NewTagService(ts distribution.TagService, reponame string) distribution.TagService {
	return wrapped.NewTagService(ts, newWrapper(reponame))
}
