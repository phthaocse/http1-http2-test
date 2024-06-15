package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/prometheus"
	otelMetric "go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.25.0"
	"golang.org/x/net/http2"
	"io"
	"log"
	"net"
	"net/http"
	url2 "net/url"
	"os"
	"sync"

	"time"
)

type ApplicationResource struct {
	ServiceName string
	Version     string
	Env         string
}

var (
	DefaultHistogramBoundariesPrometheus = []float64{
		0.01, 0.05, 0.1, 0.2, 0.3, 0.4, 0.5, 0.8,
		1, 1.3, 2, 3, 5, 8,
		10, 13, 25, 50, 75, 100, 250, 500, 750, 1000, 2500, 5000, 7500, 10000, 15000,
	}
)

const (
	HTTPServerMetricComponent string = "http_server"
)

const (
	HTTPRequestDurationSecondsAttribute string = "request_duration_seconds"
)

func DefaultAggregationSelector(ik sdkmetric.InstrumentKind) sdkmetric.Aggregation {
	switch ik {
	case sdkmetric.InstrumentKindCounter, sdkmetric.InstrumentKindUpDownCounter, sdkmetric.InstrumentKindObservableCounter, sdkmetric.InstrumentKindObservableUpDownCounter:
		return sdkmetric.AggregationSum{}
	case sdkmetric.InstrumentKindObservableGauge, sdkmetric.InstrumentKindGauge:
		return sdkmetric.AggregationLastValue{}
	case sdkmetric.InstrumentKindHistogram:
		return sdkmetric.AggregationExplicitBucketHistogram{
			Boundaries: DefaultHistogramBoundariesPrometheus,
			NoMinMax:   false,
		}
	}
	panic("unknown instrumentation kind")
}

var initResourcesOnce sync.Once

func initResource(rs ApplicationResource) (resource *sdkresource.Resource) {
	initResourcesOnce.Do(func() {
		extraResources, _ := sdkresource.New(
			context.Background(),
			sdkresource.WithOS(),
			sdkresource.WithProcess(),
			sdkresource.WithContainer(),
			sdkresource.WithHost(),
		)
		applicationResource := sdkresource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(rs.ServiceName),
			semconv.ServiceVersionKey.String(rs.Version),
			attribute.String("environment", rs.Env),
			attribute.String("application", rs.ServiceName),
		)
		svcResource, _ := sdkresource.Merge(extraResources, applicationResource)
		resource, _ = sdkresource.Merge(sdkresource.Default(), svcResource)
	})
	return resource
}
func InitMeterProvideWith(ctx context.Context, exporters []string, rs ApplicationResource) (*sdkmetric.MeterProvider, error) {
	var opts []sdkmetric.Option
	for _, exporterName := range exporters {
		switch exporterName {
		case "prometheus":
			promExporter, err := prometheus.New(
				prometheus.WithAggregationSelector(DefaultAggregationSelector))
			if err != nil {
				return nil, errors.New("failed to initialize prometheus exporter")
			}
			opts = append(opts, sdkmetric.WithReader(promExporter))
		}
	}

	opts = append(opts, sdkmetric.WithResource(initResource(rs)))

	mp := sdkmetric.NewMeterProvider(
		opts...,
	)
	otel.SetMeterProvider(mp)
	return mp, nil
}

func recordLatency(url, name string, latency time.Duration) {
	ctx := context.Background()
	meter := otel.GetMeterProvider().Meter("test")
	latencyRecorder, _ := meter.Float64Histogram(
		"http_client_latency",
		otelMetric.WithUnit("ms"))
	latencyRecorder.Record(ctx,
		float64(latency.Milliseconds()),
		otelMetric.WithAttributes(
			attribute.String("url", url),
			attribute.String("name", name),
		))
}

func genCustomHeader(req *http.Request, n int) {
	for i := 0; i < n; i++ {
		customHeader := fmt.Sprintf("Custom-Header-%d", i)
		customValue := fmt.Sprintf("Custom-Value-%d", i)
		req.Header.Add(customHeader, customValue)
	}

}

type Client struct {
	client *http.Client
}

func NewClient() *Client {
	c := &Client{
		client: &http.Client{
			Transport: &http2.Transport{
				// So http2.Transport doesn't complain the URL scheme isn't 'https'
				AllowHTTP: true,
				// Pretend we are dialing a TLS endpoint.
				// Note, we ignore the passed tls.Config
				DialTLSContext: func(ctx context.Context, network, addr string, cfg *tls.Config) (net.Conn, error) {
					return net.Dial(network, addr)
				},
			},
		},
	}
	return c
}

func (c *Client) fetchHTTP2(rawUrl string) {
	url, _ := url2.ParseRequestURI(rawUrl)
	req := &http.Request{
		Method: "GET",
		URL:    url,
		Header: http.Header{},
	}
	genCustomHeader(req, 12)
	start := time.Now()
	resp, err := c.client.Do(req)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	latency := time.Since(start)

	defer resp.Body.Close()
	_, err = io.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("Error reading response body:", err)
		return
	}
	recordLatency(rawUrl, "http2", latency)
	//fmt.Printf("Response from %s:\n%s\n", url, body)
	//fmt.Printf("Time taken: %v\n", latency)
}

func main() {
	resource := ApplicationResource{
		ServiceName: "http2",
		Version:     "example version",
		Env:         "test env",
	}

	mp, err := InitMeterProvideWith(
		context.Background(),
		[]string{"prometheus"}, resource)
	if err != nil {
		log.Println(err)
	}
	defer func() {
		if err = mp.Shutdown(context.Background()); err != nil {
			log.Printf("error shutting down meter provider: %v", err)
		}
	}()

	http2URL := os.Getenv("SERVER_HTTP2_URL")

	if http2URL == "" {
		fmt.Println("Error: SERVER_HTTP1_URL or SERVER_HTTP2_URL not set")
		return
	}
	client := NewClient()
	for i := 0; i < 3; i++ {
		go func() {
			for true {
				client.fetchHTTP2(http2URL)
			}
		}()
	}
	// Expose Prometheus metrics
	http.Handle("/metrics", promhttp.Handler())
	http.ListenAndServe(":8080", nil)
}
