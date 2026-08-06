// Command grpc-smoke-client validates gRPC connectivity to the control plane.
package main

import (
	"context"
	jsoniter "github.com/json-iterator/go"
	"errors"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"sort"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/emptypb"
)

const defaultMethod = "/gateway_api_conformance.echo_basic.grpcecho.GrpcEcho/Echo"

type result struct {
	OK        bool    `json:"ok"`
	Error     string  `json:"error,omitempty"`
	LatencyMs float64 `json:"latency_ms"`
}

type summary struct {
	Addr        string             `json:"addr"`
	Authority   string             `json:"authority"`
	Method      string             `json:"method"`
	Requests    int                `json:"requests"`
	Concurrency int                `json:"concurrency"`
	Completed   int                `json:"completed"`
	Successes   int                `json:"successes"`
	SuccessRate float64            `json:"success_rate"`
	ErrorCounts map[string]int     `json:"error_counts,omitempty"`
	LatencyMs   latencyPercentiles `json:"latency_ms"`
}

type latencyPercentiles struct {
	Min  float64 `json:"min"`
	Mean float64 `json:"mean"`
	P50  float64 `json:"p50"`
	P90  float64 `json:"p90"`
	P95  float64 `json:"p95"`
	P99  float64 `json:"p99"`
	Max  float64 `json:"max"`
}

func main() {
	addr := flag.String("addr", "127.0.0.1:18080", "grpc endpoint address")
	authority := flag.String("authority", "grpc.example.com", "grpc :authority host")
	method := flag.String("method", defaultMethod, "fully qualified grpc method")
	timeout := flag.Duration("timeout", 10*time.Second, "request timeout")
	requests := flag.Int("requests", 1, "total number of requests")
	concurrency := flag.Int("concurrency", 1, "maximum concurrent requests")
	jsonOutput := flag.Bool("json", false, "print JSON summary instead of plain text")
	flag.Parse()

	if *requests < 1 {
		log.Fatal("requests must be >= 1")
	}
	if *concurrency < 1 {
		log.Fatal("concurrency must be >= 1")
	}

	conn, err := grpc.NewClient(
		*addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithAuthority(*authority),
	)
	if err != nil {
		log.Fatalf("create grpc client: %v", err)
	}
	defer func() { _ = conn.Close() }()

	results := runRequests(conn, *method, *timeout, *requests, *concurrency)
	summary := summarize(*addr, *authority, *method, *requests, *concurrency, results)

	if !*jsonOutput && *requests == 1 && *concurrency == 1 {
		if summary.Successes != 1 {
			log.Fatalf("invoke grpc method %s via %s: %s", *method, *addr, results[0].Error)
		}
		fmt.Println("grpc smoke ok")
		return
	}

	if err := jsoniter.NewEncoder(os.Stdout).Encode(summary); err != nil {
		log.Fatalf("encode summary: %v", err)
	}

	if summary.Successes != summary.Completed {
		os.Exit(1)
	}
}

func runRequests(
	conn *grpc.ClientConn,
	method string,
	timeout time.Duration,
	requests int,
	concurrency int,
) []result {
	results := make([]result, requests)
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for i := 0; i < requests; i++ {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int) {
			defer wg.Done()
			defer func() { <-sem }()
			results[idx] = invokeOnce(conn, method, timeout)
		}(i)
	}

	wg.Wait()
	return results
}

func invokeOnce(conn *grpc.ClientConn, method string, timeout time.Duration) result {
	started := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	err := conn.Invoke(ctx, method, &emptypb.Empty{}, &emptypb.Empty{})
	latencyMs := float64(time.Since(started).Microseconds()) / 1000.0
	if err != nil {
		return result{
			OK:        false,
			Error:     normalizeError(err),
			LatencyMs: latencyMs,
		}
	}

	return result{
		OK:        true,
		LatencyMs: latencyMs,
	}
}

func summarize(addr, authority, method string, requests, concurrency int, results []result) summary {
	errorCounts := make(map[string]int)
	latencies := make([]float64, 0, len(results))
	successes := 0
	for _, item := range results {
		latencies = append(latencies, item.LatencyMs)
		if item.OK {
			successes++
		} else {
			errorCounts[item.Error]++
		}
	}
	sort.Float64s(latencies)

	s := summary{
		Addr:        addr,
		Authority:   authority,
		Method:      method,
		Requests:    requests,
		Concurrency: concurrency,
		Completed:   len(results),
		Successes:   successes,
		SuccessRate: float64(successes) / float64(len(results)),
		LatencyMs: latencyPercentiles{
			Min:  percentile(latencies, 0.0),
			Mean: mean(latencies),
			P50:  percentile(latencies, 0.50),
			P90:  percentile(latencies, 0.90),
			P95:  percentile(latencies, 0.95),
			P99:  percentile(latencies, 0.99),
			Max:  percentile(latencies, 1.0),
		},
	}
	if len(errorCounts) > 0 {
		s.ErrorCounts = errorCounts
	}
	return s
}

func percentile(values []float64, ratio float64) float64 {
	if len(values) == 0 {
		return 0
	}
	if ratio <= 0 {
		return values[0]
	}
	if ratio >= 1 {
		return values[len(values)-1]
	}
	index := int(math.Ceil(float64(len(values))*ratio)) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(values) {
		index = len(values) - 1
	}
	return values[index]
}

func mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	total := 0.0
	for _, value := range values {
		total += value
	}
	return total / float64(len(values))
}

func normalizeError(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline_exceeded"
	case errors.Is(err, context.Canceled):
		return "canceled"
	default:
		return err.Error()
	}
}
