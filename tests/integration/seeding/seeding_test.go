//go:build integration

package seeding

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/canonical/authorization-service/internal/model/rules"
	"github.com/canonical/authorization-service/internal/repository"
	"github.com/canonical/authorization-service/internal/integration/postgres"
	ruleservice "github.com/canonical/authorization-service/internal/service/rules"
	"github.com/canonical/authorization-service/tests/integration/suite"
)

// newTestPostgres is a package-level helper that redirects to the suite helper.
func newTestPostgres(t *testing.T) (*postgres.Client, *pgxpool.Pool) {
	return suite.NewTestPostgres(t, pgDSN, pgConfig)
}

// cleanRulesTables removes all rows from seeding-related tables to ensure test isolation.
func cleanRulesTables(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	_, err := pool.Exec(ctx, "TRUNCATE TABLE authorization_rule_tuple, authorization_rule, federated_service CASCADE")
	require.NoError(t, err)
}

func TestSeeding_Integration_EmptyDatabase(t *testing.T) {
	ctx := context.Background()
	client, pool := newTestPostgres(t)
	cleanRulesTables(t, pool)

	repo := repository.NewPostgresRuleRepository(client, nil)
	seeder := ruleservice.NewRuleSeeder(client, repo)

	sf := rules.SeedFile{
		Version:     "1",
		Service:     "service-empty-test",
		Revision:    "2026.07.29.1",
		Description: "Integration Test Service Rules",
		Rules: []rules.SeedRule{
			{
				Method:   "GET",
				Match:    "/api/v1/resource/{id}",
				Priority: 100,
				Tuples: []rules.SeedTuple{
					{
						UserResourceType:   "user",
						Permission:         "viewer",
						ObjectResourceType: "resource",
						ObjectResourceId:   "{id}",
					},
				},
			},
		},
	}

	inserted, skipped, err := seeder.SeedService(ctx, sf)
	require.NoError(t, err)
	assert.True(t, inserted)
	assert.False(t, skipped)

	// Verify database rows
	var serviceID string
	var description string
	err = pool.QueryRow(ctx, "SELECT id, description FROM federated_service WHERE slug = $1", sf.Service).Scan(&serviceID, &description)
	require.NoError(t, err)
	assert.Equal(t, sf.Description, description)

	var ruleID, method, staticPrefix, pathRegex, revision string
	var segmentCount, priority int
	err = pool.QueryRow(ctx, `
		SELECT id, method, segment_count, static_prefix, path_regex, priority, revision 
		FROM authorization_rule 
		WHERE service_id = $1
	`, serviceID).Scan(&ruleID, &method, &segmentCount, &staticPrefix, &pathRegex, &priority, &revision)
	require.NoError(t, err)
	assert.Equal(t, "GET", method)
	assert.Equal(t, 4, segmentCount)
	assert.Equal(t, "/api/v1/resource/", staticPrefix)
	assert.Equal(t, `^/api/v1/resource/(?P<id>[^/]+)$`, pathRegex)
	assert.Equal(t, 100, priority)
	assert.Equal(t, sf.Revision, revision)

	var userResourceType, permission, objectResourceType, objectResourceId string
	err = pool.QueryRow(ctx, `
		SELECT user_resource_type, permission, object_resource_type, object_resource_id 
		FROM authorization_rule_tuple 
		WHERE rule_id = $1
	`, ruleID).Scan(&userResourceType, &permission, &objectResourceType, &objectResourceId)
	require.NoError(t, err)
	assert.Equal(t, "user", userResourceType)
	assert.Equal(t, "viewer", permission)
	assert.Equal(t, "resource", objectResourceType)
	assert.Equal(t, "{id}", objectResourceId)
}

func TestSeeding_Integration_SkipOnOlderOrEqualRevision(t *testing.T) {
	ctx := context.Background()
	client, pool := newTestPostgres(t)
	cleanRulesTables(t, pool)

	repo := repository.NewPostgresRuleRepository(client, nil)
	seeder := ruleservice.NewRuleSeeder(client, repo)

	sfOriginal := rules.SeedFile{
		Version:     "1",
		Service:     "service-skip-test",
		Revision:    "2026.07.29.2",
		Description: "Original Rules",
		Rules: []rules.SeedRule{
			{
				Method:   "GET",
				Match:    "/api/v1/items",
				Priority: 50,
				Tuples: []rules.SeedTuple{
					{
						UserResourceType:   "user",
						Permission:         "reader",
						ObjectResourceType: "catalog",
						ObjectResourceId:   "global",
					},
				},
			},
		},
	}

	// 1. First seed successfully
	inserted, skipped, err := seeder.SeedService(ctx, sfOriginal)
	require.NoError(t, err)
	assert.True(t, inserted)
	assert.False(t, skipped)

	// 2. Try seeding with equal revision (should skip)
	sfEqual := sfOriginal
	sfEqual.Description = "Modified Rules in Same Revision"
	sfEqual.Rules[0].Priority = 1000 // attempt to modify priority

	inserted, skipped, err = seeder.SeedService(ctx, sfEqual)
	require.NoError(t, err)
	assert.False(t, inserted)
	assert.True(t, skipped)

	// Verify equal revision skipped and rules/priority did NOT change
	var priority int
	err = pool.QueryRow(ctx, `
		SELECT priority FROM authorization_rule 
		WHERE service_id = (SELECT id FROM federated_service WHERE slug = $1)
	`, sfOriginal.Service).Scan(&priority)
	require.NoError(t, err)
	assert.Equal(t, 50, priority) // Kept original priority

	// 3. Try seeding with older revision (should skip)
	sfOlder := sfOriginal
	sfOlder.Revision = "2026.07.29.1"
	sfOlder.Rules[0].Priority = 999

	inserted, skipped, err = seeder.SeedService(ctx, sfOlder)
	require.NoError(t, err)
	assert.False(t, inserted)
	assert.True(t, skipped)

	// Verify older revision skipped and rules/priority did NOT change
	err = pool.QueryRow(ctx, `
		SELECT priority FROM authorization_rule 
		WHERE service_id = (SELECT id FROM federated_service WHERE slug = $1)
	`, sfOriginal.Service).Scan(&priority)
	require.NoError(t, err)
	assert.Equal(t, 50, priority) // Kept original priority
}

func TestSeeding_Integration_UpgradeOnNewerRevision(t *testing.T) {
	ctx := context.Background()
	client, pool := newTestPostgres(t)
	cleanRulesTables(t, pool)

	repo := repository.NewPostgresRuleRepository(client, nil)
	seeder := ruleservice.NewRuleSeeder(client, repo)

	sfV1 := rules.SeedFile{
		Version:     "1",
		Service:     "service-upgrade-test",
		Revision:    "1.0.0",
		Description: "Version 1 Rules",
		Rules: []rules.SeedRule{
			{
				Method:   "GET",
				Match:    "/v1/users",
				Priority: 10,
				Tuples: []rules.SeedTuple{
					{
						UserResourceType:   "user",
						Permission:         "reader",
						ObjectResourceType: "tenant",
						ObjectResourceId:   "global",
					},
				},
			},
		},
	}

	// 1. Seed v1
	inserted, skipped, err := seeder.SeedService(ctx, sfV1)
	require.NoError(t, err)
	assert.True(t, inserted)
	assert.False(t, skipped)

	// 2. Seed newer v2
	sfV2 := rules.SeedFile{
		Version:     "1",
		Service:     "service-upgrade-test",
		Revision:    "2.0.0",
		Description: "Version 2 Rules",
		Rules: []rules.SeedRule{
			{
				Method:   "POST",
				Match:    "/v2/users",
				Priority: 20,
				Tuples: []rules.SeedTuple{
					{
						UserResourceType:   "user",
						Permission:         "admin",
						ObjectResourceType: "tenant",
						ObjectResourceId:   "global",
					},
				},
			},
		},
	}

	inserted, skipped, err = seeder.SeedService(ctx, sfV2)
	require.NoError(t, err)
	assert.True(t, inserted)
	assert.False(t, skipped)

	// 3. Verify old rules and tuples are completely replaced
	var count int
	err = pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM authorization_rule 
		WHERE service_id = (SELECT id FROM federated_service WHERE slug = $1)
	`, sfV1.Service).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	var method, pathRegex string
	var priority int
	err = pool.QueryRow(ctx, `
		SELECT method, path_regex, priority FROM authorization_rule 
		WHERE service_id = (SELECT id FROM federated_service WHERE slug = $1)
	`, sfV1.Service).Scan(&method, &pathRegex, &priority)
	require.NoError(t, err)
	assert.Equal(t, "POST", method)
	assert.Equal(t, "^/v2/users$", pathRegex)
	assert.Equal(t, 20, priority)
}

func TestSeeding_Integration_ValidationErrorRollback(t *testing.T) {
	ctx := context.Background()
	client, pool := newTestPostgres(t)
	cleanRulesTables(t, pool)

	repo := repository.NewPostgresRuleRepository(client, nil)
	seeder := ruleservice.NewRuleSeeder(client, repo)

	sfValid := rules.SeedFile{
		Version:     "1",
		Service:     "service-rollback-test",
		Revision:    "1.0",
		Description: "Initial Rules",
		Rules: []rules.SeedRule{
			{
				Method:   "GET",
				Match:    "/api/v1/valid",
				Priority: 100,
				Tuples: []rules.SeedTuple{
					{
						UserResourceType:   "user",
						Permission:         "viewer",
						ObjectResourceType: "scope",
						ObjectResourceId:   "global",
					},
				},
			},
		},
	}

	// 1. Seed valid file
	inserted, skipped, err := seeder.SeedService(ctx, sfValid)
	require.NoError(t, err)
	assert.True(t, inserted)
	assert.False(t, skipped)

	// 2. Seed a newer revision but with validation failure (e.g. empty service name)
	sfInvalid := rules.SeedFile{
		Version:     "1",
		Service:     "", // Invalid slug
		Revision:    "2.0",
		Description: "Invalid Rules",
		Rules:       sfValid.Rules,
	}

	_, _, err = seeder.SeedService(ctx, sfInvalid)
	assert.Error(t, err)

	// 3. Verify original database state remains unchanged
	var revision string
	err = pool.QueryRow(ctx, `
		SELECT revision FROM authorization_rule 
		WHERE service_id = (SELECT id FROM federated_service WHERE slug = $1)
		LIMIT 1
	`, sfValid.Service).Scan(&revision)
	require.NoError(t, err)
	assert.Equal(t, "1.0", revision)
}
