package metrics

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"

	_ "k8s.io/component-base/metrics/prometheus/restclient"
)

const (
	namespace = "imageregistry"

	httpSubsystem        = "http"
	pullthroughSubsystem = "pullthrough"
	manifestsSubsystem   = "manifests"
	storageSubsystem     = "storage"
	digestCacheSubsystem = "digest_cache"
)

var (
	requestDurationSeconds = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Namespace:  namespace,
			Name:       "request_duration_seconds",
			Help:       "Request latency in seconds for each operation.",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		},
		[]string{"operation"},
	)

	HTTPInFlightRequests = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Subsystem: httpSubsystem,
		Name:      "in_flight_requests",
		Help:      "A gauge of requests currently being served by the registry.",
	})
	HTTPRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: httpSubsystem,
			Name:      "requests_total",
			Help:      "A counter for requests to the registry.",
		},
		[]string{"code", "method"},
	)
	HTTPRequestDurationSeconds = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Namespace:  namespace,
			Subsystem:  httpSubsystem,
			Name:       "request_duration_seconds",
			Help:       "A histogram of latencies for requests to the registry.",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		},
		[]string{"method"},
	)
	HTTPRequestSizeBytes = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Namespace:  namespace,
			Subsystem:  httpSubsystem,
			Name:       "request_size_bytes",
			Help:       "A histogram of sizes of requests to the registry.",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		},
		[]string{},
	)
	HTTPResponseSizeBytes = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Namespace:  namespace,
			Subsystem:  httpSubsystem,
			Name:       "response_size_bytes",
			Help:       "A histogram of response sizes for requests to the registry.",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		},
		[]string{},
	)
	HTTPTimeToWriteHeaderSeconds = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Namespace:  namespace,
			Subsystem:  httpSubsystem,
			Name:       "time_to_write_header_seconds",
			Help:       "A histogram of request durations until the response headers are written.",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		},
		[]string{},
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
	pullthroughRepositoryDurationSeconds = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Namespace:  namespace,
			Subsystem:  pullthroughSubsystem,
			Name:       "repository_duration_seconds",
			Help:       "Latency of operations with remote registries in seconds.",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
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

	storageDurationSeconds = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Namespace:  namespace,
			Subsystem:  storageSubsystem,
			Name:       "duration_seconds",
			Help:       "Latency of operations with the storage.",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		},
		[]string{"operation"},
	)
	storageErrorsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: storageSubsystem,
			Name:      "errors_total",
			Help:      "Cumulative number of failed operations with the storage.",
		},
		[]string{"operation", "code"},
	)
	digestCacheRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: digestCacheSubsystem,
			Name:      "requests_total",
			Help:      "Total number of requests without scope to the digest cache.",
		},
		[]string{"type"},
	)
	digestCacheScopedRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: digestCacheSubsystem,
			Name:      "scoped_requests_total",
			Help:      "Total number of scoped requests to the digest cache.",
		},
		[]string{"type"},
	)
	manifestOperationsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: manifestsSubsystem,
			Name:      "operations_total",
			Help:      "A counter for manifest operations to the registry.",
		},
		[]string{"operation"},
	)
)

var prometheusOnce sync.Once

type prometheusSink struct{}

func init() {
	prometheus.MustRegister(HTTPInFlightRequests)
	prometheus.MustRegister(HTTPRequestsTotal)
	prometheus.MustRegister(HTTPRequestDurationSeconds)
	prometheus.MustRegister(HTTPRequestSizeBytes)
	prometheus.MustRegister(HTTPResponseSizeBytes)
}

// NewPrometheusSink returns a sink for exposing Prometheus metrics.
func NewPrometheusSink() Sink {
	prometheusOnce.Do(func() {
		prometheus.MustRegister(requestDurationSeconds)
		prometheus.MustRegister(pullthroughBlobstoreCacheRequestsTotal)
		prometheus.MustRegister(pullthroughRepositoryDurationSeconds)
		prometheus.MustRegister(pullthroughRepositoryErrorsTotal)
		prometheus.MustRegister(storageDurationSeconds)
		prometheus.MustRegister(storageErrorsTotal)
		prometheus.MustRegister(digestCacheRequestsTotal)
		prometheus.MustRegister(digestCacheScopedRequestsTotal)
		prometheus.MustRegister(manifestOperationsTotal)
	})
	return prometheusSink{}
}

func (s prometheusSink) PushManifest() Counter {
	return manifestOperationsTotal.WithLabelValues("push")
}

func (s prometheusSink) PullManifest() Counter {
	return manifestOperationsTotal.WithLabelValues("pull")
}

func (s prometheusSink) RequestDuration(funcname string) Observer {
	return requestDurationSeconds.WithLabelValues(funcname)
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

func (s prometheusSink) StorageDuration(funcname string) Observer {
	return storageDurationSeconds.WithLabelValues(funcname)
}

func (s prometheusSink) StorageErrors(funcname, errcode string) Counter {
	return storageErrorsTotal.WithLabelValues(funcname, errcode)
}

func (s prometheusSink) DigestCacheRequests(resultType string) Counter {
	return digestCacheRequestsTotal.WithLabelValues(resultType)
}

func (s prometheusSink) DigestCacheScopedRequests(resultType string) Counter {
	return digestCacheScopedRequestsTotal.WithLabelValues(resultType)
}
