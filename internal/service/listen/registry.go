// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package listen

import (
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

// TopicPrefix is prepended to a service slug to form its permission-update topic.
// Each federated service publishes to "permissions.<slug>".
const TopicPrefix = "permissions."

// ServiceRegistry resolves the mapping between federated service slugs and their
// Kafka permission-update topics. It is the single source of truth for which
// topics the listener consumes, so topic names are never hardcoded.
//
// The registry is currently populated from the FEDERATED_SERVICES configuration
// (a list of slugs). It could later be sourced from the service federation
// folder layout without changing consumers of this type.
type ServiceRegistry struct {
	// topicToService maps "permissions.<slug>" back to its slug.
	topicToService map[string]string
	// slugs preserves the configured slugs in a stable, deduplicated form.
	slugs []string
}

// NewServiceRegistry builds a ServiceRegistry from a list of service slugs.
// It returns an error if any slug is empty, malformed, or duplicated.
func NewServiceRegistry(slugs []string) (*ServiceRegistry, error) {
	r := &ServiceRegistry{
		topicToService: make(map[string]string, len(slugs)),
		slugs:          make([]string, 0, len(slugs)),
	}

	for _, raw := range slugs {
		slug := strings.TrimSpace(raw)
		if slug == "" {
			return nil, fmt.Errorf("federated service slug must not be empty")
		}
		if err := validateSlug(slug); err != nil {
			return nil, err
		}

		topic := TopicPrefix + slug
		if _, exists := r.topicToService[topic]; exists {
			return nil, fmt.Errorf("duplicate federated service slug %q", slug)
		}

		r.topicToService[topic] = slug
		r.slugs = append(r.slugs, slug)
	}

	return r, nil
}

// Slugs returns the configured service slugs in configuration order.
func (r *ServiceRegistry) Slugs() []string {
	out := make([]string, len(r.slugs))
	copy(out, r.slugs)
	return out
}

// Topics returns the permission-update topics for all federated services.
func (r *ServiceRegistry) Topics() []string {
	topics := make([]string, 0, len(r.slugs))
	for _, slug := range r.slugs {
		topics = append(topics, TopicPrefix+slug)
	}
	return topics
}

// ResolveService returns the service slug that owns the given topic. The second
// return value is false if the topic is not a known "permissions.<slug>" topic.
func (r *ServiceRegistry) ResolveService(topic string) (string, bool) {
	slug, ok := r.topicToService[topic]
	return slug, ok
}

// NewServiceRegistryFromFS scans fsys under rootDir for subdirectories,
// excluding any directory names in excludedDirs, and constructs a ServiceRegistry.
// Non-directory entries, hidden directories (starting with '.'), and names in
// excludedDirs (e.g. "dummy", "core") are ignored.
func NewServiceRegistryFromFS(fsys fs.FS, rootDir string, excludedDirs ...string) (*ServiceRegistry, error) {
	entries, err := fs.ReadDir(fsys, rootDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read service directory %q: %w", rootDir, err)
	}

	excludeMap := make(map[string]bool, len(excludedDirs))
	for _, ex := range excludedDirs {
		excludeMap[strings.TrimSpace(ex)] = true
	}

	var slugs []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".") || excludeMap[name] {
			continue
		}
		slugs = append(slugs, name)
	}

	sort.Strings(slugs)

	return NewServiceRegistry(slugs)
}

// validateSlug enforces a conservative slug charset so that a slug maps cleanly
// to a topic name and, in the future, to a federation folder name: lowercase
// letters, digits and single internal hyphens.
func validateSlug(slug string) error {
	if strings.Contains(slug, ".") {
		return fmt.Errorf("federated service slug %q must not contain %q", slug, ".")
	}
	for _, c := range slug {
		isLower := c >= 'a' && c <= 'z'
		isDigit := c >= '0' && c <= '9'
		if !isLower && !isDigit && c != '-' {
			return fmt.Errorf("federated service slug %q contains invalid character %q", slug, string(c))
		}
	}
	return nil
}
