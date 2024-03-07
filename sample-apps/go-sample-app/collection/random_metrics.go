package collection

import (
	"context"
	"log/slog"
	"math/rand"
	"time"

	"go.opentelemetry.io/otel/metric"
)

var (
	threadCount int64 = 0
	threadsBool       = true
)

// randomMetricCollector contains all the random based metric instruments.
type randomMetricCollector struct {
	timeAlive     metric.Int64Counter
	cpuUsage      metric.Int64ObservableGauge
	totalHeapSize metric.Int64ObservableUpDownCounter
	threadsActive metric.Int64UpDownCounter
	meter         metric.Meter
}

// NewRandomMetricCollector returns a new type struct that holds and registers the 4 random based metric instruments used in the Go-Sample-App;
// HeapSize, ThreadsActive, TimeAlive, CpuUsage
func NewRandomMetricCollector(mp metric.MeterProvider) randomMetricCollector {
	rmc := randomMetricCollector{}
	rmc.meter = mp.Meter("github.com/aws-otel-commnunity/sample-apps/go-sample-app/collection")
	rmc.registerHeapSize()
	rmc.registerThreadsActive()
	rmc.registerTimeAlive()
	rmc.registerCpuUsage()
	return rmc
}

// registerTimeAlive registers a Synchronous Counter called TimeAlive.
func (rmc *randomMetricCollector) registerTimeAlive() {
	timeAliveMetric, err := rmc.meter.Int64Counter(
		timeAlive+testingId,
		metric.WithDescription("Total amount of time that the application has been alive"),
		metric.WithUnit("ms"),
	)
	if err != nil {
		slog.Error("Error registering TimeAlive metric", err)
	}
	rmc.timeAlive = timeAliveMetric
}

// registerCpuUsage registers an Asynchronous Gauge called CpuUsage.
func (rmc *randomMetricCollector) registerCpuUsage() {
	cpuUsageMetric, err := rmc.meter.Int64ObservableGauge(
		cpuUsage+testingId,
		metric.WithDescription("Cpu usage percent"),
		metric.WithUnit("1"),
	)
	if err != nil {
		slog.Error("Error registering CpuUsage metric", err)
	}
	rmc.cpuUsage = cpuUsageMetric

}

// registerHeapSize registers an Asynchronous UpDownCounter called HeapSize.
func (rmc *randomMetricCollector) registerHeapSize() {
	totalHeapSizeMetric, err := rmc.meter.Int64ObservableUpDownCounter(
		totalHeapSize+testingId,
		metric.WithDescription("The current total heap size"),
		metric.WithUnit("By"),
	)
	if err != nil {
		slog.Error("Error registering HeapSize metric", err)
	}
	rmc.totalHeapSize = totalHeapSizeMetric

}

// registerThreadsActive registers a Synchronous UpDownCounter called ThreadsActive.
func (rmc *randomMetricCollector) registerThreadsActive() {
	threadsActiveMetric, err := rmc.meter.Int64UpDownCounter(
		threadsActive+testingId,
		metric.WithUnit("1"),
		metric.WithDescription("The total amount of threads active"),
	)
	if err != nil {
		slog.Error("Error registering ThreadsActive metric", err)
	}
	rmc.threadsActive = threadsActiveMetric
}

// UpdateMetricsClient generates new metric values for Synchronous instruments every TimeInterval and
// Asynchronous instruments every CollectPeriod configured by the controller.
func (rmc *randomMetricCollector) RegisterMetricsClient(ctx context.Context, cfg Config) {
	go func() {
		for {
			rmc.updateTimeAlive(ctx, cfg)
			rmc.updateThreadsActive(ctx, cfg)
			time.Sleep(time.Second * time.Duration(cfg.TimeInterval))
		}
	}()
	rmc.updateCpuUsage(ctx, cfg)
	rmc.updateTotalHeapSize(ctx, cfg)
}

// updateTimeAlive updates TimeAlive by TimeAliveIncrementer increments.
func (rmc *randomMetricCollector) updateTimeAlive(ctx context.Context, cfg Config) {
	rmc.timeAlive.Add(ctx, cfg.TimeAliveIncrementer*1000, metric.WithAttributes(randomMetricCommonLabels...)) // in millisconds
}

// updateCpuUsage updates CpuUsage by a value between 0 and CpuUsageUpperBound every SDK call.
func (rmc *randomMetricCollector) updateCpuUsage(ctx context.Context, cfg Config) {
	min := 0
	max := int(cfg.CpuUsageUpperBound)
	if _, err := rmc.meter.RegisterCallback(
		// SDK periodically calls this function to collect data.
		func(ctx context.Context, o metric.Observer) error {
			cpuUsage := int64(rand.Intn(max-min) + min)
			o.ObserveInt64(rmc.cpuUsage, cpuUsage, metric.WithAttributes(randomMetricCommonLabels...))

			return nil
		},
		rmc.cpuUsage,
	); err != nil {
		panic(err)
	}
}

// updateTotalHeapSize updates HeapSize by a value between 0 and TotalHeapSizeUpperBound every SDK call.
func (rmc *randomMetricCollector) updateTotalHeapSize(ctx context.Context, cfg Config) {
	min := 0
	max := int(cfg.TotalHeapSizeUpperBound)
	if _, err := rmc.meter.RegisterCallback(
		// SDK periodically calls this function to collect data.
		func(ctx context.Context, o metric.Observer) error {
			totalHeapSize := int64(rand.Intn(max-min) + min)
			o.ObserveInt64(rmc.totalHeapSize, totalHeapSize, metric.WithAttributes(randomMetricCommonLabels...))

			return nil
		},
		rmc.totalHeapSize,
	); err != nil {
		panic(err)
	}
}

// updateThreadsActive updates ThreadsActive by a value between 0 and 10 in increments or decrements of 1 based on previous value.
func (rmc *randomMetricCollector) updateThreadsActive(ctx context.Context, cfg Config) {
	if threadsBool {
		if threadCount < int64(cfg.ThreadsActiveUpperBound) {
			rmc.threadsActive.Add(ctx, 1, metric.WithAttributes(randomMetricCommonLabels...))
			threadCount++
		} else {
			threadsBool = false
			threadCount--
		}

	} else {
		if threadCount > 0 {
			rmc.threadsActive.Add(ctx, -1, metric.WithAttributes(randomMetricCommonLabels...))
			threadCount--
		} else {
			threadsBool = true
			threadCount++
		}
	}
}
