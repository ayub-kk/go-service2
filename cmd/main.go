package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go-service/internal/analytics"
	"go-service/internal/cache"
	"go-service/internal/models"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	httpRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Total number of HTTP requests",
	}, []string{"method", "endpoint", "status"})

	requestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "Duration of HTTP requests",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "endpoint"})

	anomaliesDetected = promauto.NewCounter(prometheus.CounterOpts{
		Name: "anomalies_detected_total",
		Help: "Total number of anomalies detected",
	})

	metricsProcessed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "metrics_processed_total",
		Help: "Total number of metrics processed",
	})

	currentRPS = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "current_rps",
		Help: "Current requests per second",
	})

	rollingAverage = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "rolling_average",
		Help: "Rolling average of metrics",
	})
)

type Server struct {
	router      *mux.Router
	cache       *cache.RedisClient
	analyzer    *analytics.Analyzer
	metricsChan chan models.Metric
}

func NewServer(redisAddr string) (*Server, error) {
	redisClient, err := cache.NewRedisClient(redisAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	analyzer := analytics.NewAnalyzer(50, 2.0) // window=50, threshold=2σ
	metricsChan := make(chan models.Metric, 10000)

	s := &Server{
		router:      mux.NewRouter(),
		cache:       redisClient,
		analyzer:    analyzer,
		metricsChan: metricsChan,
	}

	s.setupRoutes()
	go s.processMetrics()

	return s, nil
}

func (s *Server) setupRoutes() {
	s.router.HandleFunc("/health", s.healthHandler).Methods("GET")
	s.router.HandleFunc("/metrics/ingest", s.ingestMetricsHandler).Methods("POST")
	s.router.HandleFunc("/analytics/current", s.getAnalyticsHandler).Methods("GET")
	s.router.HandleFunc("/analytics/anomalies", s.getAnomaliesHandler).Methods("GET")
	s.router.Handle("/metrics/prometheus", promhttp.Handler())
}

func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	health := map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now().UTC(),
		"version":   "1.0.0",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(health)

	duration := time.Since(start).Seconds()
	requestDuration.WithLabelValues(r.Method, r.URL.Path).Observe(duration)
	httpRequestsTotal.WithLabelValues(r.Method, r.URL.Path, "200").Inc()
}

func (s *Server) ingestMetricsHandler(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	var metric models.Metric

	if err := json.NewDecoder(r.Body).Decode(&metric); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		httpRequestsTotal.WithLabelValues(r.Method, r.URL.Path, "400").Inc()
		return
	}

	metric.Timestamp = time.Now()

	// Отправляем метрику в канал для обработки
	select {
	case s.metricsChan <- metric:
		metricsProcessed.Inc()
		currentRPS.Set(metric.RPS)

		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]string{"status": "accepted"})
	default:
		http.Error(w, "queue full", http.StatusServiceUnavailable)
	}

	duration := time.Since(start).Seconds()
	requestDuration.WithLabelValues(r.Method, r.URL.Path).Observe(duration)
	httpRequestsTotal.WithLabelValues(r.Method, r.URL.Path, "202").Inc()
}

func (s *Server) processMetrics() {
	for metric := range s.metricsChan {
		// Кэширование метрики
		if err := s.cache.StoreMetric(metric); err != nil {
			log.Printf("Failed to cache metric: %v", err)
		}

		// Анализ метрики
		analysis := s.analyzer.Analyze(metric)

		// Обновляем Prometheus метрики
		rollingAverage.Set(analysis.RollingAverage)

		if analysis.IsAnomaly {
			anomaliesDetected.Inc()
			log.Printf("Anomaly detected: RPS=%.2f, Z-score=%.2f", metric.RPS, analysis.ZScore)
		}
	}
}

func (s *Server) getAnalyticsHandler(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	analyticsData := s.analyzer.GetCurrentStats()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(analyticsData)

	duration := time.Since(start).Seconds()
	requestDuration.WithLabelValues(r.Method, r.URL.Path).Observe(duration)
	httpRequestsTotal.WithLabelValues(r.Method, r.URL.Path, "200").Inc()
}

func (s *Server) getAnomaliesHandler(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	anomalies := s.analyzer.GetRecentAnomalies(10)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(anomalies)

	duration := time.Since(start).Seconds()
	requestDuration.WithLabelValues(r.Method, r.URL.Path).Observe(duration)
	httpRequestsTotal.WithLabelValues(r.Method, r.URL.Path, "200").Inc()
}

func (s *Server) Run(addr string) error {
	srv := &http.Server{
		Addr:         addr,
		Handler:      s.router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  30 * time.Second,
	}

	done := make(chan bool, 1)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-quit
		log.Println("Server is shutting down...")

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		srv.SetKeepAlivesEnabled(false)
		if err := srv.Shutdown(ctx); err != nil {
			log.Fatalf("Could not gracefully shutdown the server: %v", err)
		}
		close(done)
	}()

	log.Printf("Server is ready to handle requests at %s", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("could not listen on %s: %w", addr, err)
	}

	<-done
	log.Println("Server stopped")
	return nil
}

func main() {
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	server, err := NewServer(redisAddr)
	if err != nil {
		log.Fatal(err)
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	if err := server.Run(":" + port); err != nil {
		log.Fatal(err)
	}
}
