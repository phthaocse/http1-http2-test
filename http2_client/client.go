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
	"net/http"
	"net/url"
	"os"
	"sync"

	"time"
)

type ApplicationResource struct {
	ServiceName string
	Version     string
	Env         string
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
			promExporter, err := prometheus.New()
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
				AllowHTTP:       true,
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		},
	}
	return c
}

func (c *Client) fetchHTTP2(rawUrl string) {
	url, _ := url.ParseRequestURI(rawUrl)
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
