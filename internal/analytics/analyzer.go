package analytics

import (
	"math"
	"sync"
	"time"

	"go-service/internal/models"
)

type Analyzer struct {
	windowSize      int
	zScoreThreshold float64
	metricsWindow   []models.Metric
	anomalies       []models.AnalysisResult
	stats           models.AnalyticsStats
	mu              sync.RWMutex
}

func NewAnalyzer(windowSize int, zScoreThreshold float64) *Analyzer {
	return &Analyzer{
		windowSize:      windowSize,
		zScoreThreshold: zScoreThreshold,
		metricsWindow:   make([]models.Metric, 0, windowSize),
		anomalies:       make([]models.AnalysisResult, 0, 100),
		stats: models.AnalyticsStats{
			WindowSize:      windowSize,
			ZScoreThreshold: zScoreThreshold,
		},
	}
}

func (a *Analyzer) Analyze(metric models.Metric) models.AnalysisResult {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Добавляем метрику в окно
	a.metricsWindow = append(a.metricsWindow, metric)
	if len(a.metricsWindow) > a.windowSize {
		a.metricsWindow = a.metricsWindow[1:]
	}

	// Вычисляем скользящее среднее
	rollingAvg := a.calculateRollingAverage()

	// Вычисляем Z-score
	zScore := a.calculateZScore(metric.RPS, rollingAvg)

	// Определяем аномалию
	isAnomaly := math.Abs(zScore) > a.zScoreThreshold && len(a.metricsWindow) >= 10

	result := models.AnalysisResult{
		Timestamp:      time.Now(),
		Metric:         metric,
		RollingAverage: rollingAvg,
		ZScore:         zScore,
		IsAnomaly:      isAnomaly,
	}

	// Обновляем статистику
	a.stats.CurrentRPS = metric.RPS
	a.stats.RollingAverage = rollingAvg
	a.stats.TotalMetrics++

	if isAnomaly {
		a.stats.TotalAnomalies++
		a.stats.LastAnomalyTime = time.Now()
		a.stats.AnomalyRate = float64(a.stats.TotalAnomalies) / float64(a.stats.TotalMetrics)

		// Сохраняем аномалию
		a.anomalies = append(a.anomalies, result)
		if len(a.anomalies) > 100 {
			a.anomalies = a.anomalies[1:]
		}
	}

	return result
}

func (a *Analyzer) calculateRollingAverage() float64 {
	if len(a.metricsWindow) == 0 {
		return 0
	}

	var sum float64
	for _, metric := range a.metricsWindow {
		sum += metric.RPS
	}

	return sum / float64(len(a.metricsWindow))
}

func (a *Analyzer) calculateZScore(value, mean float64) float64 {
	if len(a.metricsWindow) < 2 {
		return 0
	}

	// Вычисляем стандартное отклонение
	var variance float64
	for _, metric := range a.metricsWindow {
		diff := metric.RPS - mean
		variance += diff * diff
	}

	stdDev := math.Sqrt(variance / float64(len(a.metricsWindow)-1))
	if stdDev == 0 {
		return 0
	}

	return (value - mean) / stdDev
}

func (a *Analyzer) GetCurrentStats() models.AnalyticsStats {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.stats
}

func (a *Analyzer) GetRecentAnomalies(limit int) []models.AnalysisResult {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if limit > len(a.anomalies) {
		limit = len(a.anomalies)
	}

	start := len(a.anomalies) - limit
	if start < 0 {
		start = 0
	}

	return a.anomalies[start:]
}
