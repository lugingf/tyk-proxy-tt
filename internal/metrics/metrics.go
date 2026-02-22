package metrics

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	ServiceName = "tyk-proxy"

	labelService = "service"
	labelPath    = "path"
	labelMethod  = "method"
	labelCode    = "code"

	metricLatencySum = "request_latency_sum"
	metricLatencyHis = "request_latency_his"
)

var (
	dflBuckets    = []float64{0.000001, 0.00001, 0.0001, 0.0005, 0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 1.5, 2, 2.5, 3, 3.5, 4, 5, 6, 7, 8, 10}
	dflObjectives = map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.95: 0.005, 0.99: 0.001}

	metricsOnce sync.Once
	metricsInst *Metrics
)

type Metrics struct {
	latencySum  *prometheus.SummaryVec
	latencyHist *prometheus.HistogramVec
}

type StatusRecorder struct {
	http.ResponseWriter
	Status int
}

func (r *StatusRecorder) WriteHeader(status int) {
	r.Status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *StatusRecorder) Write(body []byte) (int, error) {
	if r.Status == 0 {
		r.Status = http.StatusOK
	}
	return r.ResponseWriter.Write(body)
}

func (m *Metrics) MetricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		recorder := &StatusRecorder{ResponseWriter: w, Status: http.StatusOK}

		next.ServeHTTP(recorder, r)

		m.SaveHTTPDuration(start, routePattern(r), r.Method, recorder.Status)
	})
}

func GetMetrics() *Metrics {
	metricsOnce.Do(func() {
		m := &Metrics{}
		m.latencySum = prometheus.NewSummaryVec(
			prometheus.SummaryOpts{
				Name:        metricLatencySum,
				Help:        "Request latency (seconds)",
				ConstLabels: prometheus.Labels{labelService: ServiceName},
				Objectives:  dflObjectives,
				MaxAge:      10 * time.Minute,
				AgeBuckets:  5,
			},
			[]string{labelPath, labelMethod, labelCode},
		)
		prometheus.MustRegister(m.latencySum)

		m.latencyHist = prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:        metricLatencyHis,
				Help:        "Request latency histogram (seconds)",
				ConstLabels: prometheus.Labels{labelService: ServiceName},
				Buckets:     dflBuckets,
			},
			[]string{labelPath, labelMethod, labelCode},
		)
		prometheus.MustRegister(m.latencyHist)

		metricsInst = m
	})

	return metricsInst
}

func (m *Metrics) SaveHTTPDuration(timeSince time.Time, path, method string, code int) {
	if m == nil {
		return
	}

	codeStr := strconv.Itoa(code)
	lat := time.Since(timeSince).Seconds()

	m.latencySum.WithLabelValues(path, method, codeStr).Observe(lat)
	m.latencyHist.WithLabelValues(path, method, codeStr).Observe(lat)
}

func routePattern(r *http.Request) string {
	if r == nil {
		return "unknown"
	}

	rctx := chi.RouteContext(r.Context())
	if rctx == nil {
		return "unknown"
	}

	if pattern := rctx.RoutePattern(); pattern != "" {
		return pattern
	}

	return "unknown"
}
