package permissions

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/canonical/authorization-service/internal/integration/valkey"
)

type ServiceInterface interface {
	Register(ctx context.Context, serviceID, serviceName string, permissions map[string]interface{}, version string) (*Permission, error)
	Get(ctx context.Context, serviceID string) (*Permission, error)
	Delete(ctx context.Context, serviceID string) error
}

// Compile-time check to ensure Service implements ServiceInterface
var _ ServiceInterface = (*Service)(nil)

// Service handles permission registration and management
type Service struct {
	cache  valkey.CacheClientInterface
	logger *slog.Logger
}

// Permission represents a service's permission definition
type Permission struct {
	ID           string                 `json:"id"`
	ServiceID    string                 `json:"service_id"`
	ServiceName  string                 `json:"service_name"`
	Permissions  map[string]interface{} `json:"permissions"`
	Version      string                 `json:"version"`
	RegisteredAt time.Time              `json:"registered_at"`
}

// NewService creates a new permissions service
func NewService(cache valkey.CacheClientInterface, logger *slog.Logger) *Service {
	return &Service{
		cache:  cache,
		logger: logger,
	}
}

// Register registers or updates permissions for a service
func (s *Service) Register(ctx context.Context, serviceID, serviceName string, permissions map[string]interface{}, version string) (*Permission, error) {
	perm := &Permission{
		ID:           uuid.New().String(),
		ServiceID:    serviceID,
		ServiceName:  serviceName,
		Permissions:  permissions,
		Version:      version,
		RegisteredAt: time.Now(),
	}

	key := fmt.Sprintf("permissions:%s", serviceID)
	data, err := json.Marshal(perm)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal permission: %w", err)
	}

	if err := s.cache.Set(ctx, key, data, 0); err != nil {
		return nil, fmt.Errorf("failed to store permission: %w", err)
	}

	s.logger.Info("Permission registered", "service_id", serviceID, "version", version)
	return perm, nil
}

// Get retrieves permissions for a service
func (s *Service) Get(ctx context.Context, serviceID string) (*Permission, error) {
	key := fmt.Sprintf("permissions:%s", serviceID)
	data, err := s.cache.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("permission not found: %w", err)
	}

	var perm Permission
	if err := json.Unmarshal([]byte(data), &perm); err != nil {
		return nil, fmt.Errorf("failed to unmarshal permission: %w", err)
	}

	return &perm, nil
}

// Delete removes permissions for a service
func (s *Service) Delete(ctx context.Context, serviceID string) error {
	key := fmt.Sprintf("permissions:%s", serviceID)
	if err := s.cache.Delete(ctx, key); err != nil {
		return fmt.Errorf("failed to delete permission: %w", err)
	}

	s.logger.Info("Permission deleted", "service_id", serviceID)
	return nil
}
