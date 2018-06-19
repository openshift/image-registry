package metrics

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	namespace = "imageregistry"

	pullthroughSubsystem = "pullthrough"
)

var (
	requestDurationSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "request_duration_seconds",
			Help:      "Request latency in seconds for each operation.",
		},
		[]string{"operation", "name"},
	)

	pullthroughBlobstoreCacheRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: pullthroughSubsystem,
			Name:      "blobstore_cache_requests_total",
			Help:      "Total number of requests to the BlobStore cache.",
		},
		[]string{"type"},
	)
	pullthroughRepositoryDurationSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: pullthroughSubsystem,
			Name:      "repository_duration_seconds",
			Help:      "Latency of operations with remote registries in seconds.",
		},
		[]string{"registry", "operation"},
	)
	pullthroughRepositoryErrorsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: pullthroughSubsystem,
			Name:      "repository_errors_total",
			Help:      "Cumulative number of failed operations with remote registries.",
		},
		[]string{"registry", "operation", "code"},
	)
)

var prometheusOnce sync.Once

type prometheusSink struct{}

// NewPrometheusSink returns a sink for exposing Prometheus metrics.
func NewPrometheusSink() Sink {
	prometheusOnce.Do(func() {
		prometheus.MustRegister(requestDurationSeconds)
		prometheus.MustRegister(pullthroughBlobstoreCacheRequestsTotal)
		prometheus.MustRegister(pullthroughRepositoryDurationSeconds)
		prometheus.MustRegister(pullthroughRepositoryErrorsTotal)
	})
	return prometheusSink{}
}

func (s prometheusSink) RequestDuration(funcname, reponame string) Observer {
	return requestDurationSeconds.WithLabelValues(funcname, reponame)
}

func (s prometheusSink) PullthroughBlobstoreCacheRequests(resultType string) Counter {
	return pullthroughBlobstoreCacheRequestsTotal.WithLabelValues(resultType)
}

func (s prometheusSink) PullthroughRepositoryDuration(registry, funcname string) Observer {
	return pullthroughRepositoryDurationSeconds.WithLabelValues(registry, funcname)
}

func (s prometheusSink) PullthroughRepositoryErrors(registry, funcname, errcode string) Counter {
	return pullthroughRepositoryErrorsTotal.WithLabelValues(registry, funcname, errcode)
}
