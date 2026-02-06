package permissions

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/canonical/authorization-service/internal/integrations/nats"
	"github.com/canonical/authorization-service/internal/integrations/valkey"
)

// Service handles permission registration and management
type Service struct {
	cache  valkey.CacheClient
	events nats.EventClient
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
func NewService(cache valkey.CacheClient, events nats.EventClient, logger *slog.Logger) *Service {
	return &Service{
		cache:  cache,
		events: events,
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

	// Store in cache
	key := fmt.Sprintf("permissions:%s", serviceID)
	data, err := json.Marshal(perm)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal permission: %w", err)
	}

	if err := s.cache.Set(ctx, key, data, 0); err != nil {
		return nil, fmt.Errorf("failed to store permission: %w", err)
	}

	// Publish event
	eventData, _ := json.Marshal(map[string]interface{}{
		"event_type":  "permission.registered",
		"service_id":  serviceID,
		"timestamp":   time.Now(),
		"permissions": perm,
	})
	if err := s.events.Publish(ctx, "AUTHZ.permissions.registered", eventData); err != nil {
		s.logger.Warn("Failed to publish permission registered event", "error", err)
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

	// Publish event
	eventData, _ := json.Marshal(map[string]interface{}{
		"event_type": "permission.deleted",
		"service_id": serviceID,
		"timestamp":  time.Now(),
	})
	if err := s.events.Publish(ctx, "AUTHZ.permissions.deleted", eventData); err != nil {
		s.logger.Warn("Failed to publish permission deleted event", "error", err)
	}

	s.logger.Info("Permission deleted", "service_id", serviceID)
	return nil
}
