package collection

import (
	"context"
	"log/slog"
	"math/rand"
	"sync/atomic"

	"go.opentelemetry.io/otel/metric"
)

// requestBasedMetricCollector contains all the request based metric instruments.
type requestBasedMetricCollector struct {
	totalBytesSent   metric.Int64Counter
	totalApiRequests metric.Int64ObservableCounter
	latencyTime      metric.Int64Histogram
	config           Config
	meter            metric.Meter
	counter          int64
}

// AddApiRequest adds 1 to the rqmc counter
func (rqmc *requestBasedMetricCollector) AddApiRequest() {
	atomic.AddInt64(&rqmc.counter, 1)
}

// GetApiRequest gets the rqmc counter
func (rqmc *requestBasedMetricCollector) GetApiRequest() int {
	return int(atomic.LoadInt64(&rqmc.counter))
}

// NewRequestBasedMetricCollector returns a new type struct that holds and registers the 3 request based metric instruments used in the Go-Sample-App;
// TotalBytesSent, TotalRequests, LatencyTime
func NewRequestBasedMetricCollector(ctx context.Context, cfg Config, mp metric.MeterProvider) requestBasedMetricCollector {

	rqmc := requestBasedMetricCollector{config: cfg}
	rqmc.meter = mp.Meter("github.com/aws-otel-commnunity/sample-apps/go-sample-app/collection")
	rqmc.registerTotalBytesSent()
	rqmc.registerTotalRequests()
	rqmc.registerLatencyTime()
	return rqmc
}

// registerTotalBytesSent registers a Synchronous counter called TotalBytesSent.
func (rqmc *requestBasedMetricCollector) registerTotalBytesSent() {
	totalBytesSentMetric, err := rqmc.meter.Int64Counter(
		totalBytesSent+testingId,
		metric.WithDescription("Keeps a sum of the total amount of bytes sent while the application is alive"),
		metric.WithUnit("By"),
	)
	if err != nil {
		slog.Error("Error registering TotalBytesSent metric", err)
	}
	rqmc.totalBytesSent = totalBytesSentMetric
}

// registerTotalRequests registers an Asynchronous counter called TotalApiRequests.
func (rqmc *requestBasedMetricCollector) registerTotalRequests() {
	totalApiRequestsMetric, err := rqmc.meter.Int64ObservableCounter(
		totalApiRequests+testingId,
		metric.WithDescription("Increments by one every time a sampleapp endpoint is used"),
		metric.WithUnit("1"),
	)
	if err != nil {
		slog.Error("Error registering TotalApiRequests metric", err)
	}
	rqmc.totalApiRequests = totalApiRequestsMetric
}

// registerLatencyTime registers a Synchronous histogram called LatencyTime.
func (rqmc *requestBasedMetricCollector) registerLatencyTime() {
	latencyTimeMetric, err := rqmc.meter.Int64Histogram(
		latencyTime+testingId,
		metric.WithDescription("Measures latency time in buckets of 100 300 and 500"),
		metric.WithUnit("ms"),
	)
	if err != nil {
		slog.Error("Error registering LatencyTime metric", err)
	}
	rqmc.latencyTime = latencyTimeMetric
}

// StartTotalRequestCallBack starts the callback for the TotalApiRequests.
func (rqmc *requestBasedMetricCollector) StartTotalRequestCallback() {
	if _, err := rqmc.meter.RegisterCallback(
		// SDK periodically calls this function to collect data.
		func(ctx context.Context, o metric.Observer) error {
			o.ObserveInt64(rqmc.totalApiRequests, int64(rqmc.GetApiRequest()), metric.WithAttributes(requestMetricCommonLabels...))

			return nil
		},
		rqmc.totalApiRequests,
	); err != nil {
		panic(err)
	}
}

// UpdateTotalBytesSent updates TotalBytesSent with a value between 0 and 1024
func (rqmc *requestBasedMetricCollector) UpdateTotalBytesSent(ctx context.Context) {
	min := 0
	max := 1024
	rqmc.totalBytesSent.Add(ctx, int64(rand.Intn(max-min)+min), metric.WithAttributes(requestMetricCommonLabels...))
}

// UpdateLatencyTime updates LatencyTime adds an aditional value between 0 and 512 to the histogram distribution.
func (rqmc *requestBasedMetricCollector) UpdateLatencyTime(ctx context.Context) {
	min := 0
	max := 512
	rqmc.latencyTime.Record(ctx, int64(rand.Intn(max-min)+min), metric.WithAttributes(requestMetricCommonLabels...))
}
