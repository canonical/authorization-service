// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package rules

import (
	"context"
	"regexp"
	"testing"
	"testing/fstest"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/canonical/authorization-service/internal/model/rules"
	"github.com/canonical/authorization-service/internal/repository"
)

type mockDBClient struct {
	pgxmock.PgxPoolIface
}

func (m *mockDBClient) Builder() sq.StatementBuilderType {
	return sq.StatementBuilder.PlaceholderFormat(sq.Dollar)
}

func (m *mockDBClient) Ping(ctx context.Context) error {
	return m.PgxPoolIface.Ping(ctx)
}

func (m *mockDBClient) Close() {
	m.PgxPoolIface.Close()
}

func TestCompareRevision(t *testing.T) {
	testCases := []struct {
		a, b     string
		expected int // 1 if a > b, -1 if a < b, 0 if a == b
	}{
		{"2026.06.11.10", "2026.06.11.9", 1},
		{"2026.06.11.9", "2026.06.11.10", -1},
		{"2026-06-11-001", "2026.06.11.1", 0},
		{"rc2", "rc10", -1},
		{"RC-01", "rc.1", 0},
		{"1.0", "1", 0},
		{"1.0.0", "1", 0},
		{"1.0.1", "1", 1},
		{"release-2026-rc2", "release-2026-rc1", 1},
		{"release_2026", "release-2025", 1},
		{"v1", "v2", -1},
		{"v1.12", "v1.3", 1},
	}

	for _, tc := range testCases {
		t.Run(tc.a+" vs "+tc.b, func(t *testing.T) {
			res := CompareRevision(tc.a, tc.b)
			assert.Equal(t, tc.expected, res)
		})
	}
}

func TestCompileMatch(t *testing.T) {
	testCases := []struct {
		match        string
		expectedSeg  int
		expectedPref string
		expectedReg  string
	}{
		{
			"/api/v1/groups/{groupId}",
			4,
			"/api/v1/groups/",
			`^/api/v1/groups/(?P<groupId>[^/]+)$`,
		},
		{
			"/api/v1/groups/{groupId}/members",
			5,
			"/api/v1/groups/",
			`^/api/v1/groups/(?P<groupId>[^/]+)/members$`,
		},
		{
			"/api/v1/groups/{groupId}/members/{memberId}",
			6,
			"/api/v1/groups/",
			`^/api/v1/groups/(?P<groupId>[^/]+)/members/(?P<memberId>[^/]+)$`,
		},
		{
			"/api/v1/admin/**",
			4,
			"/api/v1/admin/",
			`^/api/v1/admin/.*$`,
		},
		{
			"/api/v1/admin/reports",
			4,
			"/api/v1/admin/reports",
			`^/api/v1/admin/reports$`,
		},
		{
			"/{id}",
			1,
			"/",
			`^/(?P<id>[^/]+)$`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.match, func(t *testing.T) {
			cp, err := CompileMatch(tc.match)
			require.NoError(t, err)
			assert.Equal(t, tc.expectedSeg, cp.SegmentCount)
			assert.Equal(t, tc.expectedPref, cp.StaticPrefix)
			assert.Equal(t, tc.expectedReg, cp.PathRegex)

			// verify it compiles to valid regexp
			_, err = regexp.Compile(cp.PathRegex)
			assert.NoError(t, err)
		})
	}
}

func TestValidateSeedFile(t *testing.T) {
	t.Run("valid file", func(t *testing.T) {
		sf := rules.SeedFile{
			Version:     "1",
			Service:     "dummy",
			Revision:    "2026.07.24.1",
			Description: "Example rules",
			Rules: []rules.SeedRule{
				{
					Method:   "GET",
					Match:    "/api/v1/dummy/{dummy_id}",
					Priority: 100,
					Tuples: []rules.SeedTuple{
						{
							UserResourceType:   "user",
							Permission:         "viewer",
							ObjectResourceType: "domainAdmin",
							ObjectResourceId:   "{dummy_id}",
						},
					},
				},
			},
		}
		err := ValidateSeedFile(sf)
		assert.NoError(t, err)
	})

	t.Run("missing revision fails", func(t *testing.T) {
		sf := rules.SeedFile{
			Version: "1",
			Service: "dummy",
			Rules: []rules.SeedRule{
				{
					Method: "GET",
					Match:  "/api",
					Tuples: []rules.SeedTuple{
						{UserResourceType: "u", Permission: "p", ObjectResourceType: "o", ObjectResourceId: "id"},
					},
				},
			},
		}
		err := ValidateSeedFile(sf)
		assert.ErrorContains(t, err, "revision cannot be empty")
	})

	t.Run("invalid version fails", func(t *testing.T) {
		sf := rules.SeedFile{
			Version:  "2",
			Service:  "dummy",
			Revision: "1.0",
			Rules: []rules.SeedRule{
				{
					Method: "GET",
					Match:  "/api",
					Tuples: []rules.SeedTuple{
						{UserResourceType: "u", Permission: "p", ObjectResourceType: "o", ObjectResourceId: "id"},
					},
				},
			},
		}
		err := ValidateSeedFile(sf)
		assert.ErrorContains(t, err, "invalid version")
	})

	t.Run("invalid placeholder name fails", func(t *testing.T) {
		sf := rules.SeedFile{
			Version:  "1",
			Service:  "dummy",
			Revision: "1.0",
			Rules: []rules.SeedRule{
				{
					Method: "GET",
					Match:  "/api/v1/items/{item_id}",
					Tuples: []rules.SeedTuple{
						{
							UserResourceType:   "user",
							Permission:         "view",
							ObjectResourceType: "item",
							ObjectResourceId:   "{wrong_id}", // does not exist in path
						},
					},
				},
			},
		}
		err := ValidateSeedFile(sf)
		assert.ErrorContains(t, err, "tuple objectResourceId \"{wrong_id}\" is invalid for match \"/api/v1/items/{item_id}\": no such capture group")
	})

	t.Run("duplicate rule fails", func(t *testing.T) {
		sf := rules.SeedFile{
			Version:  "1",
			Service:  "dummy",
			Revision: "1.0",
			Rules: []rules.SeedRule{
				{
					Method: "GET",
					Match:  "/api",
					Tuples: []rules.SeedTuple{
						{UserResourceType: "u", Permission: "p", ObjectResourceType: "o", ObjectResourceId: "id"},
					},
				},
				{
					Method: "GET",
					Match:  "/api",
					Tuples: []rules.SeedTuple{
						{UserResourceType: "u2", Permission: "p", ObjectResourceType: "o", ObjectResourceId: "id"},
					},
				},
			},
		}
		err := ValidateSeedFile(sf)
		assert.ErrorContains(t, err, "duplicate rule GET:/api")
	})

	t.Run("duplicate tuple fails", func(t *testing.T) {
		sf := rules.SeedFile{
			Version:  "1",
			Service:  "dummy",
			Revision: "1.0",
			Rules: []rules.SeedRule{
				{
					Method: "GET",
					Match:  "/api",
					Tuples: []rules.SeedTuple{
						{UserResourceType: "u", Permission: "p", ObjectResourceType: "o", ObjectResourceId: "id"},
						{UserResourceType: "u", Permission: "p", ObjectResourceType: "o", ObjectResourceId: "id"},
					},
				},
			},
		}
		err := ValidateSeedFile(sf)
		assert.ErrorContains(t, err, "duplicate tuple")
	})

	t.Run("wildcard not at the end fails", func(t *testing.T) {
		sf := rules.SeedFile{
			Version:  "1",
			Service:  "dummy",
			Revision: "1.0",
			Rules: []rules.SeedRule{
				{
					Method: "GET",
					Match:  "/api/v1/admin/**/reports", // wildcard not at the end
					Tuples: []rules.SeedTuple{
						{UserResourceType: "u", Permission: "p", ObjectResourceType: "o", ObjectResourceId: "id"},
					},
				},
			},
		}
		err := ValidateSeedFile(sf)
		assert.ErrorContains(t, err, "rule match \"/api/v1/admin/**/reports\" is invalid: wildcard '**' must be terminal")
	})

	t.Run("wildcard only with non-static objectResourceId fails", func(t *testing.T) {
		sf := rules.SeedFile{
			Version:  "1",
			Service:  "dummy",
			Revision: "1.0",
			Rules: []rules.SeedRule{
				{
					Method: "GET",
					Match:  "/api/v1/admin/**",
					Tuples: []rules.SeedTuple{
						{
							UserResourceType:   "u",
							Permission:         "p",
							ObjectResourceType: "o",
							ObjectResourceId:   "{some_id}", // non-static, but no identifiers exist in path
						},
					},
				},
			},
		}
		err := ValidateSeedFile(sf)
		assert.ErrorContains(t, err, "tuple objectResourceId \"{some_id}\" is invalid for match \"/api/v1/admin/**\": dynamic objectResourceId requires at least one named placeholder in the path")
	})

	t.Run("wildcard only with static objectResourceId passes", func(t *testing.T) {
		sf := rules.SeedFile{
			Version:  "1",
			Service:  "dummy",
			Revision: "1.0",
			Rules: []rules.SeedRule{
				{
					Method: "GET",
					Match:  "/api/v1/admin/**",
					Tuples: []rules.SeedTuple{
						{
							UserResourceType:   "u",
							Permission:         "p",
							ObjectResourceType: "o",
							ObjectResourceId:   "id", // static, perfectly valid
						},
					},
				},
			},
		}
		err := ValidateSeedFile(sf)
		assert.NoError(t, err)
	})
}

func TestSeedService(t *testing.T) {
	ctx := context.Background()

	sf := rules.SeedFile{
		Version:     "1",
		Service:     "dummy",
		Revision:    "2026.07.24.1",
		Description: "Dummy Service Protection Rules",
		Rules: []rules.SeedRule{
			{
				Method:   "GET",
				Match:    "/api/v1/dummy/{dummy_id}",
				Priority: 100,
				Tuples: []rules.SeedTuple{
					{
						UserResourceType:   "user",
						Permission:         "viewer",
						ObjectResourceType: "domainAdmin",
						ObjectResourceId:   "{dummy_id}",
					},
				},
			},
		},
	}

	t.Run("db empty -> successful insert", func(t *testing.T) {
		pool, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer pool.Close()

		db := &mockDBClient{pool}
		repo := repository.NewPostgresRuleRepository(db, nil)
		seeder := NewRuleSeeder(db, repo)

		// Start tx expectation
		pool.ExpectBegin()

		// Upsert service expectation
		serviceID := "service-uuid-1111"
		pool.ExpectQuery("INSERT INTO federated_service").
			WithArgs(pgxmock.AnyArg(), sf.Service, sf.Description).
			WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(serviceID))

		// Get current revision expectation (none found)
		pool.ExpectQuery("SELECT revision FROM authorization_rule").
			WithArgs(serviceID).
			WillReturnError(pgx.ErrNoRows)

		// ReplaceServiceRules expectation:
		// Delete existing tuples
		pool.ExpectExec("DELETE FROM authorization_rule_tuple").
			WithArgs(serviceID).
			WillReturnResult(pgxmock.NewResult("DELETE", 0))

		// Delete existing rules
		pool.ExpectExec("DELETE FROM authorization_rule WHERE service_id").
			WithArgs(serviceID).
			WillReturnResult(pgxmock.NewResult("DELETE", 0))

		// Insert new rule
		pool.ExpectExec("INSERT INTO authorization_rule").
			WithArgs(pgxmock.AnyArg(), sf.Revision, serviceID, "GET", 4, "/api/v1/dummy/", `^/api/v1/dummy/(?P<dummy_id>[^/]+)$`, 100).
			WillReturnResult(pgxmock.NewResult("INSERT", 1))

		// Insert tuple
		pool.ExpectExec("INSERT INTO authorization_rule_tuple").
			WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), "user", "viewer", "domainAdmin", "{dummy_id}").
			WillReturnResult(pgxmock.NewResult("INSERT", 1))

		// Commit tx expectation
		pool.ExpectCommit()

		inserted, skipped, err := seeder.SeedService(ctx, sf)
		require.NoError(t, err)
		assert.True(t, inserted)
		assert.False(t, skipped)

		assert.NoError(t, pool.ExpectationsWereMet())
	})

	t.Run("db has equal or newer revision -> skip", func(t *testing.T) {
		pool, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer pool.Close()

		db := &mockDBClient{pool}
		repo := repository.NewPostgresRuleRepository(db, nil)
		seeder := NewRuleSeeder(db, repo)

		pool.ExpectBegin()

		serviceID := "service-uuid-1111"
		pool.ExpectQuery("INSERT INTO federated_service").
			WithArgs(pgxmock.AnyArg(), sf.Service, sf.Description).
			WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(serviceID))

		// DB has equal revision (2026.07.24.1)
		pool.ExpectQuery("SELECT revision FROM authorization_rule").
			WithArgs(serviceID).
			WillReturnRows(pgxmock.NewRows([]string{"revision"}).AddRow("2026.07.24.1"))

		// No further statements, rollback on rollback defer (tx.Rollback() does nothing if no changes or after commit, but in pgxmock it expectation is rollback since we didn't commit)
		pool.ExpectRollback()

		inserted, skipped, err := seeder.SeedService(ctx, sf)
		require.NoError(t, err)
		assert.False(t, inserted)
		assert.True(t, skipped)

		assert.NoError(t, pool.ExpectationsWereMet())
	})

	t.Run("db has older revision -> replace", func(t *testing.T) {
		pool, err := pgxmock.NewPool()
		require.NoError(t, err)
		defer pool.Close()

		db := &mockDBClient{pool}
		repo := repository.NewPostgresRuleRepository(db, nil)
		seeder := NewRuleSeeder(db, repo)

		pool.ExpectBegin()

		serviceID := "service-uuid-1111"
		pool.ExpectQuery("INSERT INTO federated_service").
			WithArgs(pgxmock.AnyArg(), sf.Service, sf.Description).
			WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(serviceID))

		// DB has older revision (2026.07.24.0)
		pool.ExpectQuery("SELECT revision FROM authorization_rule").
			WithArgs(serviceID).
			WillReturnRows(pgxmock.NewRows([]string{"revision"}).AddRow("2026.07.24.0"))

		pool.ExpectExec("DELETE FROM authorization_rule_tuple").
			WithArgs(serviceID).
			WillReturnResult(pgxmock.NewResult("DELETE", 2))

		pool.ExpectExec("DELETE FROM authorization_rule WHERE service_id").
			WithArgs(serviceID).
			WillReturnResult(pgxmock.NewResult("DELETE", 1))

		pool.ExpectExec("INSERT INTO authorization_rule").
			WithArgs(pgxmock.AnyArg(), sf.Revision, serviceID, "GET", 4, "/api/v1/dummy/", `^/api/v1/dummy/(?P<dummy_id>[^/]+)$`, 100).
			WillReturnResult(pgxmock.NewResult("INSERT", 1))

		pool.ExpectExec("INSERT INTO authorization_rule_tuple").
			WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), "user", "viewer", "domainAdmin", "{dummy_id}").
			WillReturnResult(pgxmock.NewResult("INSERT", 1))

		pool.ExpectCommit()

		inserted, skipped, err := seeder.SeedService(ctx, sf)
		require.NoError(t, err)
		assert.True(t, inserted)
		assert.False(t, skipped)

		assert.NoError(t, pool.ExpectationsWereMet())
	})
}

func TestLoadSeedFilesFromFS(t *testing.T) {
	// GIVEN an in-memory MapFS with valid and invalid files, and subfolders
	fsys := fstest.MapFS{
		"services/service-a/rules.yaml": &fstest.MapFile{
			Data: []byte(`version: "1"
service: "service-a"
revision: "2026.07.28.1"
description: "Service A rules"
rules:
  - method: "GET"
    match: "/api/a"
    tuples:
      - userResourceType: "user"
        permission: "viewer"
        objectResourceType: "a"
        objectResourceId: "static-id"
`),
		},
		"services/service-b/rules.yaml": &fstest.MapFile{
			Data: []byte(`version: "1"
service: "service-b"
revision: "1.0.0"
rules:
  - method: "POST"
    match: "/api/b"
    tuples:
      - userResourceType: "user"
        permission: "editor"
        objectResourceType: "b"
        objectResourceId: "static-id"
`),
		},
		"services/service-c/not-rules.txt": &fstest.MapFile{
			Data: []byte(`some random text`),
		},
	}

	// WHEN scanning services/ directory
	files, err := LoadSeedFilesFromFS(fsys, "services")

	// THEN scanning succeeds and finds exactly two rules.yaml files
	require.NoError(t, err)
	require.Len(t, files, 2)

	// AND the files contain correct unmarshaled data
	var foundA, foundB bool
	for _, f := range files {
		if f.Service == "service-a" {
			foundA = true
			assert.Equal(t, "2026.07.28.1", f.Revision)
			assert.Equal(t, "services/service-a/rules.yaml", f.FilePath)
		} else if f.Service == "service-b" {
			foundB = true
			assert.Equal(t, "1.0.0", f.Revision)
			assert.Equal(t, "services/service-b/rules.yaml", f.FilePath)
		}
	}
	assert.True(t, foundA)
	assert.True(t, foundB)
}
