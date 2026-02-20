package valkey

import (
    "context"
    "fmt"
    "log/slog"
    "time"
)

// Compile-time check to ensure NoopClient implements CacheClientInterface
var _ CacheClientInterface = (*NoopClient)(nil)

// NoopClient is a no-operation implementation of CacheClientInterface
// that stores data in memory without using Redis/Valkey.
// Useful for testing and development without cache infrastructure.
type NoopClient struct {
    logger *slog.Logger
    data   map[string]interface{}
    hashes map[string]map[string]string
}

// NewNoopClient creates a new noop Valkey client
func NewNoopClient(logger *slog.Logger) *NoopClient {
    return &NoopClient{
        logger: logger,
        data:   make(map[string]interface{}),
        hashes: make(map[string]map[string]string),
    }
}

// Get retrieves a value from in-memory storage
func (c *NoopClient) Get(ctx context.Context, key string) (string, error) {
    c.logger.Debug("Noop cache get", "key", key)

    val, exists := c.data[key]
    if !exists {
        return "", fmt.Errorf("key not found: %s", key)
    }

    if str, ok := val.(string); ok {
        return str, nil
    }

    return fmt.Sprintf("%v", val), nil
}

// Set stores a value in in-memory storage (expiration is ignored)
func (c *NoopClient) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
    c.logger.Debug("Noop cache set", "key", key, "expiration", expiration)
    c.data[key] = value
    return nil
}

// Delete removes values from in-memory storage
func (c *NoopClient) Delete(ctx context.Context, keys ...string) error {
    c.logger.Debug("Noop cache delete", "keys", keys)
    for _, key := range keys {
        delete(c.data, key)
    }
    return nil
}

// Exists checks if a key exists in in-memory storage
func (c *NoopClient) Exists(ctx context.Context, key string) (bool, error) {
    c.logger.Debug("Noop cache exists", "key", key)
    _, exists := c.data[key]
    return exists, nil
}

// SetNX sets a value only if the key doesn't exist
func (c *NoopClient) SetNX(ctx context.Context, key string, value interface{}, expiration time.Duration) (bool, error) {
    c.logger.Debug("Noop cache setnx", "key", key, "expiration", expiration)

    if _, exists := c.data[key]; exists {
        return false, nil
    }

    c.data[key] = value
    return true, nil
}

// Expire is a no-op (expiration not tracked in memory)
func (c *NoopClient) Expire(ctx context.Context, key string, expiration time.Duration) error {
    c.logger.Debug("Noop cache expire", "key", key, "expiration", expiration)
    // No-op: we don't track expiration in the noop client
    return nil
}

// HSet sets a field in a hash
func (c *NoopClient) HSet(ctx context.Context, key string, field string, value interface{}) error {
    c.logger.Debug("Noop cache hset", "key", key, "field", field)

    if c.hashes[key] == nil {
        c.hashes[key] = make(map[string]string)
    }

    c.hashes[key][field] = fmt.Sprintf("%v", value)
    return nil
}

// HGet retrieves a field from a hash
func (c *NoopClient) HGet(ctx context.Context, key string, field string) (string, error) {
    c.logger.Debug("Noop cache hget", "key", key, "field", field)

    hash, exists := c.hashes[key]
    if !exists {
        return "", fmt.Errorf("hash not found: %s", key)
    }

    val, exists := hash[field]
    if !exists {
        return "", fmt.Errorf("field not found: %s", field)
    }

    return val, nil
}

// HGetAll retrieves all fields from a hash
func (c *NoopClient) HGetAll(ctx context.Context, key string) (map[string]string, error) {
    c.logger.Debug("Noop cache hgetall", "key", key)

    hash, exists := c.hashes[key]
    if !exists {
        return make(map[string]string), nil
    }

    return hash, nil
}

// Ping always returns nil (no actual connection to check)
func (c *NoopClient) Ping(ctx context.Context) error {
    c.logger.Debug("Noop cache ping")
    return nil
}

// Close clears the in-memory storage
func (c *NoopClient) Close() error {
    c.logger.Info("Noop Valkey client closed")
    c.data = make(map[string]interface{})
    c.hashes = make(map[string]map[string]string)
    return nil
}
