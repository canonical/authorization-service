// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package permissions

import (
    "context"
    "fmt"
    "log/slog"
    "time"

    "github.com/google/uuid"
)

// Compile-time check to ensure NoopService implements ServiceInterface
var _ ServiceInterface = (*NoopService)(nil)

// NoopService is a no-operation implementation of ServiceInterface
// that stores permissions in memory without persisting them to cache or events.
type NoopService struct {
    logger      *slog.Logger
    permissions map[string]*Permission // in-memory storage
}

// NewNoopService creates a new noop permissions service
func NewNoopService(logger *slog.Logger) *NoopService {
    return &NoopService{
        logger:      logger,
        permissions: make(map[string]*Permission),
    }
}

// Register stores the permission in memory without persisting to cache or events
func (s *NoopService) Register(_ context.Context, serviceID, serviceName string, permissions map[string]interface{}, version string) (*Permission, error) {
    s.logger.Debug("Noop register permissions - storing in memory only",
        "service_id", serviceID,
        "service_name", serviceName,
        "version", version,
    )

    perm := &Permission{
        ID:           uuid.New().String(),
        ServiceID:    serviceID,
        ServiceName:  serviceName,
        Permissions:  permissions,
        Version:      version,
        RegisteredAt: time.Now(),
    }

    s.permissions[serviceID] = perm

    s.logger.Info("Noop permissions registered",
        "service_id", serviceID,
        "service_name", serviceName,
        "version", version,
    )

    return perm, nil
}

// Get retrieves a permission from in-memory storage
func (s *NoopService) Get(_ context.Context, serviceID string) (*Permission, error) {
    s.logger.Debug("Noop get permissions - retrieving from memory",
        "service_id", serviceID,
    )

    perm, exists := s.permissions[serviceID]
    if !exists {
        return nil, fmt.Errorf("permissions not found for service: %s", serviceID)
    }

    return perm, nil
}

// Delete removes a permission from in-memory storage
func (s *NoopService) Delete(_ context.Context, serviceID string) error {
    s.logger.Debug("Noop delete permissions - removing from memory",
        "service_id", serviceID,
    )

    if _, exists := s.permissions[serviceID]; !exists {
        return fmt.Errorf("permissions not found for service: %s", serviceID)
    }

    delete(s.permissions, serviceID)

    s.logger.Info("Noop permissions deleted",
        "service_id", serviceID,
    )

    return nil
}
