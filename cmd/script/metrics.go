package main

import (
	"fmt"
	"net/http"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	metricsRegistry = make(map[string]*AggregatorMetrics)
	metricsLock     sync.Mutex

	// Combined metric to track all aggregators in one place for easy comparison
	allAggregatorLatency *prometheus.GaugeVec

	// Pool discovery latency metric
	poolDiscoveryLatency *prometheus.GaugeVec

	// REST API latency metrics
	restAPILatency       *prometheus.HistogramVec
	restAPIErrors        *prometheus.CounterVec
	restAPIStatusCodes   *prometheus.CounterVec

	// Quote API latency metrics
	quoteAPILatency     *prometheus.HistogramVec
	quoteAPIErrors      *prometheus.CounterVec
	quoteAPIStatusCodes *prometheus.CounterVec
)

func init() {
	allAggregatorLatency = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "all_aggregator_latency_milliseconds",
			Help: "Latency in milliseconds for all aggregators by blockchain and source",
		},
		[]string{"aggregator", "chain"},
	)
	prometheus.MustRegister(allAggregatorLatency)

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
}

type AggregatorMetrics struct {
	Latency *prometheus.GaugeVec
}

func GetOrCreateMetrics(aggregator string) *AggregatorMetrics {
	metricsLock.Lock()
	defer metricsLock.Unlock()

	if metrics, exists := metricsRegistry[aggregator]; exists {
		return metrics
	}

	metrics := &AggregatorMetrics{
		Latency: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: fmt.Sprintf("%s_latency_milliseconds", aggregator),
				Help: fmt.Sprintf("Latency in milliseconds for %s by blockchain", aggregator),
			},
			[]string{"chain"},
		),
	}

	prometheus.MustRegister(metrics.Latency)

	metricsRegistry[aggregator] = metrics
	return metrics
}

func RecordLatency(aggregator string, chain string, latencyMs float64) {
	// Filter out invalid values: negative or > 2 minutes (120000ms)
	if latencyMs < 0 || latencyMs > 120000 {
		return
	}

	metrics := GetOrCreateMetrics(aggregator)
	metrics.Latency.WithLabelValues(chain).Set(latencyMs)

	// Also record to the combined metric for easy comparison
	allAggregatorLatency.WithLabelValues(aggregator, chain).Set(latencyMs)
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

func StartMetricsServer(addr string) error {
	http.Handle("/metrics", promhttp.Handler())
	return http.ListenAndServe(addr, nil)
}
