package models

import "time"

type Metric struct {
	Timestamp   time.Time `json:"timestamp"`
	DeviceID    string    `json:"device_id"`
	CPUUsage    float64   `json:"cpu_usage"`
	MemoryUsage float64   `json:"memory_usage"`
	RPS         float64   `json:"rps"`
	Latency     float64   `json:"latency_ms"`
}

type AnalysisResult struct {
	Timestamp      time.Time `json:"timestamp"`
	Metric         Metric    `json:"metric"`
	RollingAverage float64   `json:"rolling_average"`
	ZScore         float64   `json:"z_score"`
	IsAnomaly      bool      `json:"is_anomaly"`
}

type AnalyticsStats struct {
	CurrentRPS      float64   `json:"current_rps"`
	RollingAverage  float64   `json:"rolling_average"`
	AnomalyRate     float64   `json:"anomaly_rate"`
	TotalMetrics    int64     `json:"total_metrics"`
	TotalAnomalies  int64     `json:"total_anomalies"`
	LastAnomalyTime time.Time `json:"last_anomaly_time,omitempty"`
	WindowSize      int       `json:"window_size"`
	ZScoreThreshold float64   `json:"z_score_threshold"`
}
