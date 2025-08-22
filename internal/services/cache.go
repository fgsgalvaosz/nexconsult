package services

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
)

// CacheService implements caching functionality
type CacheService struct {
	client *redis.Client
	ttl    time.Duration
	logger *logrus.Logger

	// In-memory fallback cache when Redis is not available
	memCache map[string]cacheItem
	memMutex sync.RWMutex
}

type cacheItem struct {
	value     string
	expiresAt time.Time
}

// NewCacheService creates a new cache service
func NewCacheService(client *redis.Client, ttl time.Duration, logger *logrus.Logger) CacheServiceInterface {
	return &CacheService{
		client:   client,
		ttl:      ttl,
		logger:   logger,
		memCache: make(map[string]cacheItem),
	}
}

// Get retrieves a value from cache
func (c *CacheService) Get(ctx context.Context, key string) (string, error) {
	// Try Redis first if available
	if c.client != nil {
		val, err := c.client.Get(ctx, key).Result()
		if err == nil {
			c.logger.WithField("key", key).Debug("Cache hit (Redis)")
			return val, nil
		}
		if err != redis.Nil {
			c.logger.WithFields(logrus.Fields{
				"key":   key,
				"error": err.Error(),
			}).Warn("Redis get error, falling back to memory cache")
		}
	}

	// Fallback to memory cache
	c.memMutex.RLock()
	item, exists := c.memCache[key]
	c.memMutex.RUnlock()

	if !exists {
		return "", fmt.Errorf("key not found")
	}

	if time.Now().After(item.expiresAt) {
		// Item expired, remove it
		c.memMutex.Lock()
		delete(c.memCache, key)
		c.memMutex.Unlock()
		return "", fmt.Errorf("key not found")
	}

	c.logger.WithField("key", key).Debug("Cache hit (memory)")
	return item.value, nil
}

// Set stores a value in cache with TTL
func (c *CacheService) Set(ctx context.Context, key string, value string) error {
	// Try Redis first if available
	if c.client != nil {
		err := c.client.Set(ctx, key, value, c.ttl).Err()
		if err == nil {
			c.logger.WithField("key", key).Debug("Cache set (Redis)")
			return nil
		}
		c.logger.WithFields(logrus.Fields{
			"key":   key,
			"error": err.Error(),
		}).Warn("Redis set error, falling back to memory cache")
	}

	// Fallback to memory cache
	c.memMutex.Lock()
	c.memCache[key] = cacheItem{
		value:     value,
		expiresAt: time.Now().Add(c.ttl),
	}
	c.memMutex.Unlock()

	c.logger.WithField("key", key).Debug("Cache set (memory)")
	return nil
}

// Delete removes a value from cache
func (c *CacheService) Delete(ctx context.Context, key string) error {
	// Try Redis first if available
	if c.client != nil {
		err := c.client.Del(ctx, key).Err()
		if err != nil {
			c.logger.WithFields(logrus.Fields{
				"key":   key,
				"error": err.Error(),
			}).Warn("Redis delete error")
		}
	}

	// Also remove from memory cache
	c.memMutex.Lock()
	delete(c.memCache, key)
	c.memMutex.Unlock()

	c.logger.WithField("key", key).Debug("Cache delete")
	return nil
}

// Clear clears all cache entries
func (c *CacheService) Clear(ctx context.Context) error {
	// Try Redis first if available
	if c.client != nil {
		err := c.client.FlushDB(ctx).Err()
		if err != nil {
			c.logger.WithField("error", err.Error()).Warn("Redis clear error")
		}
	}

	// Clear memory cache
	c.memMutex.Lock()
	c.memCache = make(map[string]cacheItem)
	c.memMutex.Unlock()

	c.logger.Info("Cache cleared")
	return nil
}

// Exists checks if a key exists in cache
func (c *CacheService) Exists(ctx context.Context, key string) (bool, error) {
	// Try Redis first if available
	if c.client != nil {
		count, err := c.client.Exists(ctx, key).Result()
		if err == nil {
			return count > 0, nil
		}
		c.logger.WithFields(logrus.Fields{
			"key":   key,
			"error": err.Error(),
		}).Warn("Redis exists error, checking memory cache")
	}

	// Check memory cache
	c.memMutex.RLock()
	item, exists := c.memCache[key]
	c.memMutex.RUnlock()

	if !exists {
		return false, nil
	}

	// Check if expired
	if time.Now().After(item.expiresAt) {
		c.memMutex.Lock()
		delete(c.memCache, key)
		c.memMutex.Unlock()
		return false, nil
	}

	return true, nil
}

// GetStats returns cache statistics
func (c *CacheService) GetStats(ctx context.Context) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Redis stats
	if c.client != nil {
		info, err := c.client.Info(ctx, "memory").Result()
		if err == nil {
			stats["redis"] = map[string]interface{}{
				"available": true,
				"info":      info,
			}
		} else {
			stats["redis"] = map[string]interface{}{
				"available": false,
				"error":     err.Error(),
			}
		}
	} else {
		stats["redis"] = map[string]interface{}{
			"available": false,
		}
	}

	// Memory cache stats
	c.memMutex.RLock()
	memSize := len(c.memCache)
	c.memMutex.RUnlock()

	stats["memory"] = map[string]interface{}{
		"size": memSize,
		"ttl":  c.ttl.String(),
	}

	return stats, nil
}

// Health returns cache service health status
func (c *CacheService) Health() map[string]interface{} {
	health := make(map[string]interface{})

	// Check Redis health
	if c.client != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := c.client.Ping(ctx).Err(); err != nil {
			health["redis"] = map[string]interface{}{
				"status": "unhealthy",
				"error":  err.Error(),
			}
		} else {
			health["redis"] = map[string]interface{}{
				"status": "healthy",
			}
		}
	} else {
		health["redis"] = map[string]interface{}{
			"status": "disabled",
		}
	}

	// Memory cache is always available
	health["memory"] = map[string]interface{}{
		"status": "healthy",
	}

	return health
}

// cleanupExpired removes expired items from memory cache
func (c *CacheService) cleanupExpired() {
	c.memMutex.Lock()
	defer c.memMutex.Unlock()

	now := time.Now()
	for key, item := range c.memCache {
		if now.After(item.expiresAt) {
			delete(c.memCache, key)
		}
	}
}

// StartCleanupRoutine starts a goroutine to periodically clean expired items
func (c *CacheService) StartCleanupRoutine() {
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()

		for range ticker.C {
			c.cleanupExpired()
		}
	}()
}
