package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"math/rand"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	_ "github.com/lib/pq"

	"github.com/XSAM/otelsql"
	"github.com/aws-otel-commnunity/sample-apps/go-sample-app/collection"
	"github.com/gorilla/mux"
	"github.com/jmoiron/sqlx"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gorilla/mux/otelmux"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"

	"go.opentelemetry.io/contrib/instrumentation/github.com/aws/aws-sdk-go-v2/otelaws"
)

var logger = slog.New(slog.NewJSONHandler(os.Stdout, nil))

type LogHandler struct {
	slog.Handler
}

func NewLogHandler(s slog.Handler) LogHandler {
	return LogHandler{
		Handler: s,
	}
}

func (h LogHandler) Handle(ctx context.Context, r slog.Record) error {
	sc := trace.SpanContextFromContext(ctx)
	if sc.IsValid() {
		r.AddAttrs(
			slog.String("trace", sc.TraceID().String()),
			slog.String("span_id", sc.SpanID().String()),
		)
	}
	return h.Handler.Handle(ctx, r)

}

func LoggerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		start := time.Now()
		wrappedWriter := wrapResponseWriter(w)
		next.ServeHTTP(wrappedWriter, r)
		duration := time.Since(start)

		go func() {
			l := slog.New(NewLogHandler(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
				AddSource: true,
				Level:     slog.LevelInfo,
			})))
			// todo: add date
			l.InfoContext(r.Context(), "request", "method", r.Method, "path", r.URL.Path, "status", wrappedWriter.status, "duration", duration.String(), "datetime", time.Now().Format(time.RFC3339))
		}()
	})
}

// responseWriterをラップする構造体とそのコンストラクタ
type responseWriter struct {
	http.ResponseWriter
	status int
}

func wrapResponseWriter(w http.ResponseWriter) *responseWriter {
	// デフォルトのステータスコードは200 OK
	return &responseWriter{w, http.StatusOK}
}

// WriteHeaderをオーバーライドしてステータスコードをキャプチャ
func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

// This sample application is in conformance with the ADOT SampleApp requirements spec.
func main() {
	ctx := context.Background()

	// The seed for 'random' values used in this applicaiton
	rand.Seed(time.Now().UnixNano())

	// Client starts
	shutdown, err := collection.StartClient(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer shutdown(ctx)

	// (Metric related) Creates and configures random based metrics based on a configuration file (config.yaml).
	mp := otel.GetMeterProvider()
	cfg := collection.GetConfiguration()

	// (Metric related) Starts request based metric and registers necessary callbacks
	rmc := collection.NewRandomMetricCollector(mp)
	rmc.RegisterMetricsClient(ctx, *cfg)
	rqmc := collection.NewRequestBasedMetricCollector(ctx, *cfg, mp)
	rqmc.StartTotalRequestCallback()

	awsCfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		panic("configuration error, " + err.Error())
	}
	otelaws.AppendMiddlewares(&awsCfg.APIOptions)

	s3Client, err := collection.NewS3Client()
	if err != nil {
		logger.Error("Error creating S3 client", "error", err)
	}

	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		os.Getenv("POSTGRES_HOST"),
		os.Getenv("POSTGRES_PORT"),
		os.Getenv("POSTGRES_USER"),
		os.Getenv("POSTGRES_PASSWORD"),
		os.Getenv("POSTGRES_DB"),
	)
	db, err := otelsql.Open("postgres", dsn)
	if err != nil {
		log.Fatal(err)
	}
	conn := sqlx.NewDb(db, "postgres")
	if err := conn.Ping(); err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	// Creates a router, client and web server with several endpoints
	r := mux.NewRouter()
	client := http.Client{
		Transport: otelhttp.NewTransport(http.DefaultTransport),
	}

	r.Use(otelmux.Middleware("Go-Sampleapp-Server"))

	// Three endpoints
	r.HandleFunc("/aws-sdk-call", func(w http.ResponseWriter, r *http.Request) {
		collection.AwsSdkCall(w, r, &rqmc, s3Client)
	})

	r.HandleFunc("/outgoing-http-call", func(w http.ResponseWriter, r *http.Request) {
		collection.OutgoingHttpCall(w, r, client, &rqmc)
	})

	r.HandleFunc("/outgoing-sampleapp", func(w http.ResponseWriter, r *http.Request) {
		collection.OutgoingSampleApp(w, r, client, &rqmc)
	})

	r.HandleFunc("/outgoing-psql-call", func(w http.ResponseWriter, r *http.Request) {
		collection.OutgoingPsqlCall(w, r, client, &rqmc, conn)
	})

	r.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		html := `
		<!DOCTYPE html>
		<html lang="en">
		<head>
			<meta charset="UTF-8">
			<title>Test Page</title>
		</head>
		<body>
			<p><a href="/aws-sdk-call">/aws-sdk-call</a>: make an AWS SDK call</p>
			<p><a href="/outgoing-http-call">/outgoing-http-call</a>: make an outgoing HTTP call</p>
			<p><a href="/outgoing-sampleapp">/outgoing-sampleapp</a>: make an outgoing call to another sample app</p>
			<p><a href="/outgoing-psql-call">/outgoing-psql-call</a>: make an outgoing call to a Postgres database</p>
		</body>
		</html>`
		fmt.Fprint(w, html)
	})
	// Root endpoint
	http.Handle("/", r)

	srv := &http.Server{
		Addr:    net.JoinHostPort(cfg.Host, cfg.Port),
		Handler: LoggerMiddleware(r),
	}
	logger.Info("Listening on port: " + cfg.Port)
	log.Fatal(srv.ListenAndServe())

}
