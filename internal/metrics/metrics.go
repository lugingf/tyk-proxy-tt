package metrics

import (
	"bytes"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"

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
	dflObjectives = map[float64]float64{0.5: 0.5, 0.95: 0.95, 0.9: 0.9, 1: 1}
)

type Metrics struct {
	latencySum  *prometheus.SummaryVec
	latencyHist *prometheus.HistogramVec
	itemsAdded  *prometheus.CounterVec
	itemsCount  *prometheus.GaugeVec
}

type StatusRecorder struct {
	http.ResponseWriter
	ResponseBody string
	Status       int
}

func (r *StatusRecorder) WriteHeader(status int) {
	r.Status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *StatusRecorder) Write(body []byte) (int, error) {
	r.ResponseBody = string(body)
	return r.ResponseWriter.Write(body)
}

func (m Metrics) MetricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorder := &StatusRecorder{
			ResponseWriter: w,
			Status:         200,
		}

		request, err := io.ReadAll(r.Body)
		if err != nil {
			log.Error().Err(err).Msg("http_logger: read request body")
		}

		r.Body = io.NopCloser(bytes.NewBuffer(request))

		start := time.Now()

		routeContext := chi.RouteContext(r.Context())
		fullPath := routeContext.RoutePath
		path := strings.Join(routeContext.RoutePatterns, "")

		log.Info().
			Str("path", fullPath).
			Str("method", r.Method).
			Interface("params", r.URL.Query()).
			Msg("Request")

		next.ServeHTTP(recorder, r)

		log.Info().
			Int("status", recorder.Status).
			Str("body", recorder.ResponseBody).
			Msg("Response")

		m.SaveHTTPDuration(start, path, r.Method, recorder.Status)
	})
}

func GetMetrics() *Metrics {
	var m = &Metrics{}
	m.latencySum = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Name:        metricLatencySum,
			Help:        "Request latency (seconds)",
			ConstLabels: prometheus.Labels{labelService: ServiceName},
			Objectives:  dflObjectives,
			MaxAge:      500 * time.Second,
			AgeBuckets:  1,
		},
		[]string{labelPath, labelMethod, labelCode},
	)
	prometheus.MustRegister(m.latencySum)

	m.latencyHist = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:        metricLatencyHis,
		Help:        "Request latency histogram (seconds)",
		ConstLabels: prometheus.Labels{labelService: ServiceName},
		Buckets:     dflBuckets,
	},
		[]string{labelPath, labelMethod, labelCode},
	)
	prometheus.MustRegister(m.latencyHist)

	return m
}

func (m Metrics) SaveHTTPDuration(timeSince time.Time, path, method string, code int) {
	codeStr := strconv.Itoa(code)
	lat := time.Since(timeSince).Seconds()

	m.latencySum.WithLabelValues(path, method, codeStr).Observe(lat)
	m.latencyHist.WithLabelValues(path, method, codeStr).Observe(lat)
}

func (m Metrics) SaveDuration(timeSince time.Time, method string) {
	m.latencySum.WithLabelValues(method).
		Observe(time.Since(timeSince).Seconds())

	m.latencyHist.WithLabelValues(method).
		Observe(time.Since(timeSince).Seconds())
}

func (m Metrics) ItemsCountAdd(count int) {
	m.itemsAdded.WithLabelValues().Add(float64(count))
}

func (m Metrics) ItemsCountSet(count int) {
	m.itemsCount.WithLabelValues().Set(float64(count))
}
