// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package valkey

import (
    "context"
    "crypto/tls"
    "fmt"
    "log/slog"
    "time"

    "github.com/redis/go-redis/v9"
)

// CacheClientInterface defines the interface for cache operations
type CacheClientInterface interface {
    Get(ctx context.Context, key string) (string, error)
    Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error
    Delete(ctx context.Context, keys ...string) error
    Exists(ctx context.Context, key string) (bool, error)
    SetNX(ctx context.Context, key string, value interface{}, expiration time.Duration) (bool, error)
    Expire(ctx context.Context, key string, expiration time.Duration) error
    HSet(ctx context.Context, key string, field string, value interface{}) error
    HGet(ctx context.Context, key string, field string) (string, error)
    HGetAll(ctx context.Context, key string) (map[string]string, error)
    Ping(ctx context.Context) error
    Close() error
}

// Compile-time check to ensure Client implements CacheClientInterface
var _ CacheClientInterface = (*Client)(nil)

// Client wraps Valkey (Redis) functionality
type Client struct {
    client *redis.Client
    logger *slog.Logger
}

// Config holds Valkey client configuration
type Config struct {
    Address  string
    Password string
    DB       int
    PoolSize int
    Timeout  time.Duration
    UseTLS   bool
}

// NewClient creates a new Valkey client
func NewClient(cfg Config, logger *slog.Logger) (*Client, error) {
    opts := &redis.Options{
        Addr:         cfg.Address,
        Password:     cfg.Password,
        DB:           cfg.DB,
        PoolSize:     cfg.PoolSize,
        DialTimeout:  cfg.Timeout,
        ReadTimeout:  cfg.Timeout,
        WriteTimeout: cfg.Timeout,
    }

    if cfg.UseTLS {
        opts.TLSConfig = &tls.Config{
            MinVersion: tls.VersionTLS12,
        }
    }

    client := redis.NewClient(opts)

    // Test connection
    ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
    defer cancel()

    if err := client.Ping(ctx).Err(); err != nil {
        return nil, fmt.Errorf("failed to connect to Valkey: %w", err)
    }

    logger.Info("Valkey client connected", "address", cfg.Address, "db", cfg.DB)

    return &Client{
        client: client,
        logger: logger,
    }, nil
}

// Get retrieves a value by key
func (c *Client) Get(ctx context.Context, key string) (string, error) {
    val, err := c.client.Get(ctx, key).Result()
    if err == redis.Nil {
        return "", fmt.Errorf("key not found: %s", key)
    } else if err != nil {
        c.logger.Error("Failed to get value", "key", key, "error", err)
        return "", fmt.Errorf("failed to get value: %w", err)
    }
    return val, nil
}

// Set sets a value with an optional expiration
func (c *Client) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
    err := c.client.Set(ctx, key, value, expiration).Err()
    if err != nil {
        c.logger.Error("Failed to set value", "key", key, "error", err)
        return fmt.Errorf("failed to set value: %w", err)
    }
    return nil
}

// Delete deletes one or more keys
func (c *Client) Delete(ctx context.Context, keys ...string) error {
    err := c.client.Del(ctx, keys...).Err()
    if err != nil {
        c.logger.Error("Failed to delete keys", "keys", keys, "error", err)
        return fmt.Errorf("failed to delete keys: %w", err)
    }
    return nil
}

// Exists checks if a key exists
func (c *Client) Exists(ctx context.Context, key string) (bool, error) {
    val, err := c.client.Exists(ctx, key).Result()
    if err != nil {
        c.logger.Error("Failed to check key existence", "key", key, "error", err)
        return false, fmt.Errorf("failed to check key existence: %w", err)
    }
    return val > 0, nil
}

// SetNX sets a value only if the key does not exist
func (c *Client) SetNX(ctx context.Context, key string, value interface{}, expiration time.Duration) (bool, error) {
    val, err := c.client.SetNX(ctx, key, value, expiration).Result()
    if err != nil {
        c.logger.Error("Failed to set value with SetNX", "key", key, "error", err)
        return false, fmt.Errorf("failed to set value with SetNX: %w", err)
    }
    return val, nil
}

// Expire sets an expiration on a key
func (c *Client) Expire(ctx context.Context, key string, expiration time.Duration) error {
    err := c.client.Expire(ctx, key, expiration).Err()
    if err != nil {
        c.logger.Error("Failed to set expiration", "key", key, "error", err)
        return fmt.Errorf("failed to set expiration: %w", err)
    }
    return nil
}

// HSet sets a field in a hash
func (c *Client) HSet(ctx context.Context, key string, field string, value interface{}) error {
    err := c.client.HSet(ctx, key, field, value).Err()
    if err != nil {
        c.logger.Error("Failed to set hash field", "key", key, "field", field, "error", err)
        return fmt.Errorf("failed to set hash field: %w", err)
    }
    return nil
}

// HGet gets a field from a hash
func (c *Client) HGet(ctx context.Context, key string, field string) (string, error) {
    val, err := c.client.HGet(ctx, key, field).Result()
    if err == redis.Nil {
        return "", fmt.Errorf("field not found: %s", field)
    } else if err != nil {
        c.logger.Error("Failed to get hash field", "key", key, "field", field, "error", err)
        return "", fmt.Errorf("failed to get hash field: %w", err)
    }
    return val, nil
}

// HGetAll gets all fields from a hash
func (c *Client) HGetAll(ctx context.Context, key string) (map[string]string, error) {
    val, err := c.client.HGetAll(ctx, key).Result()
    if err != nil {
        c.logger.Error("Failed to get all hash fields", "key", key, "error", err)
        return nil, fmt.Errorf("failed to get all hash fields: %w", err)
    }
    return val, nil
}

// Close closes the client connection
func (c *Client) Close() error {
    if c.client != nil {
        err := c.client.Close()
        if err != nil {
            return fmt.Errorf("failed to close Valkey client: %w", err)
        }
        c.logger.Info("Valkey connection closed")
    }
    return nil
}

// Ping checks if the connection is alive
func (c *Client) Ping(ctx context.Context) error {
    return c.client.Ping(ctx).Err()
}
