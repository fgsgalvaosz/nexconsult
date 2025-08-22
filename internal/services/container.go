package services

import (
	"context"
	"fmt"

	"github.com/nexconsult/cnpj-api/internal/config"
	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
)

// Container holds all service dependencies
type Container struct {
	config         *config.Config
	logger         *logrus.Logger
	redisClient    *redis.Client
	CNPJService    CNPJServiceInterface
	CacheService   CacheServiceInterface
	BrowserService BrowserServiceInterface
}

// NewContainer creates a new service container
func NewContainer(cfg *config.Config, logger *logrus.Logger) (*Container, error) {
	container := &Container{
		config: cfg,
		logger: logger,
	}

	// Initialize Redis client
	if err := container.initRedis(); err != nil {
		return nil, fmt.Errorf("failed to initialize Redis: %w", err)
	}

	// Initialize services
	if err := container.initServices(); err != nil {
		return nil, fmt.Errorf("failed to initialize services: %w", err)
	}

	return container, nil
}

// initRedis initializes Redis client
func (c *Container) initRedis() error {
	c.redisClient = redis.NewClient(&redis.Options{
		Addr:         fmt.Sprintf("%s:%d", c.config.Redis.Host, c.config.Redis.Port),
		Password:     c.config.Redis.Password,
		DB:           c.config.Redis.DB,
		PoolSize:     c.config.Redis.PoolSize,
		DialTimeout:  c.config.Redis.DialTimeout,
		ReadTimeout:  c.config.Redis.ReadTimeout,
		WriteTimeout: c.config.Redis.WriteTimeout,
	})

	// Test Redis connection
	ctx := context.Background()
	if err := c.redisClient.Ping(ctx).Err(); err != nil {
		c.logger.Warn("Redis connection failed, running without cache")
		c.redisClient = nil
	} else {
		c.logger.Info("Redis connection established")
	}

	return nil
}

// initServices initializes all services
func (c *Container) initServices() error {
	// Initialize Cache Service
	c.CacheService = NewCacheService(c.redisClient, c.config.CNPJ.CacheTTL, c.logger)

	// Initialize Browser Service
	browserService, err := NewBrowserService(c.config.Browser, c.logger)
	if err != nil {
		return fmt.Errorf("failed to initialize browser service: %w", err)
	}
	c.BrowserService = browserService

	// Initialize CNPJ Service
	cnpjService, err := NewCNPJService(c.config.CNPJ, c.CacheService, c.BrowserService, c.logger)
	if err != nil {
		return fmt.Errorf("failed to initialize CNPJ service: %w", err)
	}
	c.CNPJService = cnpjService

	return nil
}

// Close closes all service connections
func (c *Container) Close() error {
	var errors []error

	// Close Redis connection
	if c.redisClient != nil {
		if err := c.redisClient.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close Redis: %w", err))
		}
	}

	// Close Browser Service
	if c.BrowserService != nil {
		if err := c.BrowserService.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close browser service: %w", err))
		}
	}

	// Return combined errors if any
	if len(errors) > 0 {
		return fmt.Errorf("errors during shutdown: %v", errors)
	}

	return nil
}

// Health checks the health of all services
func (c *Container) Health() map[string]interface{} {
	health := make(map[string]interface{})

	// Check Redis health
	if c.redisClient != nil {
		ctx := context.Background()
		if err := c.redisClient.Ping(ctx).Err(); err != nil {
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

	// Check Browser Service health
	if c.BrowserService != nil {
		browserHealth := c.BrowserService.Health()
		health["browser"] = browserHealth
	}

	// Check CNPJ Service health
	if c.CNPJService != nil {
		cnpjHealth := c.CNPJService.Health()
		health["cnpj"] = cnpjHealth
	}

	return health
}

// GetRedisClient returns the Redis client
func (c *Container) GetRedisClient() *redis.Client {
	return c.redisClient
}

// GetConfig returns the configuration
func (c *Container) GetConfig() *config.Config {
	return c.config
}

// GetLogger returns the logger
func (c *Container) GetLogger() *logrus.Logger {
	return c.logger
}
