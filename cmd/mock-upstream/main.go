package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	outageMode      atomic.Bool
	failureRatePerc atomic.Int32
	requestsTotal   = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mock_upstream_requests_total",
			Help: "Total count of requests received by the mock upstream",
		},
		[]string{"status"},
	)
	failedRequestsTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "mock_upstream_failed_requests_total",
			Help: "Total count of requests that failed due to simulated failures",
		},
	)
)

func init() {
	prometheus.MustRegister(requestsTotal)
	prometheus.MustRegister(failedRequestsTotal)
}

func main() {
	port := flag.String("port", "4318", "Port to listen on")
	metricsPort := flag.String("metrics-port", "8889", "Port for Prometheus metrics")
	flag.Parse()

	// Initialize with no outage and 0% failure rate
	outageMode.Store(false)
	failureRatePerc.Store(0)

	// API endpoints
	http.HandleFunc("/v1/metrics", handleOTLPRequest)
	http.HandleFunc("/v1/logs", handleOTLPRequest)
	http.HandleFunc("/v1/traces", handleOTLPRequest)

	// Control endpoints
	http.HandleFunc("/control/outage", handleOutageControl)
	http.HandleFunc("/control/failure-rate", handleFailureRateControl)
	http.HandleFunc("/control/status", handleStatusCheck)

	// Metrics endpoint
	go func() {
		http.Handle("/metrics", promhttp.Handler())
		log.Printf("Starting metrics server on :%s", *metricsPort)
		if err := http.ListenAndServe(":"+*metricsPort, nil); err != nil {
			log.Fatalf("Failed to start metrics server: %v", err)
		}
	}()

	// Start server
	srv := &http.Server{
		Addr:    ":" + *port,
		Handler: nil, // Use default ServeMux
	}

	go func() {
		log.Printf("Starting mock upstream server on :%s", *port)
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server shutdown failed: %v", err)
	}
	log.Println("Server stopped gracefully")
}

func handleOTLPRequest(w http.ResponseWriter, r *http.Request) {
	// Simulate outage if enabled
	if outageMode.Load() {
		w.WriteHeader(http.StatusServiceUnavailable)
		requestsTotal.WithLabelValues("503").Inc()
		log.Println("Simulating outage - returning 503")
		return
	}

	// Simulate random failures based on configured rate
	failureRate := failureRatePerc.Load()
	if failureRate > 0 {
		rand.Seed(time.Now().UnixNano())
		if rand.Intn(100) < int(failureRate) {
			w.WriteHeader(http.StatusTooManyRequests)
			failedRequestsTotal.Inc()
			requestsTotal.WithLabelValues("429").Inc()
			log.Println("Simulating failure - returning 429")
			return
		}
	}

	// Successfully process the OTLP data (just acknowledge it)
	requestsTotal.WithLabelValues("200").Inc()
	w.WriteHeader(http.StatusOK)
	log.Println("Request processed successfully")
}

func handleOutageControl(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	outageMode.Store(req.Enabled)
	log.Printf("Outage mode set to: %v", req.Enabled)
	w.WriteHeader(http.StatusOK)
}

func handleFailureRateControl(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		RatePercent int32 `json:"rate_percent"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Ensure rate is between 0-100%
	if req.RatePercent < 0 {
		req.RatePercent = 0
	} else if req.RatePercent > 100 {
		req.RatePercent = 100
	}

	failureRatePerc.Store(req.RatePercent)
	log.Printf("Failure rate set to: %d%%", req.RatePercent)
	w.WriteHeader(http.StatusOK)
}

func handleStatusCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	status := struct {
		OutageEnabled bool  `json:"outage_enabled"`
		FailureRate   int32 `json:"failure_rate_percent"`
	}{
		OutageEnabled: outageMode.Load(),
		FailureRate:   failureRatePerc.Load(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(status)
}
