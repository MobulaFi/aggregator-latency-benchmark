package main

import (
	"fmt"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// Pool discovery latency metric
	poolDiscoveryLatency *prometheus.GaugeVec

	// REST API latency metrics
	restAPILatency     *prometheus.HistogramVec
	restAPIErrors      *prometheus.CounterVec
	restAPIStatusCodes *prometheus.CounterVec

	// Quote API latency metrics
	quoteAPILatency     *prometheus.HistogramVec
	quoteAPIErrors      *prometheus.CounterVec
	quoteAPIStatusCodes *prometheus.CounterVec

	// Metadata coverage metrics
	metadataCoverageTotal   *prometheus.CounterVec
	metadataCoverageSuccess *prometheus.CounterVec
	metadataAPILatency      *prometheus.HistogramVec
)

func init() {
	poolDiscoveryLatency = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "pool_discovery_latency_milliseconds",
			Help: "Time from pool creation on-chain to first trade detection (pool discovery latency)",
		},
		[]string{"aggregator", "chain"},
	)
	prometheus.MustRegister(poolDiscoveryLatency)

	// REST API latency histogram with buckets optimized for API response times
	restAPILatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "rest_api_latency_milliseconds",
			Help:    "REST API response latency in milliseconds",
			Buckets: []float64{50, 100, 200, 500, 1000, 2000, 5000, 10000},
		},
		[]string{"aggregator", "endpoint", "chain"},
	)
	prometheus.MustRegister(restAPILatency)

	// REST API errors counter
	restAPIErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rest_api_errors_total",
			Help: "Total number of REST API errors",
		},
		[]string{"aggregator", "endpoint", "chain", "error_type"},
	)
	prometheus.MustRegister(restAPIErrors)

	// REST API status codes counter
	restAPIStatusCodes = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rest_api_status_codes_total",
			Help: "Total count of REST API responses by status code",
		},
		[]string{"aggregator", "endpoint", "chain", "status_code"},
	)
	prometheus.MustRegister(restAPIStatusCodes)

	// Quote API latency histogram
	quoteAPILatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "quote_api_latency_milliseconds",
			Help:    "Quote API response latency in milliseconds",
			Buckets: []float64{50, 100, 200, 300, 500, 750, 1000, 1500, 2000, 3000, 5000},
		},
		[]string{"provider", "chain"},
	)
	prometheus.MustRegister(quoteAPILatency)

	// Quote API errors counter
	quoteAPIErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "quote_api_errors_total",
			Help: "Total number of Quote API errors",
		},
		[]string{"provider", "chain", "error_type"},
	)
	prometheus.MustRegister(quoteAPIErrors)

	// Quote API status codes counter
	quoteAPIStatusCodes = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "quote_api_status_codes_total",
			Help: "Total count of Quote API responses by status code",
		},
		[]string{"provider", "chain", "status_code"},
	)
	prometheus.MustRegister(quoteAPIStatusCodes)

	// Metadata coverage - total checks per provider/chain/field
	metadataCoverageTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "metadata_coverage_checks_total",
			Help: "Total number of metadata coverage checks",
		},
		[]string{"provider", "chain", "field"},
	)
	prometheus.MustRegister(metadataCoverageTotal)

	// Metadata coverage - successful (field present) checks
	metadataCoverageSuccess = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "metadata_coverage_success_total",
			Help: "Total number of successful metadata coverage checks (field present)",
		},
		[]string{"provider", "chain", "field"},
	)
	prometheus.MustRegister(metadataCoverageSuccess)

	// Metadata API latency
	metadataAPILatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "metadata_api_latency_milliseconds",
			Help:    "Metadata API response latency in milliseconds",
			Buckets: []float64{50, 100, 200, 500, 1000, 2000, 5000, 10000},
		},
		[]string{"provider", "chain"},
	)
	prometheus.MustRegister(metadataAPILatency)
}

func RecordPoolDiscoveryLatency(aggregator string, chain string, latencyMs float64) {
	// Filter out invalid values: negative or > 2 minutes (120000ms)
	if latencyMs < 0 || latencyMs > 120000 {
		return
	}

	poolDiscoveryLatency.WithLabelValues(aggregator, chain).Set(latencyMs)
}

// RecordRESTLatency records the latency of a REST API call
func RecordRESTLatency(aggregator string, endpoint string, chain string, latencyMs float64, statusCode int) {
	// Record latency in histogram
	restAPILatency.WithLabelValues(aggregator, endpoint, chain).Observe(latencyMs)

	// Record status code
	restAPIStatusCodes.WithLabelValues(aggregator, endpoint, chain, fmt.Sprintf("%d", statusCode)).Inc()
}

// RecordRESTError records a REST API error
func RecordRESTError(aggregator string, endpoint string, chain string, errorType string) {
	restAPIErrors.WithLabelValues(aggregator, endpoint, chain, errorType).Inc()
}

// RecordQuoteAPILatency records the latency of a Quote API call
func RecordQuoteAPILatency(provider string, chain string, latencyMs float64, statusCode int) {
	// Record latency in histogram
	quoteAPILatency.WithLabelValues(provider, chain).Observe(latencyMs)

	// Record status code
	quoteAPIStatusCodes.WithLabelValues(provider, chain, fmt.Sprintf("%d", statusCode)).Inc()
}

// RecordQuoteAPIError records a Quote API error
func RecordQuoteAPIError(provider string, chain string, errorType string) {
	quoteAPIErrors.WithLabelValues(provider, chain, errorType).Inc()
}

// RecordMetadataCoverage records metadata coverage for a specific field
func RecordMetadataCoverage(provider string, chain string, field string, present bool) {
	metadataCoverageTotal.WithLabelValues(provider, chain, field).Inc()
	if present {
		metadataCoverageSuccess.WithLabelValues(provider, chain, field).Inc()
	}
}

// RecordMetadataLatency records the latency of a metadata API call
func RecordMetadataLatency(provider string, chain string, latencyMs float64) {
	metadataAPILatency.WithLabelValues(provider, chain).Observe(latencyMs)
}

func StartMetricsServer(addr string) error {
	http.Handle("/metrics", promhttp.Handler())
	return http.ListenAndServe(addr, nil)
}
