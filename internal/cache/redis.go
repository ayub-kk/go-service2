package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go-service/internal/models"

	"github.com/go-redis/redis/v8"
)

type RedisClient struct {
	client *redis.Client
	ctx    context.Context
}

func NewRedisClient(addr string) (*RedisClient, error) {
	client := redis.NewClient(&redis.Options{
		Addr:         addr,
		Password:     "",
		DB:           0,
		PoolSize:     100,
		MinIdleConns: 10,
		MaxRetries:   3,
	})

	ctx := context.Background()

	// Проверка соединения
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, err
	}

	return &RedisClient{
		client: client,
		ctx:    ctx,
	}, nil
}

func (r *RedisClient) StoreMetric(metric models.Metric) error {
	key := fmt.Sprintf("metric:%s:%d", metric.DeviceID, metric.Timestamp.UnixNano())

	data, err := json.Marshal(metric)
	if err != nil {
		return fmt.Errorf("failed to marshal metric: %w", err)
	}

	// Сохраняем на 1 час
	err = r.client.Set(r.ctx, key, data, time.Hour).Err()
	if err != nil {
		return fmt.Errorf("failed to store metric in Redis: %w", err)
	}

	// Добавляем в список последних метрик
	listKey := "metrics:recent"
	err = r.client.LPush(r.ctx, listKey, key).Err()
	if err != nil {
		return fmt.Errorf("failed to update recent metrics list: %w", err)
	}

	// Ограничиваем список 1000 элементами
	r.client.LTrim(r.ctx, listKey, 0, 999)

	return nil
}

func (r *RedisClient) GetRecentMetrics(count int64) ([]models.Metric, error) {
	listKey := "metrics:recent"

	keys, err := r.client.LRange(r.ctx, listKey, 0, count-1).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get recent metric keys: %w", err)
	}

	var metrics []models.Metric
	for _, key := range keys {
		data, err := r.client.Get(r.ctx, key).Result()
		if err != nil {
			continue // Пропускаем невалидные ключи
		}

		var metric models.Metric
		if err := json.Unmarshal([]byte(data), &metric); err != nil {
			continue
		}

		metrics = append(metrics, metric)
	}

	return metrics, nil
}

func (r *RedisClient) Close() error {
	return r.client.Close()
}
