package metrics

import (
	"github.com/amaumene/gomenarr/internal/platform/config"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds all application metrics
type Metrics struct {
	namespace string
	
	// HTTP metrics
	HTTPRequestsTotal   *prometheus.CounterVec
	HTTPRequestDuration *prometheus.HistogramVec
	
	// Database metrics
	DBQueriesTotal    *prometheus.CounterVec
	DBQueryDuration   *prometheus.HistogramVec
	
	// Cache metrics
	CacheHitsTotal   prometheus.Counter
	CacheMissesTotal prometheus.Counter
	CacheItemsTotal  prometheus.Gauge
	
	// External service metrics
	ExternalRequestsTotal   *prometheus.CounterVec
	ExternalRequestDuration *prometheus.HistogramVec
	
	// Business metrics
	MediaTotal          *prometheus.GaugeVec
	MediaOnDiskTotal    prometheus.Gauge
	NZBsTotal           prometheus.Gauge
	DownloadsTotal      *prometheus.CounterVec
	
	// Orchestrator metrics
	OrchestratorTasksTotal    *prometheus.CounterVec
	OrchestratorTaskDuration  *prometheus.HistogramVec
}

// New creates a new metrics instance
func New(cfg config.MetricsConfig) *Metrics {
	if !cfg.Enabled {
		return &Metrics{namespace: cfg.Namespace}
	}

	m := &Metrics{
		namespace: cfg.Namespace,
	}

	// HTTP metrics
	m.HTTPRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: cfg.Namespace,
			Name:      "http_requests_total",
			Help:      "Total number of HTTP requests",
		},
		[]string{"method", "path", "status"},
	)

	m.HTTPRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: cfg.Namespace,
			Name:      "http_request_duration_seconds",
			Help:      "HTTP request duration in seconds",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)

	// Database metrics
	m.DBQueriesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: cfg.Namespace,
			Name:      "db_queries_total",
			Help:      "Total number of database queries",
		},
		[]string{"operation", "status"},
	)

	m.DBQueryDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: cfg.Namespace,
			Name:      "db_query_duration_seconds",
			Help:      "Database query duration in seconds",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"operation"},
	)

	// Cache metrics
	m.CacheHitsTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: cfg.Namespace,
			Name:      "cache_hits_total",
			Help:      "Total number of cache hits",
		},
	)

	m.CacheMissesTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: cfg.Namespace,
			Name:      "cache_misses_total",
			Help:      "Total number of cache misses",
		},
	)

	m.CacheItemsTotal = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: cfg.Namespace,
			Name:      "cache_items_total",
			Help:      "Current number of items in cache",
		},
	)

	// External service metrics
	m.ExternalRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: cfg.Namespace,
			Name:      "external_requests_total",
			Help:      "Total number of external service requests",
		},
		[]string{"service", "operation", "status"},
	)

	m.ExternalRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: cfg.Namespace,
			Name:      "external_request_duration_seconds",
			Help:      "External service request duration in seconds",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"service", "operation"},
	)

	// Business metrics
	m.MediaTotal = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: cfg.Namespace,
			Name:      "media_total",
			Help:      "Total number of media items",
		},
		[]string{"type"},
	)

	m.MediaOnDiskTotal = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: cfg.Namespace,
			Name:      "media_on_disk_total",
			Help:      "Total number of media items on disk",
		},
	)

	m.NZBsTotal = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: cfg.Namespace,
			Name:      "nzbs_total",
			Help:      "Total number of NZB entries",
		},
	)

	m.DownloadsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: cfg.Namespace,
			Name:      "downloads_total",
			Help:      "Total number of downloads",
		},
		[]string{"status"},
	)

	// Orchestrator metrics
	m.OrchestratorTasksTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: cfg.Namespace,
			Name:      "orchestrator_tasks_total",
			Help:      "Total number of orchestrator tasks executed",
		},
		[]string{"task", "status"},
	)

	m.OrchestratorTaskDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: cfg.Namespace,
			Name:      "orchestrator_task_duration_seconds",
			Help:      "Orchestrator task duration in seconds",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"task"},
	)

	return m
}
