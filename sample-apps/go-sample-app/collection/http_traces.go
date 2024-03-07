package collection

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/jmoiron/sqlx"
	"go.opentelemetry.io/otel/trace"
)

// Contains all of the endpoint logic.

type response struct {
	TraceID string `json:"traceId"`
}

type s3Client struct {
	client *s3.S3
}

func NewS3Client() (*s3Client, error) {
	s, err := session.NewSession()
	if err != nil {
		return nil, err
	}
	svc := s3.New(s)
	return &s3Client{client: svc}, nil
}

// AwsSdkCall mocks a request to s3. ListBuckets are nil so no credentials are needed.
// Generates an Xray Trace ID.
func AwsSdkCall(w http.ResponseWriter, r *http.Request, rqmc *requestBasedMetricCollector, s3 *s3Client) {
	w.Header().Set("Content-Type", "application/json")

	s3.client.ListBuckets(nil) // nil or else would need real aws credentials

	ctx, span := tracer.Start(
		r.Context(),
		"aws-sdk-call",
		trace.WithAttributes(traceCommonLabels...),
	)
	defer span.End()

	// Request based metrics provided by rqmc
	rqmc.AddApiRequest()
	rqmc.UpdateTotalBytesSent(ctx)
	rqmc.UpdateLatencyTime(ctx)

	writeResponse(span, w)
}

// OutgoingSampleApp makes a request to another Sampleapp and generates an Xray Trace ID. It will also make a request to amazon.com.
func OutgoingSampleApp(w http.ResponseWriter, r *http.Request, client http.Client, rqmc *requestBasedMetricCollector) {

	ctx, span := tracer.Start(
		r.Context(),
		"invoke-sample-apps",
		trace.WithAttributes(traceCommonLabels...),
	)
	defer span.End()
	count := len(rqmc.config.SampleAppPorts)

	// If there are no sample app port list is empty then make a request to amazon.com (leaf request)
	if count == 0 {
		ctx, span := tracer.Start(
			ctx,
			"leaf-request",
			trace.WithAttributes(traceCommonLabels...),
		)

		req, _ := http.NewRequestWithContext(ctx, "GET", "https://aws.amazon.com", nil)
		res, err := client.Do(req)
		if err != nil {
			slog.ErrorContext(ctx, "Error making request to amazon.com", "error", err)
		}

		defer res.Body.Close()
		// Request based metrics provided by rqmc
		rqmc.AddApiRequest()
		rqmc.UpdateTotalBytesSent(ctx)
		rqmc.UpdateLatencyTime(ctx)

		span.End()

	} else { // If there are sample app ports to make a request to (chain request)
		invokeSampleApps(ctx, client, rqmc)
	}
	writeResponse(span, w)

}

// invokeSampleApps loops through the list of sample app ports provided in the configuration file and makes a call to invoke().
func invokeSampleApps(ctx context.Context, client http.Client, rqmc *requestBasedMetricCollector) {

	for _, port := range rqmc.config.SampleAppPorts {
		if port != "" {
			invoke(ctx, port, client)
		}
	}
}

// invoke uses the port given in the parameters to make an http request.
func invoke(ctx context.Context, port string, client http.Client) {

	ctx, span := tracer.Start(
		ctx,
		"invoke-sample-app",
		trace.WithAttributes(traceCommonLabels...),
	)
	// Consider making requests on other than localhost
	addr := "http://" + net.JoinHostPort("0.0.0.0", port) + "/outgoing-sampleapp"
	req, _ := http.NewRequestWithContext(ctx, "GET", addr, nil)
	slog.InfoContext(ctx, "Making request to sample app on port "+port)
	res, err := client.Do(req)

	if err != nil {
		slog.ErrorContext(ctx, "Error making request to amazon.com", "error", err)
	}

	defer res.Body.Close()
	defer span.End()

}

// OutgoingHttpCall makes an HTTP GET request to https://aws.amazon.com/ and generates an Xray Trace ID.
func OutgoingHttpCall(w http.ResponseWriter, r *http.Request, client http.Client, rqmc *requestBasedMetricCollector) {

	w.Header().Set("Content-Type", "application/json")

	ctx, span := tracer.Start(
		r.Context(),
		"outgoing-http-call",
		trace.WithAttributes(traceCommonLabels...),
	)

	defer span.End()

	slog.InfoContext(ctx, "Making request to aws.amazon.com")
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://aws.amazon.com/", nil)
	res, err := client.Do(req)
	if err != nil {
		slog.ErrorContext(ctx, "Error making request to amazon.com", "error", err)
	}

	defer res.Body.Close()

	// Request based metrics provided by rqmc
	rqmc.AddApiRequest()
	rqmc.UpdateTotalBytesSent(ctx)
	rqmc.UpdateLatencyTime(ctx)
	writeResponse(span, w)

}

// OutgoingPsqlCall makes a SQL request to a database and generates an Xray Trace ID.
func OutgoingPsqlCall(w http.ResponseWriter, r *http.Request, client http.Client, rqmc *requestBasedMetricCollector, conn *sqlx.DB) {

	w.Header().Set("Content-Type", "application/json")

	ctx, span := tracer.Start(
		r.Context(),
		"outgoing-psql-call",
		trace.WithAttributes(traceCommonLabels...),
	)

	defer span.End()

	for i := 0; i < 10; i++ {
		slog.InfoContext(ctx, "Making SQL request to database "+strconv.Itoa(i))
		row := conn.QueryRowContext(ctx, "SELECT 1")
		var result int
		err := row.Scan(&result)
		if err != nil {
			slog.ErrorContext(ctx, "Error making request to amazon.com", "error", err)
		}
	}

	req, _ := http.NewRequestWithContext(ctx, "GET", "https://aws.amazon.com/", nil)
	res, err := client.Do(req)
	if err != nil {
		slog.ErrorContext(ctx, "Error making request to amazon.com", "error", err)
	}

	defer res.Body.Close()

	// Request based metrics provided by rqmc
	rqmc.AddApiRequest()
	rqmc.UpdateTotalBytesSent(ctx)
	rqmc.UpdateLatencyTime(ctx)
	writeResponse(span, w)

}

// getXrayTraceID generates a trace ID in Xray format from the span context.
func getXrayTraceID(span trace.Span) string {
	xrayTraceID := span.SpanContext().TraceID().String()
	return fmt.Sprintf("1-%s-%s", xrayTraceID[0:8], xrayTraceID[8:])
}

func writeResponse(span trace.Span, w http.ResponseWriter) {
	xrayTraceID := getXrayTraceID(span)
	payload, _ := json.Marshal(response{TraceID: xrayTraceID})
	w.Header().Set("Content-Type", "application/json")
	w.Write(payload)
}
